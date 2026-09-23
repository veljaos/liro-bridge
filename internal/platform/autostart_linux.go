package platform

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// xdgAutostart is F12 §8's autostart: an XDG desktop entry in
// $XDG_CONFIG_HOME/autostart, written and removed by this program.
//
// **Not a systemd user unit**, and the reason is the phase's own: a
// user unit can start before the graphical session exists, and a GUI
// agent started without a display fails with no symptom anybody sees.
// The autostart directory is read by the session itself (gnome-session,
// KDE's, xdg-autostart-generator), after the desktop is up, which is the
// moment this agent can do anything useful.
//
// path is a field so a test can point it at a temporary directory rather
// than the person's own profile — windowsAutostart's keyPath, for the
// same reason.
type xdgAutostart struct{ path string }

// NewAutostart returns the XDG autostart backend for the person's own
// configuration directory.
func NewAutostart() Autostart { return xdgAutostart{path: AutostartFile(OSEnv)} }

// Two keys the desktop's own tools write to switch an entry off without
// deleting it. Ubuntu's "Startup Applications" (gnome-session-properties,
// installed by default on 24.04) writes the first; the second is the XDG
// specification's own, and KDE's settings write it.
const (
	gnomeEnabledKey = "X-GNOME-Autostart-enabled"
	hiddenKey       = "Hidden"
)

// IsEnabled reports whether the session will start the agent: the file
// exists and neither of the keys above switches it off. A file somebody
// switched off in the desktop's own settings is not enabled, whatever
// this program's configuration says.
func (a xdgAutostart) IsEnabled() (bool, error) {
	data, err := os.ReadFile(a.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(offSwitches(data)) == 0, nil
}

// SetEnabled is the person's explicit choice, made in Settings: the
// entry is written whole, switched on, or removed.
func (a xdgAutostart) SetEnabled(enabled bool, exePath string) error {
	if !enabled {
		if err := os.Remove(a.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	entry, err := autostartEntry(exePath, nil)
	if err != nil {
		return err
	}
	return a.write(entry)
}

// EnsureAutostart is what the agent does at startup, on Linux, and it is
// deliberately weaker than SetEnabled.
//
// Windows keeps the desktop's own "disable this at startup" switch in a
// different registry key from the Run value this program writes
// (StartupApproved), so rewriting the Run value at every start cannot
// undo it. **Linux keeps both in one file.** An agent that rewrote the
// entry at every start would silently switch itself back on after a
// person switched it off in "Startup Applications" — so this only
// creates the entry if it is missing, and when the entry names some
// other binary it points it at this one while carrying over a
// switch-off that is already there. That second case is not a
// hypothetical: a development build run once registers its own path,
// and F9b's trap was exactly a registration left pointing at a build
// directory (startup_windows.go's applyStartupRegistrations says how).
//
// With enabled false it removes the entry, as SetEnabled does: the
// person's configuration says no, and there is nothing of theirs in the
// file worth keeping.
func EnsureAutostart(enabled bool, exePath string) error {
	return xdgAutostart{path: AutostartFile(OSEnv)}.ensure(enabled, exePath)
}

func (a xdgAutostart) ensure(enabled bool, exePath string) error {
	if !enabled {
		return a.SetEnabled(false, exePath)
	}
	existing, err := os.ReadFile(a.path)
	if errors.Is(err, fs.ErrNotExist) {
		return a.SetEnabled(true, exePath)
	}
	if err != nil {
		return err
	}
	want, err := autostartEntry(exePath, offSwitches(existing))
	if err != nil {
		return err
	}
	if bytes.Equal(existing, want) {
		return nil
	}
	return a.write(want)
}

func (a xdgAutostart) write(entry []byte) error {
	// 0700 for a directory this creates: the XDG Base Directory
	// specification's own instruction for a missing destination.
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return err
	}
	return WriteFileAtomic(a.path, entry, 0o644)
}

// offSwitches returns the lines of an entry's main group that switch it
// off, exactly as they were written, so that a rewrite can carry them
// over unchanged. Normalising them would be wrong: a `Hidden=true` turned
// into `X-GNOME-Autostart-enabled=false` could no longer be switched back
// on by the tool that wrote it. Keys in a [Desktop Action …] group are
// not about whether the entry starts and are not read.
func offSwitches(entry []byte) []string {
	var off []string
	sc := bufio.NewScanner(bytes.NewReader(entry))
	inMain := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inMain = line == "[Desktop Entry]"
			continue
		}
		if !inMain {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if (key == gnomeEnabledKey && value == "false") || (key == hiddenKey && value == "true") {
			off = append(off, key+"="+value)
		}
	}
	return off
}

// autostartEntry is the whole file. NoDisplay keeps it out of the
// application menu, where the package's own entry already is; TryExec
// makes a session skip it silently once the binary is gone, which is
// what a person who removed the package and never opened Settings
// again is left with (the package cannot reach into their home to take
// it away — see D-354).
func autostartEntry(exePath string, off []string) ([]byte, error) {
	if !filepath.IsAbs(exePath) {
		// A relative Exec would be resolved against whatever directory
		// the session starts things in, which is D-348's class.
		return nil, fmt.Errorf("platform: autostart needs an absolute path to the agent, got %q", exePath)
	}
	quoted, err := desktopExecArg(exePath)
	if err != nil {
		return nil, err
	}
	tryExec, err := desktopString(exePath)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=Liro Bridge\n")
	b.WriteString("Exec=" + quoted + " " + AutostartSubcommand + "\n")
	b.WriteString("TryExec=" + tryExec + "\n")
	b.WriteString("Icon=liro-bridge\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("NoDisplay=true\n")
	if len(off) == 0 {
		b.WriteString(gnomeEnabledKey + "=true\n")
	}
	for _, line := range off {
		b.WriteString(line + "\n")
	}
	return []byte(b.String()), nil
}

// desktopExecArg quotes one argument for an Exec key, following the
// Desktop Entry Specification's two layers in the order a reader peels
// them: the value is first a key-file string (so a backslash is written
// as two), and the unescaped string is then a command line in which a
// quoted argument escapes `"`, backtick, `$` and backslash with a
// backslash. A literal `%` is `%%`, because Exec reserves `%` for field
// codes.
//
// It always quotes. A person's home directory is named after them, and
// a Serbian name is not something to guess the quoting needs of.
func desktopExecArg(arg string) (string, error) {
	if strings.ContainsAny(arg, "\n\r\t\x00") {
		return "", fmt.Errorf("platform: %q cannot be written into a desktop entry", arg)
	}
	var cmd strings.Builder
	cmd.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '"', '`', '$', '\\':
			cmd.WriteByte('\\')
		}
		cmd.WriteRune(r)
	}
	cmd.WriteByte('"')
	s := strings.ReplaceAll(cmd.String(), `\`, `\\`)
	return strings.ReplaceAll(s, "%", "%%"), nil
}

// desktopString escapes a plain string value: only the key-file layer.
func desktopString(s string) (string, error) {
	if strings.ContainsAny(s, "\n\r\t\x00") {
		return "", fmt.Errorf("platform: %q cannot be written into a desktop entry", s)
	}
	return strings.ReplaceAll(s, `\`, `\\`), nil
}
