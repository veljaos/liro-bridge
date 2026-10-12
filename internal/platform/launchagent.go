package platform

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The macOS LaunchAgent that starts the agent at login (F13, [[D-463]]).
//
// It is built here, in a file with no platform in its name, so that what
// is written can be tested on every machine this project is built on:
// the Linux machine this was written on has no launchd, and the plist is
// the part a mistake would hide in.

// LaunchAgentLabel is the job's label and the plist's name.
const LaunchAgentLabel = "rs.liro.bridge"

// LaunchAgentFile is ~/Library/LaunchAgents/rs.liro.bridge.plist.
func LaunchAgentFile(env Env) string {
	return filepath.Join(home(env, "HOME"), "Library", "LaunchAgents", LaunchAgentLabel+".plist")
}

// launchAgentPlist is the job: run exePath with "tray" at login, in the
// person's graphical session only, and never restart it — a person who
// chose Quit from the menu bar meant it, and launchd's KeepAlive would
// bring the agent straight back.
func launchAgentPlist(exePath string) ([]byte, error) {
	if !filepath.IsAbs(exePath) {
		return nil, fmt.Errorf("platform: the LaunchAgent needs an absolute path, not %q", exePath)
	}
	if strings.ContainsRune(exePath, 0) {
		return nil, fmt.Errorf("platform: %q cannot be written into a LaunchAgent", exePath)
	}
	var esc bytes.Buffer
	if err := xml.EscapeText(&esc, []byte(exePath)); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + LaunchAgentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc.String() + `</string>
		<string>tray</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes(), nil
}

// launchAgent is the file-level half of autostart on macOS: the plist is
// the switch. Writing it takes effect at the next login, as launchd reads
// ~/Library/LaunchAgents then; it is not loaded now with launchctl,
// because the agent doing the writing is already running, and loading
// the job would start a second one.
type launchAgent struct{ path string }

func (a launchAgent) IsEnabled() (bool, error) {
	_, err := os.Stat(a.path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (a launchAgent) SetEnabled(enabled bool, exePath string) error {
	if !enabled {
		if err := os.Remove(a.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	plist, err := launchAgentPlist(exePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return err
	}
	return WriteFileAtomic(a.path, plist, 0o644)
}

// ensure is EnsureAutostart: the same as SetEnabled, but a plist that is
// already exactly right is not rewritten.
func (a launchAgent) ensure(enabled bool, exePath string) error {
	if enabled {
		want, err := launchAgentPlist(exePath)
		if err != nil {
			return err
		}
		if have, err := os.ReadFile(a.path); err == nil && bytes.Equal(have, want) {
			return nil
		}
	}
	return a.SetEnabled(enabled, exePath)
}
