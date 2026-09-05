//go:build windows

package main

// The consent phase, shared by both things that sign: `sign
// --interactive` on the command line and the main window F6 §1 adds.
//
// It exists as one function for one reason. SPEC §6.5 calls the consent
// screen "the only real gate" and the most important paragraph in the
// specification; a second implementation of it, however carefully
// written, is a second thing that has to stay right about certificate
// choice, the timestamp question, the output-file question and the
// audit entry. So the main window does not draw its own consent screen
// — it opens this one, which is the same window, the same page and the
// same code the command line has been using since F5.
//
// Everything up to and including opening the card session happens here.
// What comes after — the queue, the per-document states, Stop, the
// report — is the caller's, because that is exactly what differs
// between a command line printing lines and a window drawing rows.

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// consentDecision is everything a run needs once the human has said
// yes. A zero value with approved == false means they did not.
type consentDecision struct {
	approved   bool
	thumbprint string

	// session is open and must be closed by the caller.
	session keysource.Session

	level     pades.Level
	tsaClient *tsa.Client
	allowBB   bool

	// outputs is one entry per input, in the same order, already
	// settled — F6 §4 and D-104: every output path is decided before
	// the card session opens, so cancelling never costs a PIN entry.
	outputs []interactiveOutput

	// cfg is the configuration as it stands after the consent window,
	// which may have changed it (the timestamp question can open
	// Settings; the stamp choice is persisted).
	cfg config.Config

	// stamp is the visible-stamp choice the window was showing.
	stamp consent.StampChoice
}

// consentRequest is what the consent window is being asked about.
type consentRequest struct {
	inputs []interactiveInput
	cfg    config.Config
	locale string
	// force skips the output-file question, matching the command
	// line's --force.
	force bool
	// outputDir is where signatures go, or empty for beside each input
	// (F6 §4).
	outputDir string
	// outputSuffix names the signed file.
	outputSuffix string
}

// askForConsent opens the consent window, runs it to a decision, and —
// when approved — settles the timestamp question, the output paths, and
// opens the card session.
//
// The window is closed before this returns, on every path. The caller
// gets a decision and a session, not a window: a consent window still
// on screen while a batch runs would be two windows claiming to be in
// charge of the same batch.
func askForConsent(ctx context.Context, req consentRequest) consentDecision {
	c := i18n.Load(req.locale)
	cfg := req.cfg

	report, err := gatherInteractiveCertificates(ctx)
	if err != nil {
		slog.Error("consent: listing certificates failed", "error", err)
		return consentDecision{}
	}
	certInfos := make([]classify.Info, 0, len(report.Certificates))
	for _, row := range report.Certificates {
		if row.Hidden() {
			continue
		}
		certInfos = append(certInfos, row.Info)
	}

	digests := make([][]byte, len(req.inputs))
	fileNames := make([]string, len(req.inputs))
	for i, in := range req.inputs {
		digests[i] = in.digest
		fileNames[i] = filepath.Base(in.path)
	}
	vm := consent.BuildViewModel(consent.ApplicationLocal, digests, fileNames, certInfos)
	vm.Stamp = consent.StampChoice{Visible: cfg.VisibleStamp, Position: cfg.StampPosition}.Normalised()

	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("consent.window_title"),
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
		slog.Error("consent: could not open the window", "error", err)
		return consentDecision{}
	}
	closed := false
	closeWindow := func() {
		if !closed {
			closed = true
			_ = win.Close()
		}
	}
	defer closeWindow()

	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		slog.Error("consent: could not post the batch", "error", err)
		return consentDecision{}
	}

	auditStore, auditErr := newAuditStore()

	selected := ""
	for decided := false; !decided; {
		msg := <-messages
		switch msg.Type {
		case ui.MessageTypeSelectCertificate:
			selected = msg.Thumbprint
		case ui.MessageTypeApprove:
			if selected != "" {
				decided = true
			}
		case ui.MessageTypeCancel:
			recordInteractiveAudit(auditStore, auditErr, selected, len(req.inputs), audit.OutcomeDenied, nil, false, "")
			return consentDecision{}
		}
	}

	// Read at the moment Approve is pressed — the page owns it until
	// then — and saved, so the next run starts from the same answer
	// (D-103). Only on approval: a cancelled window has nothing to
	// save and may already be gone.
	stamp := readStampChoice(win, vm.Stamp)
	cfg = persistStampChoice(cfg, stamp)

	// The timestamp question, before the card is touched (D-095): a
	// user who cancels here has not spent a PIN entry on a batch that
	// was never going to be saved.
	level := interactiveLevel(cfg)
	var tsaClient *tsa.Client
	allowBB := level == pades.LevelBB
	if level != pades.LevelBB {
		client, buildErr := buildTSAClient(cfg)
		if buildErr != nil {
			pushFailure(win, c, buildErr)
			waitForClose(messages)
			return consentDecision{}
		}
		tsaClient = client
		if tsaClient == nil {
			var proceed bool
			cfg, tsaClient, allowBB, proceed = resolveTSAChoice(win, messages, c, cfg, req.locale, consent.TSAReasonNotConfigured)
			if !proceed {
				recordInteractiveAudit(auditStore, auditErr, selected, len(req.inputs), audit.OutcomeDenied, nil, false, "")
				return consentDecision{}
			}
		}
	}

	// Then the output paths, for the same reason and in the same place
	// (D-104).
	outputs, settled := resolveOutputsIn(win, messages, c, req.inputs, req.outputDir, req.outputSuffix, req.force)
	if !settled {
		recordInteractiveAudit(auditStore, auditErr, selected, len(req.inputs), audit.OutcomeDenied, nil, false, "")
		return consentDecision{}
	}

	session, err := openInteractiveSession(ctx, keysource.Thumbprint(selected), win.Handle())
	if err != nil {
		pushFailure(win, c, err)
		waitForClose(messages)
		return consentDecision{}
	}

	// The window's work is done. Closing it here, rather than leaving
	// it up, is what keeps one window in charge of the batch at a time.
	closeWindow()

	return consentDecision{
		approved:   true,
		thumbprint: selected,
		session:    session,
		level:      level,
		tsaClient:  tsaClient,
		allowBB:    allowBB,
		outputs:    outputs,
		cfg:        cfg,
		stamp:      stamp,
	}
}

