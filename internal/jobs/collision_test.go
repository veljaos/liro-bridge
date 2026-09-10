package jobs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The rule both front doors ask: which documents in a batch cannot be
// written because their output names something the batch must not
// destroy.
//
// Pure, so it is tested here rather than through a window: neither
// --force nor the output-file question is an argument to it, which is
// the point — "overwrite" is a reasonable answer about a previous run's
// output and a destructive one about a document being signed now, and
// nothing at that level can tell the two apart.
func TestCollidingOutputs(t *testing.T) {
	dir := func(names ...string) []string {
		out := make([]string, len(names))
		for i, n := range names {
			out[i] = filepath.Join("C:\\", "docs", n)
		}
		return out
	}

	cases := []struct {
		name    string
		inputs  []string
		outputs []string
		want    []int
	}{
		{
			name:    "the ordinary batch collides with nothing",
			inputs:  dir("a.pdf", "b.pdf"),
			outputs: dir("a-signed.pdf", "b-signed.pdf"),
		},
		{
			name: "a pattern that matched a document and its own sibling",
			// `sign --in *.pdf` over a folder holding faktura.pdf and
			// faktura-signed.pdf. The sibling sorts first and is signed
			// first; the original's output is then the sibling itself.
			inputs:  dir("faktura-signed.pdf", "faktura.pdf"),
			outputs: dir("faktura-signed-signed.pdf", "faktura-signed.pdf"),
			want:    []int{1},
		},
		{
			name:    "three deep",
			inputs:  dir("f-signed-signed.pdf", "f-signed.pdf", "f.pdf"),
			outputs: dir("f-signed-signed-signed.pdf", "f-signed-signed.pdf", "f-signed.pdf"),
			want:    []int{1, 2},
		},
		{
			name: "two documents promised one output",
			// Not reachable from OutputPathFor, which is injective, but
			// reachable from the "write beside it" answer, which picks
			// the first name free on disk for each in turn.
			inputs:  dir("a.pdf", "b.pdf"),
			outputs: dir("x-signed.pdf", "x-signed.pdf"),
			want:    []int{1},
		},
		{
			name:   "the same path spelled two ways is one path",
			inputs: dir("a.pdf", "b.pdf"),
			// Built with this platform's own separator rather than
			// written out: a literal backslash is a separator on Windows
			// and an ordinary character everywhere else, so a spelling
			// hard-coded with one is not a spelling filepath.Clean
			// cleans anywhere else. Measured: written out, this case
			// passed on Windows and failed on a real Linux kernel.
			outputs: []string{
				dir("a-signed.pdf")[0],
				dir("sub")[0] + string(filepath.Separator) + ".." + string(filepath.Separator) + "a-signed.pdf",
			},
			want: []int{1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CollidingOutputs(tc.inputs, tc.outputs)
			if len(got) != len(tc.want) {
				t.Fatalf("CollidingOutputs = %v, want indices %v", got, tc.want)
			}
			for _, i := range tc.want {
				if !got[i] {
					t.Errorf("index %d not reported; got %v", i, got)
				}
			}
		})
	}
}

// TestCollidingOutputsFollowsThePlatformOnCase is the same choice
// LooksLikeOutput makes and for the same reason: Windows file names are
// case-insensitive, and treating "A.pdf" and "a.pdf" as one document
// anywhere else would refuse to sign a file over a collision that does
// not exist.
func TestCollidingOutputsFollowsThePlatformOnCase(t *testing.T) {
	inputs := []string{filepath.Join("d", "A-SIGNED.pdf"), filepath.Join("d", "a.pdf")}
	outputs := []string{filepath.Join("d", "A-SIGNED-signed.pdf"), filepath.Join("d", "a-signed.pdf")}

	got := CollidingOutputs(inputs, outputs)
	if runtime.GOOS == "windows" {
		if !got[1] {
			t.Error("on Windows a-signed.pdf and A-SIGNED.pdf are one file, and the collision was missed")
		}
		return
	}
	if got[1] {
		t.Error("off Windows they are two files, and a collision was invented")
	}
}

// TestCollidingOutputsIsNotADisguisedExistenceCheck: it answers a
// question about this batch, not about the disk. A file that exists and
// is not in the batch is a previous run's output — exactly what --force
// is for, and not this function's business.
func TestCollidingOutputsIsNotADisguisedExistenceCheck(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "a-signed.pdf")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := CollidingOutputs([]string{filepath.Join(dir, "a.pdf")}, []string{existing})
	if len(got) != 0 {
		t.Errorf("a file that exists but is not in the batch was reported as a collision: %v", got)
	}
}
