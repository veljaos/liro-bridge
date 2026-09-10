//go:build windows

package main

// What a pattern does when a folder holds a document and its own
// -signed sibling (F9b review, question 2).
//
// D-230 claimed --force cannot reach the document being signed, because
// the output name is strictly longer than the input's. That is true of
// one document and says nothing about a batch: `sign --in *.pdf` over a
// folder holding faktura.pdf and faktura-signed.pdf expands to both, and
// faktura.pdf's output IS faktura-signed.pdf — an input of this same
// batch.

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// collisionFolder writes a real PDF under each of names and returns the
// folder and the inputs a pattern over it expands to, in the order the
// signing flow would take them.
func collisionFolder(t *testing.T, names ...string) (string, []interactiveInput) {
	t.Helper()
	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), blank, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := expandInteractiveInput(filepath.Join(dir, "*.pdf"))
	if err != nil {
		t.Fatalf("expandInteractiveInput: %v", err)
	}
	inputs := make([]interactiveInput, 0, len(files))
	for _, f := range files {
		in, err := newInteractiveInput(f)
		if err != nil {
			t.Fatalf("newInteractiveInput(%s): %v", f, err)
		}
		inputs = append(inputs, in)
	}
	return dir, inputs
}

// TestAPatternPutsTheSiblingFirst records the order, because the order
// is what decides whether the collision destroys a document before or
// after it has been signed — and the order is not arbitrary.
//
// expandInteractiveInput sorts the glob's matches. Comparing
// "faktura-signed.pdf" with "faktura.pdf", the first difference is the
// suffix's own first character against the extension's dot: '-' is 0x2D
// and '.' is 0x2E, so the already-signed sibling sorts first and is
// signed first. A configured suffix beginning with a character above
// '.' — "_signed", say — reverses that, and the sibling is destroyed
// before it is read.
func TestAPatternPutsTheAlreadySignedSiblingFirst(t *testing.T) {
	_, inputs := collisionFolder(t, "faktura.pdf", "faktura-signed.pdf")
	if len(inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(inputs))
	}
	if got := filepath.Base(inputs[0].path); got != "faktura-signed.pdf" {
		t.Errorf("first input = %q, want faktura-signed.pdf", got)
	}
}

// TestNoOutputInABatchIsAnotherDocumentInTheSameBatch is the property
// D-230 should have claimed and did not.
//
// Measured failing before the fix, with the same folder and the same
// call: "signing faktura.pdf writes to faktura-signed.pdf, which is
// document 0 of this same batch (overwrite=true)".
func TestNoOutputInABatchIsAnotherDocumentInTheSameBatch(t *testing.T) {
	c := i18n.Load("en")

	// force = true takes resolveOutputConflict's first branch, so it
	// touches neither the window nor the message channel and this runs
	// headless. That is also exactly the path under test.
	_, inputs := collisionFolder(t, "faktura.pdf", "faktura-signed.pdf")
	outputs, settled := resolveOutputsIn(nil, nil, c, inputs, "", "-signed", true)
	if !settled {
		t.Fatal("resolveOutputsIn did not settle")
	}

	byInput := map[string]bool{}
	for _, in := range inputs {
		byInput[strings.ToLower(in.path)] = true
	}
	written := map[string]int{}
	for i, out := range outputs {
		if out.collides {
			// Marked, and the run skips it. Nothing is written here.
			continue
		}
		if byInput[strings.ToLower(out.path)] {
			t.Errorf("signing %s writes to %s, which is document %d of this same batch (overwrite=%v)",
				filepath.Base(inputs[i].path), filepath.Base(out.path), indexOfPath(inputs, out.path), out.overwrite)
		}
		// Two outputs landing on one path is the same defect one step
		// over: the second signature silently replaces the first.
		key := strings.ToLower(out.path)
		if prev, ok := written[key]; ok {
			t.Errorf("documents %d and %d both write to %s", prev, i, filepath.Base(out.path))
		}
		written[key] = i
	}

	// And the batch is not simply refused wholesale: the sibling still
	// gets signed, which is what the person asked for.
	if outputs[0].collides {
		t.Error("faktura-signed.pdf was skipped; only the document whose output would destroy it should be")
	}
	if !outputs[1].collides {
		t.Error("faktura.pdf was not marked as colliding, so nothing stops it replacing document 0")
	}
}

// TestTheCollisionIsDecidedBeforeAnybodyIsAsked: a document that is
// going to be skipped must not be the subject of a question whose
// answer would then be ignored.
//
// Observed rather than reasoned about: resolveOutputsIn is called with
// force = false, so any question it asks would go to the window — and
// the window here is nil. A question asked would panic; the test
// passing is the evidence that none was.
func TestTheCollisionIsDecidedBeforeAnybodyIsAsked(t *testing.T) {
	c := i18n.Load("en")
	_, inputs := collisionFolder(t, "faktura.pdf", "faktura-signed.pdf")

	outputs, settled := resolveOutputsIn(nil, nil, c, inputs, "", "-signed", false)
	if !settled {
		t.Fatal("resolveOutputsIn did not settle")
	}
	if !outputs[1].collides {
		t.Fatal("faktura.pdf was not marked as colliding")
	}
}

func indexOfPath(inputs []interactiveInput, path string) int {
	for i, in := range inputs {
		if strings.EqualFold(in.path, path) {
			return i
		}
	}
	return -1
}

// TestABatchNeverDestroysOneOfItsOwnDocuments is the same property with
// real signatures on disk: the run itself, not the resolution of paths
// before it.
//
// Every input is hashed before the batch runs and again afterwards, and
// the one that matters is faktura-signed.pdf — a document this batch
// signed, whose bytes must still be the bytes that were signed.
func TestABatchNeverDestroysOneOfItsOwnDocuments(t *testing.T) {
	dir, inputs := collisionFolder(t, "faktura.pdf", "faktura-signed.pdf")

	before := map[string][]byte{}
	for _, in := range inputs {
		before[in.path] = digestOf(t, in.path)
	}

	cfg := config.Default()
	cfg.VisibleStamp = false
	paths := make([]string, 0, len(inputs))
	for _, in := range inputs {
		paths = append(paths, in.path)
	}
	m, _ := testMainWindow(t, "sr-Latn", cfg, paths)
	m.inputs = inputs

	outputs, settled := resolveOutputsIn(nil, nil, m.c, inputs, "", cfg.OutputSuffix, true)
	if !settled {
		t.Fatal("resolveOutputsIn did not settle")
	}

	d := decisionFor(t, m, dir)
	d.outputs = outputs
	defer func() { _ = d.session.Close() }()
	m.runBatch(context.Background(), d)

	for _, in := range inputs {
		after := digestOf(t, in.path)
		if string(after) == string(before[in.path]) {
			continue
		}
		t.Errorf("%s was rewritten by this batch; it is one of the documents the batch signed, not a previous run's output",
			filepath.Base(in.path))
	}
}

func digestOf(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	return sum[:]
}
