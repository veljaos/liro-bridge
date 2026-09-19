//go:build windows || softtoken

package main

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11/worker"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/pinscreen"
)

// signerOrigin is where a session came from, carried out of the one place that
// knows so that the audit log can record it (F11 §4 step 4).
//
// It exists because the answer is produced and discarded in the same expression
// otherwise: openCardOrSoftToken decides between three backends and used to
// return only a keysource.Session, which cannot say which of them opened it.
// The alternative was a method on keysource.Session, which four implementations
// and every test fake would have to grow in order to answer a question only the
// chooser has ever had.
type signerOrigin struct {
	// backend is keysource.Source.Name(): "windows-cng", "pkcs11" or
	// "softtoken". Empty when no session was opened, which is what a refusal
	// records.
	backend string

	// module is the PKCS#11 module's path, and is empty for every other
	// backend.
	module string

	// configured is true when that path was the person's own rather than one
	// of the installation paths this project has measured. It decides how much
	// of the path the audit log may keep — see auditModule.
	configured bool
}

// auditModule is what audit.Entry.Module may hold for this origin, and it is
// where SPEC §6.7 is paid.
//
// A module at one of this project's known installation paths is recorded whole:
// those are under Program Files or System32 and carry no personal name by
// construction. A module at a path the person configured is recorded by file
// name only, because a configured path can be anywhere — including under their
// user profile, where it would carry their name, and §6.7 says this log never
// contains one.
//
// The price is real and is written down in audit.Entry.Module rather than left
// to be discovered: somebody investigating a signature years from now, made
// through a hand-configured module, will want to know where it was and will
// find only what it was called.
func (o signerOrigin) auditModule() string {
	if o.module == "" {
		return ""
	}
	if o.configured {
		return filepath.Base(o.module)
	}
	return o.module
}

// openCardOrSoftToken is the one place this program decides where a
// signing session comes from: the Windows CNG store, the issuer's own
// PKCS#11 module, and — only in a build made with the "softtoken" tag —
// the soft token behind them.
//
// CNG first and the module second (D-311), unless the person has asked
// for the reverse in their configuration (config.PreferPKCS11), which
// exists for somebody whose minidriver is broken or absent and is
// documented where it is declared.
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
func openCardOrSoftToken(hwnd uintptr, cfg config.Config) func(context.Context, keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
	// config.SetupLogging calls slog.SetDefault at startup, so the log is
	// reachable here without threading a logger through four call sites and
	// the window model to express "write this down".
	log := slog.Default()
	cngSource := windowscng.NewSource().WithWindowHandle(hwnd)
	softSource := softTokenSource() // nil unless built with the "softtoken" tag

	cng := func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
		sess, err := cngSource.Open(ctx, thumbprint)
		if err != nil {
			return nil, signerOrigin{}, err
		}
		return sess, signerOrigin{backend: cngSource.Name()}, nil
	}
	p11 := func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
		return openThroughPKCS11(ctx, thumbprint, hwnd, cfg, log)
	}
	var soft opener // nil unless the tag added one, which openInOrder checks
	if softSource != nil {
		soft = func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
			sess, err := softSource.Open(ctx, thumbprint)
			if err != nil {
				return nil, signerOrigin{}, err
			}
			return sess, signerOrigin{backend: softSource.Name()}, nil
		}
	}

	return func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
		return openInOrder(ctx, thumbprint, cfg.PreferPKCS11, cng, p11, soft, log)
	}
}

// opener is one backend's attempt at a signing session.
type opener func(context.Context, keysource.Thumbprint) (keysource.Session, signerOrigin, error)

// dropOrigin adapts the chooser for callers that have nowhere to record where a
// session came from.
//
// The headless commands are those callers: they write no audit entry at all —
// internal/audit's Entry is constructed in exactly one place, and it is the
// window flow — so there is nothing for the origin to reach. One rule in one
// place, wrapped where it is not wanted, rather than a second copy of a
// three-backend fallback that would have to be kept in step with this one
// (D-108, D-124, D-138).
func dropOrigin(open opener) func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
	return func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
		sess, _, err := open(ctx, thumbprint)
		return sess, err
	}
}

