package pdf

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

func buildTestPlaceholder(t *testing.T, in []byte, reserved int) (*Document, *Placeholder) {
	t.Helper()
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ph, err := BuildPlaceholder(doc, PlaceholderOptions{
		ReservedBytes: reserved,
		SubFilter:     Name("ETSI.CAdES.detached"),
		SigningDate:   time.Date(2026, 3, 24, 15, 55, 52, 0, time.FixedZone("CET", 3600)),
	})
	if err != nil {
		t.Fatalf("BuildPlaceholder: %v", err)
	}
	return doc, ph
}

// independentByteRange re-parses out from scratch and reads the
// signature dictionary's /ByteRange directly, without touching the
// Placeholder struct BuildPlaceholder returned — the F3 §4.5 requirement
// that this recomputation not reuse the signing code's own variables.
func independentByteRange(t *testing.T, out []byte, sigNum int) [4]int64 {
	t.Helper()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	sigDict, ok := doc2.Get(sigNum).(Dict)
	if !ok {
		t.Fatalf("Get(sigNum) = %#v, want Dict", doc2.Get(sigNum))
	}
	arr, ok := sigDict.Get(Name("ByteRange")).(Array)
	if !ok || len(arr) != 4 {
		t.Fatalf("/ByteRange = %#v, want a 4-element array", sigDict.Get(Name("ByteRange")))
	}
	var br [4]int64
	for i, v := range arr {
		n, ok := asInt64(v)
		if !ok {
			t.Fatalf("/ByteRange[%d] = %#v, not a number", i, v)
		}
		br[i] = n
	}
	return br
}

func TestPlaceholderByteRangeCoversWholeFileExceptContents(t *testing.T) {
	for name, build := range map[string]func() []byte{"classic": buildClassicFixture, "stream": buildStreamFixture} {
		t.Run(name, func(t *testing.T) {
			_, ph := buildTestPlaceholder(t, build(), 256)
			b, c, d := ph.ByteRange[1], ph.ByteRange[2], ph.ByteRange[3]

			if c+d != int64(len(ph.Bytes)) {
				t.Fatalf("c+d = %d, want len(file) = %d", c+d, len(ph.Bytes))
			}
			if b < 0 || b >= c {
				t.Fatalf("expected 0 <= b < c, got b=%d c=%d", b, c)
			}
			// The excluded span [b,c) must be exactly the hex string
			// including its '<' and '>' delimiters.
			excluded := ph.Bytes[b:c]
			if excluded[0] != '<' || excluded[len(excluded)-1] != '>' {
				t.Fatalf("excluded span = %q, want it delimited by < and >", excluded)
			}
			if int64(len(excluded)) != 2+int64(ph.contentsHexLen) {
				t.Fatalf("excluded span length = %d, want %d", len(excluded), 2+ph.contentsHexLen)
			}
		})
	}
}

// TestPlaceholderDigestMatchesIndependentRecomputation is F3 §4.5's
// "messageDigest equals SHA-256 over the /ByteRange span" test, computed
// from offsets read back out of the output file rather than reusing
// BuildPlaceholder's own offset variables — reusing them would make the
// test circular.
func TestPlaceholderDigestMatchesIndependentRecomputation(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildStreamFixture(), 256)

	br := independentByteRange(t, ph.Bytes, ph.SigObjectNum)
	h := sha256.New()
	h.Write(ph.Bytes[:br[1]])
	h.Write(ph.Bytes[br[2] : br[2]+br[3]])
	var want [32]byte
	copy(want[:], h.Sum(nil))

	got := ph.Digest()
	if got != want {
		t.Fatalf("Digest() = %x, want %x (independently recomputed from parsed offsets)", got, want)
	}
}

// TestPlaceholderTamperInsideSignedRangeChangesDigest is the security
// property the whole placeholder exists to provide: any change inside
// the signed span must be detectable.
func TestPlaceholderTamperInsideSignedRangeChangesDigest(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildClassicFixture(), 256)
	before := ph.Digest()

	tampered := append([]byte{}, ph.Bytes...)
	tampered[10] ^= 0xFF // well inside the original document body
	ph2 := &Placeholder{Bytes: tampered, ByteRange: ph.ByteRange}
	after := ph2.Digest()

	if before == after {
		t.Fatal("tampering inside the signed range did not change the digest")
	}
}

