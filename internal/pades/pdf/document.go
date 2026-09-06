package pdf

import (
	"bytes"
	"fmt"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// MaxInputSize is the size limit for a PDF Parse accepts (F3 §2.4).
// Documents this size in a signing agent are a mistake or an attack, and
// the parser holds the whole file in memory.
const MaxInputSize = 512 * 1024 * 1024

// Document is a fully parsed PDF: the original bytes, unmodified, plus a
// merged cross-reference table letting any object number resolve to its
// newest definition (F3 §2.1).
type Document struct {
	data    []byte
	xref    map[int]xrefEntry
	trailer Dict

	cache      map[int]Object
	objStmData map[int][]Object // decoded contents of each object stream, by its own object number
	resolving  map[int]bool     // objects currently being resolved, breaking self-referential cycles

	pageNums []int // page object numbers in reading order, walked once by Pages
	pageErr  error // why that walk failed, so it is not retried on every page change
}

// Data returns the document's original bytes, exactly as parsed. Callers
// building an incremental update append to this slice; they never modify
// it (F3 §3.1).
func (d *Document) Data() []byte { return d.data }

// Trailer returns the merged trailer dictionary (newest revision's keys
// win).
func (d *Document) Trailer() Dict { return d.trailer }

// UsesXrefStreams reports whether the document's last revision — the one
// its final startxref points at — used a cross-reference stream rather
// than a classic table (F3 §3.2: an incremental update must match the
// document's own mechanism). A document can carry a cross-reference
// stream somewhere in its /Prev chain and still end on a classic table
// (or vice versa); only the last revision decides.
//
// The merged trailer's own /Type key cannot answer this: trailer merging
// takes each key from the newest section that defines it, so if the last
// revision is a classic table (which has no /Type key at all), an /Type
// /XRef inherited from an older stream revision earlier in the chain
// would leak through unnoticed. That was exactly this project's bug
// ([[D-075]]): it made mechanism detection see "a stream exists
// somewhere" instead of "the last revision used a stream."
func (d *Document) UsesXrefStreams() bool {
	if isStream, ok := xrefMechanismAtStartxref(d.data); ok {
		return isStream
	}
	// No usable final startxref — the document needed Parse's
	// rebuild-by-scanning fallback, which has no notion of "the last
	// revision" to point at. The merged trailer's /Type is the best
	// remaining signal in that already-degraded case.
	return d.trailer.GetName(Name("Type")) == "XRef"
}

// xrefMechanismAtStartxref resolves data's final startxref and inspects
// only the bytes it points at — never the whole file, never an earlier
// revision reached via /Prev, never whether object streams exist
// anywhere — reporting whether that one revision is a cross-reference
// stream. ok is false when there is no usable startxref to resolve.
func xrefMechanismAtStartxref(data []byte) (isStream bool, ok bool) {
	offset, found := findStartxref(data)
	if !found || offset < 0 || offset >= int64(len(data)) {
		return false, false
	}
	p := newParser(data, nil)
	p.pos = int(offset)
	if p.peekKeyword("xref") {
		return false, true
	}
	_, _, obj, err := p.parseIndirectAt(int(offset))
	if err != nil {
		return false, false
	}
	stream, isStreamObj := obj.(*Stream)
	if !isStreamObj {
		return false, false
	}
	return stream.Dict.GetName(Name("Type")) == "XRef", true
}

// Get resolves object number num to its value, following object-stream
// indirection when needed. It returns nil for a free or unknown object
// number. Generation numbers are not matched against the reference that
// requested the lookup — every object in a well-formed PDF has exactly
// one live generation at any point in the /Prev chain, and F3 does not
// require detecting a generation mismatch.
func (d *Document) Get(num int) Object {
	if v, ok := d.cache[num]; ok {
		return v
	}
	entry, ok := d.xref[num]
	if !ok || entry.typ == xrefFree {
		d.cache[num] = nil
		return nil
	}
	// A malformed (or fuzzed) document can make an object's /Length, or
	// an object-stream entry, refer back to itself. Without this guard
	// that becomes unbounded recursion (F3 §16.5); with it, a cycle
	// simply resolves to nil, which callers already treat as "not
	// resolvable" (e.g. streamLength's resolver falls back to scanning
	// for endstream).
	if d.resolving == nil {
		d.resolving = map[int]bool{}
	}
	if d.resolving[num] {
		return nil
	}
	d.resolving[num] = true
	defer delete(d.resolving, num)

	switch entry.typ {
	case xrefInFile:
		p := d.newResolvingParser()
		_, _, obj, err := p.parseIndirectAt(int(entry.offset))
		if err != nil {
			d.cache[num] = nil
			return nil
		}
		d.cache[num] = obj
		return obj
	case xrefInStream:
		objs, err := d.objectStreamContents(int(entry.offset))
		if err != nil || entry.index < 0 || entry.index >= len(objs) {
			d.cache[num] = nil
			return nil
		}
		d.cache[num] = objs[entry.index]
		return objs[entry.index]
	default:
		return nil
	}
}

// Resolve returns o if it is not a Reference, or the resolved value of
// the object it refers to otherwise. It is not recursive: a resolved
// value that itself contains References is returned as-is, since most
// callers need only one level.
func (d *Document) Resolve(o Object) Object {
	if ref, ok := o.(Reference); ok {
		return d.Get(ref.Num)
	}
	return o
}

// ResolveDict resolves o and type-asserts it to Dict, also accepting a
// *Stream (whose own Dict is returned) since a signature widget's
// /Parent or similar is sometimes a stream in malformed producers.
func (d *Document) ResolveDict(o Object) (Dict, bool) {
	switch v := d.Resolve(o).(type) {
	case Dict:
		return v, true
	case *Stream:
		return v.Dict, true
	default:
		return nil, false
	}
}

// newResolvingParser returns a parser over the document's bytes whose
// indirect-/Length resolver is this document's own object resolution —
// used so a stream's /Length may itself be an indirect reference
// (F3 §2.2).
func (d *Document) newResolvingParser() *parser {
	return newParser(d.data, func(n, _ int) (int64, bool) {
		v := d.Get(n)
		return asInt64(v)
	})
}

// objectStreamContents decodes and parses every object inside the object
// stream with object number stmNum (F3 §2.1's object-stream mechanism),
// caching the result.
func (d *Document) objectStreamContents(stmNum int) ([]Object, error) {
	if objs, ok := d.objStmData[stmNum]; ok {
		return objs, nil
	}
	obj := d.Get(stmNum)
	stream, ok := obj.(*Stream)
	if !ok {
		return nil, fmt.Errorf("pdf: object stream %d is not a stream", stmNum)
	}
	decoded, err := decodeStream(d.Resolve, stream.Dict, stream.Raw)
	if err != nil {
		return nil, fmt.Errorf("pdf: decoding object stream %d: %w", stmNum, err)
	}
	n, _ := asInt64(stream.Dict.Get(Name("N")))
	first, _ := asInt64(stream.Dict.Get(Name("First")))
	// /N and /First come straight from the (possibly fuzzed) stream
	// dictionary. Each header pair needs at least one byte, so N cannot
	// exceed the decoded stream's length; rejecting anything outside
	// that range up front stops a negative or astronomically large /N
	// from being used as a slice-capacity hint below (F3 §16.5).
	if n < 0 || n > int64(len(decoded)) {
		return nil, fmt.Errorf("pdf: object stream %d has an out-of-range /N %d", stmNum, n)
	}

	hp := newParser(decoded, nil)
	type pair struct{ num, off int64 }
	pairs := make([]pair, 0, n)
	for i := int64(0); i < n; i++ {
		hp.skipWhitespaceAndComments()
		numTok, ok1, err := hp.parseNumber()
		if err != nil || !ok1 {
			return nil, fmt.Errorf("pdf: object stream %d has a malformed header", stmNum)
		}
		hp.skipWhitespaceAndComments()
		offTok, ok2, err := hp.parseNumber()
		if err != nil || !ok2 {
			return nil, fmt.Errorf("pdf: object stream %d has a malformed header", stmNum)
		}
		numVal, _ := asInt64(numTok)
		offVal, _ := asInt64(offTok)
		pairs = append(pairs, pair{numVal, offVal})
	}

	objs := make([]Object, len(pairs))
	for i, pr := range pairs {
		op := newParser(decoded, nil)
		op.pos = int(first + pr.off)
		v, err := op.parseValue()
		if err != nil {
			return nil, fmt.Errorf("pdf: object stream %d entry %d: %w", stmNum, i, err)
		}
		objs[i] = v
	}
	if d.objStmData == nil {
		d.objStmData = map[int][]Object{}
	}
	d.objStmData[stmNum] = objs
	return objs, nil
}

// Parse reads a PDF document from data (F3 §2). data is retained
// unmodified — Document.Data returns the same slice, never a copy — and
// its bytes must not be mutated by the caller afterwards.
func Parse(data []byte) (*Document, error) {
	if len(data) > MaxInputSize {
		return nil, errs.WithDetails(errs.CodePDFInvalid,
			fmt.Errorf("input is %d bytes, over the %d byte limit", len(data), MaxInputSize),
			map[string]any{"sizeBytes": len(data), "limitBytes": MaxInputSize})
	}

	startxref, ok := findStartxref(data)
	var xrefEntries map[int]xrefEntry
	var trailer Dict
	var err error
	if ok {
		xrefEntries, trailer, err = parseXrefChain(data, startxref)
	}
	if !ok || err != nil || trailer.Get(Name("Root")) == nil {
		// F3 §2.2: a wrong startxref, or a chain that never reaches a
		// /Root, triggers the rebuild-by-scanning fallback.
		xrefEntries, trailer, err = rebuildXref(data)
		if err != nil {
			return nil, errs.New(errs.CodePDFInvalid, fmt.Errorf("no usable cross-reference information: %w", err))
		}
	}

	if trailer.Get(Name("Encrypt")) != nil {
		return nil, errs.New(errs.CodePDFEncrypted, fmt.Errorf("document trailer declares /Encrypt"))
	}

	doc := &Document{
		data:    data,
		xref:    xrefEntries,
		trailer: trailer,
		cache:   map[int]Object{},
	}
	if doc.Trailer().Get(Name("Root")) == nil {
		return nil, errs.New(errs.CodePDFInvalid, fmt.Errorf("trailer has no /Root"))
	}
	return doc, nil
}

// findStartxref finds the last "startxref" keyword in the file (there
// may be more than one across several revisions; the last is the
// newest) and reads the offset following it.
func findStartxref(data []byte) (int64, bool) {
	idx := bytes.LastIndex(data, []byte("startxref"))
	if idx < 0 {
		return 0, false
	}
	p := newParser(data, nil)
	p.pos = idx
	if err := p.consumeKeyword("startxref"); err != nil {
		return 0, false
	}
	p.skipWhitespaceAndComments()
	n, isInt, err := p.parseNumber()
	if err != nil || !isInt {
		return 0, false
	}
	return n.(int64), true
}
