package jobs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CoalesceWindow is how long the collector waits for the inbox to go
// quiet before opening a window for what is in it (F6 §2).
//
// Windows invokes a classic `shell` verb **once per selected file**, in
// a separate process. Twenty selected documents start twenty processes,
// each told about one file, and opening twenty windows is not what the
// person asked for. Coalescing them has to work *between* processes.
//
// **Measured**, on this development machine, launching twenty real
// invocations of the real binary: the arrivals spread over **2390 ms**
// end to end, with a typical gap between consecutive ones of about
// **50 ms** — and one gap of **1070 ms**, a stall somewhere in process
// creation. That single stall is not an outlier to design around; it is
// what a machine with an on-access virus scanner, a roaming profile or
// any other load does routinely.
//
// The first attempt was a 750 ms quiet period and nothing else, and the
// measurement above is what it produced: the 1070 ms stall ended the
// quiet period early, ten documents were signed, and **the other ten
// were left sitting in the inbox with nobody to open them** — a silent
// half-batch, which is the worst outcome this product has. No unit test
// caught it, because a fake clock has no stalls in it.
//
// So the window is not what makes this correct, and it is deliberately
// not sized to swallow a 2390 ms spread: doing that would make a
// single right-clicked document wait two and a half seconds for a
// window, which is the common case paying for the rare one. 600 ms
// covers the ordinary consecutive gap more than ten times over and
// opens promptly for one file. **What makes it correct is that the
// window keeps collecting after it opens** — a straggler joins the list
// that is already on screen, exactly as a dropped file does — and that
// the collector drains the inbox again before it exits. Nothing is
// stranded, whatever the timing.
const CoalesceWindow = 600 * time.Millisecond

// CoalesceStaleAfter is when an inbox left behind by a collector that
// died is discarded rather than merged into an unrelated batch. A
// person right-clicking two documents an hour after a crash must not
// get nine forgotten ones alongside them.
const CoalesceStaleAfter = 5 * time.Minute

// coalesceMaxWait bounds the whole gather. Something appending to the
// inbox forever must not keep the window from ever opening; after this
// the batch is taken as it stands. Twenty files take about a second and
// a half, so this is two orders of magnitude of headroom.
const coalesceMaxWait = 60 * time.Second

// Inbox is the file separate Explorer invocations append to so that one
// of them can open a single window for all of them.
//
// A file rather than a socket or a pipe: this is a per-user, local,
// short-lived handover between processes that are all this same
// program, and F6 is explicit that no server of any kind is built this
// phase. The file lives beside the agent's other per-user state, so a
// second user signed in over RDP has their own (SPEC §14.1 — one agent
// per user session, never per machine).
type Inbox struct{ path string }

// NewInbox returns the inbox in dir. The file itself is created on the
// first Append.
func NewInbox(dir string) *Inbox {
	return &Inbox{path: filepath.Join(dir, "shell-inbox.txt")}
}

// Path is where the inbox lives, for logging.
func (b *Inbox) Path() string { return b.path }

// Append adds paths to the inbox, creating it if needed.
//
// One write per call, in append mode: Windows serialises appends to a
// file opened this way, so several processes writing at once interleave
// whole lines rather than fragments of them. A path containing a
// newline cannot occur — Windows forbids it in a file name — so a line
// is a path and nothing else needs escaping.
func (b *Inbox) Append(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(b.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	_, err = f.WriteString(sb.String())
	return err
}

// Count is how many paths the inbox currently holds.
func (b *Inbox) Count() (int, error) {
	f, err := os.Open(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer func() { _ = f.Close() }()

	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.TrimSpace(s.Text()) != "" {
			n++
		}
	}
	return n, s.Err()
}

// Take removes the inbox and returns everything it held.
//
// The file is renamed before being read, which is what makes this
// atomic enough: a process appending at the same moment either got in
// before the rename (and is in this batch) or finds no file and creates
// a fresh one (and is in the next). Reading and then deleting would
// lose whatever arrived in between.
func (b *Inbox) Take() ([]string, error) {
	taken := fmt.Sprintf("%s.taken-%d", b.path, os.Getpid())
	if err := os.Rename(b.path, taken); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = os.Remove(taken) }()

	f, err := os.Open(taken)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []string
	s := bufio.NewScanner(f)
	// A path can be long (F6 §7 asks about over 260 characters), and
	// bufio's default 64KB line limit is far above anything Windows
	// will produce, but the buffer is grown explicitly so the bound is
	// stated rather than assumed.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for s.Scan() {
		if line := strings.TrimSpace(s.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out, s.Err()
}

// Clock is the time source CollectBatch uses, injected so the gather
// can be tested deterministically instead of by sleeping.
type Clock struct {
	Now   func() time.Time
	Sleep func(time.Duration)
}

// RealClock is the production clock.
func RealClock() Clock {
	return Clock{Now: time.Now, Sleep: time.Sleep}
}

// CollectBatch waits for the inbox to stop growing and then takes it.
//
// "Stop growing" is quiet for the whole coalescing window, not one
// poll: twenty processes arriving over a second and a half are one
// batch, and the timer restarts on each one.
func CollectBatch(b *Inbox, quiet time.Duration, clock Clock) ([]string, error) {
	if quiet <= 0 {
		quiet = CoalesceWindow
	}
	// A fifth of the window: fine enough that the batch opens promptly
	// after the last arrival, coarse enough that waiting costs a
	// handful of stat calls rather than hundreds.
	poll := quiet / 5
	if poll <= 0 {
		poll = time.Millisecond
	}

	start := clock.Now()
	lastCount, lastChange := -1, start

	for {
		n, err := b.Count()
		if err != nil {
			return nil, err
		}
		now := clock.Now()
		if n != lastCount {
			lastCount, lastChange = n, now
		}
		if n > 0 && now.Sub(lastChange) >= quiet {
			return b.Take()
		}
		if now.Sub(start) >= coalesceMaxWait {
			// Something is still appending, or the clock has gone
			// backwards. Take what is here rather than never opening.
			return b.Take()
		}
		clock.Sleep(poll)
	}
}

// DiscardIfStale removes the inbox if nothing has been added to it for
// maxAge, and reports how many paths were thrown away.
//
// The only way to reach a stale inbox is a collector that died between
// taking a batch and draining what arrived after it. Merging those
// paths into whatever the person right-clicks next would sign documents
// they did not choose this time, which is worse than losing them — and
// losing them is logged, not silent.
func (b *Inbox) DiscardIfStale(maxAge time.Duration, now time.Time) (int, error) {
	info, err := os.Stat(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if now.Sub(info.ModTime()) < maxAge {
		return 0, nil
	}
	n, err := b.Count()
	if err != nil {
		return 0, err
	}
	if err := os.Remove(b.path); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return n, nil
}
