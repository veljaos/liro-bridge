package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file with n bytes of content and returns its path.
func writeFile(t *testing.T, dir, name string, n int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func TestAddFilesKeepsOrderAndReadsSizes(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.pdf", 10)
	b := writeFile(t, dir, "b.pdf", 20)

	var q Queue
	added, notices := q.Add([]string{a, b})
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	if len(notices) != 0 {
		t.Fatalf("notices = %v, want none", notices)
	}
	items := q.Items()
	if items[0].DisplayName != "a.pdf" || items[1].DisplayName != "b.pdf" {
		t.Fatalf("order not preserved: %q, %q", items[0].DisplayName, items[1].DisplayName)
	}
	if !items[0].SizeKnown || items[0].Size != 10 {
		t.Fatalf("size = %d (known %t), want 10", items[0].Size, items[0].SizeKnown)
	}
	total, complete := q.TotalSize()
	if total != 30 || !complete {
		t.Fatalf("TotalSize = %d, %t; want 30, true", total, complete)
	}
}

// TestAddFolderTakesItsPDFsWithoutRecursing is F6 §1's folder rule: the
// PDFs directly inside, a count to report, and nothing from a subfolder.
func TestAddFolderTakesItsPDFsWithoutRecursing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "second.pdf", 1)
	writeFile(t, dir, "first.pdf", 1)
	writeFile(t, dir, "notes.txt", 1)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "buried.pdf", 1)

	var q Queue
	added, notices := q.Add([]string{dir})
	if added != 2 {
		t.Fatalf("added = %d, want 2 (the two PDFs directly inside)", added)
	}
	if len(notices) != 1 || notices[0].Kind != NoticeFolderScanned || notices[0].Count != 2 {
		t.Fatalf("notices = %+v, want one folderScanned with Count 2", notices)
	}
	names := q.DisplayNames()
	if names[0] != "first.pdf" || names[1] != "second.pdf" {
		t.Fatalf("names = %v, want them sorted", names)
	}
	for _, n := range names {
		if n == "buried.pdf" {
			t.Fatal("recursed into a subfolder")
		}
		if n == "notes.txt" {
			t.Fatal("took a non-PDF out of a folder")
		}
	}
}

func TestAddFolderWithNoPDFsSaysSo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", 1)

	var q Queue
	added, notices := q.Add([]string{dir})
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}
	if len(notices) != 1 || notices[0].Kind != NoticeFolderEmpty {
		t.Fatalf("notices = %+v, want one folderEmpty", notices)
	}
}

// TestDeliberatelyDroppedNonPDFIsKept is F6 §1's explicit instruction:
// a file the user chose is never silently discarded. It joins the queue
// and the signing step is what names the problem.
func TestDeliberatelyDroppedNonPDFIsKept(t *testing.T) {
	dir := t.TempDir()
	doc := writeFile(t, dir, "contract.docx", 5)

	var q Queue
	added, notices := q.Add([]string{doc})
	if added != 1 {
		t.Fatalf("added = %d, want 1 — a deliberately chosen file is not filtered", added)
	}
	if len(notices) != 0 {
		t.Fatalf("notices = %+v, want none", notices)
	}
	if q.Items()[0].DisplayName != "contract.docx" {
		t.Fatalf("DisplayName = %q", q.Items()[0].DisplayName)
	}
}

// TestSameFileTwiceIsOneDocument is F6 §7's "the same file listed
// twice". Signing it twice would produce two signed copies of one
// document, the second working around the first's output name.
func TestSameFileTwiceIsOneDocument(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.pdf", 1)

	var q Queue
	if added, _ := q.Add([]string{a}); added != 1 {
		t.Fatal("first add did not add")
	}
	added, notices := q.Add([]string{a})
	if added != 0 {
		t.Fatalf("added = %d on a repeat, want 0", added)
	}
	if len(notices) != 1 || notices[0].Kind != NoticeDuplicate {
		t.Fatalf("notices = %+v, want one duplicate", notices)
	}
	if q.Len() != 1 {
		t.Fatalf("Len = %d, want 1", q.Len())
	}
}

