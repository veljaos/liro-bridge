//go:build windows

package main

// The batch actually running (F6 §3, §4, §5): one session, documents in
// order, real signatures on disk, a report that matches them.
//
// This drives runBatch itself — the same function the window calls once
// consent is given — with a consent decision built here rather than
// clicked, because what is being tested is the run, and the consent
// screen has its own tests.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/verify"
)

// batchFixtures copies the blank fixture n times into a fresh folder.
func batchFixtures(t *testing.T, n int) (dir string, paths []string) {
	t.Helper()
	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	dir = t.TempDir()
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, "ugovor-"+itoaTest(i)+".pdf")
		if err := os.WriteFile(p, blank, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return dir, paths
}

// decisionFor builds the consent decision a run needs, with output
// paths settled exactly as askForConsent settles them.
func decisionFor(t *testing.T, m *mainWindow, outDir string) consentDecision {
	t.Helper()
	items := m.queue.Items()
	outputs := make([]interactiveOutput, 0, len(items))
	for _, it := range items {
		outputs = append(outputs, interactiveOutput{
			path: jobs.OutputPathFor(it.Path, outDir, m.cfg.OutputSuffix),
		})
	}
	return consentDecision{
		approved: true,
		session:  newStampSession(t),
		level:    pades.LevelBB,
		allowBB:  true,
		outputs:  outputs,
		cfg:      m.cfg,
	}
}

// assertVerifies runs this project's own independent verifier (SPEC
// §16.4) over a signed document.
func assertVerifies(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no signed document at %s: %v", path, err)
	}
	slots, err := verify.FindSignatures(data)
	if err != nil || len(slots) != 1 {
		t.Fatalf("%s: FindSignatures returned %d slots (err %v)", path, len(slots), err)
	}
	r := verify.VerifySignature(data, slots[0])
	if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
		t.Fatalf("%s: the independent verifier rejected the signature: %+v", path, r.Errors)
	}
}

// TestBatchSignsEveryDocumentAndReportsIt is the phase exit condition in
// miniature: a list goes in, signed documents come out, and the report
// says so.
func TestBatchSignsEveryDocumentAndReportsIt(t *testing.T) {
	_, paths := batchFixtures(t, 6)
	outDir := t.TempDir()

	cfg := config.Default()
	cfg.VisibleStamp = false // the stamp has its own tests
	cfg.OutputFolder = outDir
	m, _ := testMainWindow(t, "sr-Latn", cfg, paths)

	d := decisionFor(t, m, outDir)
	defer func() { _ = d.session.Close() }()
	m.runBatch(context.Background(), d)

	if m.report == nil {
		t.Fatal("the run produced no report")
	}
	if m.report.Succeeded != 6 || m.report.Failed != 0 || m.report.Skipped != 0 {
		t.Fatalf("report = %+v, want six signed", *m.report)
	}
	if m.report.AchievedLevel != string(pades.LevelBB) {
		t.Fatalf("AchievedLevel = %q, want B-B", m.report.AchievedLevel)
	}
	if m.report.OutputDir != filepath.Clean(outDir) {
		t.Fatalf("OutputDir = %q, want %q", m.report.OutputDir, outDir)
	}

	for _, in := range paths {
		assertVerifies(t, jobs.OutputPathFor(in, outDir, cfg.OutputSuffix))
		// F6 §4 / SPEC §18.10: the original is untouched.
		if _, err := os.Stat(in); err != nil {
			t.Fatalf("the input %s is gone: %v", in, err)
		}
	}

	for _, it := range m.queue.Items() {
		if it.State != jobs.StateDone {
			t.Fatalf("%s left in state %q", it.DisplayName, it.State)
		}
		if it.OutputPath == "" {
			t.Fatalf("%s has no output path recorded", it.DisplayName)
		}
	}
}

// TestBatchWritesBesideTheInputByDefault is F6 §4's default.
func TestBatchWritesBesideTheInputByDefault(t *testing.T) {
	dir, paths := batchFixtures(t, 2)

	cfg := config.Default()
	cfg.VisibleStamp = false
	cfg.OutputFolder = "" // the default
	m, _ := testMainWindow(t, "sr-Latn", cfg, paths)

	d := decisionFor(t, m, "")
	defer func() { _ = d.session.Close() }()
	m.runBatch(context.Background(), d)

	if m.report.Succeeded != 2 {
		t.Fatalf("report = %+v", *m.report)
	}
	for _, in := range paths {
		want := filepath.Join(dir, strings.TrimSuffix(filepath.Base(in), ".pdf")+cfg.OutputSuffix+".pdf")
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("no signed document beside its input at %s: %v", want, err)
		}
	}
}

