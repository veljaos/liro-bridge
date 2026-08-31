package config

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Log output is always English, regardless of interface locale (SPEC
// §9.2): logs are read by developers, not users. Callers must never pass
// a localised (i18n.Catalogue.T) string into a log call.
//
// Never logged: JMBG, email addresses, personal names, file names,
// document contents, PINs, device secrets (SPEC §18.3). Use Redact for
// anything borderline. Certificate identification in logs uses the SHA-1
// thumbprint, never the subject.

const (
	// logMaxBytes is the size at which the active log file is rotated.
	logMaxBytes = 5 * 1024 * 1024
	// logFilesRetained is the total number of log files kept, including
	// the active one (F0 §3.2: "5 MB per file, 3 files retained").
	logFilesRetained = 3
	logFileName      = "bridge.log"
)

// SetupLogging configures and installs the global slog logger: JSON lines
// to a rotating file in logDir, and, when debug is true (the agent sets
// this from LIRO_DEBUG=1), human-readable text to stderr as well.
//
// The returned io.Closer must be closed on shutdown to flush the log file.
func SetupLogging(level string, logDir string, debug bool) (*slog.Logger, io.Closer, error) {
	slogLevel := parseLevel(level)

	rw, err := newRotatingWriter(logDir, logFileName, logMaxBytes, logFilesRetained)
	if err != nil {
		return nil, nil, err
	}

	handlers := []slog.Handler{
		slog.NewJSONHandler(rw, &slog.HandlerOptions{Level: slogLevel}),
	}
	if debug {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel}))
	}

	var handler slog.Handler = handlers[0]
	if len(handlers) > 1 {
		handler = &multiHandler{handlers: handlers}
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, rw, nil
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Redact replaces the middle of s with "…", keeping at most 4 leading and
// 4 trailing characters. Strings of 8 characters or fewer are collapsed
// entirely: 4 leading plus 4 trailing would leave nothing hidden, which
// defeats the point of calling this in the first place.
func Redact(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return "…"
	}
	return string(r[:4]) + "…" + string(r[len(r)-4:])
}

// multiHandler fans a record out to every wrapped handler. The standard
// library has no built-in equivalent; this one is deliberately minimal.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}

// rotatingWriter is an io.WriteCloser that rotates the underlying file by
// size, keeping maxFiles total files (the active one plus backups),
// implemented by hand per F0 §3.2 to avoid a dependency.
type rotatingWriter struct {
	mu       sync.Mutex
	dir      string
	name     string
	maxBytes int64
	maxFiles int
	size     int64
	file     *os.File
}

func newRotatingWriter(dir, name string, maxBytes int64, maxFiles int) (*rotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &rotatingWriter{dir: dir, name: name, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := w.openCurrent(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) path() string { return filepath.Join(w.dir, w.name) }

func (w *rotatingWriter) rotatedPath(i int) string {
	return w.path() + "." + strconv.Itoa(i)
}

func (w *rotatingWriter) openCurrent() error {
	f, err := os.OpenFile(w.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate closes the active file, shifts every backup up by one slot
// (dropping the oldest beyond maxFiles), and opens a fresh active file.
func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}

	backups := w.maxFiles - 1
	_ = os.Remove(w.rotatedPath(backups))
	for i := backups - 1; i >= 1; i-- {
		_ = os.Rename(w.rotatedPath(i), w.rotatedPath(i+1))
	}
	_ = os.Rename(w.path(), w.rotatedPath(1))

	return w.openCurrent()
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
