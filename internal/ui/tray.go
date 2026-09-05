package ui

// Package-level tray API (F5 §3): a notification-area icon present from
// startup, with Open/Settings/Certificates/View-audit-log/Quit on a
// right-click menu and a left-click opening the main window. Like the
// window host, the real implementation is Windows-only glue
// (tray_windows.go); tray_other.go stubs it out for cross-compilation.

// TrayOptions configures the tray icon.
type TrayOptions struct {
	// Version is shown in the tooltip (F5 §3).
	Version string

	// Labels supplies the menu's text, pre-localised by the caller
	// (SPEC §9: every user-visible string is in all three catalogues;
	// internal/ui has no i18n dependency of its own, matching SPEC
	// §4.2 rule 4).
	//
	// A function rather than a value because the interface language is
	// a setting, and the menu that opened Settings has to be in the
	// language chosen there the next time it is opened, not the one the
	// process started in. It is called each time the menu is built, on
	// the tray's own thread.
	Labels func() TrayLabels

	OnOpen         func()
	OnSettings     func()
	OnCertificates func()
	OnAuditLog     func()
	OnQuit         func()
}

// TrayLabels holds the right-click menu's text (F5 §3: Open, Settings,
// Certificates, View audit log, Quit).
type TrayLabels struct {
	Open         string
	Settings     string
	Certificates string
	AuditLog     string
	Quit         string
}

// Tray is a live notification-area icon.
type Tray interface {
	Close() error
}

// NewTray creates and shows the tray icon. The agent starts minimised
// to tray with no window (F5 §3) — callers create the tray and nothing
// else at startup.
func NewTray(opts TrayOptions) (Tray, error) {
	return newTray(opts)
}
