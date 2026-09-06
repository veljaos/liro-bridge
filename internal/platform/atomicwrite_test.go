package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

func TestWriteFileAtomicWritesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")

	if err := WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", got, "new")
	}
	assertNoTemporaryFilesLeft(t, dir)
}

func TestWriteFileAtomicReplacesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", got, "new")
	}
	assertNoTemporaryFilesLeft(t, dir)
}

// TestWriteFileAtomicLeavesTheOriginalIntactWhenTheDestinationIsHeldOpen
// is J-8's own case, and the accepted cost of the change: on Windows a
// rename over a destination another program has open fails, where
// os.WriteFile succeeded by truncating it. Refusing is better than
// destroying — so the assertion is that the original survives byte for
// byte, that the temporary file is gone, and that the failure says what
// it is rather than "access denied".
func TestWriteFileAtomicLeavesTheOriginalIntactWhenTheDestinationIsHeldOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	original := []byte("the previously good signed document")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	release, held := holdFileOpen(t, path)
	if !held {
		t.Skip("this platform's rename replaces a file other programs hold open, which is the behaviour this test is about not having")
	}
	defer release()

	err := WriteFileAtomic(path, []byte("the new signed document"), 0o600)
	if err == nil {
		t.Fatal("WriteFileAtomic replaced a destination held open by another program; it must refuse rather than destroy it")
	}

	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("error %v is not an *errs.Error, so nothing can render a message for it", err)
	}
	if e.Code != errs.CodeOutputInUse {
		t.Errorf("Code = %s, want %s — a destination held open needs a file closed, not a disk checked", e.Code, errs.CodeOutputInUse)
	}
	if !errors.Is(err, ErrDestinationInUse) {
		t.Error("the error does not wrap ErrDestinationInUse")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the destination after the refused write: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("the original was modified: %q, want %q", got, original)
	}
	assertNoTemporaryFilesLeft(t, dir)
}

// TestWriteFileAtomicRemovesItsTemporaryFileWhenTheRenameFails covers
// the same clean-up for a failure that is not "held open": a destination
// that is a directory cannot be renamed over on any platform.
func TestWriteFileAtomicRemovesItsTemporaryFileWhenTheRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new"), 0o600); err == nil {
		t.Fatal("WriteFileAtomic succeeded onto a directory")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the destination was disturbed: %v", err)
	}
	assertNoTemporaryFilesLeft(t, dir)
}

// TestWriteFileAtomicNeverTruncatesTheDestination is the property the
// whole change exists for: whatever a watcher sees at the destination
// path is either the whole old file or the whole new one, never zero
// bytes and never a prefix.
//
// It cannot watch a real race deterministically, so it asserts the
// mechanism instead: while the write is under way — and it is, because
// the data is large enough that the write cannot be instantaneous — the
// destination still holds every byte of the old file, and no partial
// file exists under the destination's own name at any point.
func TestWriteFileAtomicNeverTruncatesTheDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	old := strings.Repeat("o", 300*1024)
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	sizes := make(chan int64, 4096)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if info, err := os.Stat(path); err == nil {
				select {
				case sizes <- info.Size():
				default:
				}
			}
		}
	}()

	newData := []byte(strings.Repeat("n", 4*1024*1024))
	if err := WriteFileAtomic(path, newData, 0o600); err != nil {
		close(stop)
		<-done
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	close(stop)
	<-done
	close(sizes)

	observed := map[int64]int{}
	for s := range sizes {
		observed[s]++
	}
	for size := range observed {
		if size != int64(len(old)) && size != int64(len(newData)) {
			t.Fatalf("a watcher observed the destination at %d bytes — neither the old file (%d) nor the new one (%d)",
				size, len(old), len(newData))
		}
	}
	if len(observed) == 0 {
		t.Fatal("the watcher observed nothing at all, so this proves nothing")
	}
	assertNoTemporaryFilesLeft(t, dir)
}

// assertNoTemporaryFilesLeft fails if anything but the expected outputs
// is left in dir: WriteFileAtomic must clean up after itself on every
// path, including the ones that fail.
func assertNoTemporaryFilesLeft(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".liro-") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

// TestWriteFileAtomicTellsAReadOnlyFileFromAHeldOneApart is the
// ambiguity the platform check exists to resolve: measured directly,
// Windows returns ERROR_ACCESS_DENIED for a rename onto a read-only
// destination *and* for a rename onto one another program is holding.
// The two need opposite answers — clear an attribute, or close a file —
// so reporting both as "it is open in another program" would send half
// the people who see it looking for a program that is not running.
func TestWriteFileAtomicTellsAReadOnlyFileFromAHeldOneApart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	original := []byte("the previously good signed document")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	restore, marked := markReadOnly(t, path)
	if !marked {
		t.Skip("this platform has no read-only file attribute to confuse with a sharing violation")
	}
	defer restore()

	err := WriteFileAtomic(path, []byte("the new signed document"), 0o600)
	if err == nil {
		t.Fatal("WriteFileAtomic replaced a read-only destination")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("error %v is not an *errs.Error", err)
	}
	if e.Code != errs.CodeOutputInUse {
		// Deliberately the assertion in this direction: a read-only file
		// is a write that could not happen, not a file being held.
		if e.Code != errs.CodeOutputWriteFailed {
			t.Fatalf("Code = %s, want %s", e.Code, errs.CodeOutputWriteFailed)
		}
	} else {
		t.Fatalf("a read-only destination was reported as %s, which sends the person looking for a program that is not running", e.Code)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the destination: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("the original was modified: %q", got)
	}
	assertNoTemporaryFilesLeft(t, dir)
}