// TestPlaceholderTamperInsideContentsDoesNotChangeDigest proves
// /Contents is genuinely excluded from the signed span, which is what
// makes it possible to write the signature into that span after the
// digest (and therefore the signature over it) has already been fixed.
func TestPlaceholderTamperInsideContentsDoesNotChangeDigest(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildClassicFixture(), 256)
	before := ph.Digest()

	tampered := append([]byte{}, ph.Bytes...)
	tampered[ph.contentsStart+5] = 'F'
	ph2 := &Placeholder{Bytes: tampered, ByteRange: ph.ByteRange}
	after := ph2.Digest()

	if before != after {
		t.Fatal("tampering inside /Contents changed the digest, but /Contents must be excluded from /ByteRange")
	}
}

// TestPlaceholderByteRangeIsLeftAlignedForAdobesRawScanner is Task 1's
// regression test (see docs/decisions.md): Adobe Acrobat scans
// /ByteRange from raw bytes before the document is resolved, and
// rejected the previous right-aligned-per-field rendering
// ("[         0     603053     668591        615]") with "Unexpected
// byte range values defining scope of signed data". The fix packs the
// real numbers together with single-space separators and moves all
// padding after the last one, matching every reference implementation.
func TestPlaceholderByteRangeIsLeftAlignedForAdobesRawScanner(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildStreamFixture(), 256)

	idx := bytes.Index(ph.Bytes, []byte("/ByteRange ["))
	if idx < 0 {
		t.Fatal("/ByteRange [ not found in output")
	}
	start := idx + len("/ByteRange [")
	end := bytes.IndexByte(ph.Bytes[start:], ']')
	if end < 0 {
		t.Fatal("closing ] of /ByteRange not found")
	}
	raw := string(ph.Bytes[start : start+end])

	if len(raw) != byteRangeTotalWidth {
		t.Fatalf("/ByteRange span length = %d, want unchanged width %d", len(raw), byteRangeTotalWidth)
	}

	want := fmt.Sprintf("%d %d %d %d", ph.ByteRange[0], ph.ByteRange[1], ph.ByteRange[2], ph.ByteRange[3])
	if !strings.HasPrefix(raw, want) {
		t.Fatalf("/ByteRange = %q, want it to start with the compact form %q", raw, want)
	}
	for i := len(want); i < len(raw); i++ {
		if raw[i] != ' ' {
			t.Fatalf("/ByteRange = %q, want only trailing spaces after byte %d", raw, len(want))
		}
	}

	// No run of two or more consecutive spaces may appear before the
	// final number — i.e. among the three separators that sit between
	// real values, exactly one space each.
	beforeFinal := want[:strings.LastIndex(want, " ")+1]
	if strings.Contains(beforeFinal, "  ") {
		t.Fatalf("/ByteRange = %q, contains a run of consecutive spaces before the final number", raw)
	}
}

// TestPlaceholderPreservesDirectAcroForm is the fix's own unit test,
// independent of the gitignored real fixtures: when the input catalog
// carries /AcroForm as a direct dictionary (buildClassicFixtureWithDirectAcroForm,
// the shape measured in all three real fixtures — see docs/decisions.md),
// BuildPlaceholder must not promote it to a new indirect object. The
// rewritten catalog must contain /AcroForm inline, with /Fields extended
// to include the new widget alongside the original field, and no new
// object number must be consumed for a standalone AcroForm object.
func TestPlaceholderPreservesDirectAcroForm(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildClassicFixtureWithDirectAcroForm(), 256)

	doc2, err := Parse(ph.Bytes)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	root, ok := doc2.ResolveDict(doc2.Trailer().Get(Name("Root")))
	if !ok {
		t.Fatal("/Root did not resolve")
	}
	acroForm, ok := root.Get(Name("AcroForm")).(Dict)
	if !ok {
		t.Fatalf("catalog /AcroForm = %#v, want a direct Dict (the input's own shape)", root.Get(Name("AcroForm")))
	}
	fields, ok := acroForm.Get(Name("Fields")).(Array)
	if !ok || len(fields) != 2 {
		t.Fatalf("/Fields = %#v, want a 2-element array (the original field plus the new widget)", acroForm.Get(Name("Fields")))
	}
	if ref, ok := fields[0].(Reference); !ok || ref.Num != 5 {
		t.Fatalf("/Fields[0] = %#v, want a reference to the original field, object 5", fields[0])
	}
	widget, ok := doc2.ResolveDict(fields[1])
	if !ok || widget.Get(Name("FT")) != Name("Sig") {
		t.Fatalf("/Fields[1] = %#v, want the new signature widget", fields[1])
	}
	if da, ok := acroForm.Get(Name("DA")).(String); !ok || string(da) != "/Helv 0 Tf 0 g " {
		t.Fatalf("/AcroForm /DA = %#v, want the original value carried forward", acroForm.Get(Name("DA")))
	}

	// No standalone AcroForm object was appended: the only newly written
	// objects are the catalog (1), the signature dictionary and the
	// widget — never a fourth, separate AcroForm object.
	for num := range doc2.xref {
		if num != 0 && num != 1 && num != 2 && num != 3 && num != 4 && num != 5 &&
			num != ph.SigObjectNum && num != fields[1].(Reference).Num {
			t.Fatalf("unexpected object number %d in the output — a standalone AcroForm object should never have been created", num)
		}
	}
}

