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
