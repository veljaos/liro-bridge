//go:build windows

package main

// The user guide's screenshots, and the only way they are made.
//
//	LIRO_GUIDE_SHOTS=1 go test ./cmd/liro-bridge/ -run TestCaptureGuideScreens -v
//
// Behind an environment variable, like the window-cycle test is behind
// LIRO_WINDOW_CYCLES (D-169): eight window captures do not belong in
// every `go test ./...`, and regenerating a committed asset is not
// something an ordinary test run should do as a side effect.
//
// It is committed rather than created and deleted in the session that
// needed it, which is the opposite of what this project does with
// verification scaffolding (D-100) and deliberately so. These are not
// scaffolding: they are committed artefacts that ship inside the guide,
// and this project has twice paid for a generated artefact nobody could
// regenerate — the font subset whose source font had to be hunted for
// afterwards (D-128), and the stylesheet whose generator would have
// silently deleted five screens' worth of CSS (D-183). A screenshot set
// that cannot be remade is a guide that goes stale the first time a
// window changes, and stays stale because nobody knows how it was made.
//
// It photographs each window with PrintWindow and PW_RENDERFULLCONTENT,
// so the capture is that window's own pixels wherever it sits and
// whatever is on top of it — and, more to the point, taking it does not
// take the foreground away from whoever is using the machine (D-122).
// Nothing here simulates a mouse or a keystroke (D-094); every screen is
// reached by posting the payload the program itself posts.

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// Relative to this package's own directory, which is where `go test`
// runs it. An absolute path would work on exactly one machine.
const shotDir = "../../docs/guide/slike"

var (
	modUser32 = windows.NewLazySystemDLL("user32.dll")
	modGdi32  = windows.NewLazySystemDLL("gdi32.dll")

	procGetWindowRect          = modUser32.NewProc("GetWindowRect")
	procGetWindowDC            = modUser32.NewProc("GetWindowDC")
	procReleaseDC              = modUser32.NewProc("ReleaseDC")
	procPrintWindow            = modUser32.NewProc("PrintWindow")
	procCreateCompatibleDC     = modGdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = modGdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = modGdi32.NewProc("SelectObject")
	procDeleteObject           = modGdi32.NewProc("DeleteObject")
	procDeleteDC               = modGdi32.NewProc("DeleteDC")
	procGetDIBits              = modGdi32.NewProc("GetDIBits")
)

type rect struct{ Left, Top, Right, Bottom int32 }

