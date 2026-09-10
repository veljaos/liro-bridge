//go:build softtoken

package main

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// This file is the only thing that names the signing path with no
// consent window, and it is compiled only into a build made with the
// "softtoken" tag. See internal/cli/sign_noconsent.go for why that path
// exists at all; the short version is that CI has to sign a PDF and
// verify it with OpenSSL on every push, with no human present, and a
// test build is where a test's needs belong.
//
// noconsent_release.go is what a release binary compiles instead, and
// it knows no command name and calls nothing.

// buildOnlyCommands are the subcommands this build has that a release
// build does not. They are listed by --help rather than hidden: a
// person running a build with a signing path that asks nobody should be
// able to see that it is there.
func buildOnlyCommands() []command {
	return []command{
		{"sign-no-consent", "Sign a PDF with no consent window (softtoken builds only; never in a release)"},
	}
}

// runBuildOnlyCommand dispatches sign-no-consent. handled is false for
// anything else, so run() carries on to its own next branch.
func runBuildOnlyCommand(ctx context.Context, args []string, stdout, stderr io.Writer, cfg config.Config) (code int, handled bool) {
	if len(args) == 0 || args[0] != "sign-no-consent" {
		return 0, false
	}
	return runSignWithoutConsent(ctx, args[1:], stdout, stderr, cfg), true
}

// runSignWithoutConsent wires the real Windows CNG source, the soft
// token fallback and the F1 Trusted List into
// internal/cli.RunSignWithoutConsent (F3 §9). Chain completion (F3
// §5.4) searches the Trusted List's CA/QC service certificates first,
// before falling back to AIA.
//
// The timestamp authority's credentials come from cfg and never from a
// flag: on Windows every process on the machine can read another
// process's full command line, so a password given as an argument is a
// password published to the machine (see cli.TSACredentials).
func runSignWithoutConsent(ctx context.Context, args []string, stdout, stderr io.Writer, cfg config.Config) int {
	cngSource := windowscng.NewSource()
	softSource := softTokenSource()

	open := func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
		sess, err := cngSource.Open(ctx, thumbprint)
		if err == nil {
			return sess, nil
		}
		// Fall back to the soft token only when the CNG store genuinely
		// has no such certificate — any other error (card removed, PIN
		// blocked, ...) is real and must not be masked by a confusing
		// second attempt against an unrelated backend (D-033).
		var e *errs.Error
		if softSource != nil && errors.As(err, &e) && e.Code == errs.CodeCertNotFound {
			return softSource.Open(ctx, thumbprint)
		}
		return nil, err
	}

	cachePath := filepath.Join(filepath.Dir(platform.DefaultConfigFile()), "tsl-cache.xml")
	store, err := tsl.NewFileStore(cachePath, tsl.DefaultURL, tsl.HTTPFetcher)
	var trustStore []*x509.Certificate
	if err == nil {
		if list, _, err := store.Current(ctx); err == nil {
			trustStore = list.CACertificates()
		}
	}

	return cli.RunSignWithoutConsent(ctx, args, stdout, stderr, cfg.Locale, cli.SignPDFDeps{
		Open:       open,
		TrustStore: trustStore,
		TSACredentials: cli.TSACredentials{
			BasicUsername:      cfg.TSAUser,
			BasicPassword:      cfg.TSAPassword,
			ClientCertPassword: cfg.TSAClientCertPassword,
		},
	})
}
