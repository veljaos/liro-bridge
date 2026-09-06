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

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
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
