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
	"github.com/veljaos/liro-bridge/internal/pades/dss"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// runSignCommand is `liro-bridge sign`: the same signing flow the
// window opens, entered one step in.
//
// The documents came in on the command line, so the flow has no
// document step; everything after that — the certificate and the
// approval, the signing method, the position, the timestamp question,
// the output paths, the progress and the report — is the same window
// and the same code the tray's own window runs. There is no second
// implementation of the consent screen to keep right (SPEC §6.5), and
// no window opens on top of another one.
//
// This was what `sign --interactive` did. It is now what `sign` does,
// with no way to ask for anything else: SPEC §18.2 forbids a signature
// without human approval and says no flag bypasses the consent screen,
// and SPEC §4.3 puts every entry point through the same screen, session
// and audit log. The path that asked nobody was F3-era code that
// predated the consent screen (F5) and had never been reconciled with
// either rule; it exists now only in a build made with the "softtoken"
// tag, under its own name (noconsent_softtoken.go).
//
// Windows-only, matching internal/ui's own scope (SPEC §11.11).
func runSignCommand(ctx context.Context, args []string, out io.Writer, locale string, cfg config.Config) int {
	c := i18n.Load(locale)

	inPattern, force, err := parseSignArgs(args)
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

	return runSigningFlow(ctx, cfg, locale, flowRequest{inputs: inputs, force: force})
}

// recordInteractiveAudit appends one batch outcome. A store that could
// not be opened (auditErr), or an append that fails, does not stop a
// signature that has already happened — but it is said out loud in the
// log rather than swallowed.
//
// Both errors used to be discarded outright (`_, _ = store.Append(...)`
// and a bare `return`), which made a real and reachable state silent:
// measured during FTEST, a single unparseable line anywhere in the log
// — one truncated last line is what a power cut leaves — makes every
// subsequent Append fail forever, because the chain's last entry cannot
// be read and so the next PrevHash cannot be computed.
//
// That is now recovered from rather than merely reported: the store
// leaves the broken file exactly as it is and starts a new chain beside
// it, whose first entry records the break. The returned Discontinuity is
// that record, and it is non-nil for exactly one batch — the one whose
// entry opened the new chain — which is what makes "tell the person
// once, not on every subsequent signature" true without a flag anybody
// has to clear.
// auditRecord is one batch's worth of what the log records, gathered
// into a value because the argument list had grown past the point where
// a reader could tell which of eight positional arguments was which —
// and because F7 adds two more: who asked, and which front door they
// asked through.
type auditRecord struct {
	thumbprint string

	// application is the name bound at pairing, or "local" for a batch
	// a person started here. Never a name supplied in a request (SPEC
	// §6.6).
	application string

	// channel is which front door the batch came through (F7 §6): the
	// hash path never let the agent see a document, and that is a
	// different fact about a signature from the other two.
	channel audit.Channel

	documents int
	outcome   audit.Outcome
	lastErr   error
	isTestKey bool
	level     string
}

func recordInteractiveAudit(store *audit.Store, auditErr error, rec auditRecord) *audit.Discontinuity {
	application := rec.application
	if application == "" {
		application = consent.ApplicationLocal
	}
	if auditErr != nil {
		slog.Error("audit: the log could not be opened, so this batch is not recorded",
			"error", auditErr, "outcome", rec.outcome, "documents", rec.documents)
		return nil
	}
	entry, err := store.Append(audit.Entry{
		Timestamp:     time.Now(),
		Thumbprint:    rec.thumbprint,
		Application:   application,
		DocumentCount: rec.documents,
		Outcome:       rec.outcome,
		FailureCode:   codeOfInteractive(rec.lastErr),
		IsTestKey:     rec.isTestKey,
		AchievedLevel: rec.level,
		Channel:       rec.channel,
	})
	if err != nil {
		// No file name, no personal name, no document content: SPEC
		// §18.3. The outcome and the count are already what the entry
		// itself would have carried.
		slog.Error("audit: this batch could not be appended to the log",
			"error", err, "outcome", rec.outcome, "documents", rec.documents)
		return nil
	}
	if entry.Discontinuity != nil {
		// A fact about the log itself, not about this batch, and worth
		// a line of its own: the previous chain could not be continued,
		// so it was left where it was and this entry opened a new one.
		// The previous file's name is this program's own generated name
		// and carries nothing personal (SPEC §18.3).
		slog.Warn("audit: the previous chain could not be continued, so a new one was started beside it",
			"previousFile", entry.Discontinuity.PreviousFile,
			"previousChain", entry.Discontinuity.PreviousChain,
			"line", entry.Discontinuity.Line,
			"reason", entry.Discontinuity.Reason)
	}
	return entry.Discontinuity
}

