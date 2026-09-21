package main

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/cli"
)

// closePKCS11Modules shuts down every worker this process started.
//
// The file is named _enabled rather than _windows on purpose: a _windows
// suffix is itself a build constraint and ANDs with the tag above, which would
// have excluded the GOOS=linux softtoken build that CI signs with. That is the
// same trap keysources.go avoids by not carrying one, and it cost a red build
// here before it was caught.
//
// It is a function rather than a direct call to modules.close so that main.go —
// which is built for every platform — does not have to know whether this build
// has a PKCS#11 path at all. The build constraint here is keysources.go's, for
// keysources.go's reason: a plain non-Windows build has no signing path, and a
// file compiled into it would hold a function nothing could reach, which
// golangci-lint's `unused` says out loud in the GOOS=linux view.
func closePKCS11Modules(ctx context.Context) error { return modules.close(ctx) }

// configurePKCS11Modules records the configured module path. See
// pkcs11Backends.configure; this is the same platform split as
// closePKCS11Modules, for the same reason.
func configurePKCS11Modules(path string) { modules.configure(path) }

// pkcs11CertificateProvider is cli.Deps.ModuleCertificates for a build that has
// a PKCS#11 path.
func pkcs11CertificateProvider() func(context.Context) ([]cli.ModuleCertificate, []cli.ModuleFailure, error) {
	return moduleCertificates
}