func TestPlaceholderOversizedCMSFailsLoudly(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildClassicFixture(), 16) // deliberately tiny reservation
	fakeCMS := bytes.Repeat([]byte{0xAB}, 64)                   // far larger than 16 reserved bytes

	err := ph.InjectSignature(fakeCMS)
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeSignFailed {
		t.Fatalf("InjectSignature(oversized) error = %v, want CodeSignFailed", err)
	}
	if e.Details["cmsBytes"] != len(fakeCMS) {
		t.Fatalf("Details[cmsBytes] = %v, want %d", e.Details["cmsBytes"], len(fakeCMS))
	}
}

func TestPlaceholderInjectAndRoundTrip(t *testing.T) {
	_, ph := buildTestPlaceholder(t, buildStreamFixture(), 256)
	fakeCMS := bytes.Repeat([]byte{0xAB, 0xCD}, 10)

	if err := ph.InjectSignature(fakeCMS); err != nil {
		t.Fatalf("InjectSignature: %v", err)
	}

	doc2, err := Parse(ph.Bytes)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	sigDict, ok := doc2.Get(ph.SigObjectNum).(Dict)
	if !ok {
		t.Fatalf("Get(sigNum) = %#v, want Dict", doc2.Get(ph.SigObjectNum))
	}
	contents, ok := sigDict.Get(Name("Contents")).(String)
	if !ok {
		t.Fatalf("/Contents = %#v, want String", sigDict.Get(Name("Contents")))
	}
	if !bytes.HasPrefix(contents, fakeCMS) {
		t.Fatalf("/Contents does not start with the injected CMS bytes")
	}
	for _, b := range contents[len(fakeCMS):] {
		if b != 0 {
			t.Fatalf("padding after the injected CMS is not all zero")
		}
	}

	// AcroForm/field wiring (F3 §3.3) must be present and correct.
	root, ok := doc2.ResolveDict(doc2.Trailer().Get(Name("Root")))
	if !ok {
		t.Fatal("/Root did not resolve")
	}
	acroForm, ok := doc2.ResolveDict(root.Get(Name("AcroForm")))
	if !ok {
		t.Fatal("/AcroForm did not resolve")
	}
	if sf, _ := asInt64(acroForm.Get(Name("SigFlags"))); sf != 3 {
		t.Fatalf("/SigFlags = %v, want 3", acroForm.Get(Name("SigFlags")))
	}
	fields, ok := doc2.Resolve(acroForm.Get(Name("Fields"))).(Array)
	if !ok || len(fields) != 1 {
		t.Fatalf("/Fields = %#v, want a one-element array", acroForm.Get(Name("Fields")))
	}
	widget, ok := doc2.ResolveDict(fields[0])
	if !ok || widget.Get(Name("FT")) != Name("Sig") {
		t.Fatalf("field = %#v, want /FT /Sig", widget)
	}
	if ref, ok := widget.Get(Name("V")).(Reference); !ok || ref.Num != ph.SigObjectNum {
		t.Fatalf("field /V = %#v, want a reference to the signature object", widget.Get(Name("V")))
	}
}
