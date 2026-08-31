package platform

import (
	"path/filepath"
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
