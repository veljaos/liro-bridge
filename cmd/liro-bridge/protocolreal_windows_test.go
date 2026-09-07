//go:build windows

package main

// The three real, already-signed documents, through the protocol
// (F7 §12: "Both signing paths, producing signatures that the
// independent verifier accepts, over the three real fixtures").
//
// They are not in this repository — a real Serbian qualified signature
// embeds the signer's name, national identity number and email address
// ([[D-038]]) — so this skips itself when they are not there, exactly
// as every other real-fixture test in this project does.

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/verify"
)

// realFixtures returns the three documents, or skips.
func realFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, name := range []string{"mup.pdf", "posta.pdf", "halcom.pdf"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "local", name))
		if err != nil {
			t.Skipf("the real fixtures are not present: %v", err)
		}
		out[name] = data
	}
	return out
}

// TestTheRealFixturesSignedOverTheDocumentPathVerify signs each of the
// three real documents through the protocol's whole-document path and
// checks the result with this project's own independent verifier
// (SPEC §16.4).
//
// It is the same engine a person's own batch uses, reached the same
// way, so what this actually proves is that the protocol path does not
// change any of it: the original bytes are still a literal prefix, the
// document's own existing signatures still verify, and the new one
// verifies too.
func TestTheRealFixturesSignedOverTheDocumentPathVerify(t *testing.T) {
	fixtures := realFixtures(t)
	session := newStampSession(t)

	for name, original := range fixtures {
		t.Run(name, func(t *testing.T) {
			req := api.SignRequest{
				Application: "Knjigovodstvo doo",
				Kind:        api.SignDocuments,
				Documents:   []api.Document{{Name: name, Content: original}},
				Digests:     [][]byte{consent.DigestOf(original)},
				Labels:      []string{name},
			}
			m := protocolWindowFor(t, req)
			m.cfg.VisibleStamp = false
			m.auditStore = tempAuditStore(t)

			m.runBatch(context.Background(), consentDecision{
				approved: true, thumbprint: "AABB", session: session,
				level: pades.LevelBB, allowBB: true, cfg: m.cfg,
			})

			result := m.remote.result()
			if result.Code != "" {
				t.Fatalf("the batch reported %q", result.Code)
			}
			signed := result.Outcomes[0].Document
			if result.Outcomes[0].Code != "" || len(signed) == 0 {
				t.Fatalf("the document came back with code %q and %d bytes",
					result.Outcomes[0].Code, len(signed))
			}

			// F3 §3.1: an incremental update never touches what was
			// there before.
			if len(signed) <= len(original) || string(signed[:len(original)]) != string(original) {
				t.Fatal("the original bytes are not a literal prefix of the signed document")
			}

			slots, err := verify.FindSignatures(signed)
			if err != nil {
				t.Fatalf("FindSignatures: %v", err)
			}
			if len(slots) < 2 {
				t.Fatalf("the signed document has %d signature slots; the fixture already had at least one", len(slots))
			}

			// The one this run added is the last slot. The document's
			// own earlier ones are checked by
			// internal/pades/verify's own real-fixture tests, which
			// know which of them carry the SHA-1 imprint defect SPEC
			// §12.4 records from the state's signing tool.
			r := verify.VerifySignature(signed, slots[len(slots)-1])
			if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
				t.Fatalf("the independent verifier rejected the new signature: %+v", r.Errors)
			}
			if result.Outcomes[0].AchievedLevel != string(pades.LevelBB) {
				t.Fatalf("achievedLevel is %q, want the level actually reached", result.Outcomes[0].AchievedLevel)
			}
		})
	}
}

// TestTheRealFixturesSignedOverTheHashPathVerify is the other half, and
// it is a different check because the agent never sees the document on
// that path: what it produces is a raw signature over a digest the
// caller computed, so what can be verified is exactly that — the
// signature is over the bytes that were sent, under the key the person
// approved.
//
// The digests here are SHA-256 over the three real documents, which is
// what a caller that had built its own CMS would be sending.
func TestTheRealFixturesSignedOverTheHashPathVerify(t *testing.T) {
	fixtures := realFixtures(t)
	session := newStampSession(t)

	req := api.SignRequest{
		Application: "Knjigovodstvo doo",
		Kind:        api.SignDigests,
		Thumbprint:  "AABB",
	}
	var names []string
	for name, data := range fixtures {
		req.Digests = append(req.Digests, consent.DigestOf(data))
		req.Labels = append(req.Labels, name)
		names = append(names, name)
	}

	m := protocolWindowFor(t, req)
	m.auditStore = tempAuditStore(t)
	m.runBatch(context.Background(), consentDecision{
		approved: true, thumbprint: "AABB", session: session,
		level: pades.LevelBB, allowBB: true, cfg: m.cfg,
	})

	result := m.remote.result()
	if result.Code != "" {
		t.Fatalf("the batch reported %q", result.Code)
	}
	cert, err := x509.ParseCertificate(session.Certificate().DER)
	if err != nil {
		t.Fatalf("parsing the signer certificate: %v", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("the signer's key is %T", cert.PublicKey)
	}
	for i, o := range result.Outcomes {
		if o.Code != "" {
			t.Fatalf("%s failed with %q", names[i], o.Code)
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, req.Digests[i], o.Signature); err != nil {
			t.Fatalf("%s: the signature does not verify against the digest that was sent: %v", names[i], err)
		}
	}
}
