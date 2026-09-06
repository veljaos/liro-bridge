//go:build windows

package main

// F6 §7, "Things people do": every case gets a test, and the standard
// for each is a clear message and a clean state — never a crash, never
// a silent skip, never a half-written file left on disk.
//
// The cases about the *queue* (two hundred files, the same file twice,
// a corrupt file among a hundred, a card removed at fifty, Stop, a
// closing window, a disk filling) live in internal/jobs, where they are
// ordinary tests on any platform. The cases here are the ones that need
// a real document, a real signing session and this package's own
// signing step — which is where a bad input actually gets recognised
// and named.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"golang.org/x/sys/windows"
)

// signOneForTest runs this package's real signing step against in,
// writing to out, at B-B with no timestamp and no stamp — the smallest
// configuration that still exercises the whole path.
func signOneForTest(t *testing.T, in, out string) error {
	t.Helper()
	_, err := signInteractiveOne(t.Context(), in, newStampSession(t), interactiveSignOptions{
		level:   pades.LevelBB,
		outPath: out,
		allowBB: true,
	})
	return err
}

// codeFor is the code a failure carries, or "" for success.
func codeFor(err error) errs.Code {
	if err == nil {
		return ""
	}
	return codeOfInteractive(err)
}

// assertNamedNotInternal is F6 §7's standard: whatever went wrong, the
// user is told what it was, and never "an unexpected error occurred".
func assertNamedNotInternal(t *testing.T, err error, want errs.Code) {
	t.Helper()
	if err == nil {
		t.Fatal("the operation succeeded; it should have been refused")
	}
	got := codeFor(err)
	if got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got == errs.CodeInternal {
		t.Fatal("a recognised condition was reported as an unexpected error")
	}
	// And it reaches the person as a sentence, in every locale.
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		msg := cliErrorMessage(c, got)
		if strings.TrimSpace(msg) == "" || strings.HasPrefix(msg, "error.") {
			t.Fatalf("%s has no message for %q: %q", locale, got, msg)
		}
		if msg == c.T("error.internal") {
			t.Fatalf("%s renders %q as the unexpected-error message", locale, got)
		}
	}
}

// assertNothingWritten is the other half of the standard: a refused
// document leaves nothing behind.
func assertNothingWritten(t *testing.T, out string) {
	t.Helper()
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("a refused document still left a file at %s", out)
	}
}

func TestZeroByteFileIsNamedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "prazan.pdf")
	if err := os.WriteFile(in, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "prazan-potpisan.pdf")

	err := signOneForTest(t, in, out)
	assertNamedNotInternal(t, err, errs.CodePDFInvalid)
	assertNothingWritten(t, out)
}

func TestNotAPDFDespiteTheExtensionIsNamedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "ugovor.pdf")
	if err := os.WriteFile(in, []byte("This is a Word document, whatever the name says."), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ugovor-potpisan.pdf")

	err := signOneForTest(t, in, out)
	assertNamedNotInternal(t, err, errs.CodePDFInvalid)
	assertNothingWritten(t, out)
}

// TestDeliberatelyChosenNonPDFIsReportedByName is F6 §1's other half:
// the file is not filtered out of the list, so the signing step is what
// names it — and it does, with the same code any other unparseable
// document gets.
func TestDeliberatelyChosenNonPDFIsReportedByName(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "beleške.txt")
	if err := os.WriteFile(in, []byte("ovo nije PDF"), 0o600); err != nil {
		t.Fatal(err)
	}

	var q jobs.Queue
	if added, _ := q.Add([]string{in}); added != 1 {
		t.Fatal("a deliberately chosen non-PDF was dropped from the list")
	}

	err := signOneForTest(t, in, filepath.Join(dir, "beleške-potpisan.txt"))
	assertNamedNotInternal(t, err, errs.CodePDFInvalid)

	// And a report of that batch names the file, not the code.
	report := jobs.Report{Failed: 1, Failures: []jobs.Failure{
		{Name: q.Items()[0].DisplayName, Code: codeFor(err)},
	}}
	if report.Failures[0].Name != "beleške.txt" {
		t.Fatalf("the report names %q", report.Failures[0].Name)
	}
}

func TestEncryptedPDFIsNamedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "zaključan.pdf")
	// The smallest document whose trailer declares /Encrypt, which is
	// exactly what internal/pades/pdf checks before anything else
	// (D-043).
	body := "%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n" +
		"trailer\n<< /Size 2 /Root 1 0 R /Encrypt 2 0 R >>\nstartxref\n0\n%%EOF"
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "zaključan-potpisan.pdf")

	err := signOneForTest(t, in, out)
	assertNamedNotInternal(t, err, errs.CodePDFEncrypted)
	assertNothingWritten(t, out)
}

// TestFileLockedByAnotherProgramIsNamedAndSkipped is F6 §7's "file
// locked by another program, open in Acrobat". Acrobat holds the file
// open without FILE_SHARE_READ while a document is open for editing,
// which is what this reproduces — a Go os.Open cannot express that, so
// the handle is opened through CreateFile with no sharing at all.
func TestFileLockedByAnotherProgramIsNamedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "otvoren-u-acrobatu.pdf")
	if err := os.WriteFile(in, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := openExclusive(t, in)
	defer func() { _ = windows.CloseHandle(h) }()

	out := filepath.Join(dir, "otvoren-u-acrobatu-potpisan.pdf")
	err := signOneForTest(t, in, out)
	assertNamedNotInternal(t, err, errs.CodeInputUnreadable)
	assertNothingWritten(t, out)
}

// openExclusive opens path with no sharing, the way an editor holding a
// document open does.
func openExclusive(t *testing.T, path string) windows.Handle {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE,
		0 /* no sharing at all */, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("could not take an exclusive handle on %s: %v", path, err)
	}
	return h
}

// TestFolderWhereAFileWasExpectedIsNamedAndSkipped is F6 §7's "a folder
// dropped where a file was expected". A folder dropped on the *window*
// is expanded into its PDFs (F6 §1, covered in internal/jobs); this is
// the other route — one reaching the signing step directly, from the
// command line or a stale list.
func TestFolderWhereAFileWasExpectedIsNamedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "dokumenti")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "dokumenti-potpisan")

	err := signOneForTest(t, folder, out)
	assertNamedNotInternal(t, err, errs.CodeInputUnreadable)
	assertNothingWritten(t, out)
}

// TestMissingFileIsNamedAndSkipped covers a network drive disappearing
// mid-batch, at the point the batch notices: the document that was
// there when the list was built is not there when its turn comes.
func TestMissingFileIsNamedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "nestao.pdf")
	out := filepath.Join(dir, "nestao-potpisan.pdf")

	err := signOneForTest(t, in, out)
	assertNamedNotInternal(t, err, errs.CodeInputUnreadable)
	assertNothingWritten(t, out)
}

// TestOutputHeldOpenIsNamedAndTheOriginalSurvives is F6 §7's "output
// file opened in Acrobat while being written": the write fails, and
// what was already there must be exactly as it was.
//
// It says OUTPUT_IN_USE now rather than the generic OUTPUT_WRITE_FAILED
// it said while the signed bytes went straight onto the destination
// with os.WriteFile (J-8). That is not a relabelling: the two situations
// need different things from the person — close a file, versus look at
// the disk — and this one has a specific, actionable answer, which is
// the whole reason the code exists.
func TestOutputHeldOpenIsNamedAndTheOriginalSurvives(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	out := filepath.Join(dir, "blank-potpisan.pdf")

	// An existing output, held open with no sharing — an editor with
	// the previous signed copy open.
	original := []byte("prethodna verzija koja mora da preživi")
	if err := os.WriteFile(out, original, 0o600); err != nil {
		t.Fatal(err)
	}
	h := openExclusive(t, out)
	defer func() { _ = windows.CloseHandle(h) }()

	// overwrite: true, so the existing-file question is already
	// answered and the disk is what refuses.
	_, err := signInteractiveOne(t.Context(), in, newStampSession(t), interactiveSignOptions{
		level:     pades.LevelBB,
		outPath:   out,
		allowBB:   true,
		overwrite: true,
	})
	assertNamedNotInternal(t, err, errs.CodeOutputInUse)
	if msg := cli.ErrorMessage(err, i18n.Load("sr-Latn")); !strings.Contains(strings.ToLower(msg), "zatvorite") {
		t.Errorf("the message does not say to close the file: %q", msg)
	}

	_ = windows.CloseHandle(h)
	got, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatalf("the existing output is gone: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("the existing output was changed: %q", string(got))
	}
}

