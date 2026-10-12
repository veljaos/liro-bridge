//go:build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// The scheme's per-user registration: HKCU\Software\Classes\liro-bridge,
// with "URL Protocol" marking it as a scheme rather than a file type,
// and shell\open\command running LinkCommand. HKCR and HKLM are never
// written, for the reason shellmenu_windows.go gives.
const urlSchemeKeyPath = `Software\Classes\` + LinkScheme

// URLScheme is the scheme's registration.
type URLScheme struct{ keyPath string }

// NewURLScheme returns the real registration.
func NewURLScheme() URLScheme { return URLScheme{keyPath: urlSchemeKeyPath} }

func (s URLScheme) commandPath() string { return s.keyPath + `\shell\open\command` }

// Register writes the scheme, pointing at exePath. Writing it twice
// rewrites it, which is how an agent that moved keeps the link
// pointing at the binary actually running.
func (s URLScheme) Register(exePath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, s.keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating the link scheme key: %w", err)
	}
	defer func() { _ = k.Close() }()
	if err := k.SetStringValue("", "URL:Liro Bridge"); err != nil {
		return fmt.Errorf("writing the link scheme's name: %w", err)
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return fmt.Errorf("marking the link scheme as a URL protocol: %w", err)
	}
	ck, _, err := registry.CreateKey(registry.CURRENT_USER, s.commandPath(), registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating the link scheme's command key: %w", err)
	}
	defer func() { _ = ck.Close() }()
	if err := ck.SetStringValue("", LinkCommand(exePath)); err != nil {
		return fmt.Errorf("writing the link scheme's command: %w", err)
	}
	return nil
}

// Command is the command line registered, or "" when there is none.
func (s URLScheme) Command() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, s.commandPath(), registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("")
	if err == registry.ErrNotExist {
		return "", nil
	}
	return v, err
}

// Unregister removes the scheme and every key it owns, deepest first: a
// key with subkeys cannot be deleted.
func (s URLScheme) Unregister() error {
	for _, p := range []string{s.commandPath(), s.keyPath + `\shell\open`, s.keyPath + `\shell`, s.keyPath} {
		if err := registry.DeleteKey(registry.CURRENT_USER, p); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("removing %s: %w", p, err)
		}
	}
	return nil
}