// TestBatchSkipsABadDocumentAndCarriesOn is SPEC §12.10 end to end: the
// bad one is named in the report, the good ones are on disk.
func TestBatchSkipsABadDocumentAndCarriesOn(t *testing.T) {
	_, paths := batchFixtures(t, 4)
	if err := os.WriteFile(paths[2], []byte("ovo nije PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()

	cfg := config.Default()
	cfg.VisibleStamp = false
	cfg.OutputFolder = outDir
	m, _ := testMainWindow(t, "sr-Latn", cfg, paths)

	d := decisionFor(t, m, outDir)
	defer func() { _ = d.session.Close() }()
	m.runBatch(context.Background(), d)

	if m.report.Succeeded != 3 || m.report.Failed != 1 {
		t.Fatalf("report = %+v, want three signed and one failed", *m.report)
	}
	if len(m.report.Failures) != 1 {
		t.Fatalf("failures = %+v", m.report.Failures)
	}
	if m.report.Failures[0].Name != filepath.Base(paths[2]) {
		t.Fatalf("the failure names %q, want %q", m.report.Failures[0].Name, filepath.Base(paths[2]))
	}
	if m.report.Failures[0].Code != errs.CodePDFInvalid {
		t.Fatalf("failure code = %q", m.report.Failures[0].Code)
	}
	bad := jobs.OutputPathFor(paths[2], outDir, cfg.OutputSuffix)
	if _, err := os.Stat(bad); err == nil {
		t.Fatalf("a failed document left a file at %s", bad)
	}
}

// TestStoppingABatchLeavesNoPartialFile is F6 §3's Stop and its "never
// leave a half-written file" rule, end to end.
func TestStoppingABatchLeavesNoPartialFile(t *testing.T) {
	_, paths := batchFixtures(t, 8)
	outDir := t.TempDir()

	cfg := config.Default()
	cfg.VisibleStamp = false
	cfg.OutputFolder = outDir
	m, _ := testMainWindow(t, "sr-Latn", cfg, paths)

	d := decisionFor(t, m, outDir)
	defer func() { _ = d.session.Close() }()

	// Stop once three documents are done, from another goroutine,
	// exactly as the button does.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stopped:
				return
			default:
			}
			r := m.runner
			if r == nil {
				continue
			}
			done := 0
			for _, it := range m.queue.Items() {
				if it.State == jobs.StateDone {
					done++
				}
			}
			if done >= 3 {
				r.Stop()
				return
			}
		}
	}()

	m.runBatch(context.Background(), d)
	<-stopped

	if !m.report.Stopped {
		t.Fatalf("report = %+v, want it marked stopped", *m.report)
	}
	if m.report.Succeeded+m.report.Skipped+m.report.Failed != len(paths) {
		t.Fatalf("report = %+v does not account for all %d documents", *m.report, len(paths))
	}
	// Every output that exists is a complete, verifiable signature. A
	// stopped batch may leave fewer files; it may not leave a broken one.
	for i, in := range paths {
		out := jobs.OutputPathFor(in, outDir, cfg.OutputSuffix)
		if _, err := os.Stat(out); err != nil {
			continue
		}
		assertVerifies(t, out)
		_ = i
	}
}

// TestBatchReportExportNamesTheDocuments is F6 §5's export. It is for
// the person, so it carries file names — which the audit log
// deliberately never does (SPEC §6.7), and which is why the two are
// separate things.
func TestBatchReportExportNamesTheDocuments(t *testing.T) {
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)

	text := m.reportText(jobs.Report{
		Succeeded:     2,
		Failed:        1,
		OutputDir:     `D:\potpisano`,
		AchievedLevel: "B-T",
		Failures: []jobs.Failure{
			{Name: "Уговор о раду.pdf", Code: errs.CodePDFEncrypted},
		},
	})
	for _, want := range []string{"Уговор о раду.pdf", `D:\potpisano`, "B-T"} {
		if !strings.Contains(text, want) {
			t.Errorf("the exported report does not mention %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, string(errs.CodePDFEncrypted)) {
		t.Errorf("the exported report contains an error code:\n%s", text)
	}
}
