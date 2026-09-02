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
