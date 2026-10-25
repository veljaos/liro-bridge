package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A link handed over by a launch (F14 §1, [[D-461]]): `liro-bridge
// open-link <url>` finds an agent running and leaves the link here for
// it, one file per link, beside the inbox. One file each rather than a
// line in the inbox: a link is not a document, the window that drains
// the inbox must never see one, and two links clicked in a row are two
// requests, not a batch.
const linkFilePrefix = "link-"

// maxLinkFile bounds what TakeLinks reads; cloud.MaxLinkLength is the
// rule, this only stops a stray file being read whole.
const maxLinkFile = 4096

// AppendLink leaves link for the running agent.
func (b *Inbox) AppendLink(link string) error {
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".tmp-"+linkFilePrefix+hex.EncodeToString(id[:]))
	if err := os.WriteFile(tmp, []byte(link), 0o600); err != nil {
		return err
	}
	// Renamed into place, so the agent never reads half of one.
	return os.Rename(tmp, filepath.Join(dir, linkFilePrefix+hex.EncodeToString(id[:])))
}

// TakeLinks returns every link waiting, oldest first by the time it was
// left, and removes each before returning it — a link consumed by an
// agent that then died is one a person can click again.
func (b *Inbox) TakeLinks() ([]string, error) {
	dir := filepath.Dir(b.path)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type found struct {
		name string
		mod  int64
	}
	var files []found
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), linkFilePrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, found{e.Name(), info.ModTime().UnixNano()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod < files[j].mod })
	var out []string
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		data, err := os.ReadFile(path)
		_ = os.Remove(path)
		if err != nil || len(data) == 0 || len(data) > maxLinkFile {
			continue
		}
		out = append(out, string(data))
	}
	return out, nil
}
