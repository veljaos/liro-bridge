//go:build windows

package main

// The visual stamp placement window (F6b).
//
// It shows the document a page at a time, drawn by this project's own
// renderer rather than by WebView2's PDF viewer, with the stamp on top
// of it as a rectangle that can be dragged. F6b §2.1 gives the reason
// for not using the built-in viewer and it is not a matter of taste:
// that viewer owns its own scrolling, zoom and page navigation, and
// there is no way to put a rectangle over it and read where the
// rectangle is in *page* coordinates, which is the entire job here.
//
// What the page never does is decide where anything is. The box, the
// rotation, the bounds a position may take and the four corner
// positions all arrive from Go in points, computed by
// internal/placement — the same package the tests exercise exhaustively
// — and the answer is clamped and snapped again in Go on the way back
// out. The page converts points to pixels to draw, and back to points
// when a drag ends, and that is the whole of its geometry.

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/render"
	"github.com/veljaos/liro-bridge/internal/placement"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The window's size, in DPI-independent points. It is a viewer, so it
// is the largest window this project has; it still has to fit a
// laptop's work area at 100 per cent scaling, which is what 900 x 640
// is chosen against. Zoom changes the page image inside it and never
// the window itself (F6b §2.3).
const (
	placeWindowWidth  = 900
	placeWindowHeight = 640
)

// placeScratchHost is the second virtual host the rendered page images
// are served from. It is separate from the asset host because these
// files are written while the window is open and deleted when it
// closes, which is the opposite of the shared, content-addressed asset
// directory.
const placeScratchHost = "preview.liro.invalid"

// zoomSteps are F6b §2.3's steps, as fractions. Fit is a mode rather
// than a step and is the default.
var zoomSteps = []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4}

