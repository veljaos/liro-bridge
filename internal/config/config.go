// Package config implements the agent's persistent configuration (SPEC
// §5, F0 §2) and its logging setup (F0 §3).
package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// Config is the agent's persistent configuration. Every field has a usable
// zero-value default, so a missing or partial file is never an error.
type Config struct {
	// Locale is the interface language: "sr-Latn", "sr-Cyrl" or "en".
	Locale string `json:"locale"`

	// LogLevel is one of "debug", "info", "warn", "error".
	LogLevel string `json:"logLevel"`

	// PortRangeStart and PortRangeEnd bound the loopback listener search.
	PortRangeStart int `json:"portRangeStart"`
	PortRangeEnd   int `json:"portRangeEnd"`

	// StartWithWindows controls the HKCU autostart registration (F5
	// §3/§7). Defaults on — SPEC's product shape is a background agent
	// that is simply there, not something the user remembers to launch.
	StartWithWindows bool `json:"startWithWindows"`

	// TSAURL is the configured timestamp authority (F5 §7). Empty means
	// "none configured" — D-067 (F3) already established that sign
	// fails rather than falling back to a hard-coded default endpoint
	// when a level is requested and no TSA is set; this field is what
	// the settings window writes.
	TSAURL string `json:"tsaURL"`

	// TSAUser and TSAPassword are HTTP Basic credentials for TSAURL
	// (Task 6, F5 review): SPEC §12.7's Pošta test endpoint requires
	// them, and the CLI has carried --tsa-user/--tsa-password since F3
	// — this is that same credential pair, reachable from Settings.
	// Empty means no Basic auth header is sent. Never logged: see
	// tray_windows.go's handleSettingsAction, which only ever logs a
	// save failure's error, not the Config value itself.
	TSAUser     string `json:"tsaUser"`
	TSAPassword string `json:"tsaPassword"`

	// TSAClientCertPath and TSAClientCertPassword configure TLS client
	// certificate authentication (Task 6): a PKCS#12 file and its
	// password, matching the CLI's --tsa-client-cert/
	// --tsa-client-cert-password (SPEC §12.7's second Pošta test
	// endpoint exercises this; production TSAs generally require it).
	// Empty means no client certificate is presented.
	TSAClientCertPath     string `json:"tsaClientCertPath"`
	TSAClientCertPassword string `json:"tsaClientCertPassword"`

	// OutputSuffix is F3 §12.11's default-output-naming suffix, exposed
	// as a setting per F5 §7. "document.pdf" -> "document-signed.pdf".
	OutputSuffix string `json:"outputSuffix"`

	// SignatureLevel is "b-b", "b-t" or "b-lt" (SPEC §12.6); B-LT is the
	// project's default.
	//
	// "b-b" is a deliberate decision, not a fallback: a user who has
	// settled that they do not want a timestamp has said so once, here,
	// and the consent window then never asks them again (D-105). SPEC
	// §12.6 calls B-B "fallback only, on explicit user choice" — this
	// field is where that explicit choice lives.
	SignatureLevel string `json:"signatureLevel"`

	// VisibleStamp requests the visual signature stamp (SPEC §13) for
	// signatures started from the consent window. On by default: a
	// signature nobody can see reads as a signature that was never
	// applied, which is exactly how the invisible default was first
	// reported (D-103). The command line is unchanged — there, --stamp
	// is still the only thing that draws one.
	VisibleStamp bool `json:"visibleStamp"`

	// StampPosition is one of the four page corners the consent window
	// offers: "bottom-right" (SPEC §13.1's own default),
	// "bottom-left", "top-right", "top-left". A visual placement picker
	// is a later phase (SPEC §13.1).
	StampPosition string `json:"stampPosition"`

	// UpdateCheckEnabled toggles the daily GitHub Releases check (SPEC
	// §6.8/§15.2). On by default; disableable, never silently ignored.
	UpdateCheckEnabled bool `json:"updateCheckEnabled"`

	// StampPage is which page the visible stamp is drawn on (F6 §6):
	// "first", "last", or a positive page number written as a decimal
	// string. A number past the end of a document is clamped to its
	// last page rather than refused — the same reasoning F6 §6 gives
	// for coordinates, that a stamp nudged inside is better than a
	// refusal.
	StampPage string `json:"stampPage"`

	// StampReference is the optional free-text line the stamp carries
	// (SPEC §13.5's "optional identifier line"), previously reachable
	// only as the CLI's --stamp-reference. Empty means no such line.
	StampReference string `json:"stampReference"`

	// StampShowDocumentID adds the signer's identity document number to
	// the stamp. Off by default and deliberately kept that way: SPEC
	// §13.5 calls it personal data appearing on a document that will be
	// sent to third parties, "available as an option, never the
	// default". The national identity number is a different thing again
	// and is unreachable from the stamp entirely (D-054/D-064).
	StampShowDocumentID bool `json:"stampShowDocumentID"`

	// StampX, StampY and StampPlacedPage are the position chosen in the
	// placement window: the stamp's lower-left corner in the page's own
	// coordinates, in points, and the page it was chosen on (F6b §3).
	//
	// They are read only when StampPosition is "custom", and they are
	// the whole of what is remembered. One position rather than a named
	// list of them: the need this answers is a person who signs the
	// same shaped document over and over and wants the stamp under the
	// same printed initials each time, which one position covers
	// completely — see internal/placement.Saved for the rest of that
	// reasoning.
	//
	// A saved page past the end of a particular document falls back to
	// that document's last page, and a position off the edge of a
	// smaller page is brought inside its margin; both are reported
	// afterwards rather than refused.
	StampX          float64 `json:"stampX"`
	StampY          float64 `json:"stampY"`
	StampPlacedPage int     `json:"stampPlacedPage"`

	// OutputFolder is where signed documents are written. Empty — the
	// default — means beside each input, which is F6 §4's own default
	// and the only one that behaves sensibly for a batch gathered from
	// several folders.
	OutputFolder string `json:"outputFolder"`

	// ExplorerMenuEnabled controls the "Potpiši koristeći Liro Bridge"
	// entry on .pdf files (F6 §2). On by default, registered under
	// HKCU so it needs no administrator rights, and removed cleanly
	// when switched off.
	ExplorerMenuEnabled bool `json:"explorerMenuEnabled"`
}

