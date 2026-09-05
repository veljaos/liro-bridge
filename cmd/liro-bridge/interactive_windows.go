//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
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
		in, err := newInteractiveInput(f)
		if err != nil {
			fprintln(out, "liro-bridge: sign:", f, err)
			return 1
		}
		inputs = append(inputs, in)
	}

	report, err := gatherInteractiveCertificates(ctx)
	if err != nil {
		fprintln(out, "liro-bridge: sign:", err)
		return 1
	}
	// Task 3: the consent window applies the same default visibility
	// rule as "liro-bridge certs" (no --all) — a certificate that is
	// both PurposeUnknown and not qualified is a Windows-internal
	// artefact (GUID subject, no recognised use) the user has never
	// heard of and cannot sign with, not a real choice to present.
	certInfos := make([]classify.Info, 0, len(report.Certificates))
	for _, row := range report.Certificates {
		if row.Hidden() {
			continue
		}
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
		Title: c.T("consent.window_title"),
		// The same measured size the main window's consent phase uses
		// (consentphase_windows.go) — one window, one size, whichever
		// front door opened it.
		Width:       consentWindowWidth,
		Height:      consentWindowHeight,
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

	// Step 3 of the three-step flow: how to sign, in its own window,
	// after the certificate has been chosen and approved. The command
	// line reaches the same window the main window does, so the two
	// front doors ask the same question in the same place.
	var stampProceed bool
	cfg, stampProceed = askHowToSign(cfg, locale, nil, win.Handle())
	if !stampProceed {
		if auditErr == nil {
			recordInteractiveAudit(auditStore, auditErr, selected, len(inputs), audit.OutcomeDenied, nil, false, "")
		}
		return 0
	}

	// Task 1 (F5 second-real-run review): the timestamp question is
	// settled before the card is touched, not after. With no TSA
	// configured — this project's out-of-the-box state, since it ships
	// no default authority (D-067 and this task's own decision) — the
	// previous build reached this point, asked for the PIN, signed, and
	// only then failed the whole batch with TSA_UNAVAILABLE. Asking
	// first costs a user who cancels nothing, and never spends a PIN
	// entry on a batch that was going to be refused anyway.
	//
	// Task 3 (F5 fourth-real-run review) adds the one case where the
	// question is not asked at all: a configured level of B-B *is* the
	// answer. The user settled it in Settings, deliberately, and asking
	// again on every signature would be asking them to re-decide
	// something they have already decided.
	level := interactiveLevel(cfg)
	var tsaClient *tsa.Client
	allowBB := level == pades.LevelBB
	if level != pades.LevelBB {
		var err error
		tsaClient, err = buildTSAClient(cfg)
		if err != nil {
			pushFailure(win, c, err)
			waitForClose(messages)
			return 1
		}
		if tsaClient == nil {
			var proceed bool
			cfg, tsaClient, allowBB, proceed = resolveTSAChoice(win, messages, c, cfg, locale, consent.TSAReasonNotConfigured)
			if !proceed {
				recordInteractiveAudit(auditStore, auditErr, selected, len(inputs), audit.OutcomeDenied, nil, false, "")
				return 0
			}
		}
	}

	// Task 4 (F5 fourth-real-run review): where each signature will be
	// written is settled here, before the card session is opened, for
	// the same reason the timestamp question is (Task 1, previous
	// round): a user who answers "cancel" must not have spent a PIN
	// entry on a batch that was never going to be saved. Every output
	// path is known before signing begins — it comes from the input
	// name and the configured suffix, not from anything the signature
	// produces.
	outputs, outputsSettled := resolveInteractiveOutputs(win, messages, c, inputs, cfg.OutputSuffix, force)
	if !outputsSettled {
		recordInteractiveAudit(auditStore, auditErr, selected, len(inputs), audit.OutcomeDenied, nil, false, "")
		return 0
	}

	session, err := openInteractiveSession(ctx, keysource.Thumbprint(selected), win.Handle())
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
	batchLevel := ""
	cancelled := false

	for i, in := range inputs {
		start := time.Now()
		outPath := outputs[i].path

		opts := interactiveSignOptions{
			level:      level,
			trustStore: trustStore,
			tsaClient:  tsaClient,
			outPath:    outPath,
			overwrite:  outputs[i].overwrite,
			allowBB:    allowBB,
			stamp:      stampOptionsFor(c, cfg),
		}

		var result *pades.Result
		var signErr error
		for {
			result, signErr = signInteractiveOne(ctx, in.path, wrapped, opts)
			if signErr == nil || !isTSAFailure(signErr) {
				break
			}
			// SPEC §12.8: a TSA outage presents the choice, it does not
			// end the batch. The same three actions as before signing
			// began, now about an authority that was actually tried.
			var proceed bool
			cfg, tsaClient, allowBB, proceed = resolveTSAChoice(win, messages, c, cfg, locale, consent.TSAReasonUnreachable)
			if !proceed {
				cancelled = true
				break
			}
			opts.tsaClient, opts.allowBB = tsaClient, allowBB
			_ = win.PostJSON(consentProgressPayload(consent.ProgressForTiming(i, len(inputs), firstDuration(durations), 0, signing.PINPolicyUnknown), c))
		}
		durations = append(durations, time.Since(start))
		if cancelled {
			break
		}
		if signErr != nil {
			failed++
			lastErr = signErr
		} else {
			succeeded++
			lastOutput = outPath
			batchLevel = lowerLevel(batchLevel, string(result.AchievedLevel))
		}

		first := durations[0]
		var median time.Duration
		if len(durations) > 1 {
			median = medianDuration(durations[1:])
		}
		progress := consent.ProgressForTiming(i+1, len(inputs), first, median, signing.PINPolicyUnknown)
		_ = win.PostJSON(consentProgressPayload(progress, c))
	}

	skipped := len(inputs) - succeeded - failed
	outcome := audit.OutcomeApproved
	switch {
	case succeeded == 0 && cancelled:
		outcome = audit.OutcomeDenied
	case succeeded == 0:
		outcome = audit.OutcomeFailed
	case failed > 0 || skipped > 0:
		outcome = audit.OutcomePartial
	}
	recordInteractiveAudit(auditStore, auditErr, selected, len(inputs), outcome, lastErr, session.Certificate().IsTestKey, batchLevel)

	if succeeded == 0 {
		if cancelled {
			return 0
		}
		pushFailure(win, c, lastErr)
		waitForClose(messages)
		return 1
	}

	done := consent.Progress{
		State:         consent.StateDone,
		Succeeded:     succeeded,
		Failed:        failed + skipped,
		OutputPath:    lastOutput,
		AchievedLevel: batchLevel,
	}
	_ = win.PostJSON(consentDonePayload(done, c))
	waitForClose(messages)
	return 0
}

