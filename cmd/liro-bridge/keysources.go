//go:build windows || softtoken

package main

import (
	"context"
	"errors"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
)

// openCardOrSoftToken is the one place this program decides where a
// signing session comes from: the Windows CNG store, and — only in a
// build made with the "softtoken" tag — the soft token behind it.
//
// The soft token is tried only when the CNG store genuinely has no such
// certificate. Any other error (card removed, PIN blocked, reader
// gone) is real, and masking it behind a second attempt against an
// unrelated backend produces a confusing failure about the wrong thing
// (D-033).
//
// hwnd is the window the KSP's PIN dialog should be parented to, or 0
// where there is no window. Without it NCRYPT_WINDOW_HANDLE_PROPERTY
// stays 0 and the OS PIN dialog can appear behind the agent's own
// window, which looks like a frozen program rather than a prompt (F2
// §2.3).
//
// One function rather than the three near-identical copies there used
// to be — one in main.go, one in noconsent_softtoken.go and one in
// interactive_windows.go. They agreed, which is the only reason it was
// not already a defect; three copies of one rule is how they stop
// agreeing (D-108, D-124, D-138).
//
// The build constraint is the set of builds that have a signing path at
// all: Windows, where the window flow signs, and any build made with
// the "softtoken" tag, whose two headless commands are what CI signs
// with on an Ubuntu runner. A plain non-Windows build has neither, and
// a file compiled into it would be a function nothing could reach —
// which golangci-lint's `unused` says out loud in the GOOS=linux view,
// and which //nolint is the wrong answer to (D-111, D-222).
func openCardOrSoftToken(hwnd uintptr) func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
	cngSource := windowscng.NewSource().WithWindowHandle(hwnd)
	softSource := softTokenSource() // nil unless built with the "softtoken" tag

	return func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
		sess, err := cngSource.Open(ctx, thumbprint)
		if err == nil {
			return sess, nil
		}
		var e *errs.Error
		if softSource != nil && errors.As(err, &e) && e.Code == errs.CodeCertNotFound {
			return softSource.Open(ctx, thumbprint)
		}
		return nil, err
	}
}
