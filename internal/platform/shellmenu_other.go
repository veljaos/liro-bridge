//go:build !windows

package platform

import "errors"

// errShellMenuUnsupported is returned on platforms with no Explorer.
// macOS and Linux get their own shell integration with their own
// windowing phases (SPEC §11.11 scopes phases 1-10 to Windows).
var errShellMenuUnsupported = errors.New("platform: the shell context menu is only supported on Windows")

type unsupportedShellMenu struct{}

// NewShellMenu returns a ShellMenu that reports the entry as absent and
// refuses to change it, rather than pretending to succeed.
func NewShellMenu() ShellMenu { return unsupportedShellMenu{} }

func (unsupportedShellMenu) IsRegistered() (bool, error)   { return false, nil }
func (unsupportedShellMenu) Register(_, _, _ string) error { return errShellMenuUnsupported }
func (unsupportedShellMenu) Unregister() error             { return errShellMenuUnsupported }
