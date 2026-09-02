package pdf

import "testing"

// buildMultiPageFixture returns a three-page document (object numbers
// 1=Catalog, 2=Pages, 3/4/5=Page in reading order), each page carrying
// its own /MediaBox directly — the baseline case FindPage's tests build
// on.
func buildMultiPageFixture() []byte {
	b := newFixtureBuilder()
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.writeObj(2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>")
	b.writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << >> >>")
	b.writeObj(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> >>")
	b.writeObj(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << >> >>")
	b.writeClassicXref(6, 1, "")
	return b.bytesOut()
}

func mustParseAndCatalog(t *testing.T, data []byte) (*Document, Dict) {
	t.Helper()
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rootRef, ok := doc.Trailer().Get(Name("Root")).(Reference)
	if !ok {
		t.Fatal("trailer /Root is not a reference")
	}
	catalog, ok := doc.ResolveDict(rootRef)
	if !ok {
		t.Fatal("/Root does not resolve to a dictionary")
	}
	return doc, catalog
}

func TestFindPageFirstMiddleLastAndOutOfRange(t *testing.T) {
	doc, catalog := mustParseAndCatalog(t, buildMultiPageFixture())

	tests := []struct {
		n       int
		want    int
		wantErr bool
	}{
		{1, 3, false},
		{2, 4, false},
		{3, 5, false},
		{-1, 5, false}, // last page
		{0, 0, true},
		{4, 0, true},
		{-2, 0, true},
	}
	for _, tt := range tests {
		got, err := FindPage(doc, catalog, tt.n)
		if tt.wantErr {
			if err == nil {
				t.Errorf("FindPage(%d): want error, got page %d", tt.n, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("FindPage(%d): %v", tt.n, err)
			continue
		}
		if got != tt.want {
			t.Errorf("FindPage(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestResolveMediaBoxDirect(t *testing.T) {
	doc, catalog := mustParseAndCatalog(t, buildMultiPageFixture())
	pageNum, err := FindPage(doc, catalog, 2)
	if err != nil {
		t.Fatal(err)
	}
	pageDict, ok := doc.ResolveDict(Reference{Num: pageNum})
	if !ok {
		t.Fatal("page does not resolve")
	}
	box := ResolveMediaBox(doc, pageDict)
	want := [4]float64{0, 0, 200, 200}
	if box != want {
		t.Fatalf("ResolveMediaBox = %v, want %v", box, want)
	}
}

// buildInheritedMediaBoxFixture is F4 §2.1's central trap: the page
// itself carries no /MediaBox at all — it is set only on the /Pages
// node, two levels up (Catalog -> Pages(with MediaBox) -> Pages -> Page)
// — which is common in real documents (F4 §2.1) and is exactly the case
// a parser that only checks the page object itself gets wrong.
func buildInheritedMediaBoxFixture() []byte {
	b := newFixtureBuilder()
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.writeObj(2, "<< /Type /Pages /Kids [6 0 R] /Count 1 /MediaBox [0 0 595 842] /Rotate 90 >>")
	b.writeObj(6, "<< /Type /Pages /Parent 2 0 R /Kids [3 0 R] /Count 1 >>")
	b.writeObj(3, "<< /Type /Page /Parent 6 0 R /Resources << >> >>")
	b.writeClassicXref(7, 1, "")
	return b.bytesOut()
}

func TestResolveMediaBoxInheritedThroughGrandparent(t *testing.T) {
	doc, catalog := mustParseAndCatalog(t, buildInheritedMediaBoxFixture())
	pageNum, err := FindPage(doc, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	pageDict, _ := doc.ResolveDict(Reference{Num: pageNum})

	box := ResolveMediaBox(doc, pageDict)
	want := [4]float64{0, 0, 595, 842}
	if box != want {
		t.Fatalf("ResolveMediaBox (inherited) = %v, want %v", box, want)
	}

	rotate := ResolveRotate(doc, pageDict)
	if rotate != 90 {
		t.Fatalf("ResolveRotate (inherited) = %d, want 90", rotate)
	}
}

func TestResolveMediaBoxFallsBackToA4WhenAbsentEverywhere(t *testing.T) {
	b := newFixtureBuilder()
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	b.writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << >> >>")
	b.writeClassicXref(4, 1, "")

	doc, catalog := mustParseAndCatalog(t, b.bytesOut())
	pageNum, err := FindPage(doc, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	pageDict, _ := doc.ResolveDict(Reference{Num: pageNum})

	box := ResolveMediaBox(doc, pageDict)
	if box != a4MediaBox {
		t.Fatalf("ResolveMediaBox (absent everywhere) = %v, want A4 %v", box, a4MediaBox)
	}
	if r := ResolveRotate(doc, pageDict); r != 0 {
		t.Fatalf("ResolveRotate (absent everywhere) = %d, want 0", r)
	}
}

func TestResolveMediaBoxNormalisesReversedCorners(t *testing.T) {
	b := newFixtureBuilder()
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	// Corners written high-to-low: PDF permits this (PDF 32000-1 §7.7.3.3
	// only requires two opposite corners, not a canonical order).
	b.writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [612 792 0 0] /Resources << >> >>")
	b.writeClassicXref(4, 1, "")

	doc, catalog := mustParseAndCatalog(t, b.bytesOut())
	pageNum, _ := FindPage(doc, catalog, 1)
	pageDict, _ := doc.ResolveDict(Reference{Num: pageNum})
	box := ResolveMediaBox(doc, pageDict)
	want := [4]float64{0, 0, 612, 792}
	if box != want {
		t.Fatalf("ResolveMediaBox (reversed corners) = %v, want %v", box, want)
	}
}