const (
	defaultLocale         = "sr-Latn"
	defaultLogLevel       = "info"
	defaultPortRangeStart = 17580
	defaultPortRangeEnd   = 17590

	minPort = 1024
	maxPort = 65535

	defaultStartWithWindows   = true
	defaultOutputSuffix       = "-signed"
	defaultSignatureLevel     = "b-lt"
	defaultUpdateCheckEnabled = true
	defaultVisibleStamp       = true
	defaultStampPosition      = "bottom-right"
	defaultStampPage          = "first"

	// defaultExplorerMenuEnabled is on: F6 §2 asks for the context-menu
	// entry to be registered by default, and a signing agent nobody can
	// reach by right-clicking a document is one people forget they have.
	defaultExplorerMenuEnabled = true
)

// StampPageFirst and StampPageLast are the two symbolic values
// StampPage accepts alongside a decimal page number (F6 §6).
const (
	StampPageFirst = "first"
	StampPageLast  = "last"
)

// validSignatureLevels are the three levels the settings window offers
// (SPEC §12.6). B-B is included because a user may decide against a
// timestamp deliberately; it is never selected for them.
var validSignatureLevels = map[string]bool{
	"b-b":  true,
	"b-t":  true,
	"b-lt": true,
}

// StampPositionCustom is the fifth value StampPosition can take: not a
// corner, but the exact place chosen in the placement window and kept
// in StampX, StampY and StampPlacedPage (F6b §3).
const StampPositionCustom = "custom"

// validStampPositions are the four corners SPEC §13.1 names, plus the
// placed position F6b §2 adds. Explicit x/y coordinates are still a
// command-line capability too (--stamp-xy); what is new is that the
// window can now produce them.
var validStampPositions = map[string]bool{
	"bottom-right":      true,
	"bottom-left":       true,
	"top-right":         true,
	"top-left":          true,
	StampPositionCustom: true,
}

// Default returns the configuration used when no file exists and when a
// field is missing or invalid.
func Default() Config {
	return Config{
		Locale:             defaultLocale,
		LogLevel:           defaultLogLevel,
		PortRangeStart:     defaultPortRangeStart,
		PortRangeEnd:       defaultPortRangeEnd,
		StartWithWindows:   defaultStartWithWindows,
		OutputSuffix:       defaultOutputSuffix,
		SignatureLevel:     defaultSignatureLevel,
		UpdateCheckEnabled: defaultUpdateCheckEnabled,
		VisibleStamp:       defaultVisibleStamp,
		StampPosition:      defaultStampPosition,

		StampPage:           defaultStampPage,
		ExplorerMenuEnabled: defaultExplorerMenuEnabled,
	}
}

