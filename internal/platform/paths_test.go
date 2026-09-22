package platform

import (
	"path/filepath"
	"strings"
	"testing"
)

func fakeEnv(vars map[string]string) Env {
	return func(key string) string { return vars[key] }
}

func TestConfigDirWindows(t *testing.T) {
	env := fakeEnv(map[string]string{"LOCALAPPDATA": `C:\Users\test\AppData\Local`})
	got := ConfigDir("windows", env)
	want := filepath.Join(`C:\Users\test\AppData\Local`, "Liro")
	if got != want {
		t.Fatalf("ConfigDir(windows) = %q, want %q", got, want)
	}
}

func TestConfigDirDarwin(t *testing.T) {
	env := fakeEnv(map[string]string{"HOME": "/Users/test"})
	got := ConfigDir("darwin", env)
	want := filepath.Join("/Users/test", "Library", "Application Support", "Liro")
	if got != want {
		t.Fatalf("ConfigDir(darwin) = %q, want %q", got, want)
	}
}

func TestConfigDirLinuxWithXDG(t *testing.T) {
	env := fakeEnv(map[string]string{"XDG_CONFIG_HOME": "/home/test/.config", "HOME": "/home/test"})
	got := ConfigDir("linux", env)
	want := filepath.Join("/home/test/.config", "liro")
	if got != want {
		t.Fatalf("ConfigDir(linux, XDG set) = %q, want %q", got, want)
	}
}

func TestConfigDirLinuxFallback(t *testing.T) {
	env := fakeEnv(map[string]string{"HOME": "/home/test"})
	got := ConfigDir("linux", env)
	want := filepath.Join("/home/test", ".config", "liro")
	if got != want {
		t.Fatalf("ConfigDir(linux, no XDG) = %q, want %q", got, want)
	}
}

func TestConfigFileAppendsConfigJSON(t *testing.T) {
	env := fakeEnv(map[string]string{"LOCALAPPDATA": `C:\fake`})
	got := ConfigFile("windows", env)
	want := filepath.Join(`C:\fake`, "Liro", "config.json")
	if got != want {
		t.Fatalf("ConfigFile = %q, want %q", got, want)
	}
}

func TestLogDirIsUnderConfigRoot(t *testing.T) {
	env := fakeEnv(map[string]string{"LOCALAPPDATA": `C:\fake`})
	got := LogDir("windows", env)
	want := filepath.Join(`C:\fake`, "Liro", "logs")
	if got != want {
		t.Fatalf("LogDir = %q, want %q", got, want)
	}
}

func TestDataDirIsTheConfigDirOnWindowsAndMacOS(t *testing.T) {
	// The audit log has always lived beside the configuration on these
	// two, and F12 §7's decision moves one platform's path rather than
	// three. A change here would move somebody's existing chain.
	win := fakeEnv(map[string]string{"LOCALAPPDATA": `C:\Users\test\AppData\Local`})
	if got, want := DataDir("windows", win), ConfigDir("windows", win); got != want {
		t.Errorf("DataDir(windows) = %q, want the config dir %q", got, want)
	}
	mac := fakeEnv(map[string]string{"HOME": "/Users/test"})
	if got, want := DataDir("darwin", mac), ConfigDir("darwin", mac); got != want {
		t.Errorf("DataDir(darwin) = %q, want the config dir %q", got, want)
	}
}

func TestDataDirOnLinuxIsTheXDGDataDirectory(t *testing.T) {
	// $XDG_DATA_HOME and deliberately not $XDG_STATE_HOME: the XDG
	// specification describes state as data not important enough to
	// keep with real data, and backup tools commonly skip it. An audit
	// chain is evidence (SPEC §6.7). D-340.
	env := fakeEnv(map[string]string{
		"XDG_DATA_HOME":   "/home/test/.local/share",
		"XDG_STATE_HOME":  "/home/test/.local/state",
		"XDG_CONFIG_HOME": "/home/test/.config",
		"HOME":            "/home/test",
	})
	got := DataDir("linux", env)
	if want := filepath.Join("/home/test/.local/share", "liro"); got != want {
		t.Fatalf("DataDir(linux, XDG_DATA_HOME set) = %q, want %q", got, want)
	}
	if got == ConfigDir("linux", env) {
		t.Errorf("DataDir(linux) = %q, which is the config directory: the audit log is not configuration", got)
	}
	if strings.Contains(got, "state") {
		t.Errorf("DataDir(linux) = %q, which is under the state directory: see this test's own comment", got)
	}
}

