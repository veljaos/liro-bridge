package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// This file holds the error rendering every front door shares. It
// carries no build tag deliberately: the signing command that does not
// ask anybody exists only under the "softtoken" tag (see
// sign_noconsent.go), and the window flow — which is what `sign` is in a
// release build — renders its failures through exactly this code
// (cmd/liro-bridge's pushFailure and the report screen). One renderer,
// one behaviour, in every build.

// ErrorMessage renders err the way this package's own output does, for
// a caller outside it that shows the same errors to the same person —
// the signing window, which had been rendering the bare catalogue
// message and so printed a stamp-glyph failure with its two "%s"
// placeholders unfilled. One renderer, one behaviour: a code's Details
// reach the user in exactly one shape, whichever front door the
// signature was started from.
func ErrorMessage(err error, c *i18n.Catalogue) string {
	return errMessage(err, c)
}

// errMessage renders err for the CLI's own local, English-or-localised
// output — not an API boundary (SPEC §7 governs internal/api's JSON
// responses). A structured *errs.Error's Details are appended, since
// they carry exactly the kind of fact a person debugging a failed
// signing run needs — the character and code point of a glyph missing
// from the stamp's font subset (F4 §3.3), for instance, which the
// localised code alone ("SIGN_FAILED") would not convey.
func errMessage(err error, c *i18n.Catalogue) string {
	var e *errs.Error
	if errors.As(err, &e) {
		if e.Code == errs.CodeStampGlyphMissing {
			// The catalogue message itself names the character and its
			// code point ("...: %s (%s)."), rather than having them
			// appended generically the way other codes' Details are —
			// this is the one error whose whole point is to be read as a
			// sentence, not a code plus a debugging fragment.
			return fmt.Sprintf(c.T(i18n.CodeKey(e.Code)), e.Details["character"], e.Details["codePoint"])
		}
		msg := c.T(i18n.CodeKey(e.Code))
		if len(e.Details) > 0 {
			msg += " (" + formatDetails(e.Details) + ")"
		}
		return msg
	}
	return err.Error()
}

func formatDetails(details map[string]any) string {
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%v", k, details[k])
	}
	return strings.Join(parts, ", ")
}
