package windowscng

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// Source implements keysource.Source over the Windows CNG certificate
// store (F2 §2). newConn (conn_windows.go / conn_other.go) supplies the
// real platform connection; a nil conn field falls back to it lazily so
// the zero value, Source{}, is directly usable.
type Source struct {
	conn         ncryptConn
	windowHandle uintptr
}

// NewSource returns a Source backed by the real Windows CNG APIs.
func NewSource() Source { return Source{} }

// Name implements keysource.Source.
func (Source) Name() string { return "windows-cng" }

// List implements keysource.Source by delegating to Enumerate (F1 §3),
// dropping the enumeration-specific fields (Provider, OnHardware,
// KeyContainer) that only internal/trust/classify needs.
func (Source) List(ctx context.Context) ([]keysource.Certificate, error) {
	certs, err := Enumerate(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]keysource.Certificate, 0, len(certs))
	for _, c := range certs {
		out = append(out, keysource.Certificate{Thumbprint: keysource.Thumbprint(c.Thumbprint), DER: c.DER})
	}
	return out, nil
}

// Open implements keysource.Source.
func (s Source) Open(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn := s.conn
	if conn == nil {
		conn = newConn()
	}
	return openSession(conn, thumbprint, s.windowHandle)
}

// Presence reports whether the hardware backing one certificate is
// currently present (Task 2 / SPEC §11.10): unlike a machine-wide "is any
// card in any reader" answer, this is evaluated per certificate, by
// attempting to open its own key — silently, since opening a key never
// prompts for a PIN (F2 §2.3) — so a certificate whose card has been
// removed is reported CARD_NOT_PRESENT on its own, rather than every
// hardware-backed certificate sharing one answer because some other card
// happens to be in some other reader.
func (s Source) Presence(ctx context.Context, thumbprint keysource.Thumbprint) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	conn := s.conn
	if conn == nil {
		conn = newConn()
	}
	return conn.probePresence(string(thumbprint))
}