func TestDataDirOnLinuxFallsBackToTheSpecifiedDefault(t *testing.T) {
	// The XDG default for $XDG_DATA_HOME is $HOME/.local/share.
	env := fakeEnv(map[string]string{"HOME": "/home/test"})
	got := DataDir("linux", env)
	if want := filepath.Join("/home/test", ".local", "share", "liro"); got != want {
		t.Fatalf("DataDir(linux, no XDG) = %q, want %q", got, want)
	}
}

// TestStateAndCacheAreTheConfigDirOnWindowsAndMacOS: the XDG split is
// Linux's, and D-348 moves one platform's paths rather than three.
// Windows has kept its logs in %LOCALAPPDATA%\Liro\logs since F0 and a
// change here would orphan every installed agent's log directory.
func TestStateAndCacheAreTheConfigDirOnWindowsAndMacOS(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		env := fakeEnv(map[string]string{
			"LOCALAPPDATA": `C:\fake`,
			"HOME":         "/Users/test",
			// Set every XDG variable, so that a Windows path which
			// started consulting one would be caught here rather than
			// on somebody's machine.
			"XDG_STATE_HOME": "/xdg/state", "XDG_CACHE_HOME": "/xdg/cache",
			"XDG_CONFIG_HOME": "/xdg/config", "XDG_DATA_HOME": "/xdg/data",
		})
		if got, want := StateDir(goos, env), ConfigDir(goos, env); got != want {
			t.Errorf("StateDir(%q) = %q, want the config dir %q", goos, got, want)
		}
		if got, want := CacheDir(goos, env), ConfigDir(goos, env); got != want {
			t.Errorf("CacheDir(%q) = %q, want the config dir %q", goos, got, want)
		}
		if got, want := LogDir(goos, env), filepath.Join(ConfigDir(goos, env), "logs"); got != want {
			t.Errorf("LogDir(%q) = %q, want %q", goos, got, want)
		}
	}
}

