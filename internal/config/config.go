// Package config implements the agent's persistent configuration (SPEC
// §5, F0 §2) and its logging setup (F0 §3).
package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

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
}

const (
	defaultLocale         = "sr-Latn"
	defaultLogLevel       = "info"
	defaultPortRangeStart = 17580
	defaultPortRangeEnd   = 17590

	minPort = 1024
	maxPort = 65535
)

// Default returns the configuration used when no file exists and when a
// field is missing or invalid.
func Default() Config {
	return Config{
		Locale:         defaultLocale,
		LogLevel:       defaultLogLevel,
		PortRangeStart: defaultPortRangeStart,
		PortRangeEnd:   defaultPortRangeEnd,
	}
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