// approve is the main window's own call into the consent phase.
func (m *mainWindow) approve(ctx context.Context) (consentDecision, bool) {
	items := m.queue.Items()
	inputs := make([]interactiveInput, 0, len(items))
	for _, it := range items {
		in, err := newInteractiveInput(it.Path)
		if err != nil {
			// A document that cannot even be read for its digest is
			// not something to ask consent about. It is reported in
			// the queue instead, where the rest of the batch's own
			// failures are.
			slog.Warn("main window: a document could not be read for the batch fingerprint",
				"error", err)
			in = interactiveInput{path: it.Path, digest: nil}
		}
		inputs = append(inputs, in)
	}

	d := askForConsent(ctx, consentRequest{
		inputs:       inputs,
		cfg:          m.cfg,
		locale:       m.locale,
		outputDir:    m.cfg.OutputFolder,
		outputSuffix: m.cfg.OutputSuffix,
	})
	if !d.approved {
		return consentDecision{}, false
	}
	m.cfg = d.cfg
	return d, true
}

// resolveOutputsIn is resolveInteractiveOutputs with F6 §4's chosen
// output folder threaded through. An empty dir keeps F5's behaviour
// exactly: beside each input.
func resolveOutputsIn(win ui.Window, messages chan ui.Message, c *i18n.Catalogue, inputs []interactiveInput, dir, suffix string, force bool) ([]interactiveOutput, bool) {
	answer := outputConflictUnanswered
	out := make([]interactiveOutput, 0, len(inputs))
	for _, in := range inputs {
		path, overwrite, proceed := resolveOutputConflict(win, messages, c,
			outputPathIn(in.path, dir, suffix), force, &answer)
		if !proceed {
			return nil, false
		}
		out = append(out, interactiveOutput{path: path, overwrite: overwrite})
	}
	return out, true
}

// consentWindowWidth/Height are the consent window's measured size
// (D-106), named here because two files now create that window.
const (
	consentWindowWidth  = 520
	consentWindowHeight = 860
)
