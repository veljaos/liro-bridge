//go:build windows

package pinscreen

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// These drive Entry with the window replaced, which is what the collect seam
// is for. They need no display, no message loop and no card, so they run in
// CI on the windows-latest runner exactly as they do here.
//
// What they cannot establish is that the dialog appears, what it looks like,
// or that the person typing into it sees the heading. That needs a screen and
// a pair of eyes, and this project has recorded six times that a green suite
// is not evidence about what a window shows (D-087, D-122, D-161, D-172,
// D-219, D-247). The dialog itself was photographed in all three locales when
// it was written (D-279 §5, D-280).

// fakeScreen stands in for the dialog. It records what it was shown and how
// often, and answers however the test says.
type fakeScreen struct {
	calls   int
	prompts []ui.PINPrompt
	sawBuf  []uintptr // the address of each buffer it was handed
	sawLen  []int

	write  string // written into the buffer when ok
	ok     bool
	err    error
	maxLen int
}

func (f *fakeScreen) collect(owner uintptr, prompt ui.PINPrompt, maxLen int, dst []byte) (int, bool, error) {
	f.calls++
	f.prompts = append(f.prompts, prompt)
	f.maxLen = maxLen
	f.sawLen = append(f.sawLen, len(dst))
	if len(dst) > 0 {
		f.sawBuf = append(f.sawBuf, uintptr(unsafe.Pointer(&dst[0])))
	}
	if f.err != nil {
		return 0, false, f.err
	}
	if !f.ok {
		return 0, false, nil
	}
	n := copy(dst, f.write)
	return n, true, nil
}

// withScreen swaps the seam for the length of one test and puts it back.
func withScreen(t *testing.T, f *fakeScreen) {
	t.Helper()
	was := collect
	collect = f.collect
	t.Cleanup(func() { collect = was })
}

func request() pkcs11.PINRequest { return posta() }

// TestTheScreenIsAskedExactlyOnce is SPEC §6.5.1 clause 5 as a property of
// this function rather than as a sentence in its comment.
//
// "Nothing retries a PIN automatically, ever, for any reason. One wrong PIN is
// one attempt. Three block the card, and for a national identity card
// unblocking means a visit to a police station."
//
// Asked over every answer the screen can give, because a retry that only
// happened on a refusal would pass a test that only tried the happy path —
// and a refusal is exactly the answer a retry would be tempting for.
func TestTheScreenIsAskedExactlyOnce(t *testing.T) {
	cases := []struct {
		name   string
		screen fakeScreen
	}{
		{"a PIN was typed", fakeScreen{ok: true, write: "12345"}},
		{"the person cancelled", fakeScreen{ok: false}},
		{"the screen refused what was typed", fakeScreen{err: ui.ErrPINTooLong}},
		{"the screen failed", fakeScreen{err: errors.New("CreateWindowExW returned 0")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.screen
			withScreen(t, &f)
			dst := make([]byte, 15)
			_, _ = Entry(i18n.Load("sr-Latn"), 0)(dst, request())
			if f.calls != 1 {
				t.Errorf("the screen was shown %d times, want exactly 1", f.calls)
			}
		})
	}
}

// TestACancellationIsACancellationAndNeverAnEmptyPIN.
//
// Returning (0, nil) would be a PIN of length zero, and D-268 measured what an
// empty PIN costs on a real card: the module handed it straight to the card,
// the card counted it as a wrong PIN, and one of three attempts was gone. The
// token had declared a minimum of 4 the whole time and the module
// range-checked nothing.
//
// The length check in login and sendPIN would refuse a zero today, so this is
// belt over braces — and it is the belt that makes the answer *readable*: a
// caller that sees ErrPINCancelled knows a person declined, where a length
// error says the program is confused.
func TestACancellationIsACancellationAndNeverAnEmptyPIN(t *testing.T) {
	f := fakeScreen{ok: false}
	withScreen(t, &f)

	n, err := Entry(i18n.Load("sr-Latn"), 0)(make([]byte, 15), request())
	if n != 0 {
		t.Errorf("a cancellation reported %d bytes, want 0", n)
	}
	if !errors.Is(err, pkcs11.ErrPINCancelled) {
		t.Errorf("a cancellation came back as %v, want pkcs11.ErrPINCancelled — a "+
			"cancellation is not a failure and must not be reported as one, and it "+
			"must not be reported as a PIN of no characters either", err)
	}
}

