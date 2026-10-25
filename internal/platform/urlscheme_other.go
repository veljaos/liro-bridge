//go:build !windows

package platform

// URLScheme is nothing to register here: Linux's comes with the package's
// desktop entry and macOS's with the bundle's Info.plist (see
// urlscheme.go), and neither is the agent's to write at run time.
type URLScheme struct{}

// NewURLScheme returns the registration that is not the agent's.
func NewURLScheme() URLScheme { return URLScheme{} }

// Register does nothing; see URLScheme.
func (URLScheme) Register(string) error { return nil }

// Command is always "": nothing is registered by the agent.
func (URLScheme) Command() (string, error) { return "", nil }

// Unregister does nothing; see URLScheme.
func (URLScheme) Unregister() error { return nil }
