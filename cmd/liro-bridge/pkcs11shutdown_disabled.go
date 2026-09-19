//go:build !windows && !softtoken

package main

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/cli"
)

// closePKCS11Modules has nothing to close: this build has no signing path, so
// no worker was ever started. See keysources.go's build constraint.
func closePKCS11Modules(context.Context) error { return nil }

// configurePKCS11Modules has nothing to configure on a build with no PKCS#11
// path.
func configurePKCS11Modules(string) {}

// pkcs11CertificateProvider is nil on a build with no PKCS#11 path, which makes
// Gather behave exactly as it did before any of this existed.
func pkcs11CertificateProvider() func(context.Context) ([]cli.ModuleCertificate, []cli.ModuleFailure, error) {
	return nil
}
