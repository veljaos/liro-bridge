// Package jobs holds the list of documents a person has gathered to
// sign and, once they approve, runs it (F6 §1, §3, §4, §5).
//
// Everything here is plain Go with no window and no card: the queue's
// ordering, its duplicate and folder handling, its skip-and-continue
// rule, its stop behaviour and its ETA arithmetic are all exercised by
// ordinary tests on any platform. The Windows main window serialises a
// snapshot of this state to its page and calls Run; it makes none of
// these decisions itself. That split is the same one F5 §10 required of
// the consent screen, applied to the surface F6 adds.
package jobs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
)

// State is where one document has got to. Exactly the five F6 §3 names.
type State string

const (
	StateWaiting State = "waiting"
	StateSigning State = "signing"
	StateDone    State = "done"
	StateFailed  State = "failed"
	// StateSkipped is a document the batch never reached — because it
	// was aborted (card removed, PIN blocked) or stopped by the user.
	// It is distinct from StateFailed, which is a document that was
	// attempted and did not work.
	StateSkipped State = "skipped"
)

// pdfExtension is the only extension a folder scan picks up (F6 §1).
const pdfExtension = ".pdf"

// Item is one document in the queue.
type Item struct {
	// Path is the absolute path on disk. It is never rendered: a name
	// shown to a person always comes from DisplayName, which has been
	// through F5 §5.3's sanitiser.
	Path string

	// DisplayName is the sanitised, truncated base name — safe to put
	// on screen, never safe to open.
	DisplayName string

	// Folder is the folder the document is in, in full — what tells
	// two documents apart when DisplayName cannot, which is every time
	// a person gathers "ugovor.pdf" from two clients' folders. The
	// whole path rather than its last segment, because two folders can
	// share a last segment as easily as two documents share a name.
	//
	// Sanitised and truncated through the same pipeline as
	// DisplayName — a path is no more trustworthy than a file name —
	// and shown only where it is needed (see NeedsFolder).
	Folder string

	// Size is the file's size in bytes at the time it was added, and
	// SizeKnown says whether it could be read at all. A file that has
	// disappeared or cannot be stat'd is still listed, with SizeKnown
	// false: F6 §1 is explicit that a file the user chose deliberately
	// is not silently dropped, and the signing step is what names the
	// problem.
	Size      int64
	SizeKnown bool

	State State

	// FailureCode explains StateFailed. Never rendered as a code — the
	// interface maps it to a sentence (SPEC §7, F6 §5).
	FailureCode errs.Code

	// OutputPath is where this document's signature was written, set
	// once State is StateDone.
	OutputPath string

	// AchievedLevel is the PAdES level this document actually reached.
	AchievedLevel string
}

// NoticeKind names something worth telling the user about an Add that
// otherwise succeeded. Every kind is rendered by the interface as a
// sentence in the user's own language — none of these are errors, and
// none of them stop anything.
type NoticeKind string

const (
	// NoticeFolderScanned: a folder was dropped and Count PDFs were
	// taken from it (F6 §1: "say how many were found").
	NoticeFolderScanned NoticeKind = "folderScanned"
	// NoticeFolderEmpty: a folder was dropped and held no PDFs.
	NoticeFolderEmpty NoticeKind = "folderEmpty"
	// NoticeDuplicate: this exact path is already in the queue.
	NoticeDuplicate NoticeKind = "duplicate"
	// NoticeUnreadable: the path could not be examined at all.
	NoticeUnreadable NoticeKind = "unreadable"
)

// Notice is one thing to say about an Add.
type Notice struct {
	Kind NoticeKind
	// Name is the sanitised display name of the file or folder
	// concerned — safe to render, never a path to act on.
	Name string
	// Count applies to NoticeFolderScanned.
	Count int
}

// Queue is an ordered list of documents to sign, with no duplicates.
//
// The zero Queue is ready to use.
type Queue struct {
	items []Item
	// seen indexes items by path so a duplicate is O(1) to spot. A
	// hundred files dropped twice is an ordinary thing to do, and F6 §7
	// asks for "the same file listed twice" to be handled rather than
	// producing two signatures of one document.
	seen map[string]bool
}

