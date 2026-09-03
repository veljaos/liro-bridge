//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/signing"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// runSignInteractive implements F5's exit condition: "a person runs
// `liro-bridge sign --interactive`, a window appears, they choose a
// certificate and press Approve, the card asks for a PIN, the document
// is signed, and an audit entry is written." It is Windows-only,
// matching internal/ui's own scope this phase (SPEC §11.11 already
// establishes Windows-only for this project's phases 1 through 10).
func runSignInteractive(ctx context.Context, args []string, out io.Writer, locale string, cfg config.Config) int {
	c := i18n.Load(locale)

	inPattern, force, err := parseInteractiveArgs(args)
	if err != nil {
		fprintln(out, "liro-bridge: sign:", err)
		return 2
	}
	if inPattern == "" {
		fprintln(out, "liro-bridge: sign:", c.T("sign.in_required"))
		return 2
	}

	files, err := expandInteractiveInput(inPattern)
	if err != nil || len(files) == 0 {
		fprintln(out, "liro-bridge: sign:", c.T("sign.no_input_files"))
		return 1
	}

	inputs := make([]interactiveInput, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			fprintln(out, "liro-bridge: sign:", f, err)
			return 1
		}
		sum := sha256.Sum256(b)
		inputs = append(inputs, interactiveInput{path: f, bytes: b, digest: sum[:]})
	}

	report, err := gatherInteractiveCertificates(ctx)
	if err != nil {
		fprintln(out, "liro-bridge: sign:", err)
		return 1
	}
	certInfos := make([]classify.Info, 0, len(report.Certificates))
	for _, row := range report.Certificates {
		certInfos = append(certInfos, row.Info)
	}

	digests := make([][]byte, len(inputs))
	fileNames := make([]string, len(inputs))
	for i, in := range inputs {
		digests[i] = in.digest
		fileNames[i] = filepath.Base(in.path)
	}
	vm := consent.BuildViewModel(consent.ApplicationLocal, digests, fileNames, certInfos)

	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("consent.window_title"),
		Width:       480,
		Height:      420,
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/consent.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		fprintln(out, "liro-bridge: sign:", err)
		return 1
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		fprintln(out, "liro-bridge: sign:", err)
		return 1
	}

	auditStore, auditErr := newAuditStore()

	selected := ""
	decided := false
	approved := false
	for !decided {
		msg := <-messages
		switch msg.Type {
		case ui.MessageTypeSelectCertificate:
			selected = msg.Thumbprint
		case ui.MessageTypeApprove:
			if selected != "" {
				approved, decided = true, true
			}
		case ui.MessageTypeCancel:
			approved, decided = false, true
		}
	}

	if !approved {
		if auditErr == nil {
			_, _ = auditStore.Append(audit.Entry{
				Timestamp:     time.Now(),
				Thumbprint:    selected,
				Application:   consent.ApplicationLocal,
				DocumentCount: len(inputs),
				Outcome:       audit.OutcomeDenied,
			})
		}
		return 0
	}

	session, err := openInteractiveSession(ctx, keysource.Thumbprint(selected))
	if err != nil {
		pushFailure(win, c, err)
		waitForClose(messages)
		return 1
	}
	defer func() { _ = session.Close() }()
	wrapped := signing.WrapSession(session)

	_ = win.PostJSON(consentProgressPayload(consent.ProgressForTiming(0, len(inputs), 0, 0, signing.PINPolicyUnknown), c))

	trustStore := interactiveTrustStore(ctx)
	succeeded, failed := 0, 0
	var lastOutput string
	var durations []time.Duration
	var lastErr error

	tsaClient, err := buildTSAClient(cfg)
	if err != nil {
		pushFailure(win, c, err)
		waitForClose(messages)
		return 1
	}

	for i, in := range inputs {
		start := time.Now()
		outPath := defaultInteractiveOutputPath(in.path, cfg.OutputSuffix)
		_, signErr := signInteractiveOne(ctx, in.bytes, wrapped, cfg, trustStore, tsaClient, outPath, force)
		durations = append(durations, time.Since(start))
		if signErr != nil {
			failed++
			lastErr = signErr
		} else {
			succeeded++
			lastOutput = outPath
		}

		first := durations[0]
		var median time.Duration
		if len(durations) > 1 {
			median = medianDuration(durations[1:])
		}
		progress := consent.ProgressForTiming(i+1, len(inputs), first, median, signing.PINPolicyUnknown)
		_ = win.PostJSON(consentProgressPayload(progress, c))
	}

	outcome := audit.OutcomeApproved
	if failed > 0 && succeeded == 0 {
		outcome = audit.OutcomeFailed
	} else if failed > 0 {
		outcome = audit.OutcomePartial
	}
	if auditErr == nil {
		_, _ = auditStore.Append(audit.Entry{
			Timestamp:     time.Now(),
			Thumbprint:    selected,
			Application:   consent.ApplicationLocal,
			DocumentCount: len(inputs),
			Outcome:       outcome,
			FailureCode:   codeOfInteractive(lastErr),
			IsTestKey:     session.Certificate().IsTestKey,
		})
	}

	if failed > 0 && succeeded == 0 {
		pushFailure(win, c, lastErr)
	} else {
		done := consent.Progress{State: consent.StateDone, Succeeded: succeeded, Failed: failed, OutputPath: lastOutput}
		_ = win.PostJSON(consentDonePayload(done, c))
	}
	waitForClose(messages)

	if failed > 0 && succeeded == 0 {
		return 1
	}
	return 0
}

type interactiveInput struct {
	path   string
	bytes  []byte
	digest []byte
}

