//go:build softtoken

package main

import (
	"context"
	"crypto/x509"
	"io"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// This file is the only thing that names the two signing paths with no
// consent window, and it is compiled only into a build made with the
// "softtoken" tag. See internal/cli/sign_noconsent.go and
// internal/cli/signdigest.go for why they exist at all; the short
// version is the same for both — CI has to sign a PDF and a digest and
// verify each with OpenSSL on every push, with no human present, and a
// test build is where a test's needs belong.
//
// sign-digest was in a release binary until F9b. It is the same defect
// as `sign` was, one command over (D-227): a signature over an
// attacker-chosen digest is a signature over an attacker-chosen
// document, and SPEC §6.6's consent screen cannot be built in front of
// a digest, because a digest has no document count, no file names and
// no batch to fingerprint. The hash-only case has a better front door
// already, POST /v2/sign, which has all three.
//
// noconsent_release.go is what a release binary compiles instead, and
// it knows no command name and calls nothing.

// hasNoConsentPaths is what this build is; see noconsent_release.go's
// own copy for why the constant exists.
const hasNoConsentPaths = true

// buildOnlyCommands are the subcommands this build has that a release
// build does not. They are listed by --help rather than hidden: a
// person running a build with a signing path that asks nobody should be
// able to see that it is there.
func buildOnlyCommands() []command {
	return []command{
		{"sign-no-consent", "Sign a PDF with no consent window (softtoken builds only; never in a release)"},
		{"sign-digest", "Sign a pre-computed digest, with no consent window (softtoken builds only; never in a release)"},
	}
}

// runBuildOnlyCommand dispatches this build's own two commands. handled
// is false for anything else, so run() carries on to its own next
// branch.
func runBuildOnlyCommand(ctx context.Context, args []string, stdout, stderr io.Writer, cfg config.Config) (code int, handled bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "sign-no-consent":
		return runSignWithoutConsent(ctx, args[1:], stdout, stderr, cfg), true
	case "sign-digest":
		return runSignDigest(ctx, args[1:], stdout, stderr, cfg.Locale), true
	}
	return 0, false
}

// runSignDigest wires the real Windows CNG source and the soft token
// into internal/cli.RunSignDigest. CNG first: the tag adds the soft
// token, it does not take the card away, so F2 §6.1's manual acceptance
// against real hardware still runs from a build made this way.
func runSignDigest(ctx context.Context, args []string, stdout, stderr io.Writer, locale string) int {
	return cli.RunSignDigest(ctx, args, stdout, stderr, locale, cli.SignDeps{Open: openCardOrSoftToken(0)})
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
	open := openCardOrSoftToken(0)

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
