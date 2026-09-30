package main

import (
	"errors"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// chooserFailureText is what a window says when a file or folder chooser
// failed (D-410): failedKey's sentence for one that could not be shown,
// chooser.expired's for one closed by its ceiling, and nothing for any
// other error.
//
// Only the Linux choosers return ui.ErrChooserFailed or ErrChooserExpired.
// A Windows dialog's error is neither, so there this returns "" and every
// window behaves exactly as it did before.
func chooserFailureText(c *i18n.Catalogue, err error, failedKey string) string {
	switch {
	case errors.Is(err, ui.ErrChooserExpired):
		return c.T("chooser.expired")
	case errors.Is(err, ui.ErrChooserFailed):
		return c.T(failedKey)
	}
	return ""
}
