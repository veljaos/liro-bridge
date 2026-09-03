//go:build windows

package ui

// SetVirtualHostNameToFolderMapping (F5 §2.4) needs a real folder on
// disk; Options.Assets is an fs.FS, most commonly an embed.FS compiled
// into the binary with no on-disk path at all. materializeAssets copies
// it out once per unique content set, under the agent's per-user
// config directory — never file:// and never a local HTTP server (F5
// §2.4's own reasoning: a local server is a second listening socket,
// which contradicts SPEC §6.1).
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// materializeAssets extracts every file in fsys under a directory named
// after the content's own SHA-256 (truncated) so that different builds
// (or different windows' asset sets) never collide, and re-extraction
// is skipped once a matching directory already exists — the directory
// name itself is the cache key, so a stale or partial extraction from a
// previous, different build can never be mistaken for current content.
func materializeAssets(fsys fs.FS) (string, error) {
	h := sha256.New()
	var files []string
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return "", fmt.Errorf("ui: walking assets: %w", err)
	}
	for _, name := range files {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", fmt.Errorf("ui: reading asset %s: %w", name, err)
		}
		h.Write([]byte(name))
		h.Write(b)
	}
	sum := hex.EncodeToString(h.Sum(nil))[:16]

	dir := filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "ui-assets", sum)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir, nil
	}

	tmp := dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return "", err
	}
	for _, name := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := copyAssetFile(fsys, name, dst); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		// Another process (or an earlier attempt in this one) may have
		// won the race and already created dir; that is success, not a
		// failure to propagate.
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			_ = os.RemoveAll(tmp)
			return dir, nil
		}
		return "", err
	}
	return dir, nil
}

func copyAssetFile(fsys fs.FS, name, dst string) error {
	src, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, src)
	return err
}
