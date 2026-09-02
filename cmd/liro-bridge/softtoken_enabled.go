//go:build softtoken

package main

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/softtoken"
)

// softTokenSource returns the soft token backend (F2 §3) when this
// binary was built with the "softtoken" tag. A release build never
// passes that tag, so softtoken_disabled.go — not this file — is
// compiled instead, and the whole internal/keysource/softtoken package
// is absent from the binary (SPEC §16.6).
func softTokenSource() keysource.Source { return softtoken.NewSource() }

// softTokenExtraCertificates adapts the soft token's certificate list
// to cli.ExtraCertificate, for wiring into cli.Deps.ExtraCertificates
// (F2 §3, so "certs --json" can list and export the soft token's
// certificate exactly like a real one).
func softTokenExtraCertificates(ctx context.Context) ([]cli.ExtraCertificate, error) {
	certs, err := softtoken.NewSource().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cli.ExtraCertificate, 0, len(certs))
	for _, c := range certs {
		out = append(out, cli.ExtraCertificate{Thumbprint: string(c.Thumbprint), DER: c.DER, IsTestKey: c.IsTestKey})
	}
	return out, nil
}