// recordInteractiveAudit appends one batch outcome, or does nothing if
// the store could not be opened (auditErr) — the same tolerance the
// previous code had inline, now in one place because three call sites
// need it.
func recordInteractiveAudit(store *audit.Store, auditErr error, thumbprint string, documents int, outcome audit.Outcome, lastErr error, isTestKey bool, level string) {
	if auditErr != nil {
		return
	}
	_, _ = store.Append(audit.Entry{
		Timestamp:     time.Now(),
		Thumbprint:    thumbprint,
		Application:   consent.ApplicationLocal,
		DocumentCount: documents,
		Outcome:       outcome,
		FailureCode:   codeOfInteractive(lastErr),
		IsTestKey:     isTestKey,
		AchievedLevel: level,
	})
}

// firstDuration is the measured first-signature time, or 0 when no
// signature has completed yet — StatePreparingCard's own trigger
// (consent.ProgressForTiming).
func firstDuration(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	return d[0]
}

// levelRank orders the three PAdES levels so lowerLevel can pick the
// weakest one a batch actually reached. Reporting the weakest — not the
// strongest, and not the last — is what keeps the reported level honest
// for a batch where only some documents got a timestamp (SPEC §18.11).
func levelRank(level string) int {
	switch pades.Level(level) {
	case pades.LevelBB:
		return 1
	case pades.LevelBT:
		return 2
	case pades.LevelBLT:
		return 3
	default:
		return 0
	}
}