// TestStateAndCacheOnLinuxAreTheirOwnXDGDirectories is D-348: the log
// is "actions history" and belongs in $XDG_STATE_HOME; the page images
// and the trust list are losable and belong in $XDG_CACHE_HOME. Neither
// is configuration, and until this they were both under
// $XDG_CONFIG_HOME because that is where Windows keeps everything.
func TestStateAndCacheOnLinuxAreTheirOwnXDGDirectories(t *testing.T) {
	env := fakeEnv(map[string]string{
		"XDG_CONFIG_HOME": "/home/u/.config",
		"XDG_STATE_HOME":  "/home/u/.local/state",
		"XDG_CACHE_HOME":  "/home/u/.cache",
		"XDG_DATA_HOME":   "/home/u/.local/share",
		"HOME":            "/home/u",
	})
	cases := map[string][2]string{
		"StateDir": {StateDir("linux", env), "/home/u/.local/state/liro"},
		"CacheDir": {CacheDir("linux", env), "/home/u/.cache/liro"},
		"LogDir":   {LogDir("linux", env), "/home/u/.local/state/liro/logs"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}

	// And none of the four is any of the others. A refactor that made
	// two of these return the same string would pass every test that
	// checks one path at a time.
	dirs := map[string]string{
		"config": ConfigDir("linux", env),
		"data":   DataDir("linux", env),
		"state":  StateDir("linux", env),
		"cache":  CacheDir("linux", env),
	}
	seen := map[string]string{}
	for name, dir := range dirs {
		if other, dup := seen[dir]; dup {
			t.Errorf("%s and %s are both %q, so XDG's split is not being made", name, other, dir)
		}
		seen[dir] = name
	}
}

func TestStateAndCacheOnLinuxFallBackToTheSpecifiedDefaults(t *testing.T) {
	env := fakeEnv(map[string]string{"HOME": "/home/u"})
	if got, want := StateDir("linux", env), "/home/u/.local/state/liro"; got != want {
		t.Errorf("StateDir with no XDG_STATE_HOME = %q, want %q", got, want)
	}
	if got, want := CacheDir("linux", env), "/home/u/.cache/liro"; got != want {
		t.Errorf("CacheDir with no XDG_CACHE_HOME = %q, want %q", got, want)
	}
}

// TestNoAgentPathIsRelative is D-339's defect turned into a guard for
// every path rather than for the one that was found.
//
// The audit log was written to "./Liro/audit" — relative to wherever
// the program happened to be started — because ConfigDir was handed the
// literal string "windows" on a machine that was not Windows. Nobody
// reported it; it was found while looking for something else. What made
// it possible is that every one of these functions returns a string,
// and a string built from an environment variable that is not set is a
// relative path that looks exactly like an absolute one in a log line.
//
// So: with an environment that answers nothing at all — which is a real
// state, for an agent started by a session manager or a cron entry that
// exports no HOME — no path this program writes to may be relative.
// That is what D-348 added `home` for, and this is the guard on it.
//
// The account database is faked per platform rather than read, because
// this test runs on one operating system and asks about three: a real
// user.Current() here answers "/home/…" for the Windows case too, which
// would make the Windows half of this guard assert nothing.
func TestNoAgentPathIsRelative(t *testing.T) {
	empty := fakeEnv(map[string]string{})
	homes := map[string]string{
		"linux":   "/home/test",
		"darwin":  "/Users/test",
		"windows": `C:\Users\test`,
	}
	for goos, h := range homes {
		restore := accountHome
		accountHome = func() string { return h }
		for name, p := range everyAgentPath(goos, empty) {
			if p == "" {
				t.Errorf("%s(%q) with an empty environment is empty", name, goos)
				continue
			}
			if !isAbsoluteFor(goos, p) {
				t.Errorf("%s(%q) with an empty environment is %q, which is relative to whatever "+
					"directory the agent was started in (D-339)", name, goos, p)
			}
		}
		accountHome = restore
	}
}

// TestTheRelativePathGuardWouldFire is the control for the test above.
//
// Every path it checks is absolute because `home` falls back to the
// account database. Taking that fallback away is the state the guard
// exists for, and the guard must fail there — otherwise it is a test
// that asserts an account database exists.
func TestTheRelativePathGuardWouldFire(t *testing.T) {
	restore := accountHome
	accountHome = func() string { return "" }
	t.Cleanup(func() { accountHome = restore })

	empty := fakeEnv(map[string]string{})
	relative := 0
	for _, goos := range []string{"linux", "windows", "darwin"} {
		for _, p := range everyAgentPath(goos, empty) {
			if !isAbsoluteFor(goos, p) {
				relative++
			}
		}
	}
	if relative == 0 {
		t.Fatal("with no environment and no account database every path still looked absolute, " +
			"so the guard above would pass whatever these functions returned")
	}
}

// everyAgentPath is every location this program writes to, so that a
// new one is covered by being added here rather than by somebody
// remembering to write a test for it.
func everyAgentPath(goos string, env Env) map[string]string {
	return map[string]string{
		"ConfigDir":  ConfigDir(goos, env),
		"DataDir":    DataDir(goos, env),
		"StateDir":   StateDir(goos, env),
		"CacheDir":   CacheDir(goos, env),
		"LogDir":     LogDir(goos, env),
		"ConfigFile": ConfigFile(goos, env),
		"BridgeFile": BridgeFile(goos, env),
	}
}

// isAbsoluteFor answers for the named platform rather than for the one
// the test is running on. filepath.IsAbs asks about the host's
// separator, which is the wrong question for a Windows path evaluated
// on Linux — and getting that wrong is how this guard would pass on
// every runner while checking nothing about half its cases.
func isAbsoluteFor(goos, p string) bool {
	if goos != "windows" {
		return strings.HasPrefix(p, "/")
	}
	if strings.HasPrefix(p, `\\`) {
		return true // a UNC path
	}
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}
