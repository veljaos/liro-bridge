//go:build windows

package main

// A remembered stamp position is an answer to "where on this document",
// given by a person who was looking at the page. A request that arrived
// over the protocol carries documents nobody has looked at, so it never
// inherits that answer.

import (
	"path/filepath"
	"testing"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// placedConfig is a configuration with a position somebody placed by
// hand: page 2, well inside an A4 page, and nowhere near a corner.
func placedConfig() config.Config {
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 2
	cfg.StampX, cfg.StampY = 371, 79
	return cfg
}

// The defect, stated as the property the engine actually sees: a
// protocol batch with no stamp in the request must not be signed at
// explicit coordinates.
//
// The owner's first protocol signature landed the stamp on top of the
// document's existing MUP signature, because the position remembered
// from a document he had placed one on by hand was applied to a
// document he had not.
func TestAProtocolBatchNeverSignsAtTheRememberedPosition(t *testing.T) {
	c := i18n.Load("sr-Latn")
	local := stampOptionsFor(c, placedConfig())
	if local == nil || !local.UseXY {
		t.Fatal("the fixture does not reproduce a remembered position for a local batch; nothing is being measured")
	}
	if local.Page != 2 || local.X != 371 || local.Y != 79 {
		t.Fatalf("a local batch signs at %+v, want the placed page and coordinates", local)
	}

	for _, tc := range []struct {
		name string
		req  func(t *testing.T) requestUnderTest
	}{
		{"documents, no stamp supplied", func(t *testing.T) requestUnderTest {
			return requestUnderTest{req: documentRequest(t, 1)}
		}},
		{"documents, a stamp supplied", func(t *testing.T) requestUnderTest {
			r := documentRequest(t, 1)
			r.Stamp = &consent.StampChoice{Visible: true, Position: consent.StampPositionTopLeft}
			return requestUnderTest{req: r, wantPosition: consent.StampPositionTopLeft}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			under := tc.req(t)
			m := protocolWindowWith(t, placedConfig(), under.req)

			opts := stampOptionsFor(m.c, m.cfg)
			if opts == nil {
				t.Fatal("the batch would draw no stamp at all")
			}
			if opts.UseXY {
				t.Errorf("a protocol batch signs at the remembered coordinates (page %d, %v, %v)",
					opts.Page, opts.X, opts.Y)
			}
			if m.cfg.StampPosition == config.StampPositionCustom {
				t.Error("the run still holds the placed position as its stamp position")
			}
			if m.cfg.StampPlacedPage != 0 || m.cfg.StampX != 0 || m.cfg.StampY != 0 {
				t.Errorf("the placed page and coordinates survived into the run: page %d, %v, %v",
					m.cfg.StampPlacedPage, m.cfg.StampX, m.cfg.StampY)
			}
			if under.wantPosition != "" && m.cfg.StampPosition != under.wantPosition {
				t.Errorf("the corner is %q, want the one the caller supplied (%q)",
					m.cfg.StampPosition, under.wantPosition)
			}
		})
	}
}

type requestUnderTest struct {
	req          api.SignRequest
	wantPosition string
}

// A batch with no stamp supplied still asks, and what it offers is the
// corner method rather than the position method — because there is no
// remembered position left to offer.
func TestAProtocolBatchWithNoStampOffersACornerNotAPlacedPosition(t *testing.T) {
	m := protocolWindowWith(t, placedConfig(), documentRequest(t, 1))

	if got := stampMethodOf(m.cfg); got != stampMethodCorners {
		t.Fatalf("the method screen would preselect %q, want %q", got, stampMethodCorners)
	}
	steps := m.stepsFor()
	if len(steps) != 2 || steps[1] != stepMethod {
		t.Fatalf("steps = %v, want the approval and the method — the person still chooses", steps)
	}
}

// Nothing is written to disk. The remembered position is the person's
// own standing answer, and a request from a program does not change it.
func TestAProtocolBatchLeavesTheRememberedPositionOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, placedConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	m := protocolWindowWith(t, before, documentRequest(t, 1))
	if m.cfg.StampPosition == config.StampPositionCustom {
		t.Fatal("the run inherited the placed position")
	}

	after, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if after.StampPosition != config.StampPositionCustom ||
		after.StampPlacedPage != before.StampPlacedPage ||
		after.StampX != before.StampX || after.StampY != before.StampY {
		t.Fatalf("the configuration on disk changed: %q page %d (%v, %v)",
			after.StampPosition, after.StampPlacedPage, after.StampX, after.StampY)
	}
}

// withoutRememberedPlacement drops the position and nothing else: which
// corner a person chose for the case where they had not placed one, the
// page, the reference line and the identity-document answer are all
// theirs and are left alone.
func TestDroppingTheRememberedPositionLeavesEverythingElse(t *testing.T) {
	cfg := placedConfig()
	cfg.StampPage = config.StampPageLast
	cfg.StampReference = "Ugovor 12/2026"
	cfg.StampShowDocumentID = true

	got := withoutRememberedPlacement(cfg)
	if got.StampPosition != config.DefaultStampPosition {
		t.Errorf("StampPosition = %q, want the default corner", got.StampPosition)
	}
	if got.StampPage != config.StampPageLast {
		t.Errorf("StampPage = %q, want it untouched", got.StampPage)
	}
	if got.StampReference != "Ugovor 12/2026" || !got.StampShowDocumentID || !got.VisibleStamp {
		t.Errorf("something other than the position changed: %+v", got)
	}

	// A configuration that never held a placed position is unchanged
	// except for the coordinates it was not using anyway.
	corner := config.Default()
	corner.StampPosition = consent.StampPositionTopRight
	if got := withoutRememberedPlacement(corner); got.StampPosition != consent.StampPositionTopRight {
		t.Errorf("a chosen corner became %q", got.StampPosition)
	}
}
