package pdf

import (
	"bytes"
	"fmt"
)

// Catalog resolves the trailer's /Root to the document catalog.
func (d *Document) Catalog() (Dict, bool) {
	ref, ok := d.trailer.Get(Name("Root")).(Reference)
	if !ok {
		return nil, false
	}
	return d.ResolveDict(ref)
}

// Pages returns every page's object number, in reading order. The walk
// is done once and cached: a page-at-a-time reader (the placement
// preview, F6b §2.2) asks for this on every page change, and a
// two-hundred-page tree is not something to re-walk each time.
func (d *Document) Pages() ([]int, error) {
	if d.pageNums != nil {
		return d.pageNums, nil
	}
	if d.pageErr != nil {
		return nil, d.pageErr
	}
	catalog, ok := d.Catalog()
	if !ok {
		d.pageErr = fmt.Errorf("pdf: /Root does not resolve to a dictionary")
		return nil, d.pageErr
	}
	ref, ok := catalog.Get(Name("Pages")).(Reference)
	if !ok {
		d.pageErr = fmt.Errorf("pdf: catalog /Pages is not an indirect reference")
		return nil, d.pageErr
	}
	nums, err := collectPages(d, ref, 0)
	if err != nil {
		d.pageErr = err
		return nil, err
	}
	d.pageNums = nums
	return nums, nil
}

// PageCount is len(Pages()), with the same error.
func (d *Document) PageCount() (int, error) {
	pages, err := d.Pages()
	if err != nil {
		return 0, err
	}
	return len(pages), nil
}

// PageDict returns the dictionary of page n (1-based reading order).
func (d *Document) PageDict(n int) (Dict, error) {
	pages, err := d.Pages()
	if err != nil {
		return nil, err
	}
	if n < 1 || n > len(pages) {
		return nil, fmt.Errorf("pdf: page %d requested, document has %d page(s)", n, len(pages))
	}
	dict, ok := d.ResolveDict(Reference{Num: pages[n-1]})
	if !ok {
		return nil, fmt.Errorf("pdf: page %d does not resolve to a dictionary", n)
	}
	return dict, nil
}

// Inherited returns pageDict's own value for key, or the nearest
// ancestor's, following /Parent — the mechanism /Resources, /MediaBox,
// /CropBox and /Rotate all use (PDF 32000-1 §7.7.3.4). The value is
// resolved; a missing key anywhere in the chain returns nil.
func (d *Document) Inherited(pageDict Dict, key Name) Object {
	dict := pageDict
	for depth := 0; depth < maxPageTreeDepth; depth++ {
		if v := dict.Get(key); v != nil {
			return d.Resolve(v)
		}
		parent, ok := dict.Get(Name("Parent")).(Reference)
		if !ok {
			return nil
		}
		next, ok := d.ResolveDict(parent)
		if !ok {
			return nil
		}
		dict = next
	}
	return nil
}

// DecodeStream applies s's whole filter chain and returns the decoded
// bytes. /Filter, /DecodeParms and the values inside /DecodeParms may
// each be indirect references; this resolves them.
func (d *Document) DecodeStream(s *Stream) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("pdf: nil stream")
	}
	return decodeStream(d.Resolve, s.Dict, s.Raw)
}

// imageCodecs are the filters that produce pixels rather than bytes.
// DecodeImageStream stops in front of one and hands it to the caller,
// because decoding it needs the image's own /Width, /Height,
// /BitsPerComponent and colour space — things the generic stream reader
// has no business knowing.
func isImageCodec(f Name) bool {
	switch f {
	case "DCTDecode", "DCT", "JPXDecode", "CCITTFaxDecode", "CCF", "JBIG2Decode":
		return true
	}
	return false
}

// DecodeImageStream applies every filter up to (but not including) a
// final image codec. codec is "" when the data came out fully decoded;
// otherwise it names the codec left unapplied and parms carries that
// filter's own /DecodeParms, already resolved.
func (d *Document) DecodeImageStream(s *Stream) (data []byte, codec Name, parms Dict, err error) {
	if s == nil {
		return nil, "", nil, fmt.Errorf("pdf: nil stream")
	}
	return decodeStreamUntil(d.Resolve, s.Dict, s.Raw, isImageCodec)
}

// PageContent returns a page's content stream, decoded — /Contents is
// either one stream or an array of them, and PDF 32000-1 §7.8.2 defines
// an array as the concatenation of its parts *with a separator between
// them*, which matters: a producer is free to end one part mid-operator.
func (d *Document) PageContent(pageDict Dict) ([]byte, error) {
	var parts [][]byte
	switch v := d.Resolve(pageDict.Get(Name("Contents"))).(type) {
	case *Stream:
		b, err := d.DecodeStream(v)
		if err != nil {
			return nil, err
		}
		parts = append(parts, b)
	case Array:
		for _, e := range v {
			s, ok := d.Resolve(e).(*Stream)
			if !ok {
				continue
			}
			b, err := d.DecodeStream(s)
			if err != nil {
				// One unreadable part of a multi-part content stream
				// is not a reason to render nothing: the rest of the
				// page is still worth showing behind a stamp being
				// placed on it.
				continue
			}
			parts = append(parts, b)
		}
	case nil:
		return nil, nil
	}
	return bytes.Join(parts, []byte("\n")), nil
}