func parseInteractiveArgs(args []string) (in string, force bool, err error) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--interactive":
			continue
		case args[i] == "--force":
			force = true
		case strings.HasPrefix(args[i], "--in="):
			in = strings.TrimPrefix(args[i], "--in=")
		case args[i] == "--in" && i+1 < len(args):
			i++
			in = args[i]
		default:
			return "", false, fmt.Errorf("unrecognised argument %q", args[i])
		}
	}
	return in, force, nil
}

func expandInteractiveInput(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		sort.Strings(matches)
		return matches, nil
	}
	if _, statErr := os.Stat(pattern); statErr == nil {
		return []string{pattern}, nil
	}
	return nil, fmt.Errorf("no files matched %q", pattern)
}

func defaultInteractiveOutputPath(in, suffix string) string {
	ext := filepath.Ext(in)
	base := strings.TrimSuffix(in, ext)
	return base + suffix + ext
}

func medianDuration(d []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), d...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func waitForClose(messages chan ui.Message) {
	<-messages
}

func codeOfInteractive(err error) errs.Code {
	if err == nil {
		return ""
	}
	var e *errs.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return errs.CodeInternal
}

func pushFailure(win ui.Window, c *i18n.Catalogue, err error) {
	msg := err.Error()
	code := codeOfInteractive(err)
	if code != "" {
		msg = c.T(i18n.CodeKey(code))
	}
	_ = win.PostJSON(consentFailedPayload(msg, err, c))
}

// gatherInteractiveCertificates wires the real Windows CNG source and
// soft token (when built with the "softtoken" tag) into cli.Gather —
// the same certificate list "liro-bridge certs" itself uses.
func gatherInteractiveCertificates(ctx context.Context) (cli.Report, error) {
	svc := platform.NewSmartCardService()
	cngSource := windowscng.NewSource()
	cachePath := filepath.Join(filepath.Dir(platform.DefaultConfigFile()), "tsl-cache.xml")
	store, err := tsl.NewFileStore(cachePath, tsl.DefaultURL, tsl.HTTPFetcher)
	if err != nil {
		return cli.Report{}, err
	}
	deps := cli.Deps{
		Readers:           svc.Readers,
		PresenceCheck:     cngSource.Presence,
		Enumerate:         windowscng.Enumerate,
		Store:             store,
		ExtraCertificates: softTokenExtraCertificates,
	}
	return cli.Gather(ctx, deps, time.Now())
}

func openInteractiveSession(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
	cngSource := windowscng.NewSource()
	sess, err := cngSource.Open(ctx, thumbprint)
	if err == nil {
		return sess, nil
	}
	softSource := softTokenSource()
	var e *errs.Error
	if softSource != nil && errors.As(err, &e) && e.Code == errs.CodeCertNotFound {
		return softSource.Open(ctx, thumbprint)
	}
	return nil, err
}

// buildTSAClient wires Task 6's four TSA credential fields (config.Config,
// reachable from Settings) into a *tsa.Client exactly the way
// internal/cli.RunSign's --tsa-user/--tsa-password/--tsa-client-cert/
// --tsa-client-cert-password flags already do (internal/cli/sign.go) —
// the interactive path had never read them at all, silently signing
// with no TSA authentication even when Settings had credentials saved.
// A nil, nil return means no TSA is configured (cfg.TSAURL == ""),
// exactly like RunSign's own *tsa.Client stays nil in that case.
func buildTSAClient(cfg config.Config) (*tsa.Client, error) {
	if cfg.TSAURL == "" {
		return nil, nil
	}
	auth := tsa.Auth{BasicUsername: cfg.TSAUser, BasicPassword: cfg.TSAPassword}
	if cfg.TSAClientCertPath != "" {
		p12, err := os.ReadFile(cfg.TSAClientCertPath)
		if err != nil {
			return nil, fmt.Errorf("reading TSA client certificate: %w", err)
		}
		cert, err := tsa.LoadPKCS12ClientCert(p12, cfg.TSAClientCertPassword)
		if err != nil {
			return nil, fmt.Errorf("loading TSA client certificate: %w", err)
		}
		auth.ClientCertificate = &cert
	}
	return tsa.NewClient(cfg.TSAURL, auth), nil
}

func interactiveTrustStore(ctx context.Context) []*x509.Certificate {
	cachePath := filepath.Join(filepath.Dir(platform.DefaultConfigFile()), "tsl-cache.xml")
	store, err := tsl.NewFileStore(cachePath, tsl.DefaultURL, tsl.HTTPFetcher)
	if err != nil {
		return nil
	}
	list, _, err := store.Current(ctx)
	if err != nil {
		return nil
	}
	return caCertificatesFromTSL(list)
}

func signInteractiveOne(ctx context.Context, pdfBytes []byte, session keysource.Session, cfg config.Config, trustStore []*x509.Certificate, tsaClient *tsa.Client, outPath string, force bool) (*pades.Result, error) {
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return nil, fmt.Errorf("output file already exists: %s", outPath)
		}
	}
	level := pades.LevelBLT
	if cfg.SignatureLevel == "b-t" {
		level = pades.LevelBT
	}
	result, err := pades.SignDocument(ctx, pdfBytes, session, pades.Options{
		RequestedLevel:    level,
		OnTSAFailureAbort: true,
		TSA:               tsaClient,
		TrustStore:        trustStore,
		Now:               time.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(outPath, result.Bytes, 0o600); err != nil {
		return nil, err
	}
	return result, nil
}

func newAuditStore() (*audit.Store, error) {
	dir := filepath.Join(platform.ConfigDir("windows", platform.OSEnv), "audit")
	return audit.NewStore(dir)
}