// placementRequest is what the page reports it wants, read back through
// Eval after it sends one of the three messages (D-095's pattern: the
// message surface stays at exactly three types, and what the click
// meant is explicit data the page hands over rather than something Go
// infers from which state the window was in).
type placementRequest struct {
	Action string  `json:"action"`
	Page   int     `json:"page"`
	Scale  float64 `json:"scale"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// placeUI is one open placement window: the window itself, the
// document behind it, and the scratch directory its page images live
// in.
//
// Opening it, running it and closing it are three steps rather than one
// function so that a test can open the real window, drive the real page
// through Eval — D-094's carve-out, which touches nothing outside the
// page's own DOM — and read the real answer back. The alternative is a
// test that rebuilds the window from the same parts, which proves the
// parts and not the window.
type placeUI struct {
	win       ui.Window
	messages  chan ui.Message
	session   *placeSession
	c         *i18n.Catalogue
	cfg       config.Config
	scratch   string
	pageCount int
}

// runPlacementWindow opens the placement window on one document and
// blocks until it is answered.
//
// ok is false when the person cancelled or closed the window.
// previewable is false when the document could not be drawn at all —
// an encrypted file, or one this renderer cannot read — which F6b §5
// says is not a document that cannot be *signed*: the caller falls back
// to the corner selector and says so.
func runPlacementWindow(cfg config.Config, locale, docPath string, cert *x509.Certificate, owner uintptr) (saved placement.Saved, ok, previewable bool) {
	p, err := openPlacement(cfg, locale, docPath, cert, owner)
	if err != nil {
		slog.Warn("placement: the document could not be previewed", "error", err, "document", filepath.Base(docPath))
		return placement.Saved{}, false, false
	}
	defer p.close()
	if err := p.start(); err != nil {
		slog.Warn("placement: the first page could not be drawn", "error", err)
		return placement.Saved{}, false, false
	}
	saved, ok = p.run()
	return saved, ok, true
}

// openPlacement reads the document, renders the stamp, and opens the
// window. It draws nothing into it yet.
func openPlacement(cfg config.Config, locale, docPath string, cert *x509.Certificate, owner uintptr) (*placeUI, error) {
	c := i18n.Load(locale)

	data, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("reading the document: %w", err)
	}
	doc, err := render.Open(data)
	if err != nil {
		// F6b §5: a document that cannot be previewed is not a document
		// that cannot be signed. The caller falls back to the corner
		// selector, and says so.
		return nil, fmt.Errorf("previewing the document: %w", err)
	}
	pageCount := doc.PageCount()
	if pageCount < 1 {
		return nil, fmt.Errorf("the document has no pages")
	}

	scratch, err := os.MkdirTemp(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "preview-")
	if err != nil {
		return nil, fmt.Errorf("making a directory for the page images: %w", err)
	}

	stampOpts := stampOptionsFor(c, cfg)
	if stampOpts == nil {
		// The window is about *where* the stamp goes, so it draws one
		// even when the current answer is an invisible signature —
		// otherwise there would be nothing to place.
		stampOpts = &pades.StampOptions{Label: c.T("sign.stamp_label")}
	}
	stampURL, stampW, stampH := writeStampPreview(scratch, cert, stampOpts)

	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("place.title"),
		Width:       placeWindowWidth,
		Height:      placeWindowHeight,
		Owner:       owner,
		AlwaysOnTop: owner == 0,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		ScratchHost: placeScratchHost,
		ScratchDir:  scratch,
		StartPage:   "/pages/place.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		_ = os.RemoveAll(scratch)
		return nil, fmt.Errorf("opening the window: %w", err)
	}

	return &placeUI{
		win:      win,
		messages: messages,
		c:        c,
		cfg:      cfg,
		scratch:  scratch,
		session: &placeSession{
			doc:      doc,
			scratch:  scratch,
			stampW:   stampW,
			stampH:   stampH,
			notes:    map[string]int{},
			rendered: map[string]string{},
			initURL:  stampURL,
		},
		pageCount: pageCount,
	}, nil
}

// close shuts the window and removes the page images. They are pictures
// of the document being signed; they live for as long as the window
// does and not one moment longer.
func (p *placeUI) close() {
	_ = p.win.Close()
	if err := os.RemoveAll(p.scratch); err != nil {
		slog.Warn("placement: could not remove the page image directory", "error", err)
	}
}

// start posts the window's own strings and its first page.
func (p *placeUI) start() error {
	init := buildPlaceInit(p.c, p.cfg, p.pageCount, p.session.initURL, p.session.stampW, p.session.stampH)
	if err := p.win.PostJSON(init); err != nil {
		return fmt.Errorf("posting the window's strings: %w", err)
	}

	// Where to start: the remembered position if there is one, on the
	// page it was chosen on, fitted to this document.
	start := placement.Saved{Page: p.cfg.StampPlacedPage, X: p.cfg.StampX, Y: p.cfg.StampY}
	page, _ := placement.ResolvePage(start.Page, p.pageCount)
	if !start.IsSet() {
		page = 1
	}
	if _, err := p.session.post(p.win, page, 0, start); err != nil {
		return err
	}
	p.session.notePreviewLimits(p.win, p.c)
	return nil
}

// run is the message loop: every page change and zoom change comes back
// through it as a request to draw something, and the one answer that
// ends it is "done".
func (p *placeUI) run() (placement.Saved, bool) {
	for {
		msg := <-p.messages
		switch msg.Type {
		case ui.MessageTypeCancel:
			return placement.Saved{}, false
		case ui.MessageTypeApprove:
			req, err := readPlacementRequest(p.win)
			if err != nil {
				slog.Warn("placement: reading what the window wants failed", "error", err)
				return placement.Saved{}, false
			}
			if req.Action == "done" {
				return p.answer(req)
			}
			next, err := p.session.post(p.win, req.Page, req.Scale,
				placement.Saved{Page: req.Page, X: req.X, Y: req.Y})
			if err != nil {
				slog.Warn("placement: a page could not be drawn", "error", err, "page", req.Page)
				continue
			}
			p.session.prefetch(next.page, next.scale)
		}
	}
}

// answer turns what the page reports into the position that is saved.
// It is clamped again here, in Go, against the page's real box: the
// page does its own clamping so a drag feels right, and this is what
// makes the answer right.
func (p *placeUI) answer(req placementRequest) (placement.Saved, bool) {
	info, err := p.session.pageInfo(req.Page)
	if err != nil {
		slog.Warn("placement: the chosen page could not be measured", "error", err, "page", req.Page)
		return placement.Saved{}, false
	}
	x, y, _ := placement.Clamp(info.Box, info.Rotate, req.X, req.Y, p.session.stampW, p.session.stampH)
	return placement.Saved{Page: req.Page, X: x, Y: y}, true
}

// placeSession holds what one open window needs: the document, the
// scratch directory the images go to, and which images already exist.
type placeSession struct {
	doc     *render.Document
	scratch string
	stampW  float64
	stampH  float64

	notes map[string]int

	// rendered maps "page@scale" to the file name already written.
	// Everything at a different scale is thrown away when the scale
	// changes, which is what F6b §6 asks for: an image at the wrong
	// zoom is not worth the disk it sits on.
	rendered  map[string]string
	lastScale float64

	// initURL is where the stamp's own image was written, held here so
	// starting the window does not have to be told twice.
	initURL string
}

type placeState struct {
	page  int
	scale float64
}

func (s *placeSession) pageInfo(page int) (render.Page, error) {
	return s.doc.PageInfo(page)
}

// post renders one page if it is not already rendered, and tells the
// window everything it needs to draw that page: the image, the box, the
// rotation, the bounds a position may take, and where the four corners
// are — all in points.
func (s *placeSession) post(win ui.Window, page int, scale float64, want placement.Saved) (placeState, error) {
	if page < 1 {
		page = 1
	}
	if n := s.doc.PageCount(); page > n {
		page = n
	}
	info, err := s.doc.PageInfo(page)
	if err != nil {
		return placeState{}, err
	}
	if scale <= 0 {
		scale = 1
	}
	name, err := s.render(page, scale)
	if err != nil {
		return placeState{}, err
	}

	minX, minY, maxX, maxY := placement.Bounds(info.Box, info.Rotate, s.stampW, s.stampH)
	x, y := want.X, want.Y
	if !want.IsSet() {
		// Nothing remembered: the stamp starts where a stamp goes when
		// nobody has said otherwise (SPEC §13.1's own default corner).
		if cs := placement.Corners(info.Box, info.Rotate, s.stampW, s.stampH); len(cs) > 0 {
			x, y = cs[0].X, cs[0].Y
		}
	}
	x, y, _ = placement.Clamp(info.Box, info.Rotate, x, y, s.stampW, s.stampH)

	corners := placement.Corners(info.Box, info.Rotate, s.stampW, s.stampH)
	jsCorners := make([]map[string]any, 0, len(corners))
	for _, c := range corners {
		jsCorners = append(jsCorners, map[string]any{"name": c.Name, "x": c.X, "y": c.Y})
	}

	payload := map[string]any{
		"type":      "page",
		"page":      page,
		"pageCount": s.doc.PageCount(),
		"scale":     scale,
		"box":       []float64{info.Box[0], info.Box[1], info.Box[2], info.Box[3]},
		"rotate":    info.Rotate,
		"image":     "https://" + placeScratchHost + "/" + name,
		"bounds":    map[string]float64{"minX": minX, "minY": minY, "maxX": maxX, "maxY": maxY},
		"corners":   jsCorners,
		"x":         x,
		"y":         y,
	}
	if err := win.PostJSON(payload); err != nil {
		return placeState{}, err
	}
	return placeState{page: page, scale: scale}, nil
}

// render draws one page to a PNG in the scratch directory, unless it is
// already there.
func (s *placeSession) render(page int, scale float64) (string, error) {
	if scale != s.lastScale {
		// A different zoom means every cached image is the wrong size.
		for _, name := range s.rendered {
			_ = os.Remove(filepath.Join(s.scratch, name))
		}
		s.rendered = map[string]string{}
		s.lastScale = scale
	}
	key := fmt.Sprintf("%d@%s", page, strconv.FormatFloat(scale, 'f', 4, 64))
	if name, ok := s.rendered[key]; ok {
		return name, nil
	}

	started := time.Now()
	res, err := s.doc.RenderPage(page, scale)
	if err != nil {
		return "", err
	}
	for k, v := range res.Notes {
		s.notes[k] += v
	}
	name := fmt.Sprintf("p%d-%s.png", page, strconv.FormatFloat(scale, 'f', 4, 64))
	f, err := os.Create(filepath.Join(s.scratch, name))
	if err != nil {
		return "", err
	}
	// The window is waiting for this image, so speed is what matters
	// rather than the last few percent of compression: a page is mostly
	// white, which the fast setting still packs down well.
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, res.Image); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	s.rendered[key] = name
	slog.Debug("placement: page rendered",
		"page", page, "scale", scale,
		"pixels", res.Image.Bounds().Dx()*res.Image.Bounds().Dy(),
		"ms", time.Since(started).Milliseconds())
	return name, nil
}

// prefetch draws the page either side of the one on screen, so turning
// a page is immediate. F6b §2.1: the current page and at most one
// either side — a two-hundred-page document costs three renders, not
// two hundred.
func (s *placeSession) prefetch(page int, scale float64) {
	for _, p := range []int{page - 1, page + 1} {
		if p < 1 || p > s.doc.PageCount() {
			continue
		}
		if _, err := s.render(p, scale); err != nil {
			slog.Debug("placement: could not pre-render a neighbouring page", "page", p, "error", err)
		}
	}
}

// notePreviewLimits tells the person, once, when the renderer could not
// draw part of the page faithfully — a font it had to substitute, an
// image codec it does not read. Saying nothing would leave them
// wondering whether the blank area is the document or the preview.
func (s *placeSession) notePreviewLimits(win ui.Window, c *i18n.Catalogue) {
	if len(s.notes) == 0 {
		return
	}
	for k, v := range s.notes {
		slog.Info("placement: the preview could not draw part of the page exactly", "what", k, "times", v)
	}
	if _, ok := s.notes["operator budget exhausted"]; ok {
		_ = win.PostJSON(map[string]any{"type": "note", "text": c.T("place.preview_partial")})
	}
}

func readPlacementRequest(win ui.Window) (placementRequest, error) {
	raw, err := win.Eval("window.__liroPlacementRequest()")
	if err != nil {
		return placementRequest{}, err
	}
	var inner string
	if err := json.Unmarshal([]byte(raw), &inner); err != nil {
		return placementRequest{}, fmt.Errorf("decoding the request envelope: %w", err)
	}
	var req placementRequest
	if err := json.Unmarshal([]byte(inner), &req); err != nil {
		return placementRequest{}, fmt.Errorf("decoding the request: %w", err)
	}
	return req, nil
}

// writeStampPreview draws the stamp itself and puts it beside the page
// images. It is drawn at four pixels per point — enough that it stays
// sharp at the 400 per cent zoom step, and small enough (190 x 48
// points) that the cost does not matter.
func writeStampPreview(scratch string, cert *x509.Certificate, stamp *pades.StampOptions) (url string, w, h float64) {
	w, h = float64(appearance.StampWidth), 48
	if cert == nil {
		return "", w, h
	}
	preview, err := pades.RenderStampPreview(cert, time.Now(), stamp, 4)
	if err != nil {
		slog.Warn("placement: the stamp could not be drawn for the preview", "error", err)
		return "", w, h
	}
	f, err := os.Create(filepath.Join(scratch, "stamp.png"))
	if err != nil {
		slog.Warn("placement: the stamp image could not be written", "error", err)
		return "", preview.WidthPt, preview.HeightPt
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, preview.Image); err != nil {
		slog.Warn("placement: the stamp image could not be encoded", "error", err)
		return "", preview.WidthPt, preview.HeightPt
	}
	return "https://" + placeScratchHost + "/stamp.png", preview.WidthPt, preview.HeightPt
}

func buildPlaceInit(c *i18n.Catalogue, cfg config.Config, pageCount int, stampURL string, stampW, stampH float64) map[string]any {
	keys := []string{
		"place.title", "place.of", "place.fit", "place.snapped_to",
		"place.corner_bottom_right", "place.corner_bottom_left",
		"place.corner_top_right", "place.corner_top_left",
		"place.preview_partial",
	}
	strs := make(map[string]string, len(keys))
	for _, k := range keys {
		strs[k] = c.T(k)
	}
	return map[string]any{
		"type":         "init",
		"strings":      strs,
		"pageCount":    pageCount,
		"margin":       float64(placement.Margin),
		"snapDistance": float64(placement.SnapDistance),
		"zoomSteps":    zoomSteps,
		"stampImage":   stampURL,
		"stampWidth":   stampW,
		"stampHeight":  stampH,
		"useLabel":     c.T("place.use"),
		"cancelLabel":  c.T("place.cancel"),
		"firstLabel":   c.T("place.first"),
		"prevLabel":    c.T("place.prev"),
		"nextLabel":    c.T("place.next"),
		"lastLabel":    c.T("place.last"),

		// Task 2: the remembered position is shown here, where it means
		// something, rather than under the option that chooses to place
		// a stamp — someone at that point is about to place it, and
		// coordinates from last time are noise at the moment of
		// deciding. Here they are both a reference and somewhere to go
		// back to.
		"saved":           savedPositionPayload(cfg),
		"savedText":       placedSummaryIfSet(c, cfg),
		"savedResetLabel": c.T("place.reset_to_saved"),
	}
}

// savedPositionPayload is the remembered position, or nil when nothing
// has ever been placed.
func savedPositionPayload(cfg config.Config) map[string]any {
	if cfg.StampPlacedPage < 1 {
		return nil
	}
	return map[string]any{"page": cfg.StampPlacedPage, "x": cfg.StampX, "y": cfg.StampY}
}

// placedSummaryIfSet is the line naming what is currently remembered —
// the page and the coordinates, so a position can be read off, noted
// and reproduced — and empty when nothing has been placed, since a
// window that is about to save one has nothing to say about not having
// one and the page hides the line entirely.
//
// This is the only place the remembered position is written out. It was
// also on a step of the signing flow, which named it and then opened
// this window; here it is a reference and somewhere to go back to,
// which is the difference between a fact worth showing and a fact worth
// a screen.
func placedSummaryIfSet(c *i18n.Catalogue, cfg config.Config) string {
	if cfg.StampPlacedPage < 1 {
		return ""
	}
	return fmt.Sprintf(c.T("stampwindow.placed_at"), cfg.StampPlacedPage, cfg.StampX, cfg.StampY)
}
