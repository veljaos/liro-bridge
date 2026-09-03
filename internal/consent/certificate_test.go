package consent

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// TestBuildCertificateOptionsNeverHidesAnything is F5 §5.2: "Non-signing
// certificates are shown disabled with the reason, not hidden."
func TestBuildCertificateOptionsNeverHidesAnything(t *testing.T) {
	certs := []classify.Info{
		{Thumbprint: "AAAA1111", Purpose: classify.PurposeSigning},
		{Thumbprint: "BBBB2222", Purpose: classify.PurposeAuthentication, NotUsableReason: errs.CodeCertNotUsable},
		{Thumbprint: "CCCC3333", Purpose: classify.PurposeUnknown, NotUsableReason: errs.CodeCertExpired},
	}
	got := BuildCertificateOptions(certs)
	if len(got) != len(certs) {
		t.Fatalf("got %d options, want %d — every certificate must appear", len(got), len(certs))
	}
	if got[1].Usable {
		t.Fatal("an authentication-purpose certificate with a NotUsableReason must not report Usable")
	}
	if got[1].DisabledReason != errs.CodeCertNotUsable {
		t.Fatalf("got DisabledReason %q, want %q", got[1].DisabledReason, errs.CodeCertNotUsable)
	}
}

// TestBuildCertificateOptionsRoleFromPurpose covers the label F5 §5.2
// requires ("for signing" / "for login") is driven by KeyUsage-derived
// Purpose, never by anything else.
func TestBuildCertificateOptionsRoleFromPurpose(t *testing.T) {
	tests := []struct {
		purpose classify.Purpose
		want    Role
	}{
		{classify.PurposeSigning, RoleSigning},
		{classify.PurposeAuthentication, RoleAuthentication},
		{classify.PurposeUnknown, RoleUnknown},
	}
	for _, tc := range tests {
		got := BuildCertificateOptions([]classify.Info{{Purpose: tc.purpose}})
		if got[0].Role != tc.want {
			t.Errorf("purpose %v: got role %q, want %q", tc.purpose, got[0].Role, tc.want)
		}
	}
}

// TestBuildCertificateOptionsDistinguishesIdenticalSubjects is SPEC
// §11.5: two certificates on one card can share a Subject byte-for-byte
// — the thumbprint tail and role must be enough to tell them apart even
// when DisplayName and Issuer are identical.
func TestBuildCertificateOptionsDistinguishesIdenticalSubjects(t *testing.T) {
	shared := classify.Subject{DisplayName: "Zoran Milovanović"}
	certs := []classify.Info{
		{Thumbprint: "0123456789ABCDEF01234567", Subject: shared, IssuerCN: "Halcom", Purpose: classify.PurposeSigning},
		{Thumbprint: "FEDCBA9876543210FEDCBA98", Subject: shared, IssuerCN: "Halcom", Purpose: classify.PurposeAuthentication},
	}
	got := BuildCertificateOptions(certs)
	if got[0].ThumbprintTail == got[1].ThumbprintTail {
		t.Fatal("two certificates with different thumbprints produced the same ThumbprintTail")
	}
	if got[0].Role == got[1].Role {
		t.Fatal("the signing and authentication certificates must have different roles")
	}
}

func TestThumbprintTail(t *testing.T) {
	if got := thumbprintTail("0123456789ABCDEF", 8); got != "89ABCDEF" {
		t.Fatalf("got %q, want %q", got, "89ABCDEF")
	}
	// A thumbprint shorter than the requested tail length must not
	// panic — it is returned whole.
	if got := thumbprintTail("ABC", 8); got != "ABC" {
		t.Fatalf("got %q, want %q", got, "ABC")
	}
}

// TestBuildCertificateOptionsMarksTestKeys is SPEC §16.6 applied here:
// a soft-token certificate must carry an unmissable marker.
func TestBuildCertificateOptionsMarksTestKeys(t *testing.T) {
	got := BuildCertificateOptions([]classify.Info{{IsTestKey: true}, {IsTestKey: false}})
	if !got[0].IsTestKey {
		t.Fatal("test-key certificate lost its IsTestKey marker")
	}
	if got[1].IsTestKey {
		t.Fatal("a real certificate was marked as a test key")
	}
}