// openInOrder holds every rule about which backend signs, separated from the
// wiring that builds them so that the rules can be exercised without a card, a
// module or a machine that has either.
//
// It is a decomposition rather than a seam (D-100): openCardOrSoftToken below
// is now only the three constructors, and everything that is a *decision* is
// here. The decisions are the delicate part — each one is a rule about what a
// person is shown or asked when something goes wrong — and until this split
// none of them could be read except by running the agent against hardware.
//
// soft may be nil: the soft token exists only in a build made with the
// "softtoken" tag.
func openInOrder(ctx context.Context, thumbprint keysource.Thumbprint, prefer bool, cng, p11, soft opener, log *slog.Logger) (keysource.Session, signerOrigin, error) {
	// The person's own inversion of the default order, from their config file
	// and from nowhere else (config.PreferPKCS11). Tried first and completely:
	// if the module has the certificate, the module signs, and CNG is never
	// asked.
	//
	// A failure here does not stop the chain, and that is the whole point of
	// the option. Somebody sets it because their minidriver is broken; somebody
	// else sets it, finds their module is the broken one, and **must not be
	// left worse off than if they had never touched it**. So this is the one
	// place a non-not-found error is walked past — deliberately, and with the
	// original kept for the message at the end.
	var preferredErr error
	if prefer {
		sess, origin, err := p11(ctx, thumbprint)
		if err == nil {
			return sess, origin, nil
		}
		// A cancelled PIN is a decision, not a failure, and the chain stops on
		// it. Falling through would ask the next backend, which on Windows
		// means CNG raising the operating system's own PIN dialog — so a
		// person who pressed Otkaži would be shown a second PIN prompt
		// immediately, for the signature they had just declined.
		//
		// SPEC §6.5.1 clause 5 is about one wrong PIN being one attempt; this
		// is its neighbour, and it is the case where the person did not give a
		// PIN at all. Nothing may turn "no" into "ask somewhere else".
		if errors.Is(err, pkcs11.ErrPINCancelled) {
			return nil, signerOrigin{}, err
		}
		preferredErr = err
		if !errors.Is(err, pkcs11.ErrCertificateNotFound) {
			log.Warn("pkcs11: preferred by configuration and could not sign; falling back",
				slog.String("thumbprint", string(thumbprint)),
				slog.String("error", err.Error()))
		}
	}

	sess, cngOrigin, err := cng(ctx, thumbprint)
	if err == nil {
		return sess, cngOrigin, nil
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeCertNotFound {
		// D-033: any other error is real. Masking a card that was removed, a
		// blocked PIN or a missing reader behind a second attempt against an
		// unrelated backend produces a confusing failure about the wrong thing.
		return nil, signerOrigin{}, err
	}

	// CNG does not have it, so this certificate is on a card Windows has no
	// minidriver for — or on no card at all. D-311: CNG signs when it offers
	// the certificate, and this is the branch where it does not.
	//
	// Skipped when the preference already asked. **One signature must never ask
	// one card for two PINs**, and SPEC §6.5.1 clause 5 is about one wrong PIN
	// being one attempt: on a machine with one card behind two modules (D-271)
	// a second walk would spend a second attempt on the same mistake.
	if !prefer {
		if sess, o, err := p11(ctx, thumbprint); err == nil {
			return sess, o, nil
		} else if !errors.Is(err, pkcs11.ErrCertificateNotFound) {
			// Including a cancelled PIN, for the reason above: the person
			// declined, and the soft token below must not be offered as a
			// second chance at a decision already made.
			return nil, signerOrigin{}, err
		}
	}

	if soft != nil {
		return soft(ctx, thumbprint)
	}
	// When the person asked for PKCS#11 first and it failed for a reason other
	// than not having the certificate, that reason is the one worth showing:
	// they configured this path, and "CNG has no such certificate" explains
	// nothing about why the module they chose did not work.
	if preferredErr != nil && !errors.Is(preferredErr, pkcs11.ErrCertificateNotFound) {
		return nil, signerOrigin{}, preferredErr
	}
	// Otherwise the original error rather than the last one tried. CNG's
	// CodeCertNotFound is the answer a person can act on; "no PKCS#11 module
	// has it either" is this program explaining itself.
	return nil, signerOrigin{}, err
}

// openThroughPKCS11 asks each module on this machine in turn and returns the
// first session one opens.
//
// In turn, and not in parallel, because opening means logging in: two modules
// asked at once means two PIN screens for one signature, and SPEC §6.5.1
// clause 5 is about one wrong PIN being one attempt. One card behind two
// modules (D-271) would also spend two attempts on one mistake.
//
// The PIN entry is built here, per call, because the window it must be owned by
// is the window the person is looking at — which is not a property of a module
// and cannot be cached with one. The workers themselves are cached and shared.
//
// It returns pkcs11.ErrCertificateNotFound only when *every* module said so. A
// module that failed for any other reason stops the walk, for D-033's reason:
// the next module answering "not mine" must not become the explanation a person
// is shown for a card that was removed.
func openThroughPKCS11(ctx context.Context, thumbprint keysource.Thumbprint, hwnd uintptr, cfg config.Config, log *slog.Logger) (keysource.Session, signerOrigin, error) {
	sources, _ := modules.ensure(log)
	if len(sources) == 0 {
		return nil, signerOrigin{}, pkcs11.ErrCertificateNotFound
	}
	entry := pinscreen.Entry(i18n.Load(cfg.Locale), hwnd)
	for _, s := range sources {
		sess, err := s.WithPINEntry(entry).Open(ctx, thumbprint)
		if err == nil {
			log.Info("pkcs11: signing through a module",
				slog.String("module", s.ModulePath()),
				slog.String("thumbprint", string(thumbprint)))
			return sess, signerOrigin{
				backend:    s.Name(),
				module:     s.ModulePath(),
				configured: s.Origin() == pkcs11.OriginConfigured,
			}, nil
		}
		if errors.Is(err, pkcs11.ErrCertificateNotFound) {
			continue
		}
		if worker.IsWorkerFailure(err) {
			// A broken worker is not an answer about this certificate, so the
			// walk carries on — this is F11 §3's "log it and carry past"
			// applied to a module that loaded and then died, and it is the one
			// case where continuing past a non-not-found error is right.
			log.Warn("pkcs11: a module stopped answering",
				slog.String("module", s.ModulePath()),
				slog.String("error", err.Error()),
				slog.String("said", "see the worker log lines above"))
			continue
		}
		return nil, signerOrigin{}, err
	}
	return nil, signerOrigin{}, pkcs11.ErrCertificateNotFound
}
