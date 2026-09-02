package pdf

import (
	"bytes"
	"testing"
)

// TestIncrementalUpdatePreservesOriginalBytesAsPrefix is F3 §3.4's first,
// most direct test: bytes.HasPrefix(out, in), asserted literally.
func TestIncrementalUpdatePreservesOriginalBytesAsPrefix(t *testing.T) {
	for name, build := range map[string]func() []byte{
		"classic": buildClassicFixture,
		"stream":  buildStreamFixture,
	} {
		t.Run(name, func(t *testing.T) {
			in := build()
			doc, err := Parse(in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			u := NewUpdate(doc)
			marker := u.NewObjectNumber()
			u.Set(marker, Dict{Name("Type"): Name("Marker")})
			out, err := u.Apply()
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !bytes.HasPrefix(out, in) {
				t.Fatal("output is not a byte-for-byte prefix extension of the input")
			}
		})
	}
}

// TestIncrementalUpdateMatchesMechanism is F3 §3.2: an incremental update
// must match the mechanism of the input's *last* revision, never scan
// the whole file or consider an earlier revision. buildMixedHistoryFixture
// ([[D-075]]) pins this against the exact shape that used to fool
// detection: a classic base, a middle revision that upgrades to a
// genuine cross-reference stream, then a further classic revision on
// top — the document's last revision is classic even though a stream
// appears earlier in its /Prev chain, and signing it must append a
// classic table, not a stream.
func TestIncrementalUpdateMatchesMechanism(t *testing.T) {
	cases := []struct {
		name       string
		build      func() []byte
		wantStream bool
	}{
		{"classic", buildClassicFixture, false},
		{"stream", buildStreamFixture, true},
		{"mixed-history-classic-last", buildMixedHistoryFixture, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.build()
			doc, err := Parse(in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.UsesXrefStreams() != c.wantStream {
				t.Fatalf("UsesXrefStreams() = %v, want %v", doc.UsesXrefStreams(), c.wantStream)
			}

			u := NewUpdate(doc)
			m := u.NewObjectNumber()
			u.Set(m, Dict{Name("Type"): Name("Marker")})
			out, err := u.Apply()
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			appended := out[len(doc.Data()):]
			if c.wantStream {
				if bytes.Contains(appended, []byte("\nxref\n")) {
					t.Fatal("stream document's update contains a classic xref table")
				}
			} else if bytes.Contains(appended, []byte("/XRef")) {
				t.Fatal("classic-table document's update contains an /XRef stream")
			}

			reparsed, err := Parse(out)
			if err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			if reparsed.UsesXrefStreams() != c.wantStream {
				t.Fatalf("updated document's UsesXrefStreams() = %v, want %v", reparsed.UsesXrefStreams(), c.wantStream)
			}
		})
	}
}

// TestIncrementalUpdateRoundTrips parses the updated file again and
// confirms both the new object and the original content are reachable
// (F3 §3.4).
func TestIncrementalUpdateRoundTrips(t *testing.T) {
	for name, build := range map[string]func() []byte{
		"classic": buildClassicFixture,
		"stream":  buildStreamFixture,
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse(build())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			u := NewUpdate(doc)
			marker := u.NewObjectNumber()
			u.Set(marker, Dict{Name("Type"): Name("Marker"), Name("Value"): int64(42)})
			out, err := u.Apply()
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}

			doc2, err := Parse(out)
			if err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			got, ok := doc2.Get(marker).(Dict)
			if !ok || got.GetName(Name("Type")) != "Marker" {
				t.Fatalf("Get(marker) = %#v, want the new marker dict", doc2.Get(marker))
			}
			content := pageContent(t, doc2)
			if !bytes.Contains(content, []byte("Hello")) {
				t.Fatalf("original content unreachable after update: %q", content)
			}
		})
	}
}

// TestIncrementalUpdateModifyExistingObjectShadowsIt exercises F3 §3.3:
// redefining an existing object number (never editing it in place)
// makes the newest revision win, exactly like TestMultiRevisionMergeNewestWins
// but going through the Update API this project actually ships.
func TestIncrementalUpdateModifyExistingObjectShadowsIt(t *testing.T) {
	doc, err := Parse(buildClassicFixture())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	u := NewUpdate(doc)
	u.Set(1, Dict{Name("Type"): Name("Catalog"), Name("Pages"): Reference{Num: 2}, Name("Marker"): Name("Shadowed")})
	out, err := u.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	catalog, ok := doc2.Get(1).(Dict)
	if !ok || catalog.GetName(Name("Marker")) != "Shadowed" {
		t.Fatalf("Get(1) = %#v, want the shadowed definition", doc2.Get(1))
	}
}
