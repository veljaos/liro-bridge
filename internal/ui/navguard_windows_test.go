//go:build windows

package ui

import "testing"

// The rule this program's windows live under, stated as a table: the
// document is a page of its own, served from its own virtual host, and
// nothing else ever is.
func TestOnlyThisWindowsOwnPagesMayBecomeTheDocument(t *testing.T) {
	const host = "liro.invalid"

	cases := []struct {
		uri   string
		allow bool
		why   string
	}{
		// What this program itself navigates to.
		{"https://liro.invalid/pages/main.html", true, "the page setUpWebView2 starts on"},
		{"https://liro.invalid/pages/consent.html", true, "the consent screen"},
		{"https://liro.invalid/", true, "the host's own root"},
		{"https://liro.invalid", true, "the host with no path at all"},
		{"https://liro.invalid/pages/main.html#step2", true, "a fragment"},
		{"https://liro.invalid/pages/main.html?x=1", true, "a query"},
		{"HTTPS://LIRO.INVALID/pages/main.html", true, "a host is case-insensitive"},
		{"about:blank", true, "where a WebView2 starts before anything is navigated to"},
		{"About:Blank", true, "and it is case-insensitive too"},

		// The defect this guard was written for: a dropped document.
		{"file:///C:/Users/Someone/Desktop/TEST.pdf", false, "a PDF dropped on the window"},
		{"file://server/share/ugovor.pdf", false, "a document dropped from a share"},

		// Every other way a browser navigates.
		{"https://example.com/", false, "a link in rendered content"},
		{"http://liro.invalid/pages/main.html", false, "the same host over http"},
		{"data:text/html,<h1>hello", false, "a data URL"},
		{"blob:https://liro.invalid/2f1c", false, "a blob URL"},
		{"javascript:alert(1)", false, "a javascript URL"},
		{"ms-appx-web:///x.html", false, "a scheme nobody here uses"},

		// The prefix trap: every one of these starts with the allowed
		// string and is a different host.
		{"https://liro.invalid.example.com/", false, "a longer host with ours as a prefix"},
		{"https://liro.invalidextra/", false, "a host with ours as a prefix, no dot"},
		{"https://liro.invalid:8443/", false, "our host on a port this program never uses"},
		{"https://liro.invalid@example.com/", false, "our host as userinfo on another one"},

		// Nothing at all.
		{"", false, "an empty URI"},
	}

	for _, c := range cases {
		if got := allowedNavigation(c.uri, host); got != c.allow {
			t.Errorf("allowedNavigation(%q) = %v, want %v — %s", c.uri, got, c.allow, c.why)
		}
	}
}

// A window with no virtual host of its own has no page to allow, so it
// allows nothing but the empty document. Stated as its own test because
// the alternative — an empty host matching an empty prefix, and so
// allowing every https URL there is — is exactly the shape of mistake a
// prefix rule invites.
func TestAWindowWithNoVirtualHostAllowsNothingButTheEmptyDocument(t *testing.T) {
	for _, uri := range []string{
		"https://example.com/",
		"https://",
		"file:///C:/x.pdf",
		"",
	} {
		if allowedNavigation(uri, "") {
			t.Errorf("allowedNavigation(%q, \"\") = true, want false", uri)
		}
	}
	if !allowedNavigation("about:blank", "") {
		t.Error("allowedNavigation(about:blank, \"\") = false, want true")
	}
}

// SPEC §18.3 keeps file names out of log files, and the URI of a
// dropped document is a file name. What the guard writes about a
// refusal must therefore carry the scheme, at most the host, and never
// a path.
func TestARefusalNeverCarriesAPathIntoTheLog(t *testing.T) {
	cases := []struct {
		uri    string
		scheme string
		host   string
	}{
		{"file:///C:/Users/Veljko/Desktop/ugovor-za-potpis.pdf", "file", ""},
		{"file://fileserver/klijenti/ugovor.pdf", "file", "fileserver"},
		{"https://example.com/doc?name=ugovor.pdf", "https", "example.com"},
		{"HTTPS://Example.COM:443/x", "https", "example.com:443"},
		{"https://user:pw@example.com/x", "https", "example.com"},
		{"data:application/pdf;base64,JVBERi0x", "data", ""},
		{"blob:https://liro.invalid/2f1c-4a", "blob", ""},
		{"about:blank", "about", ""},
		{"nonsense", "malformed", ""},
		{"", "malformed", ""},
	}

	for _, c := range cases {
		scheme, host := navigationTarget(c.uri)
		if scheme != c.scheme || host != c.host {
			t.Errorf("navigationTarget(%q) = (%q, %q), want (%q, %q)",
				c.uri, scheme, host, c.scheme, c.host)
		}
	}

	// The property behind the table, checked directly against the one
	// URI this guard exists to refuse: nothing a document is called
	// survives into what gets logged.
	const doc = "file:///C:/Users/Veljko/Desktop/TAJNI-UGOVOR-2026.pdf"
	scheme, host := navigationTarget(doc)
	for _, leak := range []string{"TAJNI", "UGOVOR", "Veljko", "Desktop", ".pdf"} {
		if contains(scheme, leak) || contains(host, leak) {
			t.Errorf("navigationTarget(%q) leaked %q into (%q, %q)", doc, leak, scheme, host)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
