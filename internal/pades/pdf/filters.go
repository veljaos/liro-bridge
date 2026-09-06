package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// resolver resolves an indirect reference to the object it names.
// decodeStream takes one because /Filter, /DecodeParms and the values
// inside a /DecodeParms dictionary are all permitted to be indirect
// references in a real document (PDF 32000-1 §7.3.10 places no
// restriction on them), and a document that uses one is not rare enough
// to treat as malformed. A nil resolver means "these bytes are being
// read before the cross-reference table itself is usable" — the xref
// and object-stream callers, where an indirect reference could not be
// followed even in principle.
type resolver func(Object) Object

func (r resolver) call(o Object) Object {
	if r == nil {
		return o
	}
	return r(o)
}

// decodeStream applies the filter chain named in dict's /Filter (a Name
// or an Array of Names) to raw, using /DecodeParms for any predictor
// parameters. An unsupported filter is a clear error, not a silent
// pass-through.
func decodeStream(res resolver, dict Dict, raw []byte) ([]byte, error) {
	data, remaining, _, err := decodeStreamUntil(res, dict, raw, nil)
	if err != nil {
		return nil, err
	}
	if remaining != "" {
		return nil, fmt.Errorf("pdf: filter %s: unsupported filter %q", remaining, remaining)
	}
	return data, nil
}

// decodeStreamUntil applies the filter chain to raw, stopping before the
// first filter for which stopAt reports true and returning that filter's
// name and its own /DecodeParms unapplied. It exists for image XObjects,
// whose final filter is an image codec (DCTDecode, JPXDecode,
// CCITTFaxDecode, JBIG2Decode) that produces pixels rather than bytes
// and therefore belongs to whoever is decoding the image, not to the
// generic stream reader. stopAt nil means "apply everything".
func decodeStreamUntil(res resolver, dict Dict, raw []byte, stopAt func(Name) bool) (data []byte, stopped Name, stoppedParms Dict, err error) {
	filters := filterNames(res.call(dict.Get(Name("Filter"))))
	parms := decodeParms(res.call(dict.Get(Name("DecodeParms"))), len(filters))

	data = raw
	for i, f := range filters {
		p := resolveParms(res, parms[i])
		if stopAt != nil && stopAt(f) {
			return data, f, p, nil
		}
		data, err = applyFilter(f, data, p)
		if err != nil {
			return nil, "", nil, fmt.Errorf("pdf: filter %s: %w", f, err)
		}
	}
	return data, "", nil, nil
}

// resolveParms resolves the values inside one /DecodeParms dictionary.
// Only the scalar entries matter to any filter implemented here, so a
// shallow resolve is enough — and a nil dictionary stays nil rather
// than becoming an empty one, since applyPredictor distinguishes them.
func resolveParms(res resolver, parms Dict) Dict {
	if parms == nil || res == nil {
		return parms
	}
	out := make(Dict, len(parms))
	for k, v := range parms {
		out[k] = res(v)
	}
	return out
}

func filterNames(o Object) []Name {
	switch v := o.(type) {
	case nil:
		return nil
	case Name:
		return []Name{v}
	case Array:
		names := make([]Name, 0, len(v))
		for _, e := range v {
			if n, ok := e.(Name); ok {
				names = append(names, n)
			}
		}
		return names
	default:
		return nil
	}
}

func decodeParms(o Object, n int) []Dict {
	out := make([]Dict, n)
	switch v := o.(type) {
	case Dict:
		if n > 0 {
			out[0] = v
		}
	case Array:
		for i := 0; i < n && i < len(v); i++ {
			if d, ok := v[i].(Dict); ok {
				out[i] = d
			}
		}
	}
	return out
}

func applyFilter(name Name, data []byte, parms Dict) ([]byte, error) {
	switch name {
	case "FlateDecode", "Fl":
		out, err := flateDecode(data)
		if err != nil {
			return nil, err
		}
		return applyPredictor(out, parms)
	case "ASCIIHexDecode", "AHx":
		return asciiHexDecode(data)
	case "ASCII85Decode", "A85":
		return ascii85Decode(data)
	case "LZWDecode", "LZW":
		early := int64(1)
		if v, ok := asInt64(parms.Get(Name("EarlyChange"))); ok {
			early = v
		}
		out, err := lzwDecode(data, early != 0)
		if err != nil {
			return nil, err
		}
		return applyPredictor(out, parms)
	case "RunLengthDecode", "RL":
		return runLengthDecode(data)
	case "Crypt":
		// The only /Crypt filter a document this project will open can
		// carry is /Name /Identity: an encrypted document is refused
		// outright, before any stream is read (D-043).
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported filter %q", name)
	}
}

// maxDecodedStreamSize bounds decompressed stream output at the same
// 512 MB ceiling as the whole document (F3 §2.4/§16.5): a small,
// maliciously crafted FlateDecode stream must not be able to force an
// unbounded allocation ("zip bomb").
const maxDecodedStreamSize = MaxInputSize

