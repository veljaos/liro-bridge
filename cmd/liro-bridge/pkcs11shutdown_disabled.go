//go:build !windows && !softtoken

package main

import "context"

// closePKCS11Modules has nothing to close: this build has no signing path, so
// no worker was ever started. See keysources.go's build constraint.
func closePKCS11Modules(context.Context) error { return nil }