// ValidStampPage reports whether s is a page selection this project
// accepts: "first", "last", or a positive decimal page number. Anything
// else — a zero, a negative, a word — is not a page and is replaced
// with the default rather than guessed at.
func ValidStampPage(s string) bool {
	if s == StampPageFirst || s == StampPageLast {
		return true
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1
}

// validLocales are the only three locales the agent recognises (SPEC §9.1).
// A bare "sr" is deliberately absent: in CLDR it resolves to Cyrillic,
// which would silently give the wrong script to users who expect Latin.
var validLocales = map[string]bool{
	"sr-Latn": true,
	"sr-Cyrl": true,
	"en":      true,
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Load reads the config file at path, fills any missing or invalid field
// with its default, and returns the result.
//
// A missing file is not an error: it returns Default(). A malformed file
// is a recoverable error: Load logs a warning, returns Default(), and
// never overwrites the file — replacing a file the user hand-edited would
// be hostile. An invalid value in an otherwise well-formed file (a bare
// "sr", a port outside 1024-65535) is replaced with its default and a
// warning is logged for that field alone.
func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		slog.Warn("config: file is not valid JSON, using defaults", "path", path, "error", err)
		return Default(), err
	}

	validate(&cfg)
	return cfg, nil
}

// validate replaces any invalid field with its default, logging a warning
// naming the field that was rejected.
func validate(cfg *Config) {
	if !validLocales[cfg.Locale] {
		slog.Warn("config: invalid locale, using default", "value", cfg.Locale, "default", defaultLocale)
		cfg.Locale = defaultLocale
	}
	if !validLogLevels[cfg.LogLevel] {
		slog.Warn("config: invalid logLevel, using default", "value", cfg.LogLevel, "default", defaultLogLevel)
		cfg.LogLevel = defaultLogLevel
	}
	if cfg.PortRangeStart < minPort || cfg.PortRangeStart > maxPort {
		slog.Warn("config: portRangeStart out of range, using default", "value", cfg.PortRangeStart, "default", defaultPortRangeStart)
		cfg.PortRangeStart = defaultPortRangeStart
	}
	if cfg.PortRangeEnd < minPort || cfg.PortRangeEnd > maxPort {
		slog.Warn("config: portRangeEnd out of range, using default", "value", cfg.PortRangeEnd, "default", defaultPortRangeEnd)
		cfg.PortRangeEnd = defaultPortRangeEnd
	}
	if !validSignatureLevels[cfg.SignatureLevel] {
		slog.Warn("config: invalid signatureLevel, using default", "value", cfg.SignatureLevel, "default", defaultSignatureLevel)
		cfg.SignatureLevel = defaultSignatureLevel
	}
	if !validStampPositions[cfg.StampPosition] {
		slog.Warn("config: invalid stampPosition, using default", "value", cfg.StampPosition, "default", defaultStampPosition)
		cfg.StampPosition = defaultStampPosition
	}
	if cfg.StampPosition == StampPositionCustom && cfg.StampPlacedPage < 1 {
		// "custom" with nothing placed is not a position; it is a
		// half-written configuration, and the corner it falls back to
		// is the one a stamp gets when nobody has said otherwise.
		slog.Warn("config: stampPosition is custom but no position is stored, using default",
			"default", defaultStampPosition)
		cfg.StampPosition = defaultStampPosition
	}
	if cfg.StampPlacedPage < 0 {
		cfg.StampPlacedPage = 0
	}
	if cfg.OutputSuffix == "" {
		cfg.OutputSuffix = defaultOutputSuffix
	}
	if !ValidStampPage(cfg.StampPage) {
		slog.Warn("config: invalid stampPage, using default", "value", cfg.StampPage, "default", defaultStampPage)
		cfg.StampPage = defaultStampPage
	}
}

// Save writes cfg to path atomically: it writes to a temporary file in the
// same directory, then renames it into place, so a crash mid-write never
// leaves a truncated config behind.
func Save(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// DefaultPath returns the config.json path for the running platform.
func DefaultPath() string {
	return platform.DefaultConfigFile()
}
