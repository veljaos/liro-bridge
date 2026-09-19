//go:build windows || softtoken

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// TestAConfiguredModulesPathIsNotWrittenIntoTheAuditLog is where SPEC §6.7 is
// paid, and the test is here because the rule is easy to undo by accident:
// audit.Entry.Module is a string, rec.origin.module is a string, and assigning
// one to the other compiles and looks right.
//
// A module found at one of this project's known installation paths is recorded
// whole — those are under Program Files or System32 and carry no personal name
// by construction. A module found where a person put it themselves is recorded
// by file name only, because that path can be anywhere, including under their
// user profile, where it carries their name.
func TestAConfiguredModulesPathIsNotWrittenIntoTheAuditLog(t *testing.T) {
	t.Run("a known path is kept whole", func(t *testing.T) {
		o := signerOrigin{
			backend:    "pkcs11",
			module:     `C:\Program Files\MUP RS\Celik\netsetpkcs11_x64.dll`,
			configured: false,
		}
		if got := o.auditModule(); got != o.module {
			t.Errorf("auditModule() = %q, want the whole path %q", got, o.module)
		}
	})

	t.Run("a configured path is reduced to its file name", func(t *testing.T) {
		// A path with a person's name in it, which is the case the rule exists
		// for and the one a reader of this test should see.
		o := signerOrigin{
			backend:    "pkcs11",
			module:     `C:\Users\Veljko\Desktop\vendor\netsetpkcs11_x64.dll`,
			configured: true,
		}
		got := o.auditModule()
		if got != "netsetpkcs11_x64.dll" {
			t.Errorf("auditModule() = %q, want %q", got, "netsetpkcs11_x64.dll")
		}
		if strings.Contains(got, "Veljko") || strings.Contains(got, `\`) {
			t.Errorf("auditModule() = %q, which still carries a directory: SPEC §6.7 "+
				"says this log never contains personal names, and a configured path "+
				"can be under a user profile", got)
		}
	})

	t.Run("no module means no module", func(t *testing.T) {
		if got := (signerOrigin{backend: "windows-cng"}).auditModule(); got != "" {
			t.Errorf("auditModule() = %q for a CNG session, want empty", got)
		}
		// Including the case where nothing was opened at all, which is what a
		// refusal records.
		if got := (signerOrigin{}).auditModule(); got != "" {
			t.Errorf("auditModule() = %q for the zero origin, want empty", got)
		}
	})
}

// TestDropOriginPassesBothAnswersThrough. The headless commands write no audit
// entry, so they take the chooser with the origin dropped rather than a second
// copy of a three-backend fallback that would have to be kept in step.
func TestDropOriginPassesBothAnswersThrough(t *testing.T) {
	want := errors.New("the card is not in the reader")
	called := 0
	open := dropOrigin(func(context.Context, keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
		called++
		return nil, signerOrigin{backend: "pkcs11"}, want
	})
	sess, err := open(context.Background(), "ABCD")
	if called != 1 {
		t.Errorf("the wrapped chooser was called %d times, want 1", called)
	}
	if sess != nil {
		t.Error("a session came back from a call that failed")
	}
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
