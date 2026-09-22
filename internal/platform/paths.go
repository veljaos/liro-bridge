// Package platform resolves OS-specific filesystem locations for the
// agent's configuration, logs and runtime state.
//
// Every function here accepts an environment lookup so tests can override
// the platform's usual environment variables without touching the real
// user profile. Production code calls the *Dir functions with os.Getenv.
package platform

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// Env is a subset of os.Getenv, injected so tests can supply a fake
// environment instead of touching the real user profile.
type Env func(key string) string

// OSEnv is the real process environment. Pass it in production code.
func OSEnv(key string) string { return os.Getenv(key) }

// accountHome is the account database's idea of this user's home
// directory, as a variable so a test can take it away. Nothing but a
// test replaces it.
var accountHome = func() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.HomeDir
}

// home returns the directory every path below is rooted in, and **it
// exists so that none of them can be relative** (D-348).
//
// Each of these functions returns a string and no error, and a string
// built from an environment variable that is not set is a path relative
// to whatever directory the agent happened to be started in — which
// reads exactly like an absolute one in a log line. That is not a
// hypothetical: [[D-339]] found the audit log being written to
// "./Liro/audit" for a different reason and nobody had reported it.
//
// An agent started by something that exports no HOME — a session
// manager, a cron entry, `env -i` — is the case this covers. The
// account database answers when the environment does not, because
// getpwuid and the Windows profile directory do not depend on a
// variable somebody forgot to pass on.
func home(env Env, keys ...string) string {
	for _, key := range keys {
		if v := env(key); v != "" {
			return v
		}
	}
	return accountHome()
}

// ConfigDir returns the directory holding the agent's config.json, per
// SPEC §5 / F0 §2.1. goos selects the platform (normally runtime.GOOS);
// env supplies environment variables (normally OSEnv).
func ConfigDir(goos string, env Env) string {
	switch goos {
	case "windows":
		if local := env("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "Liro")
		}
		return filepath.Join(home(env, "USERPROFILE"), "AppData", "Local", "Liro")
	case "darwin":
		return filepath.Join(home(env, "HOME"), "Library", "Application Support", "Liro")
	default:
		if xdg := env("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "liro")
		}
		return filepath.Join(home(env, "HOME"), ".config", "liro")
	}
}

// StateDir returns the directory holding the agent's own record of what
// it has been doing: the rotated log files, and when it last looked for
// an update.
//
// **On Windows and macOS it is the same directory as ConfigDir.** The
// split exists for Linux, where the XDG specification has a directory
// for precisely this and names its contents in the same breath —
// $XDG_STATE_HOME holds "actions history (logs, history, recently used
// files)" — and where nothing was using it (F12 §7, D-348).
//
// **This is the other half of D-340's question and it comes out the
// other way.** The audit chain went to $XDG_DATA_HOME because it is
// evidence of what somebody signed and must survive a backup that skips
// the unimportant. The diagnostic log is the thing $XDG_STATE_HOME was
// invented for: useful while diagnosing, worthless in an archive, and
// nothing is lost if a backup tool passes it by. The two are not the
// same kind of file and it took Linux to make that a question anybody
// had to answer.
func StateDir(goos string, env Env) string {
	switch goos {
	case "windows", "darwin":
		return ConfigDir(goos, env)
	default:
		if xdg := env("XDG_STATE_HOME"); xdg != "" {
			return filepath.Join(xdg, "liro")
		}
		return filepath.Join(home(env, "HOME"), ".local", "state", "liro")
	}
}

// CacheDir returns the directory holding what this agent can lose
// without losing anything: the downloaded trust list, and the page
// images a placement window renders.
//
// **On Windows and macOS it is the same directory as ConfigDir**, as
// with StateDir and DataDir, and for the same reason — the split is
// XDG's and Linux is where it matters.
//
// **The page images are why this is not merely tidiness.** A placement
// window renders the document somebody is about to sign into a scratch
// directory, and that directory was under $XDG_CONFIG_HOME — which is
// the one directory on a Linux desktop that backup and sync tools are
// most likely to be pointed at. Page images of a person's contract are
// not configuration and do not belong anywhere a synchroniser will find
// them. $XDG_CACHE_HOME is excluded by convention and is on disk rather
// than in $XDG_RUNTIME_DIR's tmpfs, which a two-hundred-page document
// would render into the machine's memory.
//
// The stale-preview sweep (F6b) is unchanged and simply sweeps here
// instead: nothing about its reason depends on which directory it is
// pointed at.
func CacheDir(goos string, env Env) string {
	switch goos {
	case "windows", "darwin":
		return ConfigDir(goos, env)
	default:
		if xdg := env("XDG_CACHE_HOME"); xdg != "" {
			return filepath.Join(xdg, "liro")
		}
		return filepath.Join(home(env, "HOME"), ".cache", "liro")
	}
}

// LogDir returns the directory holding the agent's rotated log files:
// the "logs" subdirectory of StateDir, which on Windows and macOS is
// still the same per-user root it has always been.
func LogDir(goos string, env Env) string {
	return filepath.Join(StateDir(goos, env), "logs")
}

// DataDir returns the directory holding the agent's own data, as
// distinct from its configuration: today that is the audit log, and
// nothing else.
//
// **On Windows and macOS it is the same directory as ConfigDir**, which
// is where those platforms put both and where this program's audit log
// has always been. The split exists for Linux, where XDG separates the
// two and the audit log is on the data side of that line.
//
// **$XDG_DATA_HOME rather than $XDG_STATE_HOME, and that is a decision
// about what the audit log is** (F12 §7, D-340). The XDG specification
// describes $XDG_STATE_HOME as holding data that is "not important
// enough to be stored in $XDG_DATA_HOME", and backup tools commonly
// skip it — which is the wrong fate for SPEC §6.7's hash chain, whose
// whole purpose is to be evidence of what somebody signed, possibly in
// front of a court. It is not configuration either, so not
// $XDG_CONFIG_HOME.
func DataDir(goos string, env Env) string {
	switch goos {
	case "windows", "darwin":
		return ConfigDir(goos, env)
	default:
		if xdg := env("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "liro")
		}
		return filepath.Join(home(env, "HOME"), ".local", "share", "liro")
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

// DefaultStateDir returns the state directory for the running platform
// and environment.
func DefaultStateDir() string {
	return StateDir(runtime.GOOS, OSEnv)
}

// DefaultCacheDir returns the cache directory for the running platform
// and environment.
func DefaultCacheDir() string {
	return CacheDir(runtime.GOOS, OSEnv)
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
		return filepath.Join(StateDir(goos, env), "bridge.json")
	}
}

// DefaultBridgeFile returns the discovery file path for the running
// platform and environment.
func DefaultBridgeFile() string {
	return BridgeFile(runtime.GOOS, OSEnv)
}