// Items returns a copy of the queue's contents, in order. A copy, so a
// caller rendering a snapshot cannot mutate the queue by accident.
func (q *Queue) Items() []Item {
	out := make([]Item, len(q.items))
	copy(out, q.items)
	return out
}

// Len is the number of documents in the queue.
func (q *Queue) Len() int { return len(q.items) }

// TotalSize is the sum of every known file size, and whether every
// size was known. A queue with one unreadable file reports the sum of
// the rest, and false — better than reporting a confident total that
// is quietly missing a document.
func (q *Queue) TotalSize() (total int64, complete bool) {
	complete = true
	for _, it := range q.items {
		if !it.SizeKnown {
			complete = false
			continue
		}
		total += it.Size
	}
	return total, complete
}

// Add puts every path in the queue, expanding folders, and reports how
// many documents were added and anything worth saying about it.
//
// A folder contributes the PDFs directly inside it, sorted by name and
// **not** recursing into subfolders (F6 §1). A file is added whatever
// its extension: F6 §1 is explicit that a file the user dropped
// deliberately is never silently discarded, so a .docx joins the queue
// and the signing step is what reports, by name, that it is not a PDF.
// Silently dropping it would leave the person looking for a document
// they know they added.
func (q *Queue) Add(paths []string) (added int, notices []Notice) {
	for _, p := range paths {
		a, n := q.addOne(p)
		added += a
		notices = append(notices, n...)
	}
	return added, notices
}

func (q *Queue) addOne(path string) (int, []Notice) {
	clean := normalisePath(path)
	info, err := os.Stat(clean)
	if err != nil {
		return 0, []Notice{{Kind: NoticeUnreadable, Name: displayNameOf(clean)}}
	}
	if !info.IsDir() {
		if q.appendItem(clean) {
			return 1, nil
		}
		return 0, []Notice{{Kind: NoticeDuplicate, Name: displayNameOf(clean)}}
	}

	pdfs, err := pdfsDirectlyIn(clean)
	if err != nil {
		return 0, []Notice{{Kind: NoticeUnreadable, Name: displayNameOf(clean)}}
	}
	if len(pdfs) == 0 {
		return 0, []Notice{{Kind: NoticeFolderEmpty, Name: displayNameOf(clean)}}
	}

	var notices []Notice
	count := 0
	for _, p := range pdfs {
		if q.appendItem(p) {
			count++
		}
	}
	notices = append(notices, Notice{Kind: NoticeFolderScanned, Name: displayNameOf(clean), Count: count})
	return count, notices
}

// appendItem adds one file, returning false when it was already here.
func (q *Queue) appendItem(path string) bool {
	if q.seen == nil {
		q.seen = make(map[string]bool)
	}
	key := duplicateKey(path)
	if q.seen[key] {
		return false
	}
	q.seen[key] = true

	item := Item{
		Path:        path,
		DisplayName: displayNameOf(path),
		Folder:      folderNameOf(path),
		State:       StateWaiting,
	}
	if info, err := os.Stat(path); err == nil {
		item.Size, item.SizeKnown = info.Size(), true
	}
	q.items = append(q.items, item)
	return true
}

// Remove takes one document out of the queue by path, reporting
// whether it was there (F6 §1: "let a file be removed from the list
// before signing").
func (q *Queue) Remove(path string) bool {
	key := duplicateKey(normalisePath(path))
	for i, it := range q.items {
		if duplicateKey(it.Path) != key {
			continue
		}
		q.items = append(q.items[:i], q.items[i+1:]...)
		delete(q.seen, key)
		return true
	}
	return false
}

// Clear empties the queue.
func (q *Queue) Clear() {
	q.items = nil
	q.seen = nil
}

// Paths returns every document's path, in order — what the consent
// screen's digests and the runner are built from.
func (q *Queue) Paths() []string {
	out := make([]string, len(q.items))
	for i, it := range q.items {
		out[i] = it.Path
	}
	return out
}

// DisplayNames returns every document's sanitised name, in order.
func (q *Queue) DisplayNames() []string {
	out := make([]string, len(q.items))
	for i, it := range q.items {
		out[i] = it.DisplayName
	}
	return out
}

