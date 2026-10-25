package cloud

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// OriginsFileName is the person's list of servers that may send requests
// (F14 §2), beside the pairings.
const OriginsFileName = "cloud-origins.json"

// AllowedOrigin is one entry in the list.
type AllowedOrigin struct {
	Origin  string    `json:"origin"`
	Allowed time.Time `json:"allowed"`
}

// Origins is the list, on disk. Every method reads the file afresh: the
// Settings window and the agent both change it, and a copy held in memory
// is how a server the person removed would still be allowed.
type Origins struct {
	mu   sync.Mutex
	path string
}

// NewOrigins is the list in dir.
func NewOrigins(dir string) *Origins {
	return &Origins{path: filepath.Join(dir, OriginsFileName)}
}

func (o *Origins) read() ([]AllowedOrigin, error) {
	b, err := os.ReadFile(o.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []AllowedOrigin
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (o *Origins) write(list []AllowedOrigin) error {
	sort.Slice(list, func(i, j int) bool { return list[i].Origin < list[j].Origin })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(o.path), 0o700); err != nil {
		return err
	}
	return platform.WriteFileAtomic(o.path, append(b, '\n'), 0o600)
}

// Allowed reports whether origin is on the list. A list that cannot be
// read allows nothing.
func (o *Origins) Allowed(origin string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	list, err := o.read()
	if err != nil {
		return false
	}
	for _, a := range list {
		if a.Origin == origin {
			return true
		}
	}
	return false
}

// Allow adds origin, at now.
func (o *Origins) Allow(origin string, now time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	list, err := o.read()
	if err != nil {
		return err
	}
	for _, a := range list {
		if a.Origin == origin {
			return nil
		}
	}
	return o.write(append(list, AllowedOrigin{Origin: origin, Allowed: now.UTC()}))
}

// Remove takes origin off the list; removing one that is not there is
// not an error.
func (o *Origins) Remove(origin string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	list, err := o.read()
	if err != nil {
		return err
	}
	out := list[:0]
	for _, a := range list {
		if a.Origin != origin {
			out = append(out, a)
		}
	}
	return o.write(out)
}

// List is every allowed origin, sorted.
func (o *Origins) List() ([]AllowedOrigin, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.read()
}