// auditChainNotice is what the person is told, once, when the log
// continued in a new file: that it did, and where it is.
//
// A notice rather than an error. Nothing about their signature went
// wrong, the batch is recorded, and the old file is still there — what
// changed is which file the log is being written to, and that is the one
// thing they cannot find out any other way.
func auditChainNotice(c *i18n.Catalogue, store *audit.Store, d *audit.Discontinuity) string {
	if d == nil || store == nil {
		return ""
	}
	newFile, err := store.LatestChainFile()
	if err != nil || newFile == "" {
		slog.Warn("audit: could not name the new chain file", "error", err)
		return fmt.Sprintf(c.T("audit.chain_continued_here"), d.PreviousFile, store.Dir())
	}
	return fmt.Sprintf(c.T("audit.chain_continued"), d.PreviousFile, newFile, store.Dir())
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
		_ = win.PostJSON(askTSAChoicePayload(reason, c))

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
			// Its own pairing store: this is a `sign` process, not
			// the tray, so nothing else in it holds one
			// and there is no second copy to disagree with.
			if err := runSettingsWindow(cfg, win.Handle(), openPairingsOrNil()); err != nil {
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
// every other button in this window uses (D-083), so the page->Go
// message surface stays at exactly three types.
func readTSAChoice(win ui.Window) string {
	switch readWindowAction(win, "the timestamp choice") {
	case "tsaWithoutTimestamp":
		return "withoutTimestamp"
	case "tsaConfigure":
		return "configure"
	default:
		return ""
	}
}

// readWindowAction is the one read behind every recorded action:
// bridge.js's __liroAction, decoded out of ExecuteScript's own JSON
// envelope. what names the thing being read, for the log line when it
// cannot be.
func readWindowAction(win ui.Window, what string) string {
	raw, err := win.Eval("window.__liroAction()")
	if err != nil {
		slog.Warn("consent: reading "+what+" failed", "error", err)
		return ""
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		slog.Warn("consent: decoding the envelope of "+what+" failed", "error", err)
		return ""
	}
	var recorded struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &recorded); err != nil {
		slog.Warn("consent: decoding "+what+" failed", "error", err)
		return ""
	}
	return recorded.Action
}

// interactiveInput is one document waiting to be signed: where it is,
// and the digest the certificate step's batch fingerprint is built from
// (SPEC §6.6).
//
// It deliberately does not hold the document's bytes. It used to, read
// in full for every input before the approval was even shown, which
// is fine for the one or two files a command line is given and is not
// fine for F6's own stated case of two hundred dropped at once — a
// two-hundred-document batch of ordinary contracts is hundreds of
// megabytes held for the whole run, most of it long before and long
// after the moment each document is actually needed. Each document is
// now read at the moment it is signed and released when it is written.
type interactiveInput struct {
	path   string
	digest []byte

	// label is the name the consent window shows, for a batch whose
	// documents have no path — one that arrived over the protocol,
	// where the caller supplied display names and the agent may never
	// see a file at all (F7 §5). Empty for a local batch, where the
	// name comes from the path, which is the only place it can.
	label string
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

// parseSignArgs parses `sign`'s arguments. English, like every other
// argument-parsing diagnostic and like --help itself (D-092, SPEC §9.2):
// this is a person learning how to invoke the program, not a signer
// reading about their own signature.
func parseSignArgs(args []string) (in string, force bool, err error) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--interactive":
			// Accepting and ignoring it would advertise a distinction
			// that no longer exists, and this project deletes surface
			// that does nothing rather than leaving it as a trap
			// (D-132, D-146, D-151, D-175). Saying why is cheaper than
			// "unrecognised argument" for the one flag most likely to
			// still be typed.
			return "", false, errors.New("--interactive is gone: sign always shows the consent window")
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

// outputPathIn is where one document's signature goes: beside the
// input when dir is empty (F5's behaviour, and F6 §4's default), or in
// dir when the user has chosen one. jobs.OutputPathFor is the single
// implementation — the main window shows the same path this signs to,
// and two functions computing it would eventually show one and write
// the other.
func outputPathIn(in, dir, suffix string) string {
	return jobs.OutputPathFor(in, dir, suffix)
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

// openInteractiveSession opens the signing session for the certificate
// the person approved. hwnd is the signing window's own HWND
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
	return list.CACertificates()
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

	// revocationMemory is this batch's memory of revocation endpoints
	// that did not answer (J-10). One value for the whole batch, a
	// fresh one for the next: an OCSP responder that accepts a request
	// and never answers costs 20 s per document, and MUP's real
	// responder is measured as exactly that (D-076).
	revocationMemory *dss.EndpointMemory
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
		RevocationMemory:  opts.revocationMemory,
	})
	if err != nil {
		return nil, err
	}
	// Written beside the target and renamed over it, never straight
	// onto it (J-8): a program watching the output folder must not be
	// able to pick up an empty or partial signed document, and a write
	// that fails partway must not destroy a previously good one.
	//
	// The signature succeeded; only the write can fail here, which is
	// why neither outcome is SIGN_FAILED — that would send the user to
	// check the card for a problem on the disk (D-104). WriteFileAtomic
	// distinguishes the two cases that need different answers:
	// OUTPUT_IN_USE (close the file that is open) and
	// OUTPUT_WRITE_FAILED (everything else).
	if err := platform.WriteFileAtomic(opts.outPath, result.Bytes, 0o600); err != nil {
		return nil, err
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
	_ = win.PostJSON(askOutputExistsPayload(outPath, renamePath, c))

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

// alreadySignedAnswer is what the person said about the documents in
// this batch whose names already end in the output suffix (J-3).
type alreadySignedAnswer int

const (
	alreadySignedCancelled alreadySignedAnswer = iota
	alreadySignedSkip
	alreadySignedSignAnyway
)

// resolveAlreadySigned asks J-3's question once for the whole batch and
// returns which documents are to be signed.
//
// It is asked here, beside the timestamp and output-file questions and
// before the card session is opened, for the same reason those are: a
// person who cancels must not have spent a PIN entry on a batch that
// was never going to be written. The set it answers about is decided
// from the input names and the configured suffix alone, so nothing has
// to be read or signed to ask it.
//
// The whole batch gets one answer. A hundred identical questions is not
// a hundred choices (SPEC §12.10's own reasoning, and D-104's for the
// output-file question).
func resolveAlreadySigned(win ui.Window, messages chan ui.Message, closed <-chan struct{}, c *i18n.Catalogue, inputs []interactiveInput, suffix string) (skip map[string]bool, proceed bool) {
	var already []string
	for _, in := range inputs {
		if jobs.LooksLikeOutput(in.path, suffix) {
			already = append(already, in.path)
		}
	}
	if len(already) == 0 {
		return nil, true
	}

	if err := win.PostJSON(askAlreadySignedPayload(len(already), suffix, c)); err != nil {
		// A question that could not be put on screen must not be
		// answered on the person's behalf. The safe answer is the one
		// that signs nothing it was not clearly asked to.
		slog.Error("consent: could not ask about already-signed documents", "error", err)
		return nil, false
	}

	var msg ui.Message
	select {
	case msg = <-messages:
	case <-closed:
		// The window was closed while the question was on screen. That
		// is a refusal, and it is not a state to wait in forever.
		return nil, false
	}
	if msg.Type != ui.MessageTypeApprove {
		return nil, false
	}

	switch readAlreadySignedChoice(win) {
	case alreadySignedSkip:
		skip = make(map[string]bool, len(already))
		for _, p := range already {
			skip[p] = true
		}
		return skip, true
	case alreadySignedSignAnyway:
		return nil, true
	default:
		slog.Warn("consent: already-signed screen sent approve with no choice")
		return nil, false
	}
}

// readAlreadySignedChoice reads back which of the two proceeding
// actions the page recorded, exactly as readOutputChoice does.
func readAlreadySignedChoice(win ui.Window) alreadySignedAnswer {
	switch readWindowAction(win, "the already-signed choice") {
	case "alreadySkip":
		return alreadySignedSkip
	case "alreadySign":
		return alreadySignedSignAnyway
	default:
		return alreadySignedCancelled
	}
}

// readOutputChoice reads back which of the two proceeding actions the
// page recorded, exactly as readTSAChoice does.
func readOutputChoice(win ui.Window) string {
	switch readWindowAction(win, "the output choice") {
	case "outputOverwrite":
		return "overwrite"
	case "outputRename":
		return "rename"
	default:
		return ""
	}
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
