// Command liro-bridge is the entry point for the Liro Bridge desktop
// signing agent: certs, sign, open and tray (see topLevelUsage for the
// one-line description of each, also shown by --help). A build made
// with the "softtoken" tag has two more, both of them signing paths
// with no consent window; see noconsent_softtoken.go.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
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

	// A mark left behind by a process that died mid-batch would refuse
	// this user's next upgrade for ever (F10 §3.2). Clearing it is a
	// registry read on every invocation and a write on almost none: it
	// only ever clears a mark whose owning process is gone.
	clearStaleSigningGuard()

	if len(args) > 0 && args[0] == "certs" {
		return runCerts(args[1:], out, cfg.Locale)
	}
	// There is one `sign` and it shows the consent window. SPEC §18.2
	// admits no exception — "No signature without human approval. No
	// flag, no configuration, no header bypasses the consent screen" —
	// and SPEC §4.3 puts all four entry points through the same consent
	// screen, the same session and the same audit log. `--interactive`
	// used to be what made the difference; it is not a distinction any
	// more, and parseSignArgs says so to anyone who still passes it.
	if len(args) > 0 && args[0] == "sign" {
		if !requireWebView2(i18n.Load(cfg.Locale)) {
			return 1
		}
		return runSignCommand(context.Background(), args[1:], out, cfg.Locale, cfg)
	}
	// The paths with no window, present only in a build made with the
	// "softtoken" tag, under their own names. In a release build this
	// returns handled=false and neither command exists at all.
	if code, handled := runBuildOnlyCommand(context.Background(), args, out, os.Stderr, cfg); handled {
		return code
	}
	// F6 §1: the main window, opened directly. Also how the tray's Open
	// item and the Explorer context menu reach it.
	if len(args) > 0 && args[0] == "open" {
		if !requireWebView2(i18n.Load(cfg.Locale)) {
			return 1
		}
		return runOpen(context.Background(), args[1:], out, cfg)
	}
	// F6 §2: one invocation per selected file, from Explorer. Every
	// invocation hands its file over; exactly one of them opens a
	// window for the whole selection.
	if len(args) > 0 && args[0] == platform.ShellMenuVerbFlag {
		if !requireWebView2(i18n.Load(cfg.Locale)) {
			return 1
		}
		return runShellVerb(context.Background(), args[1:], cfg)
	}
	if len(args) > 0 && args[0] == "tray" {
		// F5 §3: the agent starts minimised to tray with no window. Not
		// the bare-invocation behaviour (which stays usage-and-exit,
		// SPEC/F0's own tested contract) — an explicit subcommand, the
		// simplest option for something F5 does not itself name (D-0xx).
		if !requireWebView2(i18n.Load(cfg.Locale)) {
			return 1
		}
		return runTray(cfg, version)
	}
	// F10 §3.3: the installer's last act before it removes the binary.
	// It clears what the agent extracted for itself and says, in the
	// person's own language, that the audit log is still there — the
	// one sentence Windows' own uninstall UI cannot say, because it is
	// about this program.
	if len(args) > 0 && args[0] == "uninstall-notice" {
		return runUninstallNotice(args[1:], out, cfg)
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

// command is one subcommand and its one-line description.
type command struct{ name, desc string }

// topLevelCommands lists every liro-bridge subcommand a released binary
// recognises (see run, above) — none of them was previously
// discoverable from --help, which is what this list and topLevelUsage
// fix. A build made with the "softtoken" tag adds its own; see
// buildOnlyCommands.
//
// Help and usage text is always English (D-092, SPEC §9.2) — developer-
// facing like code, comments and documentation — regardless of the
// configured UI locale, so these are plain string literals rather than
// catalogue keys.
var topLevelCommands = []command{
	{"certs", "List available signing certificates"},
	{"sign", "Sign a PDF file"},
	{"open", "Open the main window to sign documents"},
	{"tray", "Run the agent in the system tray"},
	{"uninstall-notice", "Clear what this agent extracted and say what an uninstall keeps (run by the installer)"},
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
		for _, cmd := range append(append([]command{}, topLevelCommands...), buildOnlyCommands()...) {
			fprintf(w, "  %-16s %s\n", cmd.name, cmd.desc)
		}
		fprintln(w)
		fprintln(w, "Run 'liro-bridge <command> --help' for details about a command.")
	}
}