func flateDecode(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zlib: %w", err)
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(io.LimitReader(r, maxDecodedStreamSize+1))
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("zlib read: %w", err)
	}
	if len(out) > maxDecodedStreamSize {
		return nil, fmt.Errorf("decoded stream exceeds %d bytes", maxDecodedStreamSize)
	}
	return out, nil
}

// applyPredictor reverses the PNG or TIFF predictor named in parms, if
// any. Predictor 1 (or an absent /Predictor) means no predictor was
// applied.
func applyPredictor(data []byte, parms Dict) ([]byte, error) {
	if parms == nil {
		return data, nil
	}
	predictor, _ := asInt64(parms.Get(Name("Predictor")))
	if predictor <= 1 {
		return data, nil
	}
	columns, _ := asInt64(parms.Get(Name("Columns")))
	if columns <= 0 {
		columns = 1
	}
	colors, _ := asInt64(parms.Get(Name("Colors")))
	if colors <= 0 {
		colors = 1
	}
	bpc, _ := asInt64(parms.Get(Name("BitsPerComponent")))
	if bpc <= 0 {
		bpc = 8
	}
	// Predictor parameters come straight from an attacker-controlled
	// dictionary (F3 §16.5's fuzz target feeds it foreign input). Bound
	// them well above any real image or table before they drive a row
	// width used for slicing and allocation, so a huge or negative value
	// is a clear error instead of an out-of-range slice panic or an
	// oversized allocation.
	const maxDimension = 1 << 20
	if columns > maxDimension || colors > maxDimension || bpc > 64 {
		return nil, fmt.Errorf("predictor parameters out of range: columns=%d colors=%d bpc=%d", columns, colors, bpc)
	}
	bytesPerPixel := int((colors*bpc + 7) / 8)
	if bytesPerPixel < 1 {
		bytesPerPixel = 1
	}
	rowBytes := int((colors*bpc*columns + 7) / 8)
	if rowBytes <= 0 || rowBytes > maxDecodedStreamSize {
		return nil, fmt.Errorf("predictor row width out of range: %d", rowBytes)
	}

	if predictor == 2 {
		return tiffPredictor(data, rowBytes, bytesPerPixel), nil
	}
	// Predictor >= 10: PNG predictors, one tag byte per row.
	return pngPredictor(data, rowBytes, bytesPerPixel)
}

func tiffPredictor(data []byte, rowBytes, bpp int) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	for r := 0; r+rowBytes <= len(out); r += rowBytes {
		row := out[r : r+rowBytes]
		for i := bpp; i < len(row); i++ {
			row[i] += row[i-bpp]
		}
	}
	return out
}

