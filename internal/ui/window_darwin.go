//go:build darwin && cgo

package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"
)

// The macOS Window (F13, [[D-463]]); darwin_objc.go is the AppKit half.
// The life of one is the Linux host's: a page loaded from assets before
// NewWindow returns, messages and closes delivered on a goroutine of
// their own so a callback can call back into the window, and Close from
// Go never reported as the person closing it.

const pageLoadTimeout = 30 * time.Second

type darwinWindow struct {
	id        uintptr
	hosts     map[string]fs.FS
	startHost string
	handle    uintptr

	onMessage func(Message)
	onFiles   func([]string)
	onClosed  func()

	loadMu   sync.Mutex
	loadWait chan error

	events    chan func()
	closeOnce sync.Once
	closed    chan struct{}
}

var darwinWindows = struct {
	sync.Mutex
	next uintptr
	m    map[uintptr]*darwinWindow
}{m: map[uintptr]*darwinWindow{}}

func darwinWindowByID(id uintptr) *darwinWindow {
	darwinWindows.Lock()
	defer darwinWindows.Unlock()
	return darwinWindows.m[id]
}

func darwinWindowByHandle(h uintptr) *darwinWindow {
	if h == 0 {
		return nil
	}
	darwinWindows.Lock()
	defer darwinWindows.Unlock()
	for _, w := range darwinWindows.m {
		if w.handle == h {
			return w
		}
	}
	return nil
}

func trimLeadingSlash(p string) string {
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	return p
}

func hostURL(host, path string) string {
	return assetScheme + "://" + host + "/" + trimLeadingSlash(path)
}

func iconFilePath() (string, error) { return "", ErrUnsupportedPlatform }

// NewWindow opens a window and waits for its first page.
func NewWindow(opts Options) (Window, error) {
	if opts.Assets == nil || opts.VirtualHost == "" {
		return nil, errors.New("ui: Options.Assets and Options.VirtualHost are both required")
	}
	if err := theUIThread.start(); err != nil {
		return nil, err
	}
	w := &darwinWindow{
		hosts:     map[string]fs.FS{opts.VirtualHost: opts.Assets},
		startHost: opts.VirtualHost,
		onMessage: opts.OnMessage,
		onFiles:   opts.OnFilesDropped,
		onClosed:  opts.OnClosed,
		events:    make(chan func(), 64),
		closed:    make(chan struct{}),
	}
	if opts.ScratchHost != "" && opts.ScratchDir != "" {
		w.hosts[opts.ScratchHost] = os.DirFS(opts.ScratchDir)
	}
	darwinWindows.Lock()
	darwinWindows.next++
	w.id = darwinWindows.next
	darwinWindows.m[w.id] = w
	darwinWindows.Unlock()
	go w.dispatchEvents()

	owner := uintptr(0)
	if o := darwinWindowByHandle(opts.Owner); o != nil {
		owner = o.handle
	}
	loaded := w.expectLoad()
	if err := theUIThread.do(func() {
		w.handle = nativeWindowNew(w.id, opts.Title, opts.Width, opts.Height, opts.AlwaysOnTop, owner,
			opts.OnFilesDropped != nil, hostURL(opts.VirtualHost, opts.StartPage))
	}); err != nil {
		w.markClosed()
		return nil, err
	}
	select {
	case err := <-loaded:
		if err != nil {
			_ = w.Close()
			return nil, err
		}
	case <-time.After(pageLoadTimeout):
		_ = w.Close()
		return nil, fmt.Errorf("ui: %q did not finish loading within %s", opts.StartPage, pageLoadTimeout)
	}
	return w, nil
}

func (w *darwinWindow) expectLoad() chan error {
	ch := make(chan error, 1)
	w.loadMu.Lock()
	w.loadWait = ch
	w.loadMu.Unlock()
	return ch
}

func (w *darwinWindow) loaded(errText string) {
	w.loadMu.Lock()
	ch := w.loadWait
	w.loadWait = nil
	w.loadMu.Unlock()
	if ch == nil {
		return
	}
	if errText != "" {
		ch <- errors.New("ui: loading the page: " + errText)
		return
	}
	ch <- nil
}

func (w *darwinWindow) allowNavigation(url string) bool {
	if _, _, err := resolveAsset(url, w.hosts); err != nil {
		slog.Warn("ui: refused a navigation", "uri", url, "err", err)
		return false
	}
	return true
}

func (w *darwinWindow) asset(url string) ([]byte, string, bool) {
	data, contentType, err := resolveAsset(url, w.hosts)
	if err != nil {
		slog.Warn("ui: refused an asset request", "uri", url, "err", err)
		return nil, "", false
	}
	return data, contentType, true
}

func (w *darwinWindow) dispatchEvents() {
	for f := range w.events {
		f()
	}
}

func (w *darwinWindow) emit(f func()) {
	if f == nil {
		return
	}
	select {
	case <-w.closed:
	case w.events <- f:
	}
}

func (w *darwinWindow) markClosed() {
	w.closeOnce.Do(func() {
		close(w.closed)
		darwinWindows.Lock()
		delete(darwinWindows.m, w.id)
		darwinWindows.Unlock()
		close(w.events)
	})
}

func (w *darwinWindow) isClosed() bool {
	select {
	case <-w.closed:
		return true
	default:
		return false
	}
}

func (w *darwinWindow) PostJSON(v any) error {
	payload, err := marshalMessagePayload(v)
	if err != nil {
		return fmt.Errorf("ui: encoding a message for the page: %w", err)
	}
	_, err = w.Eval(postJSONScript(payload))
	return err
}

func (w *darwinWindow) Eval(script string) (string, error) {
	if w.isClosed() {
		return "", ErrWindowClosed
	}
	return nativeEval(w.id, script)
}

func (w *darwinWindow) Navigate(page string) error {
	if w.isClosed() {
		return ErrWindowClosed
	}
	loaded := w.expectLoad()
	if err := theUIThread.do(func() { nativeWindowLoad(w.id, hostURL(w.startHost, page)) }); err != nil {
		return err
	}
	select {
	case err := <-loaded:
		return err
	case <-time.After(pageLoadTimeout):
		return fmt.Errorf("ui: %q did not finish loading within %s", page, pageLoadTimeout)
	case <-w.closed:
		return ErrWindowClosed
	}
}

func (w *darwinWindow) Resize(width, height int) error {
	if w.isClosed() {
		return ErrWindowClosed
	}
	return theUIThread.do(func() { nativeWindowResize(w.id, width, height) })
}

func (w *darwinWindow) Close() error {
	if w.isClosed() {
		return nil
	}
	err := theUIThread.do(func() { nativeWindowClose(w.id) })
	w.markClosed()
	return err
}

func (w *darwinWindow) Handle() uintptr { return w.handle }
