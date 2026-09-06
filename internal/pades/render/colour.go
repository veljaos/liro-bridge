package render

import (
	"math"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

type csKind int

const (
	csGray csKind = iota
	csRGB
	csCMYK
	csIndexed
	csSeparation
	csLab
	csPattern
)

// colorSpace is a PDF colour space, reduced to what turning components
// into a screen colour needs. The device spaces are taken literally; the
// calibrated ones (CalRGB, CalGray, ICCBased) are treated as their
// device equivalents, chosen by component count.
//
// That last simplification is deliberate and worth naming: honouring an
// ICC profile means implementing colour management, and the difference
// it makes is a few percent of saturation on a page whose purpose here
// is to show a person where the text sits. Lab is converted properly,
// because it is not close to anything otherwise.
type colorSpace struct {
	kind csKind
	n    int

	base   *colorSpace // Indexed, Pattern
	lookup []byte      // Indexed
	hival  int

	tint *function   // Separation, DeviceN
	alt  *colorSpace // Separation, DeviceN

	whitePoint [3]float64
	labRange   [4]float64
}

var (
	deviceGray = &colorSpace{kind: csGray, n: 1}
	deviceRGB  = &colorSpace{kind: csRGB, n: 3}
	deviceCMYK = &colorSpace{kind: csCMYK, n: 4}
	patternCS  = &colorSpace{kind: csPattern, n: 1}
)

// initial is the colour a space starts at when it is selected, per
// PDF 32000-1 §8.6.8: black in every device space, and the first entry
// in an indexed one.
func (cs *colorSpace) initial() []float64 {
	if cs == nil {
		return []float64{0}
	}
	switch cs.kind {
	case csCMYK:
		return []float64{0, 0, 0, 1}
	case csSeparation:
		out := make([]float64, cs.n)
		for i := range out {
			out[i] = 1
		}
		return out
	}
	return make([]float64, cs.n)
}

func (cs *colorSpace) toRGB(c []float64) (float32, float32, float32) {
	if cs == nil {
		return 0, 0, 0
	}
	switch cs.kind {
	case csGray:
		g := comp(c, 0)
		return float32(g), float32(g), float32(g)
	case csRGB:
		return float32(clampFloat(comp(c, 0), 0, 1)), float32(clampFloat(comp(c, 1), 0, 1)), float32(clampFloat(comp(c, 2), 0, 1))
	case csCMYK:
		cy, m, y, k := comp(c, 0), comp(c, 1), comp(c, 2), comp(c, 3)
		return float32(clampFloat((1-cy)*(1-k), 0, 1)),
			float32(clampFloat((1-m)*(1-k), 0, 1)),
			float32(clampFloat((1-y)*(1-k), 0, 1))
	case csIndexed:
		idx := int(comp(c, 0) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx > cs.hival {
			idx = cs.hival
		}
		bn := 1
		if cs.base != nil {
			bn = cs.base.n
		}
		vals := make([]float64, bn)
		for i := 0; i < bn; i++ {
			p := idx*bn + i
			if p < len(cs.lookup) {
				vals[i] = float64(cs.lookup[p]) / 255
			}
		}
		if cs.base != nil && cs.base.kind == csLab {
			// Lab components are not 0..1: the lookup table stores them
			// scaled into a byte over the space's own ranges.
			vals[0] *= 100
			for i := 1; i < 3 && i < bn; i++ {
				lo, hi := cs.base.labRange[(i-1)*2], cs.base.labRange[(i-1)*2+1]
				vals[i] = lo + vals[i]*(hi-lo)
			}
		}
		return cs.base.toRGB(vals)
	case csSeparation:
		if cs.tint != nil && cs.alt != nil {
			if out := cs.tint.eval(c); len(out) > 0 {
				return cs.alt.toRGB(out)
			}
		}
		// No usable tint transform: treat the tint as ink coverage,
		// which is right for the black and grey spot colours that make
		// up nearly all real use and merely desaturated for the rest.
		t := clampFloat(comp(c, 0), 0, 1)
		g := float32(1 - t)
		return g, g, g
	case csLab:
		return labToRGB(comp(c, 0), comp(c, 1), comp(c, 2), cs.whitePoint)
	case csPattern:
		// A pattern has no colour of its own; the caller substitutes a
		// representative one. Mid grey is what it gets if it does not.
		return 0.5, 0.5, 0.5
	}
	return 0, 0, 0
}

func comp(c []float64, i int) float64 {
	if i < len(c) {
		return c[i]
	}
	return 0
}

func labToRGB(l, a, bb float64, wp [3]float64) (float32, float32, float32) {
	if wp[0] == 0 {
		wp = [3]float64{0.9505, 1.0, 1.089}
	}
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - bb/200
	g := func(t float64) float64 {
		if t > 6.0/29 {
			return t * t * t
		}
		return 3 * (6.0 / 29) * (6.0 / 29) * (t - 4.0/29)
	}
	x, y, z := wp[0]*g(fx), wp[1]*g(fy), wp[2]*g(fz)
	r := 3.2406*x - 1.5372*y - 0.4986*z
	gg := -0.9689*x + 1.8758*y + 0.0415*z
	b := 0.0557*x - 0.2040*y + 1.0570*z
	srgb := func(v float64) float32 {
		v = clampFloat(v, 0, 1)
		if v <= 0.0031308 {
			return float32(12.92 * v)
		}
		return float32(1.055*math.Pow(v, 1/2.4) - 0.055)
	}
	return srgb(r), srgb(gg), srgb(b)
}

// parseColorSpace resolves a colour space object, following a name
// through the resource dictionary's /ColorSpace entry when it is not one
// of the device space names.
func parseColorSpace(doc *pdf.Document, obj pdf.Object, res pdf.Dict, depth int) *colorSpace {
	if depth > 8 {
		return deviceGray
	}
	switch v := doc.Resolve(obj).(type) {
	case pdf.Name:
		switch v {
		case "DeviceGray", "G", "CalGray":
			return deviceGray
		case "DeviceRGB", "RGB", "CalRGB":
			return deviceRGB
		case "DeviceCMYK", "CMYK":
			return deviceCMYK
		case "Pattern":
			return patternCS
		}
		if res != nil {
			table, _ := doc.Resolve(res.Get(pdf.Name("ColorSpace"))).(pdf.Dict)
			if entry := table.Get(v); entry != nil {
				return parseColorSpace(doc, entry, nil, depth+1)
			}
		}
		return deviceGray
	case pdf.Array:
		if len(v) == 0 {
			return deviceGray
		}
		family, _ := doc.Resolve(v[0]).(pdf.Name)
		switch family {
		case "ICCBased":
			n := 3
			if s, ok := doc.Resolve(at(v, 1)).(*pdf.Stream); ok {
				if k, ok := asInt(doc.Resolve(s.Dict.Get(pdf.Name("N")))); ok {
					n = k
				} else if alt := s.Dict.Get(pdf.Name("Alternate")); alt != nil {
					return parseColorSpace(doc, alt, res, depth+1)
				}
			}
			switch n {
			case 1:
				return deviceGray
			case 4:
				return deviceCMYK
			default:
				return deviceRGB
			}
		case "CalRGB":
			return deviceRGB
		case "CalGray":
			return deviceGray
		case "Lab":
			cs := &colorSpace{kind: csLab, n: 3, labRange: [4]float64{-100, 100, -100, 100}}
			if d, ok := doc.Resolve(at(v, 1)).(pdf.Dict); ok {
				if wp := floatArray(doc, d.Get(pdf.Name("WhitePoint"))); len(wp) == 3 {
					cs.whitePoint = [3]float64{wp[0], wp[1], wp[2]}
				}
				if r := floatArray(doc, d.Get(pdf.Name("Range"))); len(r) == 4 {
					cs.labRange = [4]float64{r[0], r[1], r[2], r[3]}
				}
			}
			return cs
		case "Indexed", "I":
			cs := &colorSpace{kind: csIndexed, n: 1}
			cs.base = parseColorSpace(doc, at(v, 1), res, depth+1)
			cs.hival, _ = asInt(doc.Resolve(at(v, 2)))
			switch lk := doc.Resolve(at(v, 3)).(type) {
			case pdf.String:
				cs.lookup = []byte(lk)
			case *pdf.Stream:
				if b, err := doc.DecodeStream(lk); err == nil {
					cs.lookup = b
				}
			}
			return cs
		case "Separation", "DeviceN":
			cs := &colorSpace{kind: csSeparation, n: 1}
			altIdx, tintIdx := 2, 3
			if family == "DeviceN" {
				if names, ok := doc.Resolve(at(v, 1)).(pdf.Array); ok {
					cs.n = len(names)
				}
			}
			cs.alt = parseColorSpace(doc, at(v, altIdx), res, depth+1)
			if fn, err := parseFunction(doc, at(v, tintIdx)); err == nil {
				cs.tint = fn
			}
			return cs
		case "Pattern":
			cs := &colorSpace{kind: csPattern, n: 1}
			if len(v) > 1 {
				cs.base = parseColorSpace(doc, at(v, 1), res, depth+1)
				cs.n = cs.base.n
			}
			return cs
		case "DeviceGray", "G":
			return deviceGray
		case "DeviceRGB", "RGB":
			return deviceRGB
		case "DeviceCMYK", "CMYK":
			return deviceCMYK
		}
	}
	return deviceGray
}

func at(a pdf.Array, i int) pdf.Object {
	if i < len(a) {
		return a[i]
	}
	return nil
}
