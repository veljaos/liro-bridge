// Package pdf implements a hand-written PDF object model, parser and
// incremental-update writer (F3 §2/§3). No third-party PDF library is
// imported anywhere in this package — SPEC §12.1 requires the /ByteRange
// calculation and everything upstream of it to be code this project can
// read end to end.
package pdf

import "fmt"

// Name is a PDF name object, already decoded (any "#xx" escapes resolved
// to the raw byte they represent — F3 §2.2).
type Name string

// String is a PDF string object (literal or hex), already decoded to raw
// bytes. Which syntax produced it is not retained; callers that need PDF
// text encoding (PDFDocEncoding/UTF-16BE) decode Bytes themselves.
type String []byte

// Array is a PDF array object.
type Array []Object

// Dict is a PDF dictionary object.
type Dict map[Name]Object

// Reference is an indirect reference, "N G R".
type Reference struct {
	Num int
	Gen int
}

// Stream is a PDF stream object: its dictionary plus the raw (still
// encoded) bytes between "stream" and "endstream".
type Stream struct {
	Dict Dict
	Raw  []byte
}

// Raw is a pre-serialised object body, written to the output byte for
// byte with no reinterpretation. It exists solely for
// BuildPlaceholder (F3 §4): the signature dictionary's /Contents and
// /ByteRange fields must land at exact, caller-known byte offsets with
// fixed field widths, which the generic Dict writer (map iteration order
// aside) has no way to guarantee. Nothing else in this project needs it.
type Raw []byte

// Object is any PDF object: nil (the PDF null object), bool, int64,
// float64, Name, String, Array, Dict, *Stream, Reference, or Raw. There
// is no interface method set — callers type-switch, matching how small
// and closed this set is.
type Object any

// Get returns d[key], or nil if absent. It does not resolve references —
// use Document.Resolve for that.
func (d Dict) Get(key Name) Object {
	if d == nil {
		return nil
	}
	return d[key]
}

// GetName returns d[key] as a Name, or "" if it is absent or a different
// type.
func (d Dict) GetName(key Name) Name {
	n, _ := d[key].(Name)
	return n
}

// asInt64 converts an Object that is an integer-valued number to int64.
// PDF real numbers with no fractional part (e.g. "3.0") are accepted,
// since some producers write sizes and offsets that way.
func asInt64(o Object) (int64, bool) {
	switch v := o.(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// String returns a human-readable form for error messages only — never
// used to re-serialise an object.
func (r Reference) String() string { return fmt.Sprintf("%d %d R", r.Num, r.Gen) }
