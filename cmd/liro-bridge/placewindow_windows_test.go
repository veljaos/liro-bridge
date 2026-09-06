//go:build windows

package main

// The placement window, driven through its own page.
//
// Every click here is dispatched inside the page's DOM through Eval —
// D-094's carve-out, which never touches the real cursor and cannot
// land on somebody else's window. What that leaves for a person is
// named at the bottom of this file and in the phase report.

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/render"
	"github.com/veljaos/liro-bridge/internal/placement"
)

// --- the document these tests are placed on --------------------------

// placementTestPDF builds a document with pages of different sizes and
// rotations, so that paging through it exercises the per-page box and
// rotation reading F6b §2.2 requires rather than one shape repeated.
func placementTestPDF(t *testing.T, pages int) string {
	t.Helper()
	var b strings.Builder
	var offsets []int
	obj := func(body string) int {
		offsets = append(offsets, b.Len())
		n := len(offsets)
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	b.WriteString("%PDF-1.7\n")
	obj("<</Type /Catalog /Pages 2 0 R>>")
	pagesObj := obj("placeholder")
	fontObj := obj("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding>>")

	var kids []string
	for i := 0; i < pages; i++ {
		box := "0 0 595 842"
		rotate := 0
		switch i % 4 {
		case 1:
			box = "0 0 842 595"
		case 2:
			rotate = 90
		case 3:
			box = "0 0 420 400"
		}
		content := fmt.Sprintf("BT /F1 22 Tf 60 700 Td (Page %d) Tj ET\n0.6 w 40 40 m 300 40 l S\n", i+1)
		contentObj := obj(fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(content), content))
		pageNum := obj(fmt.Sprintf(
			"<</Type /Page /Parent 2 0 R /MediaBox [%s] /Rotate %d /Resources <</Font <</F1 %d 0 R>>>> /Contents %d 0 R>>",
			box, rotate, fontObj, contentObj))
		kids = append(kids, fmt.Sprintf("%d 0 R", pageNum))
	}
	offsets[pagesObj-1] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<</Type /Pages /Count %d /Kids [%s]>>\nendobj\n",
		pagesObj, pages, strings.Join(kids, " "))

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)

	path := filepath.Join(t.TempDir(), "placement.pdf")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func placementTestCertificate(t *testing.T) *x509.Certificate {
	t.Helper()
	der, err := os.ReadFile(filepath.Join("..", "..", "testdata", "certs", "mup_signing.der"))
	if err != nil {
		t.Fatalf("reading the test certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// One placement window for the whole package, for the reason
// sharedwindow_windows_test.go gives: each ui.NewWindow builds its own
// WebView2 environment, and opening one per test is what stopped
// working at thirteen of them.
var (
	placeSharedOnce sync.Once
	placeSharedUI   *placeUI
	placeSharedErr  error
)

func sharedPlacementWindow(t *testing.T) *placeUI {
	t.Helper()
	placeSharedOnce.Do(func() {
		doc := placementTestPDF(t, 6)
		cfg := config.Default()
		cfg.VisibleStamp = true
		placeSharedUI, placeSharedErr = openPlacement(cfg, "sr-Latn", doc, placementTestCertificate(t), 0)
		if placeSharedErr == nil {
			placeSharedErr = placeSharedUI.start()
		}
	})
	if placeSharedErr != nil {
		t.Fatalf("opening the placement window: %v", placeSharedErr)
	}
	drain(placeSharedUI.messages)
	return placeSharedUI
}

// waitForImages waits for the browser to finish fetching the page and
// stamp images. They are loaded by the browser rather than posted into
// it, so they arrive a moment after the payload that named them —
// which is the point of serving them that way, and is why a test that
// looks straight after posting sees nothing.
func waitForImages(t *testing.T, p *placeUI) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got := placeEval(t, p, `(document.getElementById("page-image").naturalWidth > 0 &&
			document.getElementById("stamp-image").naturalWidth > 0) ? "yes" : "no"`)
		if got == "yes" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the page and stamp images never loaded")
}

func closeSharedPlacementWindow() {
	if placeSharedUI != nil {
		placeSharedUI.close()
	}
}

// answerOneRequest plays the part run() plays: the page asked for a
// page at a zoom, so draw it and post it back.
func answerOneRequest(t *testing.T, p *placeUI) placementRequest {
	t.Helper()
	select {
	case <-p.messages:
	case <-time.After(10 * time.Second):
		t.Fatal("the page never asked for anything")
	}
	req, err := readPlacementRequest(p.win)
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != "render" {
		return req
	}
	if _, err := p.session.post(p.win, req.Page, req.Scale,
		placement.Saved{Page: req.Page, X: req.X, Y: req.Y}); err != nil {
		t.Fatal(err)
	}
	// Fit is recomputed for each page's own shape, which can produce a
	// second request straight away; answer it too so the window settles.
	select {
	case <-p.messages:
		follow, err := readPlacementRequest(p.win)
		if err != nil {
			t.Fatal(err)
		}
		if follow.Action == "render" {
			if _, err := p.session.post(p.win, follow.Page, follow.Scale,
				placement.Saved{Page: follow.Page, X: follow.X, Y: follow.Y}); err != nil {
				t.Fatal(err)
			}
		}
		return follow
	case <-time.After(400 * time.Millisecond):
	}
	return req
}

func placeEval(t *testing.T, p *placeUI, script string) string {
	t.Helper()
	raw, err := p.win.Eval(script)
	if err != nil {
		t.Fatalf("Eval(%q): %v", script, err)
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return s
	}
	return raw
}

func readPlaceState(t *testing.T, p *placeUI) placementRequest {
	t.Helper()
	var req placementRequest
	if err := json.Unmarshal([]byte(placeEval(t, p, "window.__liroPlacementRequest()")), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

// --- what the window shows -------------------------------------------

// TestPlacementWindowShowsThePageAndTheStamp is the first thing a
// person sees: the page drawn, the stamp on it, the margin guide, and
// every control the phase asks for.
func TestPlacementWindowShowsThePageAndTheStamp(t *testing.T) {
	p := sharedPlacementWindow(t)
	waitForImages(t, p)

	if got := placeEval(t, p, `document.getElementById("page-image").naturalWidth > 0 ? "yes" : "no"`); got != "yes" {
		t.Error("the page image did not load: the window is showing an empty frame")
	}
	if got := placeEval(t, p, `document.getElementById("stamp-image").naturalWidth > 0 ? "yes" : "no"`); got != "yes" {
		t.Error("the stamp image did not load: the person is placing an empty box")
	}
	if got := placeEval(t, p, `getComputedStyle(document.getElementById("margin-guide")).display`); got == "none" {
		t.Error("the margin guide is not shown, so the limit is invisible")
	}
	for _, id := range []string{"first-btn", "prev-btn", "next-btn", "last-btn", "page-input",
		"zoom-in-btn", "zoom-out-btn", "zoom-btn", "cancel-btn", "use-btn"} {
		script := fmt.Sprintf(`(function(){var e=document.getElementById(%q);
			if (!e) return "missing";
			var r = e.getBoundingClientRect();
			return (r.width > 0 && r.height > 0) ? "shown" : "hidden";})()`, id)
		if got := placeEval(t, p, script); got != "shown" {
			t.Errorf("%s is %s", id, got)
		}
	}
	if got := placeEval(t, p, `document.getElementById("page-of").textContent`); !strings.Contains(got, "6") {
		t.Errorf("the page count line reads %q, want it to say six pages", got)
	}
	if got := placeEval(t, p, `getComputedStyle(document.getElementById("stamp")).cursor`); got != "move" {
		t.Errorf("the stamp's cursor is %q, want move", got)
	}
}

// TestPlacementWindowDoesNotScroll: the page itself is fixed and only
// the viewport scrolls, so the buttons cannot be pushed off the bottom
// (D-106's rule, applied to the one window with a picture in it).
func TestPlacementWindowDoesNotScroll(t *testing.T) {
	p := sharedPlacementWindow(t)
	if got := placeEval(t, p, `document.body.scrollHeight > document.body.clientHeight ? "scrolls" : "fixed"`); got != "fixed" {
		t.Error("the window's own page scrolls, which can push the actions out of reach")
	}
	if got := placeEval(t, p, `document.body.scrollWidth > document.body.clientWidth ? "scrolls" : "fixed"`); got != "fixed" {
		t.Error("the window scrolls sideways")
	}
}

// --- dragging ---------------------------------------------------------

// dragStamp moves the stamp by a number of *screen pixels*, through the
// same pointer events a real drag produces.
func dragStamp(t *testing.T, p *placeUI, dxPx, dyPx float64) {
	t.Helper()
	script := fmt.Sprintf(`(function(){
		var s = document.getElementById("stamp");
		var r = s.getBoundingClientRect();
		var x = r.left + r.width / 2, y = r.top + r.height / 2;
		function send(type, cx, cy) {
			s.dispatchEvent(new PointerEvent(type, {
				clientX: cx, clientY: cy, bubbles: true, pointerId: 1, isPrimary: true
			}));
		}
		send("pointerdown", x, y);
		send("pointermove", x + %g, y + %g);
		send("pointerup", x + %g, y + %g);
		return "ok";
	})()`, dxPx, dyPx, dxPx, dyPx)
	if got := placeEval(t, p, script); got != "ok" {
		t.Fatalf("the drag did not run: %q", got)
	}
}

// TestDraggingMovesTheStampByTheRightNumberOfPoints: a drag of N screen
// pixels at scale S moves the stamp N/S points, and in the direction it
// was dragged.
func TestDraggingMovesTheStampByTheRightNumberOfPoints(t *testing.T) {
	p := sharedPlacementWindow(t)
	// Start from a known page and a known zoom.
	goToPageAtScale(t, p, 1, 1)

	before := readPlaceState(t, p)
	const dx, dy = -60.0, -80.0 // left and up on screen
	dragStamp(t, p, dx, dy)
	after := readPlaceState(t, p)

	// Page 1 is unrotated, so screen-left is -x and screen-up is +y.
	if got, want := after.X-before.X, dx; !nearly(got, want, 1.5) {
		t.Errorf("x moved %g points, want %g", got, want)
	}
	if got, want := after.Y-before.Y, -dy; !nearly(got, want, 1.5) {
		t.Errorf("y moved %g points, want %g", got, want)
	}
	if got := placeEval(t, p, `document.getElementById("position-readout").textContent`); !strings.Contains(got, "x:") {
		t.Errorf("the position readout says %q, want the coordinates in points", got)
	}
}

// TestDraggingStopsAtTheMargin: F6b §2.5 — the stamp stops, it does not
// bounce and it does not disappear under the edge.
func TestDraggingStopsAtTheMargin(t *testing.T) {
	p := sharedPlacementWindow(t)
	goToPageAtScale(t, p, 1, 1)

	dragStamp(t, p, -5000, 5000) // hard into the bottom-left corner
	got := readPlaceState(t, p)
	info, err := p.session.pageInfo(got.Page)
	if err != nil {
		t.Fatal(err)
	}
	minX, minY, _, _ := placement.Bounds(info.Box, info.Rotate, p.session.stampW, p.session.stampH)
	if !nearly(got.X, minX, 0.01) || !nearly(got.Y, minY, 0.01) {
		t.Errorf("dragged to (%g, %g), want the margin corner (%g, %g)", got.X, got.Y, minX, minY)
	}
}

// TestDraggingNearACornerSnapsToIt: F6b §2.4 — within about ten points
// a corner takes the stamp, visibly.
func TestDraggingNearACornerSnapsToIt(t *testing.T) {
	p := sharedPlacementWindow(t)
	goToPageAtScale(t, p, 1, 1)

	info, err := p.session.pageInfo(1)
	if err != nil {
		t.Fatal(err)
	}
	corners := placement.Corners(info.Box, info.Rotate, p.session.stampW, p.session.stampH)
	target := corners[3] // top-left, which is not where the stamp starts

	// Put the stamp six points away from that corner, then let go.
	setStampPosition(t, p, target.X+6, target.Y-6)
	got := readPlaceState(t, p)
	if !nearly(got.X, target.X, 0.01) || !nearly(got.Y, target.Y, 0.01) {
		t.Errorf("a position six points from %s did not snap: (%g, %g) want (%g, %g)",
			target.Name, got.X, got.Y, target.X, target.Y)
	}
	if snap := placeEval(t, p, `document.getElementById("snap-readout").textContent`); snap == "" {
		t.Error("the snap happened silently: F6b §2.4 asks for it to be visible")
	}
	if cls := placeEval(t, p, `document.getElementById("stamp").className`); !strings.Contains(cls, "is-snapped") {
		t.Errorf("the stamp does not show that it snapped (class %q)", cls)
	}

	// Twenty points away, it does not.
	setStampPosition(t, p, target.X+20, target.Y-20)
	far := readPlaceState(t, p)
	if nearly(far.X, target.X, 0.01) && nearly(far.Y, target.Y, 0.01) {
		t.Error("a position twenty points from a corner snapped to it anyway")
	}
}

// setStampPosition puts the stamp at a chosen position in points, by
// working out how far that is from where it is now and dragging it
// there — the same pointer events a hand would produce, with the
// arithmetic on this side rather than in a hook the product would
// otherwise have to carry for the tests' benefit.
func setStampPosition(t *testing.T, p *placeUI, x, y float64) {
	t.Helper()
	cur := readPlaceState(t, p)
	info, err := p.session.pageInfo(cur.Page)
	if err != nil {
		t.Fatal(err)
	}
	v := placement.View{Box: info.Box, Rotate: info.Rotate, Scale: currentScale(t, p)}
	from := v.DisplayRect(cur.X, cur.Y, p.session.stampW, p.session.stampH)
	to := v.DisplayRect(x, y, p.session.stampW, p.session.stampH)
	dragStamp(t, p, to.Left-from.Left, to.Top-from.Top)
}

// currentScale is the zoom the window is actually at, worked out from
// the size of the canvas it drew and the page it drew.
func currentScale(t *testing.T, p *placeUI) float64 {
	t.Helper()
	raw := placeEval(t, p, `String(document.getElementById("canvas").offsetWidth)`)
	width, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		t.Fatalf("could not read the canvas width (%q): %v", raw, err)
	}
	cur := readPlaceState(t, p)
	info, err := p.session.pageInfo(cur.Page)
	if err != nil {
		t.Fatal(err)
	}
	w := info.Box[2] - info.Box[0]
	if info.Rotate == 90 || info.Rotate == 270 {
		w = info.Box[3] - info.Box[1]
	}
	return width / w
}

// --- arrow keys -------------------------------------------------------

// TestArrowKeysNudgeByExactlyOnePointAtAnyZoom is F6b §2.4's precision
// requirement, checked at two zooms so that the step is proven to be in
// points rather than in pixels.
func TestArrowKeysNudgeByExactlyOnePointAtAnyZoom(t *testing.T) {
	p := sharedPlacementWindow(t)
	for _, scale := range []float64{1, 3} {
		goToPageAtScale(t, p, 1, scale)
		// Away from the edges, so nothing is clamped.
		setStampPosition(t, p, 200, 300)

		before := readPlaceState(t, p)
		pressArrow(t, p, "ArrowRight", false)
		one := readPlaceState(t, p)
		if got := one.X - before.X; !nearly(got, 1, 1e-6) {
			t.Errorf("scale %g: an arrow key moved x by %g points, want 1", scale, got)
		}
		pressArrow(t, p, "ArrowUp", true)
		ten := readPlaceState(t, p)
		if got := ten.Y - one.Y; !nearly(got, 10, 1e-6) {
			t.Errorf("scale %g: a shifted arrow moved y by %g points, want 10", scale, got)
		}
	}
}

func pressArrow(t *testing.T, p *placeUI, key string, shift bool) {
	t.Helper()
	script := fmt.Sprintf(`(function(){
		document.dispatchEvent(new KeyboardEvent("keydown", {key: %q, shiftKey: %v, bubbles: true}));
		return "ok";
	})()`, key, shift)
	if got := placeEval(t, p, script); got != "ok" {
		t.Fatalf("the key press did not run: %q", got)
	}
}

// --- zoom -------------------------------------------------------------

// goToPageAtScale drives the window to one page at one zoom, through
// the controls a person uses: the page number box and the zoom buttons.
func goToPageAtScale(t *testing.T, p *placeUI, page int, scale float64) {
	t.Helper()
	if cur := readPlaceState(t, p); cur.Page != page {
		script := fmt.Sprintf(`(function(){
			var i = document.getElementById("page-input");
			i.value = "%d";
			i.dispatchEvent(new Event("change", {bubbles: true}));
			return "ok";
		})()`, page)
		if got := placeEval(t, p, script); got != "ok" {
			t.Fatalf("could not move to page %d: %q", page, got)
		}
		answerOneRequest(t, p)
	}

	// The zoom buttons walk the steps; clicking towards the target
	// until the window is there is what a person does too.
	for i := 0; i < 20; i++ {
		got := currentScale(t, p)
		if nearly(got, scale, 0.005) {
			return
		}
		button := "zoom-in-btn"
		if got > scale {
			button = "zoom-out-btn"
		}
		if out := placeEval(t, p, fmt.Sprintf(`document.getElementById(%q).click(); "ok"`, button)); out != "ok" {
			t.Fatalf("the zoom button did not respond: %q", out)
		}
		answerOneRequest(t, p)
	}
	t.Fatalf("could not reach zoom %g (stopped at %g)", scale, currentScale(t, p))
}

// TestZoomNeverMovesTheStampInTheWindow is the window's own version of
// the arithmetic test in internal/placement: after zooming through
// every step and back, through the real page's real DOM, the position
// in points is the one it started at.
func TestZoomNeverMovesTheStampInTheWindow(t *testing.T) {
	p := sharedPlacementWindow(t)
	goToPageAtScale(t, p, 1, 1)
	setStampPosition(t, p, 220, 310)
	before := readPlaceState(t, p)

	for _, s := range []float64{0.5, 0.75, 1.25, 2, 4, 1} {
		goToPageAtScale(t, p, 1, s)
	}
	after := readPlaceState(t, p)
	if !nearly(after.X, before.X, 1e-6) || !nearly(after.Y, before.Y, 1e-6) {
		t.Errorf("zooming moved the stamp from (%g, %g) to (%g, %g)",
			before.X, before.Y, after.X, after.Y)
	}
}

// TestZoomChangesTheImageAndNotTheWindow is F6b §2.3 stated as a test:
// the picture grows, the window does not.
func TestZoomChangesTheImageAndNotTheWindow(t *testing.T) {
	p := sharedPlacementWindow(t)
	goToPageAtScale(t, p, 1, 1)
	w1 := placeEval(t, p, `String(document.getElementById("canvas").offsetWidth)`)
	bodyBefore := placeEval(t, p, `String(document.body.clientWidth) + "x" + String(document.body.clientHeight)`)

	goToPageAtScale(t, p, 1, 2)
	w2 := placeEval(t, p, `String(document.getElementById("canvas").offsetWidth)`)
	bodyAfter := placeEval(t, p, `String(document.body.clientWidth) + "x" + String(document.body.clientHeight)`)

	if w1 == w2 {
		t.Errorf("the page image is the same width at 100%% and 200%% (%s)", w1)
	}
	if bodyBefore != bodyAfter {
		t.Errorf("the window changed size when the zoom did: %s then %s", bodyBefore, bodyAfter)
	}
}

// --- lazy rendering ---------------------------------------------------

// TestTwoHundredPagesDoesNotRenderTwoHundredPages is F6b §2.1 and §6:
// the page on screen and at most one either side.
func TestTwoHundredPagesDoesNotRenderTwoHundredPages(t *testing.T) {
	path := placementTestPDF(t, 200)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := render.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()
	s := &placeSession{doc: doc, scratch: scratch, stampW: 190, stampH: 48,
		notes: map[string]int{}, rendered: map[string]string{}}

	started := time.Now()
	if _, err := s.render(100, 1); err != nil {
		t.Fatal(err)
	}
	one := time.Since(started)
	s.prefetch(100, 1)
	all := time.Since(started)

	names, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Errorf("a 200-page document produced %d page images, want 3 (the page and one either side)", len(names))
	}
	t.Logf("200-page document: %d of 200 pages rendered; page 100 in %v, with both neighbours %v",
		len(names), one.Round(time.Millisecond), all.Round(time.Millisecond))
}

// TestChangingTheZoomDiscardsTheCachedImages: F6b §6 — an image at the
// wrong zoom is not worth the disk it sits on.
func TestChangingTheZoomDiscardsTheCachedImages(t *testing.T) {
	path := placementTestPDF(t, 8)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := render.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()
	s := &placeSession{doc: doc, scratch: scratch, stampW: 190, stampH: 48,
		notes: map[string]int{}, rendered: map[string]string{}}

	if _, err := s.render(2, 1); err != nil {
		t.Fatal(err)
	}
	s.prefetch(2, 1)
	if n := countFiles(t, scratch); n != 3 {
		t.Fatalf("%d images at the first zoom, want 3", n)
	}

	if _, err := s.render(2, 2); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(t, scratch); n != 1 {
		t.Errorf("%d images after the zoom changed, want 1: the old ones are the wrong size", n)
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// TestTheSamePageAtTheSameZoomIsDrawnOnce: paging back and forth must
// not redraw what is already on disk.
func TestTheSamePageAtTheSameZoomIsDrawnOnce(t *testing.T) {
	path := placementTestPDF(t, 4)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := render.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()
	s := &placeSession{doc: doc, scratch: scratch, stampW: 190, stampH: 48,
		notes: map[string]int{}, rendered: map[string]string{}}

	first, err := s.render(2, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(scratch, first))
	if err != nil {
		t.Fatal(err)
	}
	firstTime := info.ModTime()

	time.Sleep(20 * time.Millisecond)
	again, err := s.render(2, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("the second render produced %q, want the cached %q", again, first)
	}
	info2, err := os.Stat(filepath.Join(scratch, again))
	if err != nil {
		t.Fatal(err)
	}
	if !info2.ModTime().Equal(firstTime) {
		t.Error("the page was drawn a second time instead of being reused")
	}
}

// --- the answer reaches the document ----------------------------------

// TestAPlacedPositionBecomesExplicitCoordinates is this side of what
// the whole phase is for: the position chosen in the window reaches the
// signing engine as the coordinates it was chosen at, on the page it
// was chosen on, with nothing rounded or renamed on the way.
//
// The other side — those coordinates being the signature's own
// rectangle in the finished document — is
// TestPlacedCoordinatesBecomeTheAppearanceRectangle in internal/pades,
// where the engine itself can be called.
func TestAPlacedPositionBecomesExplicitCoordinates(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 7
	cfg.StampX, cfg.StampY = 137.5, 421.25

	opts := stampOptionsFor(c, cfg)
	if opts == nil || !opts.UseXY {
		t.Fatalf("a placed position did not become explicit coordinates: %+v", opts)
	}
	if opts.X != 137.5 || opts.Y != 421.25 {
		t.Errorf("the coordinates changed on the way through: (%g, %g)", opts.X, opts.Y)
	}
	if opts.Page != 7 {
		t.Errorf("the page changed on the way through: %d", opts.Page)
	}

	// And a corner is still a corner: the placed position replaces one
	// only when a position has actually been placed.
	cfg.StampPosition = "top-left"
	corner := stampOptionsFor(c, cfg)
	if corner == nil || corner.UseXY {
		t.Errorf("choosing a corner produced explicit coordinates: %+v", corner)
	}
	if corner.Corner != appearance.TopLeft {
		t.Errorf("the corner is %v, want top-left", corner.Corner)
	}
}

func nearly(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}
