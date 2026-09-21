//go:build windows || softtoken

package main

// dropOrigin, and a build tag that names where its two references live
// rather than a platform.
//
// It adapts an opener that reports which backend it used into one that
// does not, for the two callers that do not record an audit entry: the
// no-consent signing path, which exists only under the softtoken tag
// (noconsent_softtoken.go, and SPEC §18.2 is why it exists nowhere
// else), and keysources_test.go, which is itself tagged
// `windows || softtoken`. Outside those two views nothing refers to it,
// which is why it does not live in keysources.go: that file became
// platform-neutral when F12 §3 gave this program windows on a second
// platform, and a symbol with no reader in the neutral view would be
// dead code there.

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// dropOrigin adapts the chooser for callers that have nowhere to record where a
// session came from.
//
// The headless commands are those callers: they write no audit entry at all —
// internal/audit's Entry is constructed in exactly one place, and it is the
// window flow — so there is nothing for the origin to reach. One rule in one
// place, wrapped where it is not wanted, rather than a second copy of a
// three-backend fallback that would have to be kept in step with this one
// (D-108, D-124, D-138).
func dropOrigin(open opener) func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
	return func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
		sess, _, err := open(ctx, thumbprint)
		return sess, err
	}
}
