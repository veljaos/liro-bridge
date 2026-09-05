// Command liro-bridge is the entry point for the Liro Bridge desktop
// signing agent: certs, sign, sign-digest and tray (see topLevelUsage
// for the one-line description of each, also shown by --help).
package main

import (
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// Set at build time with -ldflags; see F0 §7.1.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// containsFlag reports whether name is present among args — used only
// to decide "sign" vs "sign --interactive" dispatch before either
// command's own flag.Parse runs.
func containsFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, out io.Writer) int {
	cfg, cfgErr := config.Load(platform.DefaultConfigFile())

	logger, closer, err := config.SetupLogging(cfg.LogLevel, platform.DefaultLogDir(), os.Getenv("LIRO_DEBUG") == "1")
	if err != nil {
		fmt.Fprintln(os.Stderr, "liro-bridge: failed to set up logging:", err)
		return 1
	}
	defer func() { _ = closer.Close() }()

	if cfgErr != nil {
		logger.Warn("startup: config file could not be read, using defaults", "error", cfgErr)
	}
	logger.Info("liro-bridge starting", slog.String("version", version), slog.String("commit", commit))

	if len(args) > 0 && args[0] == "certs" {
		return runCerts(args[1:], out, cfg.Locale)
	}
	if len(args) > 0 && args[0] == "sign-digest" {
		return runSignDigest(context.Background(), args[1:], out, os.Stderr, cfg.Locale)
	}
	if len(args) > 0 && args[0] == "sign" {
		if containsFlag(args[1:], "--interactive") {
			return runSignInteractive(context.Background(), args[1:], out, cfg.Locale, cfg)
		}
		return runSign(context.Background(), args[1:], out, os.Stderr, cfg.Locale)
	}
	// F6 §1: the main window, opened directly. Also how the tray's Open
	// item and the Explorer context menu reach it.
	if len(args) > 0 && args[0] == "open" {
		return runOpen(context.Background(), args[1:], out, cfg)
	}
	// F6 §2: one invocation per selected file, from Explorer. Every
	// invocation hands its file over; exactly one of them opens a
	// window for the whole selection.
	if len(args) > 0 && args[0] == platform.ShellMenuVerbFlag {
		return runShellVerb(context.Background(), args[1:], cfg)
	}
	if len(args) > 0 && args[0] == "tray" {
		// F5 §3: the agent starts minimised to tray with no window. Not
		// the bare-invocation behaviour (which stays usage-and-exit,
		// SPEC/F0's own tested contract) — an explicit subcommand, the
		// simplest option for something F5 does not itself name (D-0xx).
		return runTray(cfg, version)
	}

	fs := flag.NewFlagSet("liro-bridge", flag.ContinueOnError)
	fs.SetOutput(out)
	showVersion := fs.Bool("version", false, "print version information and exit")
	fs.Usage = topLevelUsage(fs)

	if parseErr := fs.Parse(args); parseErr != nil {
		if parseErr == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *showVersion {
		_, _ = fmt.Fprintf(out, "liro-bridge %s (commit %s, built %s, %s, %s/%s)\n",
			version, commit, buildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	}

	fs.Usage()
	return 0
}

// runCerts wires the real Windows smart card reader service, real CNG
// enumeration and a Trusted List file store into internal/cli.RunCerts.
// This is the only place those concrete implementations are chosen —
// internal/cli itself only knows about the small function-shaped Deps
// (F1 §6), so it stays testable without hardware.
func runCerts(args []string, out io.Writer, locale string) int {
	svc := platform.NewSmartCardService()
	cngSource := windowscng.NewSource()
	cachePath := filepath.Join(filepath.Dir(platform.DefaultConfigFile()), "tsl-cache.xml")
	store, err := tsl.NewFileStore(cachePath, tsl.DefaultURL, tsl.HTTPFetcher)
	if err != nil {
		_, _ = fmt.Fprintln(out, "liro-bridge: certs:", err)
		return 1
	}

	deps := cli.Deps{
		Readers:           svc.Readers,
		PresenceCheck:     cngSource.Presence,
		Enumerate:         windowscng.Enumerate,
		Store:             store,
		ExtraCertificates: softTokenExtraCertificates,
	}
	return cli.RunCerts(context.Background(), args, out, locale, deps)
}

// runSignDigest wires the real Windows CNG source — and, only in a
// binary built with the "softtoken" tag, the soft token too (F2 §3) —
// into internal/cli.RunSignDigest. This is the only place a concrete
// keysource.Source is chosen for signing.
func runSignDigest(ctx context.Context, args []string, stdout, stderr io.Writer, locale string) int {
	cngSource := windowscng.NewSource()
	softSource := softTokenSource() // nil unless built with the "softtoken" tag

	open := func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
		sess, err := cngSource.Open(ctx, thumbprint)
		if err == nil {
			return sess, nil
		}
		// Fall back to the soft token only when the CNG store genuinely
		// has no such certificate — any other error (card removed, PIN
		// blocked, ...) is real and must not be masked by a confusing
		// second attempt against an unrelated backend.
		var e *errs.Error
		if softSource != nil && errors.As(err, &e) && e.Code == errs.CodeCertNotFound {
			return softSource.Open(ctx, thumbprint)
		}
		return nil, err
	}

	return cli.RunSignDigest(ctx, args, stdout, stderr, locale, cli.SignDeps{Open: open})
}

// runSign wires the real Windows CNG (and, with the softtoken tag, soft
// token fallback) source plus the F1 Trusted List into
// internal/cli.RunSign (F3 §9). Chain completion (F3 §5.4) searches the
// Trusted List's CA/QC service certificates first, before falling back
// to AIA.
func runSign(ctx context.Context, args []string, stdout, stderr io.Writer, locale string) int {
	cngSource := windowscng.NewSource()
	softSource := softTokenSource()

	open := func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
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

	cachePath := filepath.Join(filepath.Dir(platform.DefaultConfigFile()), "tsl-cache.xml")
	store, err := tsl.NewFileStore(cachePath, tsl.DefaultURL, tsl.HTTPFetcher)
	var trustStore []*x509.Certificate
	if err == nil {
		if list, _, err := store.Current(ctx); err == nil {
			trustStore = caCertificatesFromTSL(list)
		}
	}

	return cli.RunSign(ctx, args, stdout, stderr, locale, cli.SignPDFDeps{Open: open, TrustStore: trustStore})
}

// caCertificatesFromTSL extracts every CA/QC service's certificate from
// list, parsed as *x509.Certificate, for chain completion (F3 §5.4).
// A service whose certificate does not parse is skipped rather than
// failing the whole load — chain completion is best-effort by design.
func caCertificatesFromTSL(list *tsl.List) []*x509.Certificate {
	var out []*x509.Certificate
	for _, p := range list.Providers {
		for _, svc := range p.Services {
			if !svc.IsCA() || len(svc.Certificate) == 0 {
				continue
			}
			if cert, err := x509.ParseCertificate(svc.Certificate); err == nil {
				out = append(out, cert)
			}
		}
	}
	return out
}

// topLevelCommands lists every liro-bridge subcommand and its one-line
// description: certs, sign, sign-digest and tray are the only four the
// binary actually recognises (see run, above) — none of them was
// previously discoverable from --help, which is what this list and
// topLevelUsage fix.
//
// Help and usage text is always English (D-0xx, SPEC §9.2) — developer-
// facing like code, comments and documentation — regardless of the
// configured UI locale, so these are plain string literals rather than
// catalogue keys.
var topLevelCommands = []struct{ name, desc string }{
	{"certs", "List available signing certificates"},
	{"sign", "Sign a PDF file"},
	{"sign-digest", "Sign a pre-computed digest (advanced/integration use)"},
	{"open", "Open the main window to sign documents"},
	{"tray", "Run the agent in the system tray"},
}

// topLevelUsage returns fs.Usage for the top-level flag set: an English
// synopsis, every subcommand with its one-line description, and a
// pointer to each subcommand's own --help, since sign --help etc.
// already work but were undiscoverable without already knowing the
// subcommand's name.
//
// This used to also print the flag package's own "Usage of
// liro-bridge:" block (fs.PrintDefaults, listing only -version) — a
// second, differently-formatted "usage" block that duplicated the
// command synopsis above it without adding anything a user couldn't
// get from `liro-bridge --version`. Removed (Task 1).
func topLevelUsage(fs *flag.FlagSet) func() {
	return func() {
		w := fs.Output()
		fprintln(w, "Usage: liro-bridge <command> [flags]")
		fprintln(w)
		fprintln(w, "Commands:")
		for _, cmd := range topLevelCommands {
			fprintf(w, "  %-14s %s\n", cmd.name, cmd.desc)
		}
		fprintln(w)
		fprintln(w, "Run 'liro-bridge <command> --help' for details about a command.")
	}
}
