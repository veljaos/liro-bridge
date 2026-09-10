package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// The release signature format, v1. A signature file is three lines of
// ASCII:
//
//	liro-release-v1
//	key <base64 of a 32-byte Ed25519 public key>
//	sig <base64 of the 64-byte signature>
//
// The key line names which of the agent's trusted keys signed this
// release. It is a *selector*, never a credential: VerifySignature
// checks membership in the embedded trust set first and only then
// verifies, so a signature file naming a key this agent does not trust
// is refused whatever the signature says. Carrying it is what makes a
// second key slot cost nothing at rotation time — see trustedkeys.go.
const (
	signatureMagic = "liro-release-v1"
	keyPrefix      = "key "
	sigPrefix      = "sig "
)

// signatureContext is prepended to the signed bytes so that a
// signature over a release manifest can never be presented as a
// signature over anything else this project might sign later. It is
// domain separation, and it costs one string.
const signatureContext = "liro-bridge release manifest v1\n"

// ErrUntrusted is returned when a signature does not verify, names a
// key this agent does not trust, or is malformed. One error, because a
// caller can do exactly one thing about any of them: leave the release
// alone.
//
// Deliberately one error rather than three: "this signature is invalid"
// and "I do not trust this key" are the same answer to the only
// question being asked, and telling them apart is only useful to
// somebody producing signatures this agent should refuse.
var ErrUntrusted = errors.New("update: the release is not signed by a key this agent trusts")

// ErrNoTrustedKeys is the distinct case where this build embeds no
// public key at all. That is not a rejection of a release — it is this
// agent having nothing to check one against, which the person is told
// in different words, because "the release is not signed properly" and
// "this build cannot check signatures" are different facts and only one
// of them is about the release.
var ErrNoTrustedKeys = errors.New("update: this build embeds no release signing key")

// Signature is a parsed signature file.
type Signature struct {
	PublicKey ed25519.PublicKey
	Sig       []byte
}

// maxSignatureBytes bounds a signature file: three short lines. 4 KB
// is far more than that and still refuses a server answering with a
// document.
const maxSignatureBytes = 4 * 1024

// ParseSignature reads the three-line format above. It does not
// verify anything and does not consult the trust set.
func ParseSignature(b []byte) (Signature, error) {
	if len(b) > maxSignatureBytes {
		return Signature{}, fmt.Errorf("%w: the signature file is %d bytes", ErrUntrusted, len(b))
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(b)), "\r\n", "\n"), "\n")
	if len(lines) != 3 {
		return Signature{}, fmt.Errorf("%w: a signature file is three lines, this has %d", ErrUntrusted, len(lines))
	}
	if strings.TrimSpace(lines[0]) != signatureMagic {
		return Signature{}, fmt.Errorf("%w: the first line is not %q", ErrUntrusted, signatureMagic)
	}
	key, err := decodeField(lines[1], keyPrefix, ed25519.PublicKeySize)
	if err != nil {
		return Signature{}, fmt.Errorf("%w: the key line: %v", ErrUntrusted, err)
	}
	sig, err := decodeField(lines[2], sigPrefix, ed25519.SignatureSize)
	if err != nil {
		return Signature{}, fmt.Errorf("%w: the sig line: %v", ErrUntrusted, err)
	}
	return Signature{PublicKey: ed25519.PublicKey(key), Sig: sig}, nil
}

func decodeField(line, prefix string, want int) ([]byte, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix) {
		return nil, fmt.Errorf("does not begin with %q", prefix)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
	if err != nil {
		return nil, fmt.Errorf("is not base64: %v", err)
	}
	if len(raw) != want {
		return nil, fmt.Errorf("decodes to %d bytes, want %d", len(raw), want)
	}
	return raw, nil
}

// FormatSignature renders a signature file. It is here, beside the
// parser, so that the only two pieces of code that know this format
// are next to each other and cannot drift — scripts/signrelease writes
// what this package reads.
func FormatSignature(s Signature) string {
	return signatureMagic + "\n" +
		keyPrefix + base64.StdEncoding.EncodeToString(s.PublicKey) + "\n" +
		sigPrefix + base64.StdEncoding.EncodeToString(s.Sig) + "\n"
}

// SignedBytes is what a signature is actually computed over: the
// context string followed by the manifest's exact bytes. Exported so
// the signing tool and the agent build the same message from the same
// function rather than from two readings of a comment.
func SignedBytes(manifest []byte) []byte {
	out := make([]byte, 0, len(signatureContext)+len(manifest))
	out = append(out, signatureContext...)
	return append(out, manifest...)
}

// Sign produces a signature file's contents for a manifest. It lives
// in this package rather than in the tool that calls it for the reason
// above; nothing in the agent calls it, and no private key is ever
// stored, embedded or logged by this package.
func Sign(priv ed25519.PrivateKey, manifest []byte) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("update: a private key is %d bytes, got %d", ed25519.PrivateKeySize, len(priv))
	}
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return "", errors.New("update: the private key has no Ed25519 public half")
	}
	return FormatSignature(Signature{
		PublicKey: pub,
		Sig:       ed25519.Sign(priv, SignedBytes(manifest)),
	}), nil
}

// VerifyManifest is the whole gate: it checks that the signature file
// names a key this agent embeds, that the signature verifies over the
// manifest's exact bytes, and only then parses the manifest.
//
// The order matters and is the reason this is one function rather than
// three a caller composes. SPEC §15.2 requires the signature to be
// verified "before doing anything with what it downloaded", and a
// caller that parsed first would already have acted on unverified
// input by the time it asked.
func VerifyManifest(manifestBytes, signatureBytes []byte, trusted []ed25519.PublicKey) (Manifest, error) {
	if len(trusted) == 0 {
		return Manifest{}, ErrNoTrustedKeys
	}
	sig, err := ParseSignature(signatureBytes)
	if err != nil {
		return Manifest{}, err
	}
	if !isTrusted(sig.PublicKey, trusted) {
		return Manifest{}, fmt.Errorf("%w: the key it names is not one of the %d this build embeds", ErrUntrusted, len(trusted))
	}
	if !ed25519.Verify(sig.PublicKey, SignedBytes(manifestBytes), sig.Sig) {
		return Manifest{}, fmt.Errorf("%w: the signature does not verify over these bytes", ErrUntrusted)
	}
	return ParseManifest(manifestBytes)
}

// isTrusted compares against every embedded key. ed25519.PublicKey's
// own Equal is used rather than bytes.Equal so that a future key type
// change is a compile error rather than a silent length comparison.
func isTrusted(key ed25519.PublicKey, trusted []ed25519.PublicKey) bool {
	for _, t := range trusted {
		if key.Equal(t) {
			return true
		}
	}
	return false
}
