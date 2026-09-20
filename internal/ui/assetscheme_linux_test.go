//go:build linux

package ui

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func testHosts() map[string]fs.FS {
	return map[string]fs.FS{
		"liro.invalid": fstest.MapFS{
			"index.html":         {Data: []byte("<!doctype html>root")},
			"consent.html":       {Data: []byte("<!doctype html>consent")},
			"assets/app.js":      {Data: []byte("const x = 1")},
			"assets/style.css":   {Data: []byte("body{}")},
			"assets/logo.svg":    {Data: []byte("<svg/>")},
			"assets/unknown.xyz": {Data: []byte("?")},
		},
		"scratch.invalid": fstest.MapFS{
			"page-1.png": {Data: []byte("\x89PNG")},
		},
	}
}

func TestResolveAssetServesFromTheNamedHost(t *testing.T) {
	for _, c := range []struct{ uri, body, ctype string }{
		{"liro://liro.invalid/consent.html", "<!doctype html>consent", "text/html; charset=utf-8"},
		{"liro://liro.invalid/assets/app.js", "const x = 1", "text/javascript; charset=utf-8"},
		{"liro://liro.invalid/assets/style.css", "body{}", "text/css; charset=utf-8"},
		{"liro://liro.invalid/assets/logo.svg", "<svg/>", "image/svg+xml"},
		{"liro://scratch.invalid/page-1.png", "\x89PNG", "image/png"},
		// An empty path is the host's index, the way it is everywhere else.
		{"liro://liro.invalid/", "<!doctype html>root", "text/html; charset=utf-8"},
	} {
		t.Run(c.uri, func(t *testing.T) {
			body, ctype, err := resolveAsset(c.uri, testHosts())
			if err != nil {
				t.Fatalf("resolveAsset(%q): %v", c.uri, err)
			}
			if string(body) != c.body {
				t.Errorf("body = %q, want %q", body, c.body)
			}
			if ctype != c.ctype {
				t.Errorf("content type = %q, want %q", ctype, c.ctype)
			}
		})
	}
}

// The two hosts have different lifetimes on purpose, so one must never
// answer for the other.
func TestResolveAssetRefusesAHostItDoesNotHave(t *testing.T) {
	for _, uri := range []string{
		"liro://elsewhere.invalid/index.html",
		"liro://scratch.invalid/consent.html", // real host, other host's file
		"liro:///index.html",                  // no host at all
	} {
		t.Run(uri, func(t *testing.T) {
			_, _, err := resolveAsset(uri, testHosts())
			if err == nil {
				t.Fatalf("resolveAsset(%q) succeeded, want refusal", uri)
			}
		})
	}

	_, _, err := resolveAsset("liro://elsewhere.invalid/x", testHosts())
	if !errors.Is(err, ErrUnknownAssetHost) {
		t.Errorf("error %v does not wrap ErrUnknownAssetHost", err)
	}
}

// Traversal is not a path that escapes; it is not a path.
func TestResolveAssetRefusesTraversalAndOtherSchemes(t *testing.T) {
	for _, uri := range []string{
		"liro://liro.invalid/../../../etc/passwd",
		"liro://liro.invalid/./index.html",
		"file:///etc/passwd",
		"https://example.invalid/index.html",
	} {
		t.Run(uri, func(t *testing.T) {
			if _, _, err := resolveAsset(uri, testHosts()); err == nil {
				t.Fatalf("resolveAsset(%q) succeeded, want refusal", uri)
			}
		})
	}
}

func TestResolveAssetReportsAMissingFile(t *testing.T) {
	_, _, err := resolveAsset("liro://liro.invalid/nope.html", testHosts())
	if err == nil {
		t.Fatal("expected an error for a file that is not there")
	}
	if errors.Is(err, ErrUnknownAssetHost) {
		t.Error("a missing file was reported as an unknown host; the two say different things")
	}
}

// The types this program ships are answered from the binary rather than
// from whatever mime database the machine happens to have.
func TestAssetContentTypeDoesNotDependOnTheMachine(t *testing.T) {
	for name, want := range map[string]string{
		"a.html":  "text/html; charset=utf-8",
		"a.js":    "text/javascript; charset=utf-8",
		"a.svg":   "image/svg+xml",
		"a.woff2": "font/woff2",
		"A.HTML":  "text/html; charset=utf-8",
	} {
		if got := assetContentType(name); got != want {
			t.Errorf("assetContentType(%q) = %q, want %q", name, got, want)
		}
	}
	if got := assetContentType("a.xyz"); got != "application/octet-stream" {
		t.Errorf("unknown extension = %q, want application/octet-stream", got)
	}
}