func lowerLevel(a, b string) string {
	if levelRank(a) == 0 {
		return b
	}
	if levelRank(b) == 0 {
		return a
	}
	if levelRank(b) < levelRank(a) {
		return b
	}
	return a
}

// isTSAFailure reports whether err is the timestamp step failing, as
// opposed to any other signing failure — the only kind of failure SPEC
// §12.8 says to offer a choice about.
func isTSAFailure(err error) bool {
	code := codeOfInteractive(err)
	return code == errs.CodeTSAUnavailable || code == errs.CodeTSARejected
}

// resolveTSAChoice shows SPEC §12.8's choice and loops until the user
// settles it (Task 1). It returns the possibly-updated configuration,
// the TSA client to use from here on, whether B-B is now permitted, and
// whether to proceed at all.
//
// "Configure a timestamp authority" opens the Settings window and, when
// that closes, re-reads the configuration from disk and rebuilds the
// client — so a user who fills in a URL there continues at the level
// they asked for, and one who closes Settings without setting anything
// is asked the same question again rather than silently continuing.
func resolveTSAChoice(win ui.Window, messages chan ui.Message, c *i18n.Catalogue, cfg config.Config, locale string, reason consent.TSAReason) (config.Config, *tsa.Client, bool, bool) {
	for {
		_ = win.PostJSON(consentTSAChoicePayload(reason, c))

		msg := <-messages
		if msg.Type != ui.MessageTypeApprove {
			// Cancel, Escape, or the window being closed.
			return cfg, nil, false, false
		}
		switch readTSAChoice(win) {
		case "withoutTimestamp":
			// The client is dropped, not kept: the user asked to sign
			// without a timestamp, and a batch of a hundred documents
			// must not spend F3 §6.3's three attempts and two backoffs
			// on a dead authority for every one of them. Every
			// remaining document goes straight to B-B, reported as
			// B-B.
			return cfg, nil, true, true
		case "configure":
			if err := runSettingsWindow(cfg, win.Handle()); err != nil {
				slog.Warn("consent: settings window failed", "error", err)
			}
			newCfg, cfgErr := config.Load(platform.DefaultConfigFile())
			if cfgErr != nil {
				slog.Warn("consent: re-reading configuration after Settings failed", "error", cfgErr)
			} else {
				cfg = newCfg
			}
			client, buildErr := buildTSAClient(cfg)
			if buildErr != nil {
				slog.Warn("consent: building the TSA client after Settings failed", "error", buildErr)
				client = nil
			}
			if client != nil {
				return cfg, client, false, true
			}
			// Still nothing configured: ask again rather than proceed.
			reason = consent.TSAReasonNotConfigured
		default:
			// An approve from the choice screen with no choice recorded
			// is not a decision; ask again rather than guess at one.
			slog.Warn("consent: timestamp choice screen sent approve with no choice")
		}
	}
}

