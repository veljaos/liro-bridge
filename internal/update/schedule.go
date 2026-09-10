package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// State is what the agent remembers between checks. It is a file of
// its own rather than four more fields in config.json, for one reason:
// config.json is the person's own settings and the file the settings
// window rewrites whole (D-134). A background check rewriting it once
// a day would put the agent and the settings window in a race over a
// file only one of them is meant to own.
type State struct {
	// LastCheck is when a check last completed, successfully or not.
	// Not "last succeeded": a machine with no internet must not retry
	// every minute, and SPEC §6.8 makes offline normal rather than an
	// error.
	LastCheck time.Time `json:"lastCheck"`

	// LastSeenVersion is the newest version a check has verified. It is
	// what makes "you have already been asked about this one" possible
	// without asking the network again.
	LastSeenVersion string `json:"lastSeenVersion,omitempty"`

	// DismissedVersion is a version the person said "not now" to. The
	// agent does not raise it again by itself; the manual check in
	// Settings still reports it, because that is somebody asking.
	DismissedVersion string `json:"dismissedVersion,omitempty"`
}

// StateFile is where State lives: beside config.json, in the same
// per-user directory as everything else this agent keeps (SPEC §14.1 —
// one agent per user session, and nothing shared between them).
func StateFile() string {
	return filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "update-state.json")
}

// LoadState reads the state file. A missing file is not an error: it
// means no check has ever run, which is exactly the zero value.
//
// A malformed file is also not an error, and is not rewritten. The
// worst a corrupt state file can do is cause one extra check, which is
// one HTTP GET; refusing to start the agent over it, or silently
// replacing a file somebody may have hand-edited, would both be worse
// than that (the same reading config.Load already applies to its own).
func LoadState(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}
	}
	return s
}

// SaveState writes the state file, creating its directory if it is not
// there. It is written atomically for the same reason bridge.json is
// (D-186): a reader must never see half of it.
func SaveState(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("update: creating the state directory: %w", err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("update: encoding the state: %w", err)
	}
	return platform.WriteFileAtomic(path, append(b, '\n'), 0o600)
}

// Due reports whether a check should run now: never when the person
// has switched the check off, and otherwise only once every Interval.
//
// enabled is passed in rather than read here so that this stays a pure
// function of the two facts it is about — the setting and the clock —
// and so the caller reads the setting from the file at the moment it
// asks, which is D-134's rule.
func Due(s State, enabled bool, now time.Time) bool {
	if !enabled {
		return false
	}
	if s.LastCheck.IsZero() {
		return true
	}
	// A LastCheck in the future is a clock that has moved backwards —
	// a machine whose time was wrong and has been corrected. Treating
	// it as due is what stops the check being suppressed until the
	// future catches up.
	if s.LastCheck.After(now) {
		return true
	}
	return now.Sub(s.LastCheck) >= Interval
}

// ErrNoMSI is returned when a verified release publishes no MSI. Such
// a release is something the person can be told about and pointed at;
// it is not something this agent can install.
var ErrNoMSI = errors.New("update: the release publishes no installer this agent can run")
