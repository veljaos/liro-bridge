//go:build windows

// Command p11worker takes one login on one card, through the worker process,
// with the real PIN screen — and measures what it cost.
//
// It is the tool F12's home list asks for, "a scripts/p11worker alongside
// p11probe, with p11probe's two guards copied exactly", and it exists because
// the PIN seam F12 §2 built has never touched a card. Everything below the
// screen is covered by tests that run with no card and no window; what none of
// them can establish is that a real module, asked by a real parent, accepts a
// PIN a person typed.
//
// # What it costs, said first because it is the whole reason this is a
// separate program
//
// With --login it calls C_Login exactly once. On a Serbian identity card three
// wrong PINs block it, and unblocking a MUP card means a visit to a police
// station. So:
//
//   - It refuses to call C_Login at all unless the token's three user-PIN
//     flags — CKF_USER_PIN_COUNT_LOW, CKF_USER_PIN_FINAL_TRY and
//     CKF_USER_PIN_LOCKED — are all clear beforehand. That is p11probe's own
//     guard and D-268's second condition, kept as a property of a program
//     rather than of anybody's discipline: a condition that lives only in the
//     operator's care is one that gets missed once.
//   - It reads them again immediately afterwards, so whether an attempt was
//     consumed is measured rather than inferred (D-268).
//   - There is one Open call and no loop around it. SPEC §6.5.1 clause 5:
//     "Nothing retries a PIN automatically, ever, for any reason."
//
// Without --login it does everything except the login: it starts the worker,
// reads the card through it, and reads the counter. That is free, repeatable,
// and worth doing first — it proves the worker, the module and the card before
// anything can be spent.
//
// # Why the counter is read by p11probe rather than by this program
//
// p11probe already owns "read a token's PIN counter and say what it says", it
// has carried that guard since D-269, and its output is the format both
// D-268's and D-273's measurements are quoted in. A second implementation here
// would be one rule in two places — the objection D-108, D-124 and D-138 each
// had to remove once — and the two readings either side of the login would be
// taken by two different pieces of code, which is exactly what would make them
// not comparable.
//
// So this runs `go run ./scripts/p11probe --module <path>` twice, echoes its
// output whole, and parses the three lines. The parse fails closed: anything it
// cannot read is a refusal to proceed, never a pass. It never passes --login to
// p11probe, and the arguments are literals here rather than anything a flag can
// reach.
//
// # It must be run from the repository root
//
// Because of the above. It checks for go.mod and says so rather than producing
// a confusing failure three steps later.
package main

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11/worker"
	"github.com/veljaos/liro-bridge/internal/pinscreen"
)