// TestDuplicateIgnoresCaseAndSeparators covers the ordinary Windows
// way of arriving at one file twice: once from a folder scan, once
// dropped by hand from a search result.
func TestDuplicateIgnoresCaseAndSeparators(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "Ugovor.pdf", 1)

	var q Queue
	q.Add([]string{a})
	added, _ := q.Add([]string{strings.ToUpper(a)})
	if added != 0 {
		t.Fatalf("added = %d for the same path in a different case, want 0", added)
	}
}

// TestDroppedFolderAndItsFilesDoNotDouble is the same rule reached the
// other way: a person drops a folder and then one of its files.
func TestDroppedFolderAndItsFilesDoNotDouble(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.pdf", 1)
	writeFile(t, dir, "b.pdf", 1)

	var q Queue
	q.Add([]string{dir})
	added, notices := q.Add([]string{a})
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}
	if len(notices) != 1 || notices[0].Kind != NoticeDuplicate {
		t.Fatalf("notices = %+v", notices)
	}
	if q.Len() != 2 {
		t.Fatalf("Len = %d, want 2", q.Len())
	}
}

func TestAddMissingPathIsReportedNotDropped(t *testing.T) {
	var q Queue
	added, notices := q.Add([]string{filepath.Join(t.TempDir(), "gone.pdf")})
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}
	if len(notices) != 1 || notices[0].Kind != NoticeUnreadable {
		t.Fatalf("notices = %+v, want one unreadable", notices)
	}
	if notices[0].Name != "gone.pdf" {
		t.Fatalf("Name = %q, want the file named", notices[0].Name)
	}
}

func TestRemoveTakesOneOut(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.pdf", 1)
	b := writeFile(t, dir, "b.pdf", 1)

	var q Queue
	q.Add([]string{a, b})
	if !q.Remove(a) {
		t.Fatal("Remove reported the file was not there")
	}
	if q.Len() != 1 || q.Items()[0].DisplayName != "b.pdf" {
		t.Fatalf("after Remove: %v", q.DisplayNames())
	}
	if q.Remove(a) {
		t.Fatal("Remove reported success twice for one file")
	}
	// Removing must also forget the duplicate key, or a file cannot be
	// added back after being taken out by mistake.
	if added, _ := q.Add([]string{a}); added != 1 {
		t.Fatal("a removed file could not be added again")
	}
}

// TestNamesWithCyrillicSpacesAndEmojiSurvive is F6 §7's naming case.
// The name is sanitised for display but never mangled beyond that, and
// the path it came from is untouched.
func TestNamesWithCyrillicSpacesAndEmojiSurvive(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"Уговор о раду.pdf",
		"faktura 2026 🎉.pdf",
		"Račun čćžšđ.pdf",
	}
	var paths []string
	for _, n := range names {
		paths = append(paths, writeFile(t, dir, n, 1))
	}

	var q Queue
	added, notices := q.Add(paths)
	if added != 3 {
		t.Fatalf("added = %d, want 3; notices %+v", added, notices)
	}
	for i, item := range q.Items() {
		if item.DisplayName != names[i] {
			t.Errorf("DisplayName = %q, want %q", item.DisplayName, names[i])
		}
		if _, err := os.Stat(item.Path); err != nil {
			t.Errorf("stored path is not usable: %v", err)
		}
	}
}

// TestDirectionOverrideInNameIsStripped is SPEC §6.6's spoofing case,
// reaching the queue rather than the consent screen. The file is still
// added — it is a real file the user chose — but the name it is shown
// under cannot lie about its extension.
func TestDirectionOverrideInNameIsStripped(t *testing.T) {
	dir := t.TempDir()
	// "invoice<U+202E>fdp.exe" renders as "invoice exe.pdf" unless the
	// override is removed.
	raw := "invoice\u202Efdp.exe"
	path := writeFile(t, dir, raw, 1)

	var q Queue
	if added, n := q.Add([]string{path}); added != 1 {
		t.Fatalf("added = %d, notices %+v", added, n)
	}
	name := q.Items()[0].DisplayName
	if strings.ContainsRune(name, '\u202E') {
		t.Fatalf("DisplayName still carries the direction override: %q", name)
	}
	if !strings.HasSuffix(name, ".exe") {
		t.Fatalf("DisplayName = %q, want it to end in the real extension", name)
	}
}

