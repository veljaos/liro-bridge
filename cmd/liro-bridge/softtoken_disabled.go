//go:build !softtoken

package main

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// softTokenSource returns nil: this binary was built without the
// "softtoken" tag, so internal/keysource/softtoken is not linked in at
// all (SPEC §16.6) — there is nothing for this function to return a
// handle to.
func softTokenSource() keysource.Source { return nil }

// softTokenExtraCertificates reports no certificates: the soft token is
// not present in this build.
func softTokenExtraCertificates(context.Context) ([]cli.ExtraCertificate, error) { return nil, nil }
