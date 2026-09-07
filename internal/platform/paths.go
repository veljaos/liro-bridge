// Package platform resolves OS-specific filesystem locations for the
// agent's configuration, logs and runtime state.
//
// Every function here accepts an environment lookup so tests can override
// the platform's usual environment variables without touching the real
// user profile. Production code calls the *Dir functions with os.Getenv.
package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// Env is a subset of os.Getenv, injected so tests can supply a fake
// environment instead of touching the real user profile.
type Env func(key string) string

// OSEnv is the real process environment. Pass it in production code.
func OSEnv(key string) string { return os.Getenv(key) }

// ConfigDir returns the directory holding the agent's config.json, per
// SPEC §5 / F0 §2.1. goos selects the platform (normally runtime.GOOS);
// env supplies environment variables (normally OSEnv).
func ConfigDir(goos string, env Env) string {
	switch goos {
	case "windows":
		return filepath.Join(env("LOCALAPPDATA"), "Liro")
	case "darwin":
		return filepath.Join(env("HOME"), "Library", "Application Support", "Liro")
	default:
		if xdg := env("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "liro")
		}
		return filepath.Join(env("HOME"), ".config", "liro")
	}
}

// LogDir returns the directory holding the agent's rotated log files.
// It shares the config directory's platform root, under a "logs"
// subdirectory (SPEC §14 uses the same per-user root for related state).
func LogDir(goos string, env Env) string {
	switch goos {
	case "windows":
		return filepath.Join(env("LOCALAPPDATA"), "Liro", "logs")
	case "darwin":
		return filepath.Join(env("HOME"), "Library", "Application Support", "Liro", "logs")
	default:
		if xdg := env("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "liro", "logs")
		}
		return filepath.Join(env("HOME"), ".config", "liro", "logs")
	}
}

// ConfigFile returns the full path to config.json for the current platform.
func ConfigFile(goos string, env Env) string {
	return filepath.Join(ConfigDir(goos, env), "config.json")
}

// DefaultConfigFile returns the config.json path for the running platform
// and environment. Production code should call this; tests should call
// ConfigFile directly with an overridden goos/env.
func DefaultConfigFile() string {
	return ConfigFile(runtime.GOOS, OSEnv)
}

// DefaultLogDir returns the log directory for the running platform and
// environment.
func DefaultLogDir() string {
	return LogDir(runtime.GOOS, OSEnv)
}

// BridgeFile returns the full path to the agent's discovery file,
// bridge.json (SPEC §14, F7 §4.1). An SDK reads it to find the port the
// agent bound; it must never scan ports, because scanning finds another
// user's agent on a shared machine, which is exactly what the per-user
// location prevents (SPEC §14.1).
//
// Windows and macOS put it beside config.json, in the same per-user
// directory. Linux prefers $XDG_RUNTIME_DIR, which is per user, per
// session and cleared at logout — the right home for a file whose whole
// content is "this process is listening here" — and falls back to
// ~/.local/state/liro when that is not set.
func BridgeFile(goos string, env Env) string {
	switch goos {
	case "windows", "darwin":
		return filepath.Join(ConfigDir(goos, env), "bridge.json")
	default:
		if runtimeDir := env("XDG_RUNTIME_DIR"); runtimeDir != "" {
			return filepath.Join(runtimeDir, "liro", "bridge.json")
		}
		return filepath.Join(env("HOME"), ".local", "state", "liro", "bridge.json")
	}
}

// DefaultBridgeFile returns the discovery file path for the running
// platform and environment.
func DefaultBridgeFile() string {
	return BridgeFile(runtime.GOOS, OSEnv)
}
