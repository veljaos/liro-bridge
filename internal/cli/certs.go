package cli

import (
	"context"
	"flag"
	"io"
	"time"

	"github.com/veljaos/liro-bridge/internal/i18n"
)

// RunCerts implements "liro-bridge certs" (F1 §6). It never uses a
// private key and never triggers a PIN prompt: everything it calls
// through deps only reads certificates and public Trusted List data.
func RunCerts(ctx context.Context, args []string, out io.Writer, locale string, deps Deps) int {
	fs := flag.NewFlagSet("certs", flag.ContinueOnError)
	fs.SetOutput(out)
	jsonOutput := fs.Bool("json", false, "machine-readable output")
	all := fs.Bool("all", false, "include certificates hidden by default")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	report, err := Gather(ctx, deps, time.Now())
	if err != nil {
		// No human-readable error message crosses the API (SPEC §7);
		// the CLI is a human-facing exception to that rule (it is not
		// an API boundary), but still prints English, not a code, since
		// F1 has no dedicated CLI error-message catalogue entry for
		// arbitrary internal failures.
		fprintln(out, "liro-bridge: certs:", err)
		return 1
	}

	if *jsonOutput {
		if err := RenderJSON(out, report, time.Now()); err != nil {
			fprintln(out, "liro-bridge: certs:", err)
			return 1
		}
		return 0
	}

	c := i18n.Load(locale)
	RenderText(out, report, c, time.Now(), *all)
	return 0
}