// TestReadOnlyOutputIsNamedAndTheOriginalSurvives is F6 §7's read-only
// case, kept separate from the held-open one above because Windows
// returns the same status for both and they need opposite answers:
// clear an attribute, or close a file.
func TestReadOnlyOutputIsNamedAndTheOriginalSurvives(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	out := filepath.Join(dir, "blank-potpisan.pdf")

	original := []byte("prethodna verzija koja mora da preživi")
	if err := os.WriteFile(out, original, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetFileAttributes(p, windows.FILE_ATTRIBUTE_READONLY); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.SetFileAttributes(p, windows.FILE_ATTRIBUTE_NORMAL) }()

	_, signErr := signInteractiveOne(t.Context(), in, newStampSession(t), interactiveSignOptions{
		level:     pades.LevelBB,
		outPath:   out,
		allowBB:   true,
		overwrite: true,
	})
	assertNamedNotInternal(t, signErr, errs.CodeOutputWriteFailed)

	got, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatalf("the existing output is gone: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("the existing output was changed: %q", string(got))
	}
}

// TestAnExistingOutputIsNeverSilentlyOverwritten is SPEC §18.10, at the
// signing step: without an explicit answer, the file that is there
// stays there.
func TestAnExistingOutputIsNeverSilentlyOverwritten(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	out := filepath.Join(dir, "blank-potpisan.pdf")
	original := []byte("ovo ne sme da se izgubi")
	if err := os.WriteFile(out, original, 0o600); err != nil {
		t.Fatal(err)
	}

	err := signOneForTest(t, in, out)
	assertNamedNotInternal(t, err, errs.CodeOutputExists)

	got, readErr := os.ReadFile(out)
	if readErr != nil || string(got) != string(original) {
		t.Fatalf("the existing file was disturbed: %q (err %v)", string(got), readErr)
	}
}

// TestAlreadySignedDocumentSignsAgain is F6 §7's "already signed — must
// work, this is the normal case". The real fixtures are gitignored
// (D-038), so this runs against them when present and against the blank
// fixture signed twice otherwise — which exercises the same incremental
// path this project itself produced.
func TestAlreadySignedDocumentSignsAgain(t *testing.T) {
	dir := t.TempDir()
	blank := filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf")
	if _, err := os.Stat(blank); err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}

	once := filepath.Join(dir, "jednom.pdf")
	if err := signOneForTest(t, blank, once); err != nil {
		t.Fatalf("the first signature failed: %v", err)
	}
	twice := filepath.Join(dir, "dvaput.pdf")
	if err := signOneForTest(t, once, twice); err != nil {
		t.Fatalf("signing an already-signed document failed: %v", err)
	}

	first, err := os.ReadFile(once)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(twice)
	if err != nil {
		t.Fatal(err)
	}
	// F3 §3.1 and SPEC §16.3's most important property: the earlier
	// revision is untouched, byte for byte.
	if len(second) <= len(first) {
		t.Fatalf("the re-signed document is %d bytes, not longer than the %d it started at", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("the original bytes were disturbed at offset %d", i)
		}
	}
}

