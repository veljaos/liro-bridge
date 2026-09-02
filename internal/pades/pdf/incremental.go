package pdf

import (
	"bytes"
	"fmt"
	"sort"
)

// Update builds one incremental revision on top of a parsed Document
// (F3 §3). Original bytes are never modified: Apply returns a new slice
// whose prefix is exactly Document.Data() (F3 §3.1), followed by the
// appended objects, a new cross-reference section matching the
// document's own mechanism (F3 §3.2), and a new trailer whose /Prev
// points at the document's previous startxref.
type Update struct {
	doc     *Document
	nextNum int
	objects map[int]Object
	order   []int

	// appliedOffsets is populated by Apply: the absolute byte offset,
	// in the returned file, where each object's "N 0 obj" header
	// begins. BuildPlaceholder (F3 §4) needs this to compute /ByteRange
	// from the signature dictionary's actual position.
	appliedOffsets map[int]int
}

// NewUpdate starts a new incremental revision on top of doc.
func NewUpdate(doc *Document) *Update {
	return &Update{
		doc:     doc,
		nextNum: highestObjectNumber(doc) + 1,
		objects: map[int]Object{},
	}
}

// highestObjectNumber returns the largest object number the document's
// merged xref table knows about.
func highestObjectNumber(doc *Document) int {
	max := 0
	for num := range doc.xref {
		if num > max {
			max = num
		}
	}
	return max
}

// NewObjectNumber allocates and returns an object number not used
// anywhere in the document or by an earlier call in this same update.
func (u *Update) NewObjectNumber() int {
	n := u.nextNum
	u.nextNum++
	return n
}

// Set defines (or, for an object number that already exists in the
// document, redefines) object num for this revision. F3 §3.3: modifying
// an existing object means appending a full copy under its existing
// number, never editing the earlier bytes — Set is exactly that append;
// the document's xref merge (newest revision wins) makes the new copy
// shadow the old one.
func (u *Update) Set(num int, obj Object) {
	if _, exists := u.objects[num]; !exists {
		u.order = append(u.order, num)
	}
	u.objects[num] = obj
}

// OffsetOf returns the absolute byte offset, in the file Apply returned,
// where object num's "N 0 obj" header begins. It is only valid after
// Apply has been called, and only for object numbers this Update wrote.
func (u *Update) OffsetOf(num int) (int, bool) {
	off, ok := u.appliedOffsets[num]
	return off, ok
}

// Apply serialises this revision and returns the complete new file.
func (u *Update) Apply() ([]byte, error) {
	out := make([]byte, len(u.doc.data))
	copy(out, u.doc.data)

	sort.Ints(u.order)
	offsets := make(map[int]int, len(u.order))
	for _, num := range u.order {
		offsets[num] = len(out)
		var objBuf bytes.Buffer
		writeIndirectObject(&objBuf, num, u.objects[num])
		out = append(out, objBuf.Bytes()...)
	}
	u.appliedOffsets = offsets

	prevStartxref, ok := findStartxref(u.doc.data)
	if !ok {
		return nil, fmt.Errorf("pdf: document has no startxref to chain /Prev from")
	}

	if u.doc.UsesXrefStreams() {
		return appendXrefStream(out, u.doc, offsets, u.order, u.nextNum, int64(prevStartxref)), nil
	}
	return appendClassicXref(out, u.doc, offsets, u.order, u.nextNum, int64(prevStartxref)), nil
}

// appendClassicXref appends a classic xref table listing exactly the
// changed/new object numbers, plus a trailer carrying /Prev.
func appendClassicXref(out []byte, doc *Document, offsets map[int]int, order []int, size int, prev int64) []byte {
	var b bytes.Buffer
	start := len(out)
	b.WriteString("xref\n")
	for _, run := range contiguousRuns(order) {
		fmt.Fprintf(&b, "%d %d\n", run[0], len(run))
		for _, num := range run {
			fmt.Fprintf(&b, "%010d %05d n \n", offsets[num], 0)
		}
	}
	b.WriteString("trailer\n")
	writeObject(&b, updateTrailer(doc, size, prev))
	fmt.Fprintf(&b, "\nstartxref\n%d\n%%%%EOF", start)
	return append(out, b.Bytes()...)
}

// appendXrefStream appends a cross-reference stream (uncompressed, for
// deterministic byte-exact output) describing exactly the changed/new
// object numbers plus itself, and a trailer dictionary carrying /Prev.
func appendXrefStream(out []byte, doc *Document, offsets map[int]int, order []int, xrefNum int, prev int64) []byte {
	xrefStart := len(out)
	nums := append(append([]int{}, order...), xrefNum)
	sort.Ints(nums)

	var rows bytes.Buffer
	for _, num := range nums {
		var off int64
		if num == xrefNum {
			off = int64(xrefStart)
		} else {
			off = int64(offsets[num])
		}
		rows.WriteByte(1) // type 1: in file. Free (type 0) entries are
		// never introduced by this project's updates, which only add or
		// shadow objects, never delete them.
		rows.WriteByte(byte(off >> 24))
		rows.WriteByte(byte(off >> 16))
		rows.WriteByte(byte(off >> 8))
		rows.WriteByte(byte(off))
		rows.WriteByte(0) // generation, high byte
		rows.WriteByte(0) // generation, low byte
	}

	dict := updateTrailer(doc, xrefNum+1, prev)
	dict[Name("Type")] = Name("XRef")
	dict[Name("W")] = Array{int64(1), int64(4), int64(2)}
	dict[Name("Index")] = indexArray(nums)

	var obj bytes.Buffer
	fmt.Fprintf(&obj, "%d 0 obj\n", xrefNum)
	writeObject(&obj, &Stream{Dict: dict, Raw: rows.Bytes()})
	obj.WriteString("\nendobj\n")
	fmt.Fprintf(&obj, "startxref\n%d\n%%%%EOF", xrefStart)

	return append(out, obj.Bytes()...)
}

// updateTrailer builds the new revision's trailer keys: /Root (and
// /Info, /ID when the document had them) carried forward unchanged,
// plus this revision's /Size and /Prev.
func updateTrailer(doc *Document, size int, prev int64) Dict {
	d := Dict{
		Name("Size"): int64(size),
		Name("Root"): doc.Trailer().Get(Name("Root")),
		Name("Prev"): prev,
	}
	if info := doc.Trailer().Get(Name("Info")); info != nil {
		d[Name("Info")] = info
	}
	if id := doc.Trailer().Get(Name("ID")); id != nil {
		d[Name("ID")] = id
	}
	return d
}

// contiguousRuns groups a sorted slice of object numbers into
// consecutive runs, so a classic xref subsection header can cover more
// than one object at a time. Purely a compactness nicety; one run per
// number would be equally valid PDF.
func contiguousRuns(sorted []int) [][]int {
	var runs [][]int
	for _, n := range sorted {
		if len(runs) > 0 {
			last := runs[len(runs)-1]
			if last[len(last)-1] == n-1 {
				runs[len(runs)-1] = append(last, n)
				continue
			}
		}
		runs = append(runs, []int{n})
	}
	return runs
}

// indexArray builds a cross-reference stream's /Index array from a
// sorted slice of object numbers, using the same run-grouping as the
// classic table.
func indexArray(sorted []int) Array {
	var arr Array
	for _, run := range contiguousRuns(sorted) {
		arr = append(arr, int64(run[0]), int64(len(run)))
	}
	return arr
}
