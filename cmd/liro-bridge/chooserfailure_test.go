package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/chooser"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// chooser.expired names the ceiling, because a window that closed itself
// should say why (the owner, D-410). The number in the sentence is the
// constant's, in every catalogue, so the two cannot drift apart.
func TestTheExpiredSentenceNamesTheCeiling(t *testing.T) {
	minutes := strconv.Itoa(int(chooser.Ceiling.Minutes()))
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		if s := i18n.Load(locale).T("chooser.expired"); !strings.Contains(s, minutes) {
			t.Errorf("%s: chooser.expired = %q does not name %s minutes", locale, s, minutes)
		}
	}
}

// Only the Linux choosers' two errors produce text; anything else — every
// error a Windows dialog can return — produces none, as before.
func TestOnlyTheChoosersOwnErrorsAreSaid(t *testing.T) {
	c := i18n.Load("sr-Latn")
	failed := fmt.Errorf("%w: refused: x", ui.ErrChooserFailed)
	expired := fmt.Errorf("%w: x", ui.ErrChooserExpired)
	if got := chooserFailureText(c, failed, "chooser.files_failed"); got != c.T("chooser.files_failed") {
		t.Errorf("failed: %q", got)
	}
	if got := chooserFailureText(c, expired, "chooser.files_failed"); got != c.T("chooser.expired") {
		t.Errorf("expired: %q", got)
	}
	if got := chooserFailureText(c, errors.New("CommDlgExtendedError 0x3002"), "chooser.files_failed"); got != "" {
		t.Errorf("another error said %q; Windows would have changed", got)
	}
}