// capture returns the window's own pixels, and how many of them are not
// the background — which is how this harness tells a painted window from
// an unpainted one.
func capture(t *testing.T, win ui.Window, name string) (*image.NRGBA, int) {
	t.Helper()
	hwnd := win.Handle()
	if hwnd == 0 {
		t.Fatalf("%s: the window has no handle", name)
	}

	// GetWindowRect, not GetClientRect. PrintWindow draws the whole
	// window into the DC with the frame's own top-left as the origin, so
	// a bitmap sized to the client area loses exactly the height of the
	// title bar off the bottom — measured: the first run of this harness
	// clipped Cancel and Approve off the certificate step.
	var r rect
	if ret, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		t.Fatalf("%s: GetWindowRect: %v", name, err)
	}
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		t.Fatalf("%s: the window measures %dx%d", name, w, h)
	}

	screenDC, _, _ := procGetWindowDC.Call(hwnd)
	if screenDC == 0 {
		t.Fatalf("%s: GetWindowDC returned 0", name)
	}
	defer func() { _, _, _ = procReleaseDC.Call(hwnd, screenDC) }()

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		t.Fatalf("%s: CreateCompatibleDC returned 0", name)
	}
	defer func() { _, _, _ = procDeleteDC.Call(memDC) }()

	bmp, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(w), uintptr(h))
	if bmp == 0 {
		t.Fatalf("%s: CreateCompatibleBitmap returned 0", name)
	}
	defer func() { _, _, _ = procDeleteObject.Call(bmp) }()

	old, _, _ := procSelectObject.Call(memDC, bmp)
	defer func() { _, _, _ = procSelectObject.Call(memDC, old) }()

	// PW_RENDERFULLCONTENT (0x2) is what makes this work for a window
	// whose content is drawn by another process's compositor, which is
	// exactly what a WebView2 control is.
	const pwRenderFullContent = 0x00000002
	if ret, _, err := procPrintWindow.Call(hwnd, memDC, pwRenderFullContent); ret == 0 {
		t.Fatalf("%s: PrintWindow: %v", name, err)
	}

	// A top-down 32-bit BI_RGB DIB: negative height means row 0 is the
	// top one, which saves flipping it afterwards.
	var bi struct {
		Size                                                uint32
		Width, Height                                       int32
		Planes, BitCount                                    uint16
		Compression, SizeImage                              uint32
		XPelsPerMeter, YPelsPerMeter, ClrUsed, ClrImportant uint32
	}
	bi.Size = uint32(unsafe.Sizeof(bi))
	bi.Width, bi.Height = int32(w), int32(-h)
	bi.Planes, bi.BitCount = 1, 32
	bi.Compression = 0 // BI_RGB

	buf := make([]byte, w*h*4)
	const dibRGBColors = 0
	if ret, _, err := procGetDIBits.Call(memDC, bmp, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors); ret == 0 {
		t.Fatalf("%s: GetDIBits: %v", name, err)
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	ink := 0
	// Two things in this bitmap are there whether or not the page has
	// rendered, and both were counted as content on the previous run:
	// the title bar, which Windows paints, and the black frame down the
	// left, right and bottom, which is the part of the bitmap
	// PrintWindow leaves untouched. Counting them made a blank window
	// score 15 488 and a painted one 25 871 — a difference, but not one
	// any threshold could be set on honestly.
	//
	// So the count is taken from an inset rectangle below the title bar,
	// which is nothing but the page.
	const titleBar, inset = 40, 16
	for y := titleBar; y < h-inset; y++ {
		for x := inset; x < w-inset; x++ {
			i := y*w + x
			if buf[i*4] < 0xf0 || buf[i*4+1] < 0xf0 || buf[i*4+2] < 0xf0 {
				ink++
			}
		}
	}
	for i := 0; i < w*h; i++ {
		b, g, r8 := buf[i*4], buf[i*4+1], buf[i*4+2]
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r8, g, b, 0xff
	}
	return img, ink
}

// shoot captures until the window has actually painted, then writes the
// PNG.
//
// The property is "the capture contains the window's content", and it is
// checked on the capture itself rather than inferred from the DOM.
// Measured, on the first two runs of this harness: `document.body
// .innerText.trim().length > 0` was true while PrintWindow still
// returned a white rectangle with a title bar on it. The DOM being ready
// and the compositor having painted are two different moments, and only
// one of them is the one a photograph needs. The screens that came out
// right on those runs were the ones that happened to have waited for
// something else first.
// lastShot is what each window's pixels looked like the last time it
// was photographed, so the next photograph of it can wait for them to
// change.
var lastShot = map[uintptr]uint64{}

func pixelHash(img *image.NRGBA) uint64 {
	var h uint64 = 14695981039346656037
	for _, b := range img.Pix {
		h = (h ^ uint64(b)) * 1099511628211
	}
	return h
}

// bottomBrandBlue counts the brand-blue pixels in the bottom quarter of
// a capture, which is where every screen in this program puts its
// primary action.
//
// It exists because "the pixels changed since the last shot" is not
// enough on a screen that animates. The queue screen's progress bar is
// indeterminate, so it moves on its own: 06-izvestaj was captured as the
// queue screen twice, differing from 05-napredak by the handful of blue
// pixels the bar had advanced, which satisfied a change test exactly as
// well as a whole new screen would have. The report screen has a blue
// primary action where the queue screen has only a neutral Stop, so this
// distinguishes the two by something the screens actually differ in.
func bottomBrandBlue(img *image.NRGBA) int {
	b := img.Bounds()
	n := 0
	for y := b.Dy() * 3 / 4; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			i := (y*b.Dx() + x) * 4
			r, g, bl := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
			if r < 0x50 && g > 0x60 && g < 0xa0 && bl > 0xb0 {
				n++
			}
		}
	}
	return n
}

