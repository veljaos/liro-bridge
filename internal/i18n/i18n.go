// Package i18n implements SPEC §9: exactly three locales, always with the
// script subtag. A bare "sr" is never resolved — it falls back like any
// other unknown locale, because in CLDR it silently means Cyrillic, which
// would give the wrong script to users who expect Latin.
package i18n

import (
	"embed"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"

	"github.com/veljaos/liro-bridge/internal/errs"
)

//go:embed locales/*.json
var localesFS embed.FS

// defaultLocale is used when the requested locale is unknown, empty, or
// the bare "sr" (SPEC §9.1: bare "sr" must never resolve to sr-Cyrl).
const defaultLocale = "sr-Latn"

// fallbackLocale is used for individual missing keys, per SPEC §9.2: a
// missing translation falls back to English before falling back to the key.
const fallbackLocale = "en"

var (
	cataloguesOnce sync.Once
	catalogues     map[string]map[string]string
)

func loadCatalogues() map[string]map[string]string {
	files := map[string]string{
		"sr-Latn": "locales/sr-Latn.json",
		"sr-Cyrl": "locales/sr-Cyrl.json",
		"en":      "locales/en.json",
	}
	out := make(map[string]map[string]string, len(files))
	for locale, path := range files {
		b, err := localesFS.ReadFile(path)
		if err != nil {
			// The catalogues are embedded at compile time; a read failure
			// here means the binary itself was built without them.
			panic("i18n: embedded catalogue missing: " + path + ": " + err.Error())
		}
		var data map[string]string
		if err := json.Unmarshal(b, &data); err != nil {
			panic("i18n: embedded catalogue is not valid JSON: " + path + ": " + err.Error())
		}
		out[locale] = data
	}
	return out
}

// Catalogue resolves message keys for one locale.
type Catalogue struct {
	locale string
	data   map[string]string
}

// Load returns a catalogue for the requested locale. An unknown or empty
// locale falls back to sr-Latn. A bare "sr" is treated as invalid input
// and falls back — never silently resolved to Cyrillic (see SPEC §9.1).
func Load(locale string) *Catalogue {
	cataloguesOnce.Do(func() { catalogues = loadCatalogues() })

	if data, ok := catalogues[locale]; ok {
		return &Catalogue{locale: locale, data: data}
	}
	return &Catalogue{locale: defaultLocale, data: catalogues[defaultLocale]}
}

// T returns the message for key. A missing key falls back to the English
// catalogue and logs a warning. If the key is missing there too, T returns
// the key itself — visible in the UI, which is the point: it must be
// noticed, not hidden.
func (c *Catalogue) T(key string) string {
	if v, ok := c.data[key]; ok {
		return v
	}
	if c.locale != fallbackLocale {
		if v, ok := catalogues[fallbackLocale][key]; ok {
			slog.Warn("i18n: missing translation key, falling back to en", "locale", c.locale, "key", key)
			return v
		}
	}
	slog.Warn("i18n: missing translation key in every catalogue", "key", key)
	return key
}

// CodeKey maps an error code to its message key:
// CodeCardNotPresent -> "error.card_not_present"
func CodeKey(code errs.Code) string {
	return "error." + strings.ToLower(string(code))
}
