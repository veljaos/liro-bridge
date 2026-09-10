//go:build !softtoken && windows

package main

import "github.com/veljaos/liro-bridge/internal/keysource"

// softTokenSource returns nil: this binary was built without the
// "softtoken" tag, so internal/keysource/softtoken is not linked in at
// all (SPEC §16.6) — there is nothing for this function to return a
// handle to.
//
// Windows-only, unlike softTokenExtraCertificates beside it, because
// its one caller in an untagged build is the window flow, and the
// window flow is Windows-only (SPEC §11.11). See keysources.go's build
// constraint for the whole of the reasoning.
func softTokenSource() keysource.Source { return nil }