func shoot(t *testing.T, win ui.Window, name string) {
	t.Helper()
	shootWhen(t, win, name, nil)
}

// shootWhen is shoot with an extra condition on the captured pixels.
func shootWhen(t *testing.T, win ui.Window, name string, want func(*image.NRGBA) bool) {
	t.Helper()
	hwnd := win.Handle()
	deadline := time.Now().Add(30 * time.Second)
	var img *image.NRGBA
	var ink int
	for {
		img, ink = capture(t, win, name)
		h := pixelHash(img)
		// Two conditions, because two different things go wrong. The
		// window must have painted at all — a fresh one hands back a
		// white rectangle — and, when this window has been photographed
		// before, the pixels must have changed since: PrintWindow returns
		// the last frame the compositor produced, and the DOM reports a
		// new screen well before that frame exists. Measured twice on the
		// way to this: 02-dokumenti came out byte-identical to
		// 01-dokumenti-prazno, and then, once a DOM wait had been added,
		// byte-identical to 05-napredak instead.
		//
		// A painted screen in this program is never nearly empty: the
		// emptiest one it has is the documents step, which still carries
		// a heading, a hint, a footer and two buttons.
		prev, seen := lastShot[hwnd]
		if ink > 2000 && (!seen || h != prev) && (want == nil || want(img)) {
			lastShot[hwnd] = h
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: 30s of captures and the window still shows the previous screen (ink %d)", name, ink)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := os.MkdirAll(shotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(shotDir, name+".png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	fi, _ := f.Stat()
	b := img.Bounds()
	t.Logf("SHOT %-32s %4dx%-4d %7d bytes  ink=%d", name+".png", b.Dx(), b.Dy(), fi.Size(), ink)
}

// waitFor polls a numeric expression in the page until it reaches want.
//
// The shots on one shared window follow each other, and PrintWindow
// hands back the last frame the compositor produced — so a capture taken
// before the new payload has been laid out returns the *previous*
// screen, fully painted and entirely wrong. Measured: 02-dokumenti came
// out byte-for-byte identical to 01-dokumenti-prazno, three documents
// short, and passed the blank check because the stale frame it copied
// was not blank. A blankness guard cannot see staleness; only waiting
// for the content that is supposed to be there can.
func waitFor(t *testing.T, win ui.Window, name, expr string, want int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var got float64
	for time.Now().Before(deadline) {
		got = evalNumber(t, win, expr)
		if int(got) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s: %s is %v, want %d", name, expr, got, want)
}

func TestCaptureGuideScreens(t *testing.T) {
	if os.Getenv("LIRO_GUIDE_SHOTS") == "" {
		t.Skip("set LIRO_GUIDE_SHOTS=1 to regenerate docs/guide/slike")
	}
	tempConfigHome(t)
	const locale = "sr-Latn"
	c := i18n.Load(locale)
	dir := t.TempDir()

	// ---- the documents step, empty and then with documents ----------
	m, _ := testMainWindow(t, locale, config.Default(), nil)
	shoot(t, m.win, "01-dokumenti-prazno")

	// Realistic sizes: a guide whose document list reads "2 B" is a guide
	// showing a program that looks broken.
	paths := []string{
		writeTestPDF(t, dir, "Faktura 2026-114.pdf", 86_412),
		writeTestPDF(t, dir, "Ugovor o delu.pdf", 214_908),
		writeTestPDF(t, dir, "Izjava.pdf", 41_733),
	}
	m2, _ := testMainWindow(t, locale, config.Default(), paths)
	waitFor(t, m2.win, "02-dokumenti", "document.querySelectorAll('#file-list .file-row').length", 3)
	shootWhen(t, m2.win, "02-dokumenti", func(img *image.NRGBA) bool { return bottomBrandBlue(img) > 500 })

	// ---- the certificate step ---------------------------------------
	usable := stampTestCertificate()
	// The suite's own fixture is "Test Testić" of "Test CA", which in a
	// user guide reads as a program showing test data. These are the
	// shapes a real Serbian certificate has — the display name built
	// from givenName + surname, never from CN (SPEC §11.7) — with a
	// fabricated person in them.
	usable.Subject.DisplayName = "Ana Petrović"
	usable.IssuerCN = "MUP Gradjani CA 4"
	usable.Thumbprint = "9C41B7E0D3A25F881164ACD07E5B3F2A"
	flow, _ := openSigningFlowForTest(t, locale, func(ctx context.Context) (cli.Report, error) {
		return cli.Report{
			Readers:      []platform.ReaderState{{Name: "Generic Smart Card Reader", CardPresent: true}},
			Certificates: []cli.CertRow{{OnHardware: true, Info: usable}},
		}, nil
	})
	waitForCertRows(t, flow, 1)
	shoot(t, flow.win, "03-sertifikat")

	// ---- the signing method step ------------------------------------
	// The shared window is created at the Settings role's height, and
	// the step's content in it leaves half the window empty — which is
	// what the previous capture showed. In the signing flow this step
	// gets its own size (D-148's table), so the photograph is taken at
	// that size.
	stampWin, _ := sharedStampWindow(t, c, config.Default(), stampRoleStep)
	resizeAndSettle(t, stampWin, stepMethodWidth, stepMethodHeight)
	defer resizeAndSettle(t, stampWin, stampWindowWidth, stampSettingsHeight)
	shoot(t, stampWin, "04-metod")

	// ---- the progress screen and the report -------------------------
	m3, _ := testMainWindow(t, locale, config.Default(), paths)
	m3.postPreparingCard()
	waitFor(t, m3.win, "05-napredak", "document.getElementById('state-queue').hidden ? 0 : 1", 1)
	shoot(t, m3.win, "05-napredak")

	// A folder a person recognises. The real one here is a temporary
	// directory with a random number in its name, which is what the
	// suite needs and not what a guide should show.
	m3.postReport(jobs.Report{Succeeded: 3, OutputDir: `C:\Users\Ana\Dokumenti\Fakture`, AchievedLevel: "B-LT"})
	waitFor(t, m3.win, "06-izvestaj", "document.getElementById('state-report').hidden ? 0 : 1", 1)
	// The report screen is the one with a blue primary action on it.
	shootWhen(t, m3.win, "06-izvestaj", func(img *image.NRGBA) bool { return bottomBrandBlue(img) > 500 })

	// ---- the audit log ----------------------------------------------
	now := time.Now().Truncate(time.Second)
	entries := []audit.Entry{
		{Sequence: 0, Timestamp: now.Add(-72 * time.Hour), Thumbprint: "3F2A9C0B7E1D4855AA31C7E0B3D1ECCE",
			Application: "local", DocumentCount: 1, Outcome: audit.OutcomeApproved, AchievedLevel: "b-lt"},
		{Sequence: 1, Timestamp: now.Add(-26 * time.Hour), Thumbprint: "3F2A9C0B7E1D4855AA31C7E0B3D1ECCE",
			Application: "local", DocumentCount: 12, Outcome: audit.OutcomeApproved, AchievedLevel: "b-lt"},
		{Sequence: 2, Timestamp: now.Add(-3 * time.Hour), Thumbprint: "3F2A9C0B7E1D4855AA31C7E0B3D1ECCE",
			Application: "local", DocumentCount: 2, Outcome: audit.OutcomeDenied},
		{Sequence: 3, Timestamp: now.Add(-40 * time.Minute), Thumbprint: "3F2A9C0B7E1D4855AA31C7E0B3D1ECCE",
			Application: "local", DocumentCount: 3, Outcome: audit.OutcomeApproved, AchievedLevel: "b-t"},
	}
	auditWin := sharedAuditLogWindow(t)
	if err := auditWin.PostJSON(buildAuditLogInit(c, entries)); err != nil {
		t.Fatalf("PostJSON(audit log): %v", err)
	}
	waitFor(t, auditWin, "07-dnevnik", "document.querySelectorAll('#entry-list > *').length", len(entries))
	shoot(t, auditWin, "07-dnevnik")

	// ---- settings ---------------------------------------------------
	setWin, _ := sharedSettingsWindow(t, c, config.Default())
	shoot(t, setWin, "08-podesavanja")
}