func TestOutputPathDefaultsBesideTheInput(t *testing.T) {
	got := OutputPathFor(filepath.Join("C:", "docs", "ugovor.pdf"), "", "-signed")
	want := filepath.Join("C:", "docs", "ugovor-signed.pdf")
	if got != want {
		t.Fatalf("OutputPathFor = %q, want %q", got, want)
	}
}

func TestOutputPathUsesChosenFolder(t *testing.T) {
	got := OutputPathFor(filepath.Join("C:", "docs", "ugovor.pdf"), filepath.Join("D:", "out"), "-potpisan")
	want := filepath.Join("D:", "out", "ugovor-potpisan.pdf")
	if got != want {
		t.Fatalf("OutputPathFor = %q, want %q", got, want)
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1000, "1.0 kB"},
		{9999, "10.0 kB"},
		{10000, "10 kB"},
		{340000, "340 kB"},
		{1200000, "1.2 MB"},
		{673111, "673 kB"},
	}
	for _, tc := range cases {
		if got := FormatSize(tc.in); got != tc.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTwoHundredFilesAtOnce is F6 §7's volume case. The point is not
// speed; it is that nothing about the queue is quadratic or drops
// entries at scale.
func TestTwoHundredFilesAtOnce(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 200; i++ {
		paths = append(paths, writeFile(t, dir, filepath.Base(filepath.Join(dir, itoa(i)+".pdf")), 1))
	}
	var q Queue
	added, notices := q.Add(paths)
	if added != 200 {
		t.Fatalf("added = %d, want 200 (notices %d)", added, len(notices))
	}
	if q.Len() != 200 {
		t.Fatalf("Len = %d, want 200", q.Len())
	}
	// And the same 200 again is 200 duplicates, not 200 more documents.
	added, notices = q.Add(paths)
	if added != 0 || len(notices) != 200 {
		t.Fatalf("second add: added = %d, notices = %d", added, len(notices))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestOutputPathsFollowEachInputsOwnFolder is F6 §4's rule for the case
// it exists for: a batch gathered from several folders — dropped from
// two Explorer windows, or picked out of a folder tree — writes each
// signature beside its own input, not all of them into whichever folder
// the first document happened to come from.
//
// It is here rather than only at the window layer because it is the
// answer to a question about paths, and a batch from several folders is
// the ordinary case for a drop.
func TestOutputPathsFollowEachInputsOwnFolder(t *testing.T) {
	inputs := []string{
		filepath.Join("C:", "Ugovori", "2026", "ugovor.pdf"),
		filepath.Join("D:", "Racuni", "faktura.pdf"),
		filepath.Join("C:", "Users", "Veljko", "Desktop", "izjava.pdf"),
	}
	want := []string{
		filepath.Join("C:", "Ugovori", "2026", "ugovor-signed.pdf"),
		filepath.Join("D:", "Racuni", "faktura-signed.pdf"),
		filepath.Join("C:", "Users", "Veljko", "Desktop", "izjava-signed.pdf"),
	}
	for i, in := range inputs {
		if got := OutputPathFor(in, "", "-signed"); got != want[i] {
			t.Errorf("OutputPathFor(%q, \"\") = %q, want %q", in, got, want[i])
		}
	}

	// A chosen folder overrides every one of them, and only then.
	chosen := filepath.Join("E:", "Potpisano")
	for _, in := range inputs {
		got := OutputPathFor(in, chosen, "-signed")
		if filepath.Dir(got) != chosen {
			t.Errorf("with a chosen folder, OutputPathFor(%q) = %q, want it under %q", in, got, chosen)
		}
	}
}

// TestNeedsFolderOnlyWhereTheNameIsNotEnough covers the other half of
// comparing by full path: two documents called "ugovor.pdf" in two
// folders are two documents, both are kept, and the list has to be able
// to say which is which. Every other row is left alone — a batch from
// one folder must not carry that folder on every line to make a case
// that is not happening legible.
func TestNeedsFolderOnlyWhereTheNameIsNotEnough(t *testing.T) {
	items := []Item{
		{DisplayName: "ugovor.pdf", Folder: `C:\Klijenti\A`},
		{DisplayName: "izjava.pdf", Folder: `C:\Klijenti\A`},
		{DisplayName: "UGOVOR.pdf", Folder: `C:\Klijenti\B`},
		{DisplayName: "racun.pdf", Folder: `C:\Klijenti\B`},
	}
	got := NeedsFolder(items)
	want := []bool{true, false, true, false}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d (%q): NeedsFolder = %v, want %v", i, items[i].DisplayName, got[i], want[i])
		}
	}
}

// TestItemCarriesItsFolder: the folder is recorded when a document
// enters the queue, sanitised the same way its name is, so nothing
// downstream has to reach for the path to render a row.
func TestItemCarriesItsFolder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ugovor.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var q Queue
	if added, _ := q.Add([]string{path}); added != 1 {
		t.Fatalf("Add reported %d added", added)
	}
	it := q.Items()[0]
	if it.Folder != dir {
		t.Fatalf("Item.Folder = %q, want %q", it.Folder, dir)
	}
	if it.DisplayName != "ugovor.pdf" {
		t.Fatalf("Item.DisplayName = %q", it.DisplayName)
	}
}