func main() {
	// The worker's own child mode, before any flag is parsed and before
	// anything else happens at all.
	//
	// Worker.start re-executes os.Executable() with worker.Subcommand, so
	// whatever binary is driving a Worker has to answer it. Under
	// `go run ./scripts/p11worker` that binary is this one, in a temporary
	// directory — and a binary that ignored the subcommand would run main
	// again, which is D-293's fork bomb wearing a different hat. The Worker's
	// own recursion guard (pkcs11.ChildMarker, read on the parent side) is
	// what actually stops that; this is what makes the child useful rather
	// than merely harmless.
	if len(os.Args) > 2 && os.Args[1] == worker.Subcommand {
		os.Exit(worker.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	module := flag.String("module", "",
		`the PKCS#11 module to load, e.g. C:\Windows\System32\aetpkss1.dll`)
	login := flag.Bool("login", false,
		"take one login, with the real PIN screen. Without this everything is read-only "+
			"and nothing can be spent.")
	locale := flag.String("locale", "sr-Latn",
		"which catalogue the PIN screen is drawn in: sr-Latn, sr-Cyrl or en")
	flag.Parse()

	if *module == "" {
		die("--module is required. For the Pošta card that is C:\\Windows\\System32\\aetpkss1.dll")
	}
	if _, err := os.Stat("go.mod"); err != nil {
		die("run this from the repository root: the two counter readings are taken by\n" +
			"  go run ./scripts/p11probe, which needs the module's own directory.")
	}
	if _, err := os.Stat(*module); err != nil {
		die("cannot see %s: %v", *module, err)
	}

	say("p11worker — one login on one card, through the worker")
	say("  started       %s", time.Now().Format("2006-01-02 15:04:05.000"))
	say("  module        %s", *module)
	say("  locale        %s", *locale)
	say("  mode          %s", map[bool]string{
		true:  "--login: ONE C_Login will be attempted. This can cost a PIN attempt.",
		false: "read-only. No C_Login. Nothing can be spent.",
	}[*login])
	say("")

	// ---- the counter, before ----
	before, ok := readCounter(*module, "BEFORE")
	if !ok {
		die("the token's PIN counter could not be read before anything was done.\n" +
			"  Stopping. Nothing was attempted.")
	}
	if *login && !before.full() {
		die("the PIN counter is NOT full (flags 0x%X: %s).\n"+
			"  Stopping without calling C_Login. This needs the owner.\n"+
			"  D-268's second condition, and the reason it is in the program.",
			before.flags, before.describe())
	}
	if *login {
		say("  -> full counter. Proceeding.")
	}
	say("")

	ctx := context.Background()
	w := worker.New(*module, os.Stderr)

	// ---- the card, read through the worker ----
	t0 := time.Now()
	certs, err := w.Enumerate(ctx)
	enumerated := time.Since(t0)
	if err != nil {
		closeWorker(ctx, w)
		die("reading the card through the worker: %v\n  what the child said: %s", err, quote(w.ChildStderr()))
	}
	say("ENUMERATE   %d certificate(s) in %s", len(certs), took(enumerated))
	if len(certs) == 0 {
		closeWorker(ctx, w)
		die("the worker started and the module reported no certificates.\n" +
			"  Is the card in the reader?")
	}

	chosen := -1
	for i, c := range certs {
		tp := thumbprintOf(c.DER)
		parsed, perr := x509.ParseCertificate(c.DER)
		switch {
		case perr != nil:
			say("  [%d] %5d bytes  %s  (does not parse: %v)", i, len(c.DER), tp, perr)
		default:
			// SPEC §11.4: a certificate is usable for signing if
			// contentCommitment is set. Never require digitalSignature —
			// Halcom's signing certificates do not have it, and SPEC §18.4
			// makes that a hard prohibition rather than a preference.
			signing := parsed.KeyUsage&x509.KeyUsageContentCommitment != 0
			mark := " "
			if signing && chosen < 0 {
				chosen, mark = i, "*"
			}
			say("  [%d]%s%5d bytes  %s", i, mark, len(c.DER), tp)
			say("        subject   %s", parsed.Subject.CommonName)
			say("        issuer    %s", parsed.Issuer.CommonName)
			say("        keyUsage  %d%s", parsed.KeyUsage,
				map[bool]string{true: "  (contentCommitment: for signing)", false: "  (not for signing)"}[signing])
			if c.Label != "" {
				say("        label     %s", c.Label)
			}
		}
	}
	if chosen < 0 {
		closeWorker(ctx, w)
		die("no certificate on this card has contentCommitment set, so none of them\n" +
			"  is for signing (SPEC §11.4). Stopping.")
	}
	want := keysource.Thumbprint(thumbprintOf(certs[chosen].DER))
	say("")
	say("  the one marked * is the signing certificate and is the one that would be used.")

	if !*login {
		say("")
		say("--login not given: no C_Login was attempted. Nothing was spent.")
		closeWorker(ctx, w)
		readCounter(*module, "AFTER (unchanged: nothing was attempted)")
		return
	}

	// ---- the login ----
	//
	// One call. There is no loop here and there must never be one: SPEC
	// §6.5.1 clause 5, and the worker's own allow-list deliberately excludes
	// OpLogin from the operations a dead child's request may be re-sent for.
	var asked int
	var atQuestion, atAnswer time.Time
	screen := pinscreen.Entry(i18n.Load(*locale), 0)
	ask := pkcs11.PINEntry(func(dst []byte, req pkcs11.PINRequest) (int, error) {
		// This wrapper counts and times and does nothing else. It does not
		// read dst, copy it, or take anything out of it: the buffer belongs to
		// the function that will pass it to C_Login and overwrite it.
		asked++
		atQuestion = time.Now()
		say("")
		say("  THE CARD IS ASKING. The child reported no protected authentication path,")
		say("  so the PIN screen is this program's (SPEC §6.5.1 clause 1).")
		say("    card     %s", req.TokenLabel)
		say("    serial   %s", req.TokenSerial)
		say("    accepts  %d to %d characters, which are this token's own numbers", req.MinLength, len(dst))
		say("")
		say("  >>> A WINDOW IS OPENING NOW. It is titled in %s and its first line names", *locale)
		say("      Liro Bridge. If you do not see it, look in the taskbar — it has no")
		say("      owner window to be kept above, because this is a command line tool.")
		say("      Cancel spends nothing. Type the PIN only if you mean to.")
		n, err := screen(dst, req)
		atAnswer = time.Now()
		return n, err
	})

	say("")
	say("  opening a session and logging in, once, at %s", time.Now().Format("15:04:05.000"))
	t1 := time.Now()
	sess, loginErr := w.Open(ctx, want, ask)
	whole := time.Since(t1)

	say("")
	say("=====================================================================")
	switch {
	case loginErr == nil:
		say(" IT WORKED. The card accepted the PIN and the session is open.")
		say("   certificate  %s", thumbprintOf(sess.Certificate().DER))
		say("   chain        %d certificate(s) the token supplied", len(sess.Chain()))
		if err := sess.Close(); err != nil {
			say("   closing the session: %v", err)
		}
	case errors.Is(loginErr, pkcs11.ErrPINCancelled):
		say(" CANCELLED. Nothing was sent to the card and no attempt was spent.")
		say("   The whole exchange ran: the child asked, this program drew the screen,")
		say("   and you declined. The worker was killed, which is what the code does")
		say("   on every way out of a login that is not an answer.")
	case errors.Is(loginErr, worker.ErrWorkerDied):
		say(" THE WORKER DIED. %v", loginErr)
		say("   That is a process that stopped answering, not a card that refused.")
	default:
		say(" IT DID NOT WORK: %v", loginErr)
		say("   Read the counter below before reading anything into this.")
	}
	say("=====================================================================")
	say("")
	say("TIMING")
	say("  screen shown           %d time(s)%s", asked,
		map[bool]string{true: "  <- the two-phase exchange ran", false: "  <- the child never asked"}[asked > 0])
	if !atQuestion.IsZero() {
		say("  up to the question     %s", took(atQuestion.Sub(t1)))
		say("  you                    %s", took(atAnswer.Sub(atQuestion)))
		say("  after the PIN          %s", took(whole-atAnswer.Sub(t1)))
	}
	say("  the whole Open         %s", took(whole))
	if s := w.ChildStderr(); s != "" {
		say("")
		say("WHAT THE CHILD SAID   %s", quote(s))
	} else {
		say("  the child said nothing on its standard error")
	}

	closeWorker(ctx, w)

	// ---- the counter, after ----
	say("")
	after, ok := readCounter(*module, "AFTER")
	if !ok {
		say("THE COUNTER AFTER COULD NOT BE READ. Report this — it matters.")
		return
	}
	say("")
	if before.flags == after.flags {
		say("VERDICT: the token's flags are IDENTICAL before and after (0x%X).", before.flags)
		say("         No attempt was consumed, as far as the flags can say.")
	} else {
		say("VERDICT: the token's flags CHANGED: 0x%X -> 0x%X", before.flags, after.flags)
		say("         was: %s", before.describe())
		say("         now: %s", after.describe())
		say("         AN ATTEMPT WAS PROBABLY CONSUMED. Stop and tell the owner.")
	}
}

// ---------------------------------------------------------------- the counter

// counter is what p11probe reported about the token's user-PIN flags.
//
// Both the flags word and the three booleans are kept, and read, so that a
// disagreement between them is caught rather than one of them being trusted.
type counter struct {
	flags               uint32
	countLow, final     bool
	locked, sawAllThree bool
}

func (c counter) full() bool { return c.sawAllThree && !c.countLow && !c.final && !c.locked }

func (c counter) describe() string {
	var on []string
	for _, f := range []struct {
		set  bool
		name string
	}{{c.countLow, "USER_PIN_COUNT_LOW"}, {c.final, "USER_PIN_FINAL_TRY"}, {c.locked, "USER_PIN_LOCKED"}} {
		if f.set {
			on = append(on, f.name)
		}
	}
	if len(on) == 0 {
		return "none of the three user-PIN flags is set"
	}
	return strings.Join(on, " ")
}

var (
	reFlags = regexp.MustCompile(`PIN COUNTER [A-Z]+\s+flags=0x([0-9A-Fa-f]+)`)
	reFlag  = regexp.MustCompile(`(USER_PIN_COUNT_LOW|USER_PIN_FINAL_TRY|USER_PIN_LOCKED)\s+(true|false)`)
)

// readCounter runs p11probe, echoes everything it said, and parses the three
// flags out of it.
//
// It fails closed. Anything it cannot read — p11probe not running, a line it
// does not recognise, fewer than three flags — is a false return, and the one
// caller that matters treats that as a refusal to proceed. A parse that
// silently defaulted to "full" would be a guard that passes when it stops
// working, which is the failure mode this project has recorded most often.
func readCounter(module, when string) (counter, bool) {
	say("PIN COUNTER %s — asking p11probe, which owns this reading", when)
	// Literals, never anything a flag can reach: --login is what this program
	// must never hand to p11probe.
	cmd := exec.Command("go", "run", "./scripts/p11probe", "--module", module)
	out, err := cmd.CombinedOutput()
	text := string(out)
	for _, line := range strings.Split(strings.TrimRight(text, "\r\n"), "\n") {
		say("    | %s", strings.TrimRight(line, "\r"))
	}
	if err != nil {
		say("    p11probe exited: %v", err)
		return counter{}, false
	}

	var c counter
	m := reFlags.FindStringSubmatch(text)
	if m == nil {
		say("    could not find p11probe's own 'PIN COUNTER ... flags=0x' line.")
		return c, false
	}
	v, perr := strconv.ParseUint(m[1], 16, 32)
	if perr != nil {
		say("    could not read the flags word %q: %v", m[1], perr)
		return c, false
	}
	c.flags = uint32(v)

	seen := 0
	for _, f := range reFlag.FindAllStringSubmatch(text, -1) {
		set := f[2] == "true"
		switch f[1] {
		case "USER_PIN_COUNT_LOW":
			c.countLow, seen = set, seen+1
		case "USER_PIN_FINAL_TRY":
			c.final, seen = set, seen+1
		case "USER_PIN_LOCKED":
			c.locked, seen = set, seen+1
		}
	}
	if seen != 3 {
		say("    found %d of the three user-PIN flag lines, want 3.", seen)
		return c, false
	}
	c.sawAllThree = true
	return c, true
}

// ---------------------------------------------------------------- small parts

// thumbprintOf is the identifier the agent uses throughout: SHA-1 over the DER,
// uppercase hex (F1 §3.1).
//
// It is computed here because internal/keysource/pkcs11's own thumbprint is
// unexported, and this is a second place the convention is written down. It is
// not left to drift: if it disagreed with the backend's, Open would refuse with
// "no certificate with that thumbprint" rather than doing something else, so a
// wrong answer here is a loud failure rather than a quiet one.
//
// SHA-1 here is an X.509 identification convention — the same one Windows' own
// certificate UI shows — and not a cryptographic use, so SPEC §18.8's
// prohibition on producing SHA-1 is not in question.
func thumbprintOf(der []byte) string {
	sum := sha1.Sum(der)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// took prints a duration and refuses to let a sub-millisecond one read as
// zero.
//
// D-304: this machine cannot time anything under about half a millisecond, so
// time.Since across a fast call reads exactly 0s — which is typeset
// identically to a real measurement. Everything measured here is seconds, so
// this should never fire; it is here so that if it ever does, it says so
// rather than reporting a zero.
func took(d time.Duration) string {
	if d == 0 {
		return "0s (below this machine's clock — see D-304, not an instant)"
	}
	return d.String()
}

func quote(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if s == "" {
		return "(nothing)"
	}
	return "\n    | " + strings.ReplaceAll(s, "\n", "\n    | ")
}

func closeWorker(ctx context.Context, w *worker.Worker) {
	if err := w.Close(ctx); err != nil {
		say("  closing the worker: %v", err)
	}
}

// say writes one line. os.Stdout is unbuffered in Go, so the line is on the
// terminal before the next statement runs — which matters on the one line
// printed immediately before a call that blocks on a dialog (D-269).
func say(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "p11worker: "+format+"\n", a...)
	os.Exit(1)
}
