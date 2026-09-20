//go:build linux

package ui

// Where the page's own content comes from.
//
// D-082 and D-150: never `file://`, and never a local HTTP server —
// a second listening socket would contradict SPEC §6.1's security
// model. WebView2 answers this with
// SetVirtualHostNameToFolderMapping; WebKitGTK has no such thing and
// answers it with a custom URI scheme, so `liro://<host>/<path>` is
// this platform's spelling of the same idea and the page's own links
// do not change.
//
// The resolution below is deliberately separate from the WebKit
// callback that calls it: it is ordinary Go over an fs.FS, so the part
// that decides what a URL is allowed to reach is tested directly,
// while the part that cannot be tested without a web process stays a
// few lines long. That is the same split F5 §10 already imposes on the
// Windows side.

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/url"
	"path"
	"strings"
)

// assetScheme is the URI scheme every window serves its own content
// over. It is registered once per process, on the default web context.
//
// It is not "http" or "https": a custom scheme cannot be confused with
// the network, gets its own opaque origin, and makes any attempt to
// reach the outside world from a page a scheme mismatch rather than a
// policy question.
const assetScheme = "liro"

// ErrUnknownAssetHost is returned for a URI naming a host this window
// did not map. It is separate from a missing file because the two say
// different things: a missing file is a mistake in our own pages, and
// an unmapped host is a page trying to reach something that was never
// offered to it.
var ErrUnknownAssetHost = errors.New("ui: no such asset host")

// resolveAsset answers one liro:// request from the hosts a window
// mapped, returning the bytes and the Content-Type to serve them as.
//
// hosts is keyed by the hostname in the URI — Options.VirtualHost, and
// Options.ScratchHost when the caller set one. A host that is not in
// the map is refused rather than defaulted to the other one: the two
// exist precisely because they have different lifetimes (Options'
// doc comment), and quietly serving one in place of the other would
// erase that distinction at the only point where it is checkable.
func resolveAsset(rawURI string, hosts map[string]fs.FS) ([]byte, string, error) {
	u, err := url.Parse(rawURI)
	if err != nil {
		return nil, "", fmt.Errorf("ui: unparseable asset URI %q: %w", rawURI, err)
	}
	if u.Scheme != assetScheme {
		return nil, "", fmt.Errorf("ui: %q is not a %s:// URI", rawURI, assetScheme)
	}

	fsys, ok := hosts[u.Host]
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownAssetHost, u.Host)
	}

	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		name = "index.html"
	}

	// fs.ValidPath is the whole of the traversal defence and is worth
	// being explicit about: io/fs paths are always slash-separated,
	// always relative, and may not contain "." or ".." elements at all,
	// so "../../etc/passwd" is not a path that escapes — it is not a
	// path. Rejecting it here means the error says so, rather than
	// fs.ReadFile reporting a file that does not exist and leaving the
	// reason to be guessed at.
	if !fs.ValidPath(name) {
		return nil, "", fmt.Errorf("ui: %q is not a valid asset path", name)
	}

	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, "", fmt.Errorf("ui: reading asset %q: %w", name, err)
	}

	return data, assetContentType(name), nil
}

// assetContentType names the media type for a path this program
// serves.
//
// The small table comes first because mime.TypeByExtension consults
// the *machine* — /etc/mime.types and the shared MIME database — and
// what it answers for ".js" or ".svg" therefore depends on which
// packages happen to be installed. A window whose script does not run
// because a minimal container had no mime database would be a
// spectacular thing to debug, so the handful of types this program
// actually ships are answered from the binary and the system is asked
// only about the rest.
func assetContentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".woff2":
		return "font/woff2"
	}
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	// Not "text/plain": a type this program does not recognise is not
	// something a page should be encouraged to interpret.
	return "application/octet-stream"
}