// TestLooksLikeOutputRecognisesThisProgramsOwnOutput covers J-3's rule:
// which inputs already carry the configured output suffix.
func TestLooksLikeOutputRecognisesThisProgramsOwnOutput(t *testing.T) {
	cases := []struct {
		in     string
		suffix string
		want   bool
	}{
		{`C:\docs\ugovor-signed.pdf`, "-signed", true},
		{`C:\docs\ugovor-signed-signed.pdf`, "-signed", true},
		{`C:\docs\ugovor.pdf`, "-signed", false},
		{`C:\docs\signed-ugovor.pdf`, "-signed", false},
		// Windows file names are case-insensitive, so the comparison is.
		{`C:\docs\UGOVOR-SIGNED.PDF`, "-signed", true},
		// A configured suffix other than the default works the same way.
		{`C:\docs\ugovor-potpisan.pdf`, "-potpisan", true},
		{`C:\docs\ugovor-potpisan.pdf`, "-signed", false},
		// An empty suffix would make every input look like an output;
		// config.validate never produces one, and this is the floor.
		{`C:\docs\ugovor.pdf`, "", false},
		// The suffix is matched against the name, never the extension.
		{`C:\docs\ugovor.signed`, "-signed", false},
	}
	for _, c := range cases {
		if got := LooksLikeOutput(c.in, c.suffix); got != c.want {
			t.Errorf("LooksLikeOutput(%q, %q) = %v, want %v", c.in, c.suffix, got, c.want)
		}
	}
}

// TestLooksLikeOutputAgreesWithOutputPathFor is the property that makes
// the rule right rather than merely plausible: an input this says looks
// like an output is exactly one that OutputPathFor would turn into a
// doubly-suffixed name.
func TestLooksLikeOutputAgreesWithOutputPathFor(t *testing.T) {
	const suffix = "-signed"
	in := filepath.Join(`C:\docs`, "ugovor.pdf")
	out := OutputPathFor(in, "", suffix)
	if LooksLikeOutput(in, suffix) {
		t.Fatalf("%q was taken for an output", in)
	}
	if !LooksLikeOutput(out, suffix) {
		t.Fatalf("%q, which this program just produced, was not taken for an output", out)
	}
	if again := OutputPathFor(out, "", suffix); !strings.Contains(filepath.Base(again), "-signed-signed") {
		t.Fatalf("signing the output again produced %q, which is not the doubling this rule is about", again)
	}
}
