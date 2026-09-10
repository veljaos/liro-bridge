//go:build windows

package main

// What the consent phase needs that is not a screen: the decision it
// produces, the certificate behind the chosen thumbprint, and the stamp
// the engine will draw.
//
// The screens themselves are steps of one window now
// (signflow_windows.go). There is still exactly one implementation of
// them, which is the point SPEC §6.5 makes about the consent screen
// being the product's only real gate: the command line and the window
// reach the same steps, in the same order, through the same code.

import (
	"crypto/x509"
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
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

	// cfg is the configuration as it stands after the flow, which may
	// have changed it (the timestamp question can open Settings; the
	// method step persists how to sign).
	cfg config.Config
}

// firstInputPath is the document the placement window opens on: the
// first of the batch. F6b §4 names it, and it is the right one — a
// batch is a batch because its documents are the same shape, and the
// position chosen applies to every one of them.
func firstInputPath(inputs []interactiveInput) string {
	if len(inputs) == 0 {
		return ""
	}
	return inputs[0].path
}

// certificateFor finds the chosen certificate among the enumerated
// rows, so the stamp in the placement window carries the real name,
// serial and date rather than a stand-in.
func certificateFor(report cli.Report, thumbprint string) *x509.Certificate {
	for _, row := range report.Certificates {
		if row.Info.Thumbprint != thumbprint || len(row.DER) == 0 {
			continue
		}
		cert, err := x509.ParseCertificate(row.DER)
		if err != nil {
			return nil
		}
		return cert
	}
	return nil
}

// stampOptionsFor turns the configuration into the stamp the signing
// engine draws, or nil for an invisible signature — which is the whole
// of SPEC §13.4's default path, untouched, because every line that
// reads a stamp is skipped when this is nil.
//
// One function for both entry points: the window and the command line
// must not be able to disagree about what the answer to the method
// question means.
func stampOptionsFor(c *i18n.Catalogue, cfg config.Config) *pades.StampOptions {
	opts := interactiveStampOptions(c, consent.StampChoice{
		Visible:  cfg.VisibleStamp,
		Position: cfg.StampPosition,
	}.Normalised())
	if opts == nil {
		return nil
	}
	opts.Page = stampPageNumber(cfg.StampPage)
	opts.Reference = cfg.StampReference
	opts.ShowDocumentID = cfg.StampShowDocumentID

	// A placed position overrides both the corner and the page: it was
	// chosen by looking at a page, so the page it was chosen on is part
	// of it (F6b §3). The engine clamps it into each document's own box
	// and falls back to that document's last page when the saved one is
	// past the end, reporting both afterwards.
	if cfg.StampPosition == config.StampPositionCustom && cfg.StampPlacedPage >= 1 {
		opts.UseXY = true
		opts.X, opts.Y = cfg.StampX, cfg.StampY
		opts.Page = cfg.StampPlacedPage
	}
	return opts
}

// resolveOutputsIn is resolveInteractiveOutputs with F6 §4's chosen
// output folder threaded through. An empty dir keeps F5's behaviour
// exactly: beside each input.
//
// No output it hands back may be another document in the same batch, or
// a path an earlier document in the batch has already been promised.
// Those are marked `collides` and the run skips them — see
// jobs.CollidingOutputs for the case, which is a pattern rather than a
// single file and which the output-file question cannot help with,
// because "overwrite" is a perfectly reasonable answer about a previous
// run's output and a destructive one about a document being signed now.
// Neither the question nor --force distinguishes them; this does.
func resolveOutputsIn(win ui.Window, messages chan ui.Message, c *i18n.Catalogue, inputs []interactiveInput, dir, suffix string, force bool) ([]interactiveOutput, bool) {
	// Where each document would go, before anybody has been asked
	// anything. The collisions are decided from this and not from what
	// the person answers, so a document that is going to be skipped is
	// never the subject of a question whose answer would be ignored.
	intended := make([]string, len(inputs))
	inPaths := make([]string, len(inputs))
	for i, in := range inputs {
		intended[i] = outputPathIn(in.path, dir, suffix)
		inPaths[i] = in.path
	}
	colliding := jobs.CollidingOutputs(inPaths, intended)

	answer := outputConflictUnanswered
	out := make([]interactiveOutput, 0, len(inputs))
	settledPaths := make([]string, 0, len(inputs))
	for i := range inputs {
		if colliding[i] {
			out = append(out, interactiveOutput{path: intended[i], collides: true})
			settledPaths = append(settledPaths, intended[i])
			continue
		}
		path, overwrite, proceed := resolveOutputConflict(win, messages, c, intended[i], force, &answer)
		if !proceed {
			return nil, false
		}
		out = append(out, interactiveOutput{path: path, overwrite: overwrite})
		settledPaths = append(settledPaths, path)
	}

	// A backstop over the answers themselves. "Write beside it" picks
	// the first name free on disk, which can never be an input — an
	// input exists — but two of them could pick one name as each other.
	for i := range jobs.CollidingOutputs(inPaths, settledPaths) {
		out[i].collides = true
	}

	for _, o := range out {
		if o.collides {
			// No file name here: SPEC §18.3 forbids one in any log file,
			// and the count is what a reader of the log needs anyway.
			// The report screen names them; that is a screen, not a log.
			slog.Warn("consent: a document's output would replace another document in this batch, so it will be skipped")
		}
	}
	return out, true
}
