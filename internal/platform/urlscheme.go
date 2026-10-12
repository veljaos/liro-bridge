package platform

// The liro-bridge:// link (F14 §1, [[D-461]]): a page on a server the
// person has allowed opens one, and the desktop runs this binary's
// open-link command with it.
//
// How a desktop learns of the scheme is each platform's own: on Windows
// a per-user key under HKCU\Software\Classes, written by the agent at
// every start like the Explorer verb; on Linux the package's
// liro-bridge-open-link.desktop entry with MimeType
// x-scheme-handler/liro-bridge, which the distribution's own desktop
// database picks up; on macOS CFBundleURLTypes in the bundle's
// Info.plist (F13). Nothing on this list needs administrator rights.

// LinkScheme is the scheme, without the colon. internal/cloud.Scheme is
// the same string, kept apart because that package must not import
// this one's registry code.
const LinkScheme = "liro-bridge"

// LinkSubcommand is the command the scheme runs.
const LinkSubcommand = "open-link"

// LinkCommand is the command line Windows runs for a link: the
// executable and the link, each quoted, for the reason ShellMenuCommand
// gives. A link cannot contain a double quote unescaped — a URL's
// grammar has none — so the quotes need no escaping of their own; one
// that arrives anyway is refused by the parser, never trusted.
func LinkCommand(exePath string) string {
	return shellQuote + exePath + shellQuote + " " + LinkSubcommand + " " + shellQuote + "%1" + shellQuote
}