// readTSAChoice reads back which of the two proceeding actions the page
// recorded, through Window.Eval's own return value — the same channel
// the settings window uses (D-083), so the page->Go message surface
// stays at exactly three types.
func readTSAChoice(win ui.Window) string {
	raw, err := win.Eval("window.__liroTSAChoice()")
	if err != nil {
		slog.Warn("consent: reading the timestamp choice failed", "error", err)
		return ""
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		slog.Warn("consent: decoding the timestamp choice envelope failed", "error", err)
		return ""
	}
	var choice struct {
		Choice string `json:"choice"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &choice); err != nil {
		slog.Warn("consent: decoding the timestamp choice failed", "error", err)
		return ""
	}
	return choice.Choice
}

// interactiveInput is one document waiting to be signed: where it is,
// and the digest the consent screen's batch fingerprint is built from
// (SPEC §6.6).
//
// It deliberately does not hold the document's bytes. It used to, read
// in full for every input before the consent window even opened, which
// is fine for the one or two files a command line is given and is not
// fine for F6's own stated case of two hundred dropped at once — a
// two-hundred-document batch of ordinary contracts is hundreds of
// megabytes held for the whole run, most of it long before and long
// after the moment each document is actually needed. Each document is
// now read at the moment it is signed and released when it is written.
type interactiveInput struct {
	path   string
	digest []byte
}

// newInteractiveInput computes one document's digest by streaming it,
// so the largest allocation is the copy buffer rather than the file.
func newInteractiveInput(path string) (interactiveInput, error) {
	f, err := os.Open(path)
	if err != nil {
		return interactiveInput{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return interactiveInput{}, err
	}
	return interactiveInput{path: path, digest: h.Sum(nil)}, nil
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
	return outputPathIn(in, "", suffix)
}

// outputPathIn is where one document's signature goes: beside the
// input when dir is empty (F5's behaviour, and F6 §4's default), or in
// dir when the user has chosen one. jobs.OutputPathFor is the single
// implementation — the main window shows the same path this signs to,
// and two functions computing it would eventually show one and write
// the other.
func outputPathIn(in, dir, suffix string) string {
	return jobs.OutputPathFor(in, dir, suffix)
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

// pushFailure shows one failure on the consent window's failed screen.
// The message comes from cli.ErrorMessage — the same renderer the
// command line uses — so a code carrying Details reaches the user as a
// finished sentence. Rendering the bare catalogue string here put
// "The stamp contains a character the font does not support: %s (%s)."
// on screen, placeholders and all; Task 1 makes that path reachable by
// turning the stamp on by default.
func pushFailure(win ui.Window, c *i18n.Catalogue, err error) {
	_ = win.PostJSON(consentFailedPayload(cli.ErrorMessage(err, c), err, c))
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

// openInteractiveSession opens the signing session for the consent
// window's chosen certificate. hwnd is the consent window's own HWND
// (Task 2, F2 §2.3): without it, NCRYPT_WINDOW_HANDLE_PROPERTY stays 0
// and the OS PIN dialog can appear behind the agent's window, which
// looks like a frozen program rather than a prompt.
func openInteractiveSession(ctx context.Context, thumbprint keysource.Thumbprint, hwnd uintptr) (keysource.Session, error) {
	cngSource := windowscng.NewSource().WithWindowHandle(hwnd)
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
			// Task 4: each of these has one clear cause and one clear
			// remedy — a wrong path, a wrong password — and neither is
			// an unclassified failure. They are separate codes because
			// they ask the user for different corrections.
			return nil, errs.New(errs.CodeTSAClientCertUnreadable, fmt.Errorf("reading TSA client certificate: %w", err))
		}
		cert, err := tsa.LoadPKCS12ClientCert(p12, cfg.TSAClientCertPassword)
		if err != nil {
			return nil, errs.New(errs.CodeTSAClientCertInvalid, fmt.Errorf("loading TSA client certificate: %w", err))
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

// interactiveSignOptions is everything one document's signature needs
// that is neither the document itself nor the card session — gathered
// into one value because the argument list had grown past the point
// where a reader could tell which of eight positional arguments was
// which.
type interactiveSignOptions struct {
	level      pades.Level
	trustStore []*x509.Certificate
	tsaClient  *tsa.Client
	outPath    string

	// overwrite is the user's own answer to Task 4's choice (or
	// --force). Without it an existing file is refused, never replaced
	// (SPEC §12.11/§18.10).
	overwrite bool

	// allowBB is the user's answer to SPEC §12.8's choice: false means
	// a failed timestamp step aborts this document (and the caller then
	// asks), true means it degrades to B-B, which
	// pades.Result.AchievedLevel then reports accurately — never
	// silently (SPEC §18.11).
	allowBB bool

	// stamp is Task 1's visible stamp, or nil for an invisible
	// signature. Nil leaves SPEC §13.4's default path untouched, byte
	// for byte.
	stamp *pades.StampOptions
}

// signInteractiveOne reads one document, signs it, and writes the
// result to opts.outPath. The document is read here rather than handed
// in already-loaded so that a batch holds one document in memory at a
// time, not all of them (see interactiveInput).
func signInteractiveOne(ctx context.Context, inPath string, session keysource.Session, opts interactiveSignOptions) (*pades.Result, error) {
	pdfBytes, err := os.ReadFile(inPath)
	if err != nil {
		// A document that cannot be read is not a signing failure and
		// must not be reported as one: it is open in another program,
		// or it has moved. F6 §7 asks for each to be named.
		return nil, errs.WithDetails(errs.CodeInputUnreadable, err,
			map[string]any{"path": inPath})
	}
	if !opts.overwrite {
		// Reached only if the file appeared between the choice above
		// and this moment; the code says which condition it is, so it
		// can never surface as "an unexpected error" again (Task 4).
		if _, err := os.Stat(opts.outPath); err == nil {
			return nil, errs.WithDetails(errs.CodeOutputExists,
				fmt.Errorf("output file already exists: %s", opts.outPath),
				map[string]any{"path": opts.outPath})
		}
	}
	result, err := pades.SignDocument(ctx, pdfBytes, session, pades.Options{
		RequestedLevel:    opts.level,
		OnTSAFailureAbort: !opts.allowBB,
		TSA:               opts.tsaClient,
		TrustStore:        opts.trustStore,
		Now:               time.Now(),
		Stamp:             opts.stamp,
	})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(opts.outPath, result.Bytes, 0o600); err != nil {
		// The signature succeeded; only the write did not. SIGN_FAILED
		// would send the user to check the card for a problem that is
		// on the disk (Task 4).
		return nil, errs.WithDetails(errs.CodeOutputWriteFailed, err,
			map[string]any{"path": opts.outPath})
	}
	return result, nil
}

// interactiveLevel maps the configured signature level onto the PAdES
// level to request. "b-b" is a level the user chose deliberately
// (Task 3), not one reached by failure — SPEC §12.6's "fallback only,
// on explicit user choice", with Settings as the place that choice is
// made.
func interactiveLevel(cfg config.Config) pades.Level {
	switch cfg.SignatureLevel {
	case "b-b":
		return pades.LevelBB
	case "b-t":
		return pades.LevelBT
	default:
		return pades.LevelBLT
	}
}

// interactiveStampOptions turns the window's stamp choice into
// pades.StampOptions, or nil when no stamp was asked for — nil is what
// keeps SPEC §13.4's invisible default path untouched.
//
// The identity-document line stays off (SPEC §13.5: it is personal
// data on a document that will be sent to third parties — available as
// an option, never a default), and the reference line is empty: both
// are command-line capabilities (--stamp-show-document-id,
// --stamp-reference) with nowhere to come from in this window.
func interactiveStampOptions(c *i18n.Catalogue, choice consent.StampChoice) *pades.StampOptions {
	if !choice.Visible {
		return nil
	}
	return &pades.StampOptions{
		Label:  c.T("sign.stamp_label"),
		Corner: stampCorner(choice.Position),
		Page:   1,
	}
}

// stampCorner maps one of consent's four position values onto
// appearance.Corner. An unrecognised value cannot arrive here —
// StampChoice.Normalised has already replaced it — and would anyway
// land on SPEC §13.1's own default.
func stampCorner(position string) appearance.Corner {
	switch position {
	case consent.StampPositionBottomLeft:
		return appearance.BottomLeft
	case consent.StampPositionTopRight:
		return appearance.TopRight
	case consent.StampPositionTopLeft:
		return appearance.TopLeft
	default:
		return appearance.BottomRight
	}
}

// outputConflictAnswer remembers what the user chose the first time an
// output file already existed, and applies it to the rest of the batch
// (Task 4). Asking once per document would mean a hundred questions
// for a batch signed a second time, which is not a choice — it is an
// obstacle course.
type outputConflictAnswer int

const (
	outputConflictUnanswered outputConflictAnswer = iota
	outputConflictOverwrite
	outputConflictRename
)

// interactiveOutput is where one document's signature will be written,
// and whether a file already there may be replaced.
type interactiveOutput struct {
	path      string
	overwrite bool
}

// resolveInteractiveOutputs settles every document's output path up
// front, asking about the first conflict and applying that answer to
// the rest of the batch (Task 4). It returns false when the user
// cancelled, in which case nothing is signed and no PIN was ever
// requested.
func resolveInteractiveOutputs(win ui.Window, messages chan ui.Message, c *i18n.Catalogue, inputs []interactiveInput, suffix string, force bool) ([]interactiveOutput, bool) {
	answer := outputConflictUnanswered
	out := make([]interactiveOutput, 0, len(inputs))
	for _, in := range inputs {
		path, overwrite, proceed := resolveOutputConflict(win, messages, c,
			defaultInteractiveOutputPath(in.path, suffix), force, &answer)
		if !proceed {
			return nil, false
		}
		out = append(out, interactiveOutput{path: path, overwrite: overwrite})
	}
	return out, true
}

// resolveOutputConflict decides where one document's signature will be
// written. It returns the path to write to, whether an existing file
// there may be replaced, and whether to go on at all.
//
// SPEC §12.11 forbids silently overwriting; it does not forbid
// overwriting a file the user has just been shown and asked about.
// What the previous build did instead — refuse, and report the refusal
// as "an unexpected error occurred" — honoured the rule and hid the
// reason.
func resolveOutputConflict(win ui.Window, messages chan ui.Message, c *i18n.Catalogue, outPath string, force bool, answer *outputConflictAnswer) (path string, overwrite, proceed bool) {
	if force {
		return outPath, true, true
	}
	if _, err := os.Stat(outPath); err != nil {
		return outPath, false, true
	}
	switch *answer {
	case outputConflictOverwrite:
		return outPath, true, true
	case outputConflictRename:
		return nextFreeOutputPath(outPath), false, true
	}

	renamePath := nextFreeOutputPath(outPath)
	_ = win.PostJSON(consentOutputExistsPayload(outPath, renamePath, c))

	msg := <-messages
	if msg.Type != ui.MessageTypeApprove {
		// Cancel, Escape, or the window being closed.
		return outPath, false, false
	}
	switch readOutputChoice(win) {
	case "overwrite":
		*answer = outputConflictOverwrite
		return outPath, true, true
	case "rename":
		*answer = outputConflictRename
		return renamePath, false, true
	default:
		// An approve with no choice recorded is not a decision. Nothing
		// is written and nothing is overwritten.
		slog.Warn("consent: output-exists screen sent approve with no choice")
		return outPath, false, false
	}
}

// readOutputChoice reads back which of the two proceeding actions the
// page recorded, exactly as readTSAChoice does.
func readOutputChoice(win ui.Window) string {
	raw, err := win.Eval("window.__liroOutputChoice()")
	if err != nil {
		slog.Warn("consent: reading the output choice failed", "error", err)
		return ""
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		slog.Warn("consent: decoding the output choice envelope failed", "error", err)
		return ""
	}
	var choice struct {
		Choice string `json:"choice"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &choice); err != nil {
		slog.Warn("consent: decoding the output choice failed", "error", err)
		return ""
	}
	return choice.Choice
}

// nextFreeOutputPath appends the first numeric suffix naming a file
// that does not exist: "document-signed.pdf" -> "document-signed-2.pdf"
// -> "document-signed-3.pdf". Counting from 2 makes the sequence read
// as what it is — the second copy, then the third — rather than
// starting at a "-1" that implies a "-0" somewhere.
//
// The search is bounded: after maxOutputSuffix attempts it hands back
// the last candidate, and signInteractiveOne's own existence check
// then refuses it. A folder holding a thousand copies of one signed
// document is not a case worth spinning on.
func nextFreeOutputPath(outPath string) string {
	ext := filepath.Ext(outPath)
	base := strings.TrimSuffix(outPath, ext)
	candidate := outPath
	for n := 2; n <= maxOutputSuffix; n++ {
		candidate = fmt.Sprintf("%s-%d%s", base, n, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	return candidate
}

// maxOutputSuffix bounds nextFreeOutputPath's search.
const maxOutputSuffix = 1000

func newAuditStore() (*audit.Store, error) {
	dir := filepath.Join(platform.ConfigDir("windows", platform.OSEnv), "audit")
	return audit.NewStore(dir)
}
