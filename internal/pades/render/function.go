package render

import (
	"fmt"
	"math"
	"strconv"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// function is a PDF function (PDF 32000-1 §7.10): the thing a shading
// asks for a colour at a point, and the thing a Separation colour space
// uses to turn one tint into the components of its alternate space.
//
// All four types are implemented. Types 2 and 3 are trivial; type 0 is a
// sampled table; type 4 is a small PostScript calculator, and it is here
// rather than approximated because it is what Illustrator and InDesign
// emit for a spot colour — approximating it turns a red company logo
// grey, which is exactly the kind of wrongness a person placing a stamp
// would read as "the preview is broken".
type function struct {
	domain []float64
	rng    []float64

	kind int

	// type 0
	samples       []float64 // flattened, size[0]*size[1]*...*nOut
	size          []int
	encode        []float64
	decode        []float64
	nOut          int
	bitsPerSample int

	// type 2
	c0, c1 []float64
	n      float64

	// type 3
	funcs  []*function
	bounds []float64

	// type 4
	prog []psOp
}

const (
	fnSampled = iota
	fnExponential
	fnStitching
	fnPostScript
)

func parseFunction(doc *pdf.Document, obj pdf.Object) (*function, error) {
	resolved := doc.Resolve(obj)
	var dict pdf.Dict
	var stream *pdf.Stream
	switch v := resolved.(type) {
	case pdf.Dict:
		dict = v
	case *pdf.Stream:
		dict, stream = v.Dict, v
	case pdf.Array:
		// An array of functions, one per output component. Wrapped as a
		// stitching-shaped composite so callers see one function.
		f := &function{kind: fnStitching, domain: []float64{0, 1}}
		for _, e := range v {
			sub, err := parseFunction(doc, e)
			if err != nil {
				return nil, err
			}
			f.funcs = append(f.funcs, sub)
		}
		f.kind = -1 // array-of-functions: see eval
		return f, nil
	default:
		return nil, fmt.Errorf("render: not a function")
	}

	f := &function{}
	f.domain = floatArray(doc, dict.Get(pdf.Name("Domain")))
	f.rng = floatArray(doc, dict.Get(pdf.Name("Range")))
	ft, _ := asInt(doc.Resolve(dict.Get(pdf.Name("FunctionType"))))

	switch ft {
	case 0:
		if stream == nil {
			return nil, fmt.Errorf("render: sampled function is not a stream")
		}
		data, err := doc.DecodeStream(stream)
		if err != nil {
			return nil, err
		}
		f.kind = fnSampled
		for _, v := range floatArray(doc, dict.Get(pdf.Name("Size"))) {
			f.size = append(f.size, int(v))
		}
		f.bitsPerSample, _ = asInt(doc.Resolve(dict.Get(pdf.Name("BitsPerSample"))))
		f.encode = floatArray(doc, dict.Get(pdf.Name("Encode")))
		f.decode = floatArray(doc, dict.Get(pdf.Name("Decode")))
		f.nOut = len(f.rng) / 2
		if f.nOut == 0 || len(f.size) == 0 || f.bitsPerSample == 0 {
			return nil, fmt.Errorf("render: sampled function is missing its shape")
		}
		total := f.nOut
		for _, s := range f.size {
			total *= s
		}
		f.samples = unpackSamples(data, f.bitsPerSample, total)
	case 2:
		f.kind = fnExponential
		f.c0 = floatArray(doc, dict.Get(pdf.Name("C0")))
		f.c1 = floatArray(doc, dict.Get(pdf.Name("C1")))
		if len(f.c0) == 0 {
			f.c0 = []float64{0}
		}
		if len(f.c1) == 0 {
			f.c1 = []float64{1}
		}
		f.n = 1
		if v, ok := asFloat(doc.Resolve(dict.Get(pdf.Name("N")))); ok {
			f.n = v
		}
	case 3:
		f.kind = fnStitching
		arr, _ := doc.Resolve(dict.Get(pdf.Name("Functions"))).(pdf.Array)
		for _, e := range arr {
			sub, err := parseFunction(doc, e)
			if err != nil {
				return nil, err
			}
			f.funcs = append(f.funcs, sub)
		}
		f.bounds = floatArray(doc, dict.Get(pdf.Name("Bounds")))
		f.encode = floatArray(doc, dict.Get(pdf.Name("Encode")))
	case 4:
		if stream == nil {
			return nil, fmt.Errorf("render: PostScript function is not a stream")
		}
		data, err := doc.DecodeStream(stream)
		if err != nil {
			return nil, err
		}
		prog, err := parsePostScript(string(data))
		if err != nil {
			return nil, err
		}
		f.kind = fnPostScript
		f.prog = prog
		f.nOut = len(f.rng) / 2
	default:
		return nil, fmt.Errorf("render: unsupported function type %d", ft)
	}
	return f, nil
}

// eval evaluates the function at in, returning its output components.
func (f *function) eval(in []float64) []float64 {
	if f == nil {
		return nil
	}
	// Clip inputs to the declared domain, which PDF requires.
	x := make([]float64, len(in))
	copy(x, in)
	for i := range x {
		if 2*i+1 < len(f.domain) {
			x[i] = clampFloat(x[i], f.domain[2*i], f.domain[2*i+1])
		}
	}

	var out []float64
	switch f.kind {
	case -1: // an array of one-output functions
		for _, sub := range f.funcs {
			out = append(out, sub.eval(x)...)
		}
		return out
	case fnExponential:
		n := len(f.c0)
		if len(f.c1) > n {
			n = len(f.c1)
		}
		out = make([]float64, n)
		t := 0.0
		if len(x) > 0 {
			t = x[0]
		}
		p := math.Pow(t, f.n)
		for i := 0; i < n; i++ {
			c0, c1 := 0.0, 1.0
			if i < len(f.c0) {
				c0 = f.c0[i]
			}
			if i < len(f.c1) {
				c1 = f.c1[i]
			}
			out[i] = c0 + p*(c1-c0)
		}
	case fnStitching:
		if len(f.funcs) == 0 {
			return nil
		}
		t := 0.0
		if len(x) > 0 {
			t = x[0]
		}
		d0, d1 := 0.0, 1.0
		if len(f.domain) >= 2 {
			d0, d1 = f.domain[0], f.domain[1]
		}
		k := 0
		for k < len(f.bounds) && t >= f.bounds[k] {
			k++
		}
		if k >= len(f.funcs) {
			k = len(f.funcs) - 1
		}
		lo := d0
		if k > 0 {
			lo = f.bounds[k-1]
		}
		hi := d1
		if k < len(f.bounds) {
			hi = f.bounds[k]
		}
		e0, e1 := 0.0, 1.0
		if 2*k+1 < len(f.encode) {
			e0, e1 = f.encode[2*k], f.encode[2*k+1]
		}
		out = f.funcs[k].eval([]float64{interpolate(t, lo, hi, e0, e1)})
	case fnSampled:
		out = f.evalSampled(x)
	case fnPostScript:
		out = runPostScript(f.prog, x, f.nOut)
	}

	for i := range out {
		if 2*i+1 < len(f.rng) {
			out[i] = clampFloat(out[i], f.rng[2*i], f.rng[2*i+1])
		}
	}
	return out
}

// evalSampled reads the sample table with multilinear interpolation over
// the first input only when there is one input, and nearest-sample
// lookup for the rare multi-input case (a shading with two inputs is a
// type 1 function, which no producer this project has seen emits).
func (f *function) evalSampled(x []float64) []float64 {
	m := len(f.size)
	idx := make([]int, m)
	frac := 0.0
	for i := 0; i < m; i++ {
		d0, d1 := 0.0, 1.0
		if 2*i+1 < len(f.domain) {
			d0, d1 = f.domain[2*i], f.domain[2*i+1]
		}
		e0, e1 := 0.0, float64(f.size[i]-1)
		if 2*i+1 < len(f.encode) {
			e0, e1 = f.encode[2*i], f.encode[2*i+1]
		}
		v := 0.0
		if i < len(x) {
			v = x[i]
		}
		e := clampFloat(interpolate(v, d0, d1, e0, e1), 0, float64(f.size[i]-1))
		idx[i] = int(e)
		if i == 0 {
			frac = e - float64(idx[i])
		}
	}

	sampleAt := func(first int) []float64 {
		off := 0
		stride := 1
		for i := 0; i < m; i++ {
			k := idx[i]
			if i == 0 {
				k = first
			}
			if k >= f.size[i] {
				k = f.size[i] - 1
			}
			off += k * stride
			stride *= f.size[i]
		}
		out := make([]float64, f.nOut)
		maxV := float64(uint64(1)<<uint(f.bitsPerSample) - 1)
		for j := 0; j < f.nOut; j++ {
			p := off*f.nOut + j
			if p >= len(f.samples) {
				continue
			}
			d0, d1 := 0.0, 1.0
			if 2*j+1 < len(f.decode) {
				d0, d1 = f.decode[2*j], f.decode[2*j+1]
			} else if 2*j+1 < len(f.rng) {
				d0, d1 = f.rng[2*j], f.rng[2*j+1]
			}
			out[j] = interpolate(f.samples[p], 0, maxV, d0, d1)
		}
		return out
	}

	lo := sampleAt(idx[0])
	if frac == 0 || idx[0]+1 >= f.size[0] {
		return lo
	}
	hi := sampleAt(idx[0] + 1)
	for j := range lo {
		lo[j] += frac * (hi[j] - lo[j])
	}
	return lo
}

func interpolate(x, xmin, xmax, ymin, ymax float64) float64 {
	if xmax == xmin {
		return ymin
	}
	return ymin + (x-xmin)*(ymax-ymin)/(xmax-xmin)
}

// unpackSamples reads n samples of bits each, MSB first, as PDF's
// sampled functions store them.
func unpackSamples(data []byte, bits, n int) []float64 {
	out := make([]float64, 0, n)
	bitPos := 0
	total := len(data) * 8
	for i := 0; i < n; i++ {
		if bitPos+bits > total {
			out = append(out, 0)
			continue
		}
		var v uint64
		for b := 0; b < bits; b++ {
			byteIdx := (bitPos + b) / 8
			bit := (data[byteIdx] >> (7 - uint((bitPos+b)%8))) & 1
			v = v<<1 | uint64(bit)
		}
		bitPos += bits
		out = append(out, float64(v))
	}
	return out
}

// --- Type 4: the PostScript calculator ---

type psOp struct {
	op  string
	num float64
	// blocks for if/ifelse
	a, b []psOp
}

// parsePostScript reads the { ... } program of a type 4 function.
func parsePostScript(src string) ([]psOp, error) {
	toks := psTokens(src)
	pos := 0
	// The whole program is wrapped in one pair of braces.
	if pos < len(toks) && toks[pos] == "{" {
		pos++
	}
	prog, _, err := psBlock(toks, pos)
	return prog, err
}

func psTokens(src string) []string {
	var toks []string
	cur := ""
	flush := func() {
		if cur != "" {
			toks = append(toks, cur)
			cur = ""
		}
	}
	for i := 0; i < len(src); i++ {
		ch := src[i]
		switch {
		case ch == '{' || ch == '}':
			flush()
			toks = append(toks, string(ch))
		case ch == '%':
			flush()
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case ch <= ' ':
			flush()
		default:
			cur += string(ch)
		}
	}
	flush()
	return toks
}

// psBlock reads operators until the closing brace, recursing into
// nested blocks so that "if" and "ifelse" find theirs on a side stack
// rather than on the operand stack.
func psBlock(toks []string, pos int) ([]psOp, int, error) {
	var out []psOp
	var pending [][]psOp
	for pos < len(toks) {
		t := toks[pos]
		switch t {
		case "}":
			return out, pos + 1, nil
		case "{":
			blk, next, err := psBlock(toks, pos+1)
			if err != nil {
				return nil, 0, err
			}
			pending = append(pending, blk)
			pos = next
			continue
		case "if":
			if len(pending) < 1 {
				return nil, 0, fmt.Errorf("render: postscript if without a block")
			}
			out = append(out, psOp{op: "if", a: pending[len(pending)-1]})
			pending = pending[:len(pending)-1]
		case "ifelse":
			if len(pending) < 2 {
				return nil, 0, fmt.Errorf("render: postscript ifelse without two blocks")
			}
			out = append(out, psOp{op: "ifelse", a: pending[len(pending)-2], b: pending[len(pending)-1]})
			pending = pending[:len(pending)-2]
		default:
			if v, err := strconv.ParseFloat(t, 64); err == nil {
				out = append(out, psOp{op: "#", num: v})
			} else {
				out = append(out, psOp{op: t})
			}
		}
		pos++
	}
	return out, pos, nil
}

func runPostScript(prog []psOp, in []float64, nOut int) []float64 {
	st := append([]float64{}, in...)
	st = psExec(prog, st, 0)
	if nOut <= 0 || nOut > len(st) {
		return st
	}
	return st[len(st)-nOut:]
}

// psExec is deliberately total: an unknown operator, an empty stack or a
// runaway recursion leaves the stack as it is rather than failing. A
// tint transform that produces a slightly wrong colour is a far better
// outcome, in a preview, than one that produces no page.
func psExec(prog []psOp, st []float64, depth int) []float64 {
	if depth > 32 {
		return st
	}
	pop := func() float64 {
		if len(st) == 0 {
			return 0
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	push := func(v float64) { st = append(st, v) }
	b2f := func(b bool) float64 {
		if b {
			return 1
		}
		return 0
	}

	for _, op := range prog {
		switch op.op {
		case "#":
			push(op.num)
		case "add":
			b, a := pop(), pop()
			push(a + b)
		case "sub":
			b, a := pop(), pop()
			push(a - b)
		case "mul":
			b, a := pop(), pop()
			push(a * b)
		case "div":
			b, a := pop(), pop()
			if b == 0 {
				push(0)
			} else {
				push(a / b)
			}
		case "idiv":
			b, a := pop(), pop()
			if int(b) == 0 {
				push(0)
			} else {
				push(float64(int(a) / int(b)))
			}
		case "mod":
			b, a := pop(), pop()
			if int(b) == 0 {
				push(0)
			} else {
				push(float64(int(a) % int(b)))
			}
		case "neg":
			push(-pop())
		case "abs":
			push(math.Abs(pop()))
		case "sqrt":
			push(math.Sqrt(math.Abs(pop())))
		case "sin":
			push(math.Sin(pop() * math.Pi / 180))
		case "cos":
			push(math.Cos(pop() * math.Pi / 180))
		case "atan":
			den, num := pop(), pop()
			d := math.Atan2(num, den) * 180 / math.Pi
			if d < 0 {
				d += 360
			}
			push(d)
		case "exp":
			b, a := pop(), pop()
			push(math.Pow(a, b))
		case "ln":
			v := pop()
			if v <= 0 {
				push(0)
			} else {
				push(math.Log(v))
			}
		case "log":
			v := pop()
			if v <= 0 {
				push(0)
			} else {
				push(math.Log10(v))
			}
		case "cvi", "truncate":
			push(float64(int(pop())))
		case "cvr":
			// already a real
		case "floor":
			push(math.Floor(pop()))
		case "ceiling":
			push(math.Ceil(pop()))
		case "round":
			push(math.Round(pop()))
		case "dup":
			v := pop()
			push(v)
			push(v)
		case "pop":
			pop()
		case "exch":
			b, a := pop(), pop()
			push(b)
			push(a)
		case "copy":
			n := int(pop())
			if n > 0 && n <= len(st) {
				st = append(st, st[len(st)-n:]...)
			}
		case "index":
			n := int(pop())
			if n >= 0 && n < len(st) {
				push(st[len(st)-1-n])
			} else {
				push(0)
			}
		case "roll":
			j := int(pop())
			n := int(pop())
			if n > 0 && n <= len(st) {
				part := st[len(st)-n:]
				j = ((j % n) + n) % n
				rolled := append(append([]float64{}, part[n-j:]...), part[:n-j]...)
				copy(part, rolled)
			}
		case "eq":
			b, a := pop(), pop()
			push(b2f(a == b))
		case "ne":
			b, a := pop(), pop()
			push(b2f(a != b))
		case "gt":
			b, a := pop(), pop()
			push(b2f(a > b))
		case "ge":
			b, a := pop(), pop()
			push(b2f(a >= b))
		case "lt":
			b, a := pop(), pop()
			push(b2f(a < b))
		case "le":
			b, a := pop(), pop()
			push(b2f(a <= b))
		case "and":
			b, a := pop(), pop()
			push(float64(int(a) & int(b)))
		case "or":
			b, a := pop(), pop()
			push(float64(int(a) | int(b)))
		case "xor":
			b, a := pop(), pop()
			push(float64(int(a) ^ int(b)))
		case "not":
			a := pop()
			if a == 0 || a == 1 {
				push(b2f(a == 0))
			} else {
				push(float64(^int(a)))
			}
		case "bitshift":
			b, a := pop(), pop()
			if b >= 0 {
				push(float64(int(a) << uint(b)))
			} else {
				push(float64(int(a) >> uint(-b)))
			}
		case "true":
			push(1)
		case "false":
			push(0)
		case "if":
			if pop() != 0 {
				st = psExec(op.a, st, depth+1)
			}
		case "ifelse":
			if pop() != 0 {
				st = psExec(op.a, st, depth+1)
			} else {
				st = psExec(op.b, st, depth+1)
			}
		}
	}
	return st
}

func floatArray(doc *pdf.Document, o pdf.Object) []float64 {
	arr, ok := doc.Resolve(o).(pdf.Array)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		v, _ := asFloat(doc.Resolve(e))
		out = append(out, v)
	}
	return out
}

func asFloat(o pdf.Object) (float64, bool) {
	switch v := o.(type) {
	case int64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

func asInt(o pdf.Object) (int, bool) {
	v, ok := asFloat(o)
	return int(v), ok
}