// TestTheSameFileListedTwiceIsOneDocument is F6 §7's duplicate case,
// reaching the window through the path a person actually uses.
//
// The previous version of this test handed all three paths to the queue
// in one call, as the command line's initial paths, and passed. That is
// not what a person does: they drop a file, look at the list, and drop
// it again — one Add call each time, minutes apart, with a render in
// between. Both shapes are covered here, and so is the shape that
// produced the report, which is neither of them (below).
func TestTheSameFileListedTwiceIsOneDocument(t *testing.T) {
	dir := t.TempDir()
	in := writeTestPDF(t, dir, "ugovor.pdf", 10)
	c := i18n.Load("sr-Latn")

	rows := func(m *mainWindow) float64 {
		return evalNumber(t, m.win, "document.querySelectorAll('#file-list .file-row').length")
	}

	t.Run("dropped again, later", func(t *testing.T) {
		m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)
		m.addPaths([]string{in})
		m.addPaths([]string{in})
		// The refusal is said out loud, naming the file that was
		// dropped — the whole point of the check being visible.
		notices := evalText(t, m.win, "document.getElementById('notices').textContent")
		if want := fmt.Sprintf(c.T("main.notice_duplicate"), "ugovor.pdf"); !strings.Contains(notices, want) {
			t.Fatalf("the window says %q; it should say %q", notices, want)
		}
		// A Windows path differing only in case is the same document.
		m.addPaths([]string{strings.ToUpper(in)})
		if m.queue.Len() != 1 {
			t.Fatalf("the queue holds %d documents for one file dropped three times", m.queue.Len())
		}
		if got := rows(m); got != 1 {
			t.Fatalf("the list shows %v rows for one document", got)
		}
	})

	t.Run("named twice in one drop", func(t *testing.T) {
		m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)
		m.addPaths([]string{in, in, strings.ToUpper(in)})
		if m.queue.Len() != 1 {
			t.Fatalf("the queue holds %d documents for one file named three times in one drop", m.queue.Len())
		}
		if got := rows(m); got != 1 {
			t.Fatalf("the list shows %v rows for one document", got)
		}
	})

	t.Run("a folder and a file inside it in one drop", func(t *testing.T) {
		m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)
		m.addPaths([]string{dir, in})
		if m.queue.Len() != 1 {
			t.Fatalf("the queue holds %d documents for one file reached two ways", m.queue.Len())
		}
		if got := rows(m); got != 1 {
			t.Fatalf("the list shows %v rows for one document", got)
		}
	})
}

// TestTwoDocumentsWithOneNameAreBothKeptAndBothLegible is the case the
// duplicate report was actually about.
//
// Two files called "ugovor.pdf" in two folders are two documents and
// both belong in the list — the queue compares full paths, which is
// right, and neither is refused. What went wrong is what that looked
// like: two rows reading "ugovor.pdf", one above the other, next to a
// message about a *different* file already being in the list. From the
// outside that is indistinguishable from a duplicate check that ran and
// was ignored, which is how it was reported.
//
// So the check is not what changed. What changed is that a name which
// does not identify a document in this list is no longer the only thing
// shown about it.
func TestTwoDocumentsWithOneNameAreBothKeptAndBothLegible(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "Klijent A")
	second := filepath.Join(root, "Klijent B")
	for _, d := range []string{first, second} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := writeTestPDF(t, first, "ugovor.pdf", 10)
	b := writeTestPDF(t, second, "ugovor.pdf", 20)
	alone := writeTestPDF(t, root, "izjava.pdf", 30)

	m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)
	m.addPaths([]string{a, b, alone})

	if m.queue.Len() != 3 {
		t.Fatalf("the queue holds %d documents; two files with one name in two folders are two documents", m.queue.Len())
	}
	if got := evalNumber(t, m.win, "document.querySelectorAll('#file-list .file-row').length"); got != 3 {
		t.Fatalf("the list shows %v rows for three documents", got)
	}

	// The two rows that share a name each say which folder they came
	// from; the one whose name is already unambiguous does not.
	folders := evalNumber(t, m.win, "document.querySelectorAll('#file-list .file-folder').length")
	if folders != 2 {
		t.Fatalf("%v rows carry a folder; exactly the two sharing a name should", folders)
	}
	text := evalText(t, m.win, "document.getElementById('file-list').textContent")
	for _, want := range []string{"Klijent A", "Klijent B"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the list does not say which %q is which; it reads %q", "ugovor.pdf", text)
		}
	}
	// And nothing was reported as a duplicate, because nothing was.
	if notices := evalText(t, m.win, "document.getElementById('notices').textContent"); notices != "" {
		t.Fatalf("two different documents produced a notice: %q", notices)
	}

	// The queue screen is the same list one screen later, and follows
	// the same rule: watching two rows called "ugovor.pdf" and being
	// told one of them failed is this defect happening two seconds on.
	if err := m.win.PostJSON(m.queuePayload(m.queue.Items(),
		jobs.Progress{Phase: jobs.PhaseSigning, Current: 1, Total: 3}, false)); err != nil {
		t.Fatal(err)
	}
	if got := evalNumber(t, m.win, "document.querySelectorAll('#queue-list .file-folder').length"); got != 2 {
		t.Fatalf("%v rows of the queue carry a folder; exactly the two sharing a name should", got)
	}
}