// OutputPath is where item i's signature will be written: the input's
// own name plus suffix, in dir — or beside the input when dir is empty
// (F6 §4's "defaulting to the input's own folder").
func (q *Queue) OutputPath(i int, dir, suffix string) string {
	return OutputPathFor(q.items[i].Path, dir, suffix)
}

// OutputPathFor computes one output path. Exported separately from
// OutputPath because the interface needs to show where a document will
// land before the queue is handed to a runner.
func OutputPathFor(inPath, dir, suffix string) string {
	ext := filepath.Ext(inPath)
	base := strings.TrimSuffix(filepath.Base(inPath), ext)
	name := base + suffix + ext
	if dir == "" {
		return filepath.Join(filepath.Dir(inPath), name)
	}
	return filepath.Join(dir, name)
}

// pdfsDirectlyIn lists the PDFs immediately inside dir, sorted by name,
// never descending into subfolders (F6 §1). Sorting makes a dropped
// folder produce the same order every time, which matters because the
// batch fingerprint the consent screen shows is computed over the
// documents in queue order.
func pdfsDirectlyIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), pdfExtension) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// normalisePath cleans a path and makes it absolute where it can. A
// path that cannot be made absolute is kept as given rather than
// rejected: the shell supplies absolute paths, and a caller passing
// something else still deserves a queue entry it can see and remove.
func normalisePath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// duplicateKey is what makes two paths the same document. Windows file
// names are case-insensitive, so "Ugovor.pdf" dropped twice — once from
// a search result, once from the folder — is one document, not two, and
// signing it twice would produce two signed copies of the same file
// with the second overwriting or renaming around the first.
//
// This is deliberately a lexical comparison, not an identity one: two
// different paths reaching the same file through a junction or a
// symlink are treated as two documents. Resolving that properly needs a
// file handle and a volume/index comparison per path, which is a real
// cost on a network share for a case F6 does not raise; the honest
// summary is that this catches the ordinary duplicate and not the
// exotic one.
func duplicateKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

// displayNameOf is the sanitised base name of a path — F5 §5.3's
// pipeline, reused rather than reimplemented, so a file name carrying
// direction-override characters cannot render differently from what it
// is in this window any more than it can in the consent window.
func displayNameOf(path string) string {
	return consent.SanitizeFileName(filepath.Base(path))
}

// folderNameOf is the folder a document sits in, through the same
// sanitiser its name goes through: a path is user-supplied text drawn
// on a screen exactly as a file name is, and the same 120-character cap
// with the middle elided applies (SPEC §6.6).
func folderNameOf(path string) string {
	return consent.SanitizeFileName(filepath.Dir(path))
}

// NeedsFolder says, for each item in order, whether its name alone
// identifies it in this list.
//
// Two documents called "ugovor.pdf" from two different folders are two
// different documents and both belong in the list — that is what
// comparing by full path buys, and it is right. What it costs is that
// the list then shows one name twice, and a person looking at two rows
// that read identically has no way to know whether the program kept
// both or lost count. That was reported as the duplicate check being
// broken, which it is not: the check refuses a repeat of the same path
// and says so, and this is the other case wearing the same clothes.
//
// The folder is therefore shown exactly where the name is not enough,
// and nowhere else. A batch gathered from one folder — the ordinary
// case — is not made to carry the same path on every row to make the
// rare case legible.
func NeedsFolder(items []Item) []bool {
	counts := make(map[string]int, len(items))
	for _, it := range items {
		counts[strings.ToLower(it.DisplayName)]++
	}
	out := make([]bool, len(items))
	for i, it := range items {
		out[i] = counts[strings.ToLower(it.DisplayName)] > 1
	}
	return out
}

// FormatSize renders a byte count for display: whole units, one
// decimal place below 10 units, so a list of documents reads as
// "1.2 MB" and "340 kB" rather than seven-digit byte counts.
//
// Units are decimal (kB = 1000 bytes), matching what Explorer's own
// "Size" column reports for the same file, because the person reading
// this window has that column open beside it.
func FormatSize(bytes int64) string {
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	value := float64(bytes) / float64(div)
	suffixes := [...]string{"kB", "MB", "GB", "TB"}
	if value < 10 {
		return fmt.Sprintf("%.1f %s", value, suffixes[exp])
	}
	return fmt.Sprintf("%.0f %s", value, suffixes[exp])
}
