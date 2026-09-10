//go:build !softtoken

package main

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/cli"
)

// softTokenExtraCertificates reports no certificates: the soft token is
// not present in this build.
func softTokenExtraCertificates(context.Context) ([]cli.ExtraCertificate, error) { return nil, nil }