// TestCertificateSelectionDoesNotPersistBetweenRuns is F6 §7's own
// closing requirement and SPEC §18.15: on a machine holding several
// clients' certificates, a remembered default is a wrong-signer
// incident waiting to happen.
//
// Two properties, checked separately. There is nowhere to persist a
// certificate — the same structural guarantee D-084 gave the audit
// entry — and the consent window offers no pre-selected one however
// many usable certificates it is handed.
func TestCertificateSelectionDoesNotPersistBetweenRuns(t *testing.T) {
	cfg := config.Default()
	cfg.TSAURL = "https://tsa.example.rs"
	cfg.StampReference = "Ugovor 2026/114"

	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"thumbprint", "certificate", "signer", "lastused"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("the configuration file has a %q field; SPEC §18.15 forbids remembering a certificate", forbidden)
		}
	}

	// And the consent window, given two usable certificates — SPEC
	// §11.5's own situation — preselects neither, and will not let
	// Approve be pressed until one is chosen.
	c := i18n.Load("sr-Latn")
	usable := func(tp, name string) classify.Info {
		return classify.Info{
			Thumbprint:    tp,
			Subject:       classify.Subject{DisplayName: name},
			IssuerCN:      "MUP Gradjani CA 4",
			Qualification: classify.QualificationQualified,
			Purpose:       classify.PurposeSigning,
			Usable:        true,
		}
	}
	vm := consent.BuildViewModel(consent.ApplicationLocal, [][]byte{{1}}, []string{"ugovor.pdf"},
		[]classify.Info{
			usable("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "Prvi Klijent"),
			usable("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", "Drugi Klijent"),
		})

	win, _ := sharedConsentWindow(t)
	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		t.Fatalf("PostJSON(consent init): %v", err)
	}

	if !evalBool(t, win, "document.getElementById('approve-btn').disabled") {
		t.Fatal("Approve is pressable before any certificate has been chosen")
	}

	// Choosing one, then starting a new batch, must not carry the
	// choice over. This is the "does not persist between runs" half:
	// the window is the same one, the batch is a different one.
	if _, err := win.Eval("document.querySelectorAll('#cert-list .liro-cert-row')[0].click()"); err != nil {
		t.Fatalf("Eval(choose a certificate): %v", err)
	}
	if evalBool(t, win, "document.getElementById('approve-btn').disabled") {
		t.Fatal("Approve is still disabled after a certificate was chosen")
	}
	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		t.Fatalf("PostJSON(second batch): %v", err)
	}
	if !evalBool(t, win, "document.getElementById('approve-btn').disabled") {
		t.Fatal("a new batch kept the previous batch's certificate choice; SPEC §18.15 forbids it")
	}
	selected := evalNumber(t, win, "document.querySelectorAll('#cert-list [aria-selected=\"true\"]').length")
	if selected != 0 {
		t.Fatalf("%v certificate rows are preselected; SPEC §18.15 forbids a remembered default", selected)
	}
}