func pngPredictor(data []byte, rowBytes, bpp int) ([]byte, error) {
	stride := rowBytes + 1
	if stride <= 1 || len(data)%stride != 0 {
		// Tolerate a short final row rather than failing the whole
		// stream on it.
		if stride <= 1 {
			return nil, fmt.Errorf("pdf: invalid predictor row width")
		}
	}
	rows := len(data) / stride
	out := make([]byte, 0, rows*rowBytes)
	prev := make([]byte, rowBytes)
	for i := 0; i < rows; i++ {
		row := data[i*stride : i*stride+stride]
		tag := row[0]
		cur := make([]byte, rowBytes)
		copy(cur, row[1:])
		for x := 0; x < rowBytes; x++ {
			var a, b, c byte
			if x >= bpp {
				a = cur[x-bpp]
				c = prev[x-bpp]
			}
			b = prev[x]
			switch tag {
			case 0: // None
			case 1: // Sub
				cur[x] += a
			case 2: // Up
				cur[x] += b
			case 3: // Average
				cur[x] += byte((int(a) + int(b)) / 2)
			case 4: // Paeth
				cur[x] += paeth(a, b, c)
			default:
				return nil, fmt.Errorf("pdf: unsupported PNG predictor tag %d", tag)
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func asciiHexDecode(data []byte) ([]byte, error) {
	var digits []byte
	for _, b := range data {
		if b == '>' {
			break
		}
		if isWhitespace(b) {
			continue
		}
		if !isHexDigit(b) {
			return nil, fmt.Errorf("invalid hex digit 0x%02X", b)
		}
		digits = append(digits, b)
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	for i := range out {
		out[i] = hexVal(digits[2*i])<<4 | hexVal(digits[2*i+1])
	}
	return out, nil
}

func ascii85Decode(data []byte) ([]byte, error) {
	// Strip an optional leading "<~" and a trailing "~>", and all
	// whitespace, per PDF 32000-1 §7.4.3.
	s := bytes.TrimSpace(data)
	s = bytes.TrimPrefix(s, []byte("<~"))
	if i := bytes.Index(s, []byte("~>")); i >= 0 {
		s = s[:i]
	}
	var clean []byte
	for _, b := range s {
		if !isWhitespace(b) {
			clean = append(clean, b)
		}
	}
	var out []byte
	var group [5]byte
	n := 0
	flush := func(count int) error {
		for i := count; i < 5; i++ {
			group[i] = 'u'
		}
		var val uint32
		for i := 0; i < 5; i++ {
			if group[i] < '!' || group[i] > 'u' {
				return fmt.Errorf("invalid ascii85 byte 0x%02X", group[i])
			}
			val = val*85 + uint32(group[i]-'!')
		}
		b := []byte{byte(val >> 24), byte(val >> 16), byte(val >> 8), byte(val)}
		out = append(out, b[:count-1]...)
		return nil
	}
	for _, b := range clean {
		if b == 'z' && n == 0 {
			out = append(out, 0, 0, 0, 0)
			continue
		}
		group[n] = b
		n++
		if n == 5 {
			if err := flush(5); err != nil {
				return nil, err
			}
			n = 0
		}
	}
	if n > 0 {
		if err := flush(n); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// lzwDecode implements PDF's LZWDecode filter (PDF 32000-1 §7.4.4):
// variable-width MSB-first codes over a 4096-entry table, 8-bit input
// alphabet, with 256 as the clear-table code and 257 as end-of-data.
//
// It is written out here rather than delegated to compress/lzw because
// of earlyChange. PDF's /EarlyChange parameter selects whether the code
// width grows one code before the table is actually full (the value 1,
// and the default, matching TIFF) or exactly when it fills (the value
// 0). The standard library's MSB reader implements one of those two and
// documents neither as a PDF guarantee, so a document setting
// /EarlyChange 0 would decode to plausible-looking rubbish rather than
// to an error — the worst shape of failure for a decoder.
func lzwDecode(data []byte, earlyChange bool) ([]byte, error) {
	const (
		clearCode = 256
		eodCode   = 257
		firstCode = 258
		maxCode   = 4096
	)

	var table [maxCode][]byte
	next := firstCode
	width := 9
	reset := func() {
		next = firstCode
		width = 9
	}
	reset()

	var out []byte
	var prev []byte
	bitPos := 0
	totalBits := len(data) * 8

	readCode := func() (int, bool) {
		if bitPos+width > totalBits {
			return 0, false
		}
		v := 0
		for i := 0; i < width; i++ {
			byteIdx := (bitPos + i) / 8
			bit := (data[byteIdx] >> (7 - uint((bitPos+i)%8))) & 1
			v = v<<1 | int(bit)
		}
		bitPos += width
		return v, true
	}

	entry := func(code int) ([]byte, bool) {
		if code < 256 {
			return []byte{byte(code)}, true
		}
		if code >= firstCode && code < next && table[code] != nil {
			return table[code], true
		}
		return nil, false
	}

	for {
		code, ok := readCode()
		if !ok {
			// Running out of bits without an end-of-data code is
			// common enough in real files (producers pad, or omit the
			// marker) that it is treated as the end rather than as
			// corruption.
			return out, nil
		}
		switch code {
		case eodCode:
			return out, nil
		case clearCode:
			reset()
			prev = nil
			continue
		}

		var cur []byte
		if seq, ok := entry(code); ok {
			cur = seq
		} else if code == next && prev != nil {
			// The KwKwK case: the encoder emitted a code for a sequence
			// it is defining with this very code.
			cur = append(append([]byte{}, prev...), prev[0])
		} else {
			return nil, fmt.Errorf("lzw: code %d out of range (next %d)", code, next)
		}

		if len(out)+len(cur) > maxDecodedStreamSize {
			return nil, fmt.Errorf("decoded stream exceeds %d bytes", maxDecodedStreamSize)
		}
		out = append(out, cur...)

		if prev != nil && next < maxCode {
			table[next] = append(append([]byte{}, prev...), cur[0])
			next++
		}
		prev = cur

		limit := next
		if earlyChange {
			limit = next + 1
		}
		switch {
		case limit > 2048 && width < 12:
			width = 12
		case limit > 1024 && width < 11:
			width = 11
		case limit > 512 && width < 10:
			width = 10
		}
	}
}

// runLengthDecode implements PDF's RunLengthDecode filter (PDF 32000-1
// §7.4.5): a length byte, then either that many literal bytes (0..127)
// or one byte repeated 257-length times (129..255). 128 ends the data.
func runLengthDecode(data []byte) ([]byte, error) {
	var out []byte
	for i := 0; i < len(data); {
		n := int(data[i])
		i++
		switch {
		case n == 128:
			return out, nil
		case n < 128:
			if i+n+1 > len(data) {
				return nil, fmt.Errorf("runlength: literal run of %d bytes runs past the end", n+1)
			}
			out = append(out, data[i:i+n+1]...)
			i += n + 1
		default:
			if i >= len(data) {
				return nil, fmt.Errorf("runlength: repeat run with no byte to repeat")
			}
			for j := 0; j < 257-n; j++ {
				out = append(out, data[i])
			}
			i++
		}
		if len(out) > maxDecodedStreamSize {
			return nil, fmt.Errorf("decoded stream exceeds %d bytes", maxDecodedStreamSize)
		}
	}
	return out, nil
}
