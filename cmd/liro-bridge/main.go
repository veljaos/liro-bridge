// Command liro-bridge is the entry point for the Liro Bridge desktop
// signing agent. In this phase it supports exactly --version and --help;
// everything else (certificates, signing, the HTTP API, the UI) arrives
// in later phases.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/platform"
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
	defer closer.Close()

	if cfgErr != nil {
		logger.Warn("startup: config file could not be read, using defaults", "error", cfgErr)
	}
	logger.Info("liro-bridge starting", slog.String("version", version), slog.String("commit", commit))

	fs := flag.NewFlagSet("liro-bridge", flag.ContinueOnError)
	fs.SetOutput(out)
	showVersion := fs.Bool("version", false, "print version information and exit")

	if parseErr := fs.Parse(args); parseErr != nil {
		if parseErr == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *showVersion {
		fmt.Fprintf(out, "liro-bridge %s (commit %s, built %s, %s, %s/%s)\n",
			version, commit, buildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	}

	fs.Usage()
	return 0
}
