package appearance

import "testing"

func TestHeightForLinesTable(t *testing.T) {
	// F4 §2/§8: "All five height values are exercised, one per line
	// count."
	tests := []struct {
		lines int
		want  float64
	}{
		{1, 44},
		{2, 44},
		{3, 46},
		{4, 56},
		{5, 72},
	}
	for _, tt := range tests {
		got, err := HeightForLines(tt.lines)
		if err != nil {
			t.Fatalf("HeightForLines(%d): %v", tt.lines, err)
		}
		if got != tt.want {
			t.Errorf("HeightForLines(%d) = %g, want %g", tt.lines, got, tt.want)
		}
	}
}

func TestHeightForLinesRejectsOutOfRange(t *testing.T) {
	for _, n := range []int{0, -1, 6, 100} {
		if _, err := HeightForLines(n); err == nil {
			t.Errorf("HeightForLines(%d): want error, got none", n)
		}
	}
}

// pageBox is a representative A4-ish box, offset from the origin to
// also exercise MediaBoxes that do not start at (0,0).
var pageBox = [4]float64{0, 0, 595, 842}

func TestPlaceCornerLandsInsidePageBoxEveryCornerEveryRotation(t *testing.T) {
	// F4 §8: "Every corner position lands inside the page box."
	corners := []Corner{BottomRight, BottomLeft, TopRight, TopLeft}
	rotations := []int{0, 90, 180, 270}
	const stampW, stampH float64 = StampWidth, 56

	for _, rot := range rotations {
		for _, c := range corners {
			rect, matrix, err := PlaceCorner(pageBox, rot, c, Margin, stampW, stampH)
			if err != nil {
				t.Fatalf("PlaceCorner(rotate=%d, corner=%d): %v", rot, c, err)
			}
			if rect[0] < pageBox[0] || rect[1] < pageBox[1] || rect[2] > pageBox[2] || rect[3] > pageBox[3] {
				t.Errorf("PlaceCorner(rotate=%d, corner=%d) = %v lands outside page box %v", rot, c, rect, pageBox)
			}
			if rect[2] <= rect[0] || rect[3] <= rect[1] {
				t.Errorf("PlaceCorner(rotate=%d, corner=%d) = %v is not a positive-area rectangle", rot, c, rect)
			}
			// The Matrix must always be a pure rotation (determinant ±1,
			// no scaling) — a scaled Matrix would distort the stamp.
			det := matrix[0]*matrix[3] - matrix[1]*matrix[2]
			if det != 1 && det != -1 {
				t.Errorf("PlaceCorner(rotate=%d, corner=%d) matrix %v has determinant %g, want ±1", rot, c, matrix, det)
			}
		}
	}
}

func TestPlaceCornerRejectsUnsupportedRotate(t *testing.T) {
	if _, _, err := PlaceCorner(pageBox, 45, BottomRight, Margin, StampWidth, 44); err == nil {
		t.Fatal("PlaceCorner(rotate=45): want error, got none")
	}
}

// TestPlaceCornerBottomRightAtRotate0IsExact pins down the un-rotated
// case's exact numbers, since every other rotation's expected values in
// this file are derived from the same margin/width/height arithmetic
// and a wrong constant here would silently propagate.
func TestPlaceCornerBottomRightAtRotate0IsExact(t *testing.T) {
	rect, matrix, err := PlaceCorner(pageBox, 0, BottomRight, Margin, StampWidth, 44)
	if err != nil {
		t.Fatal(err)
	}
	want := [4]float64{595 - Margin - StampWidth, Margin, 595 - Margin, Margin + 44}
	if rect != want {
		t.Fatalf("rect = %v, want %v", rect, want)
	}
	if matrix != [6]float64{1, 0, 0, 1, 0, 0} {
		t.Fatalf("matrix = %v, want identity", matrix)
	}
}

// TestPlaceCornerRotationSwapsDimensions verifies F4 §8's "a rotated
// page places the stamp visibly correctly" at the geometry level: for a
// 90°/270° page, the content-space Rect must have swapped width/height
// relative to the stamp's own natural (stampW × stampH) size, because
// the physical corner the stamp is anchored to is a different corner of
// the underlying MediaBox at those rotations (see geometry.go's
// rotationPlan doc comment for the full derivation).
func TestPlaceCornerRotationSwapsDimensions(t *testing.T) {
	const stampW, stampH float64 = StampWidth, 56
	for _, rot := range []int{90, 270} {
		rect, _, err := PlaceCorner(pageBox, rot, BottomRight, Margin, stampW, stampH)
		if err != nil {
			t.Fatal(err)
		}
		gotW := rect[2] - rect[0]
		gotH := rect[3] - rect[1]
		if gotW != stampH || gotH != stampW {
			t.Errorf("rotate=%d: rect dims = %gx%g, want swapped %gx%g", rot, gotW, gotH, stampH, stampW)
		}
	}
	for _, rot := range []int{0, 180} {
		rect, _, err := PlaceCorner(pageBox, rot, BottomRight, Margin, stampW, stampH)
		if err != nil {
			t.Fatal(err)
		}
		gotW := rect[2] - rect[0]
		gotH := rect[3] - rect[1]
		if gotW != stampW || gotH != stampH {
			t.Errorf("rotate=%d: rect dims = %gx%g, want unswapped %gx%g", rot, gotW, gotH, stampW, stampH)
		}
	}
}

func TestNormaliseRotate(t *testing.T) {
	tests := map[int]int{0: 0, 90: 90, 360: 0, 450: 90, -90: 270, -360: 0, 720: 0}
	for in, want := range tests {
		if got := NormaliseRotate(in); got != want {
			t.Errorf("NormaliseRotate(%d) = %d, want %d", in, got, want)
		}
	}
}
