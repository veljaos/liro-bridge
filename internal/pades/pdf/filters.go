package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// decodeStream applies the filter chain named in dict's /Filter (a Name
// or an Array of Names) to raw, using /DecodeParms for any predictor
// parameters. Only the filters PDF actually uses for xref streams,
// object streams and (occasionally) content streams are implemented:
// FlateDecode (with PNG/TIFF predictors), ASCIIHexDecode and
// ASCII85Decode. An unsupported filter is a clear error, not a silent
// pass-through.
func decodeStream(dict Dict, raw []byte) ([]byte, error) {
	filters := filterNames(dict.Get(Name("Filter")))
	parms := decodeParms(dict.Get(Name("DecodeParms")), len(filters))

	data := raw
	for i, f := range filters {
		var err error
		data, err = applyFilter(f, data, parms[i])
		if err != nil {
			return nil, fmt.Errorf("pdf: filter %s: %w", f, err)
		}
	}
	return data, nil
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
