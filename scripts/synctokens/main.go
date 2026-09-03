// Command synctokens generates internal/ui/assets/tokens.css and
// internal/ui/assets/intents.css (F5 §4.1) and writes them to disk.
//
// In production this project's design tokens would come from
// `@liro/tokens`, a React-monorepo package this Go repository cannot
// depend on or run (SPEC §10 states this directly: "the agent has no
// dependency on the Liro Design System"). That package is not available
// in this environment, so this script is, for now, the source of the
// values themselves rather than a puller of someone else's generated
// output — see docs/decisions.md for the reasoning and what replaces
// this once the real package exists. The contract this script upholds
// regardless of where the values come from is the one F5 §4 actually
// cares about: every colour, spacing and typography value the agent's
// pages use is a CSS custom property in one committed file, so builds
// are reproducible without the design system present, and a hex colour
// literal anywhere else in the agent's CSS is checkable and forbidden
// (scripts/checkcss).
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "synctokens:", err)
		os.Exit(1)
	}
	assetsDir := filepath.Join(root, "internal", "ui", "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "synctokens:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(assetsDir, "tokens.css"), []byte(tokensCSS), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "synctokens:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "intents.css"), []byte(intentsCSS), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "synctokens:", err)
		os.Exit(1)
	}
	fmt.Println("synctokens: wrote tokens.css and intents.css")
}
