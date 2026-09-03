// Command checkcss enforces SPEC §10.1 / F5 §4.2: a hex colour literal
// anywhere in the agent's CSS fails the build. Not a warning.
//
// tokens.css itself is the one legitimate exception — it is the file
// that *defines* every hex value as a CSS custom property in the first
// place (scripts/synctokens generates it); every other stylesheet under
// internal/ui/assets must reference colours exclusively through
// var(--liro-*).
package main

import "regexp"

// hexColorPattern matches a CSS hex colour literal: '#' followed by 3,
// 4, 6 or 8 hex digits (the valid CSS lengths — #rgb, #rgba, #rrggbb,
// #rrggbbaa), not immediately followed by another hex digit (so a
// 9-digit run, which is not a valid CSS colour and is presumably
// something else, like a hash in a URL fragment, is not flagged).
var hexColorPattern = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)

// Violation names one hex colour literal found outside tokens.css.
type Violation struct {
	File string
	Line int
	Text string
}

// exemptFile is the one file allowed to contain hex literals — see the
// package doc comment.
const exemptFile = "tokens.css"

// checkCSS scans content (one file's bytes, split into lines by the
// caller) for hex colour literals. name is the file's base name, used
// only to apply the tokens.css exemption.
func checkCSS(name string, lines []string) []Violation {
	if name == exemptFile {
		return nil
	}
	var out []Violation
	for i, line := range lines {
		if loc := hexColorPattern.FindStringIndex(line); loc != nil {
			out = append(out, Violation{File: name, Line: i + 1, Text: line})
		}
	}
	return out
}
