package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testAutostart(t *testing.T) xdgAutostart {
	t.Helper()
	// A directory that does not exist yet: the first write has to make
	// it, which is the state of a fresh profile.
	return xdgAutostart{path: filepath.Join(t.TempDir(), "config", "autostart", "liro-bridge.desktop")}
}

func TestAutostartOnWritesAnEntryAndOffRemovesIt(t *testing.T) {
	a := testAutostart(t)
	if on, err := a.IsEnabled(); err != nil || on {
		t.Fatalf("before anything: IsEnabled = %v, %v; want false, nil", on, err)
	}
	if err := a.SetEnabled(true, "/usr/bin/liro-bridge"); err != nil {
		t.Fatal(err)
	}
	if on, err := a.IsEnabled(); err != nil || !on {
		t.Fatalf("after SetEnabled(true): IsEnabled = %v, %v", on, err)
	}
	info, err := os.Stat(filepath.Dir(a.path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("a created autostart directory has mode %o, want 700 (XDG Base Directory specification)", perm)
	}
	if err := a.SetEnabled(false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.path); !os.IsNotExist(err) {
		t.Fatalf("after SetEnabled(false) the entry is still there: %v", err)
	}
	// Removing what is already gone is the requested state, not an error.
	if err := a.SetEnabled(false, ""); err != nil {
		t.Fatalf("removing twice: %v", err)
	}
}

func TestAutostartEntryRunsTheAgentNotTheUsage(t *testing.T) {
	a := testAutostart(t)
	if err := a.SetEnabled(true, "/usr/bin/liro-bridge"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	argv := execArgv(t, entryValue(t, data, "Exec"))
	want := []string{"/usr/bin/liro-bridge", AutostartSubcommand}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Exec runs %q, want %q — without the subcommand the entry prints usage and exits (F10's finding)", argv, want)
	}
	if got := entryValue(t, data, "TryExec"); got != "/usr/bin/liro-bridge" {
		t.Errorf("TryExec = %q; a session only skips a removed binary silently if this names it", got)
	}
}

// TestAutostartExecSurvivesAnyHomeDirectory checks the two quoting
// layers against a reader written from the Desktop Entry Specification
// in this file, not against the writer's own inverse.
func TestAutostartExecSurvivesAnyHomeDirectory(t *testing.T) {
	for _, exe := range []string{
		"/home/petar/bin/liro-bridge",
		"/home/Petar Petrović/.local/bin/liro-bridge",
		"/home/Петар Петровић/bin/liro-bridge",
		`/home/a"b/liro-bridge`,
		"/home/a$HOME/liro-bridge",
		"/home/a`id`/liro-bridge",
		`/home/a\b/liro-bridge`,
		"/home/100%/liro-bridge",
		"/home/%f %U/liro-bridge",
	} {
		entry, err := autostartEntry(exe, nil)
		if err != nil {
			t.Fatalf("%q: %v", exe, err)
		}
		argv := execArgv(t, entryValue(t, entry, "Exec"))
		if len(argv) != 2 || argv[0] != exe || argv[1] != AutostartSubcommand {
			t.Errorf("%q came back as %q", exe, argv)
		}
		if got := entryValue(t, entry, "TryExec"); got != exe {
			t.Errorf("TryExec for %q came back as %q", exe, got)
		}
	}
}

// TestTheExecReaderWouldCatchUnquotedOutput is the control for the test
// above: an Exec written without the command-line layer must not survive
// it, or the reader is agreeing with whatever it is given.
func TestTheExecReaderWouldCatchUnquotedOutput(t *testing.T) {
	naive := "/home/Petar Petrović/bin/liro-bridge " + AutostartSubcommand
	argv := execArgv(t, naive)
	if len(argv) == 2 && argv[0] == "/home/Petar Petrović/bin/liro-bridge" {
		t.Fatal("an unquoted path with a space parsed as one argument, so the reader cannot tell quoting from none")
	}
}

func TestAutostartRefusesWhatCannotBeWritten(t *testing.T) {
	for _, exe := range []string{"liro-bridge", "./liro-bridge", "/home/a\nb/liro-bridge"} {
		if _, err := autostartEntry(exe, nil); err == nil {
			t.Errorf("%q was accepted", exe)
		}
	}
}

// The startup reconcile must not undo a person's choice in the desktop's
// own settings. Each case starts from a file a desktop tool could have
// left and asks what ensure(true) does to it.
func TestEnsureKeepsADesktopsSwitchOff(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing string
		exe      string
		wantOff  []string
		wantSame bool
	}{
		{
			name:     "switched off in Startup Applications, same binary",
			existing: "X-GNOME-Autostart-enabled=false",
			exe:      "/usr/bin/liro-bridge",
			wantOff:  []string{"X-GNOME-Autostart-enabled=false"},
			wantSame: true,
		},
		{
			name:     "switched off, and naming a development build",
			existing: "X-GNOME-Autostart-enabled=false",
			exe:      "/usr/bin/liro-bridge",
			wantOff:  []string{"X-GNOME-Autostart-enabled=false"},
		},
		{
			name:     "hidden by KDE's settings",
			existing: "Hidden=true",
			exe:      "/usr/bin/liro-bridge",
			wantOff:  []string{"Hidden=true"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testAutostart(t)
			written := "/usr/bin/liro-bridge"
			if !tc.wantSame {
				written = "/home/dev/liro-f12probe"
			}
			base, err := autostartEntry(written, nil)
			if err != nil {
				t.Fatal(err)
			}
			// The desktop's tool edits the enabled line in place, or
			// adds its own key; both shapes end with the off switch.
			existing := strings.Replace(string(base), gnomeEnabledKey+"=true\n", "", 1) + tc.existing + "\n"
			if err := a.write([]byte(existing)); err != nil {
				t.Fatal(err)
			}

			if err := a.ensure(true, tc.exe); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(a.path)
			if err != nil {
				t.Fatal(err)
			}
			if on, _ := a.IsEnabled(); on {
				t.Fatalf("ensure switched the entry back on:\n%s", after)
			}
			if got := offSwitches(after); strings.Join(got, ",") != strings.Join(tc.wantOff, ",") {
				t.Errorf("off switches after ensure = %q, want %q exactly as the desktop wrote them", got, tc.wantOff)
			}
			if argv := execArgv(t, entryValue(t, after, "Exec")); argv[0] != tc.exe {
				t.Errorf("ensure left the entry naming %q, want %q", argv[0], tc.exe)
			}
			if tc.wantSame && !bytes.Equal(after, []byte(existing)) {
				t.Errorf("an entry already naming this binary was rewritten:\nbefore:\n%s\nafter:\n%s", existing, after)
			}
		})
	}
}

func TestEnsureCreatesAMissingEntryAndSetEnabledOverridesTheSwitch(t *testing.T) {
	a := testAutostart(t)
	if err := a.ensure(true, "/usr/bin/liro-bridge"); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.IsEnabled(); !on {
		t.Fatal("ensure(true) on a fresh profile did not enable autostart — F10's defect, on Linux")
	}

	// Switched off by the desktop; then the person ticks the box in
	// this program's own Settings, which is an explicit yes.
	data, _ := os.ReadFile(a.path)
	if err := a.write(bytes.Replace(data, []byte(gnomeEnabledKey+"=true"), []byte(gnomeEnabledKey+"=false"), 1)); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.IsEnabled(); on {
		t.Fatal("IsEnabled did not see the desktop's switch-off, so the tests above prove nothing")
	}
	if err := a.SetEnabled(true, "/usr/bin/liro-bridge"); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.IsEnabled(); !on {
		t.Fatal("an explicit SetEnabled(true) left the entry switched off")
	}

	if err := a.ensure(false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.path); !os.IsNotExist(err) {
		t.Fatal("ensure(false) left the entry in place")
	}
}

func TestOffSwitchesReadsOnlyTheMainGroup(t *testing.T) {
	entry := "[Desktop Entry]\nExec=x\n\n[Desktop Action other]\nHidden=true\nX-GNOME-Autostart-enabled=false\n"
	if got := offSwitches([]byte(entry)); len(got) != 0 {
		t.Fatalf("keys in an action group were read as switching the entry off: %q", got)
	}
}

// entryValue reads one key from the main group, undoing the key-file
// layer's escapes (Desktop Entry Specification, "Possible value types":
// \s \n \t \r \\).
func entryValue(t *testing.T, entry []byte, key string) string {
	t.Helper()
	for _, line := range strings.Split(string(entry), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || k != key {
			continue
		}
		var out strings.Builder
		for i := 0; i < len(v); i++ {
			if v[i] != '\\' || i+1 == len(v) {
				out.WriteByte(v[i])
				continue
			}
			i++
			switch v[i] {
			case 's':
				out.WriteByte(' ')
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			case 'r':
				out.WriteByte('\r')
			default:
				out.WriteByte(v[i])
			}
		}
		return out.String()
	}
	t.Fatalf("no %s= in:\n%s", key, entry)
	return ""
}

// execArgv splits an unescaped Exec value into arguments by the
// specification's rules ("The Exec key"): arguments are separated by
// spaces; a quoted argument is enclosed in double quotes and inside it
// `"`, backtick, `$` and backslash are escaped with a backslash; `%%` is
// a literal `%`. A field code would be expanded by a launcher, and none
// is written here, so any other `%x` fails the test.
func execArgv(t *testing.T, exec string) []string {
	t.Helper()
	var argv []string
	var cur strings.Builder
	inArg, quoted := false, false
	for i := 0; i < len(exec); i++ {
		c := exec[i]
		switch {
		case quoted && c == '\\' && i+1 < len(exec):
			i++
			cur.WriteByte(exec[i])
		case quoted && c == '"':
			quoted = false
		case !quoted && c == '"':
			quoted, inArg = true, true
		case !quoted && c == ' ':
			if inArg {
				argv = append(argv, cur.String())
				cur.Reset()
				inArg = false
			}
		case c == '%':
			if i+1 < len(exec) && exec[i+1] == '%' {
				i++
				cur.WriteByte('%')
				inArg = true
				continue
			}
			t.Fatalf("Exec contains a field code: %q", exec)
		default:
			cur.WriteByte(c)
			inArg = true
		}
	}
	if quoted {
		t.Fatalf("unterminated quote in %q", exec)
	}
	if inArg {
		argv = append(argv, cur.String())
	}
	return argv
}
