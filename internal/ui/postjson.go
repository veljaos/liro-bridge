package ui

import "encoding/json"

// How Go pushes a payload into a page, for every platform.
//
// This lives in a file with no build tag on purpose. It used to sit in
// webview2_windows.go, where it was correct and invisible: the Linux
// host was written against window.go's description of it — "a single
// `__liroReceive(json)` function … JSON.parse, not string
// concatenation, is what turns the payload back into an object" — and
// that description is **not what the code does**. It inlines the JSON
// as a JavaScript object literal; the page receives an object and never
// parses anything.
//
// Written from the description, the Linux side passed a JSON *string*.
// assets/bridge.js begins `if (!payload || typeof payload !== "object")
// return;`, so every payload would have been **silently dropped** —
// which is the identical failure bridge.js already carries a paragraph
// about, one level up. It was caught by reading bridge.js rather than
// by a test, because the test used a page written from the same wrong
// description.
//
// One definition, used by both, is the fix that makes the divergence
// impossible rather than merely absent.

// postJSONScript builds the call that delivers payload to the page.
//
// The payload is inlined as a literal rather than passed as a string to
// be parsed. That is safe, and not by luck: JSON is a subset of
// JavaScript literal syntax, and encoding/json escapes `<`, `>` and `&`
// to <, > and & by default — so no payload can close the
// surrounding <script> element or start a tag. It is still never
// concatenated with caller data as *text*: `v` goes through
// json.Marshal and nothing else (F5 §2.4).
//
// The `window.__liroReceive &&` guard is load-bearing. A script that
// runs before the page's own <script> tags have defined the function
// would otherwise throw; with the guard it is a no-op — which is why
// both hosts wait for the page to finish loading before the first
// PostJSON, and why NewWindow blocks until it has.
func postJSONScript(payload []byte) string {
	return "window.__liroReceive && window.__liroReceive(" + string(payload) + ");"
}

// marshalMessagePayload is a small seam so the window hosts do not need
// to import encoding/json themselves.
func marshalMessagePayload(v any) ([]byte, error) { return json.Marshal(v) }