// TestARefusedLengthIsNotACancellation is the defect this package's first
// caller found in the dialog it calls.
//
// ui.CollectPIN answered "what was typed does not fit the token's buffer" with
// the same (0, false, nil) it answers Cancel with, so a person who typed
// something and pressed OK would have been recorded as having cancelled. It
// returns ui.ErrPINTooLong now, and this asserts the entry passes it through
// rather than folding it back into a cancellation.
func TestARefusedLengthIsNotACancellation(t *testing.T) {
	f := fakeScreen{err: ui.ErrPINTooLong}
	withScreen(t, &f)

	n, err := Entry(i18n.Load("sr-Latn"), 0)(make([]byte, 15), request())
	if n != 0 {
		t.Errorf("a refusal reported %d bytes, want 0", n)
	}
	if errors.Is(err, pkcs11.ErrPINCancelled) {
		t.Error("a PIN too long for the token came back as a cancellation; nobody cancelled")
	}
	if !errors.Is(err, ui.ErrPINTooLong) {
		t.Errorf("a PIN too long for the token came back as %v, want ui.ErrPINTooLong", err)
	}
}

// TestTheCallersBufferIsHandedThroughAndNotCopied.
//
// The whole reason pkcs11.PINEntry fills a buffer the caller allocated instead
// of returning a PIN is that there is then exactly one copy, owned by the
// function that pins it, passes it to C_Login and overwrites it. A copy made
// on the way through would be a second one that nothing wipes — and it would
// be invisible to the AST guard, because it would live in a local with an
// innocent name.
//
// So the address is compared, not the contents: same backing array, same
// length, and the length the dialog is told to limit itself to is that same
// length rather than a number from anywhere else.
func TestTheCallersBufferIsHandedThroughAndNotCopied(t *testing.T) {
	f := fakeScreen{ok: true, write: "12345"}
	withScreen(t, &f)

	dst := make([]byte, 15)
	want := uintptr(unsafe.Pointer(&dst[0]))

	n, err := Entry(i18n.Load("sr-Latn"), 0)(dst, request())
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if n != 5 {
		t.Fatalf("Entry reported %d bytes, want 5", n)
	}
	if len(f.sawBuf) != 1 || f.sawBuf[0] != want {
		t.Errorf("the dialog was handed a different buffer from the caller's (%#x vs %#x)",
			f.sawBuf, want)
	}
	if len(f.sawLen) != 1 || f.sawLen[0] != len(dst) {
		t.Errorf("the dialog was handed a buffer of %v bytes, want %d", f.sawLen, len(dst))
	}
	if f.maxLen != len(dst) {
		t.Errorf("the dialog was told to limit itself to %d, want the buffer's own %d — "+
			"the buffer is the token's maximum, and a second number for it is one "+
			"that can disagree with the buffer it describes", f.maxLen, len(dst))
	}
	if got := string(dst[:n]); got != "12345" {
		t.Errorf("the caller's buffer holds %q, so what the dialog wrote did not land in it", got)
	}
}

// TestTheScreenIsShownTheLocalisedPrompt checks the join rather than the text
// — the text is Text's, tested in every locale in the neutral file — so that a
// field dropped between Prompt and ui.PINPrompt is caught. Seven assignments
// by hand is seven chances to leave one out, and the one most worth catching
// is the heading, which is clause 6 entire.
func TestTheScreenIsShownTheLocalisedPrompt(t *testing.T) {
	f := fakeScreen{ok: true, write: "12345"}
	withScreen(t, &f)

	cat := i18n.Load("sr-Cyrl")
	if _, err := Entry(cat, 0)(make([]byte, 15), request()); err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if len(f.prompts) != 1 {
		t.Fatalf("the dialog was shown %d prompts, want 1", len(f.prompts))
	}
	want := Text(cat, request(), 15)
	got := f.prompts[0]
	for _, field := range []struct{ name, got, want string }{
		{"Title", got.Title, want.Title},
		{"Heading", got.Heading, want.Heading},
		{"Subject", got.Subject, want.Subject},
		{"Label", got.Label, want.Label},
		{"Hint", got.Hint, want.Hint},
		{"OK", got.OK, want.OK},
		{"Cancel", got.Cancel, want.Cancel},
	} {
		if field.got != field.want {
			t.Errorf("%s reached the dialog as %q, want %q", field.name, field.got, field.want)
		}
	}
}

// TestTheOwnerIsPassedThrough. A dialog with no owner is not kept above the
// window it was opened from, is centred on the monitor under the cursor rather
// than on that window, and leaves it answering clicks — which is D-129's
// measured defect, where a window opened from another with no owner was
// created underneath it and the one behind then froze.
func TestTheOwnerIsPassedThrough(t *testing.T) {
	var saw uintptr
	was := collect
	collect = func(owner uintptr, prompt ui.PINPrompt, maxLen int, dst []byte) (int, bool, error) {
		saw = owner
		return copy(dst, "12345"), true, nil
	}
	t.Cleanup(func() { collect = was })

	const owner = uintptr(0x1234)
	if _, err := Entry(i18n.Load("en"), owner)(make([]byte, 15), request()); err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if saw != owner {
		t.Errorf("the dialog was given owner %#x, want %#x", saw, owner)
	}
}
