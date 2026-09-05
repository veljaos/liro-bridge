package classify

// This file holds the one rule that decides which certificates a listing
// shows by default. Every surface that lists certificates — the `certs`
// command, the consent window, the Certificates window — goes through
// IsWindowsInternal, so there is one implementation and not three
// (F6 §0b).

// IsWindowsInternal reports whether this certificate is an artefact
// Windows created for its own use rather than something a person could
// ever sign with. Two shapes count, and a listing hides both by default
// (`--all` still shows them).
//
// The first is the shape F1 §5.4 measured: a certificate of no
// recognised purpose that the Trusted List does not know. That was
// D-023's whole rule.
//
// The second is why D-023's rule was not enough. Measured on the owner's
// machine while starting F6, with the real Windows store:
//
//	5a26d334-110e-4468-910f-34313774f081   purpose authentication
//	    issuer 5a26d334-110e-4468-910f-34313774f081, software, not qualified
//
// A GUID for a name, its own issuer, in software, unknown to the Trusted
// List — the same artefact D-023 was written about — but its KeyUsage
// carries digitalSignature + keyEncipherment, so purposeFromKeyUsage
// calls it "authentication" and D-023's purpose-keyed test let it
// through. Purpose was never what made these certificates internal; it
// only happened to be a reliable proxy on the machine F1 was measured
// on. Naming the actual shape — self-signed, GUID subject, unknown to
// the Trusted List — is what stops the next one of these from
// depending on which KeyUsage bits Windows happened to set.
//
// A soft-token certificate is self-signed and software-backed too, so
// it is excluded explicitly: SPEC §16.6 requires a test key to be
// loudly visible wherever it appears, never quietly filtered out.
func (i Info) IsWindowsInternal() bool {
	if i.IsTestKey {
		return false
	}
	if i.Qualification != QualificationNotQualified {
		return false
	}
	if i.Purpose == PurposeUnknown {
		return true
	}
	return i.SelfSigned && isGUID(i.Subject.CommonName) && isGUID(i.IssuerCN)
}

// guidHyphens are the positions of the '-' separators in the canonical
// 8-4-4-4-12 form.
var guidHyphens = [...]int{8, 13, 18, 23}

// isGUID reports whether s is a canonical GUID, bare
// ("5a26d334-110e-4468-910f-34313774f081", the form measured on the
// owner's machine) or in the braced form Windows also writes
// ("{3F2504E0-4F89-11D3-9A0C-0305E82C3301}", the form F1's own
// selfsigned_unrelated.der fixture reproduces). A "urn:uuid:" prefix
// and any other surrounding text are deliberately not accepted: this is
// matching one specific thing Windows writes into the CN of its own
// certificates, and a looser match is a way to hide a real certificate
// whose name merely resembles one.
//
// Written out rather than compiled as a package-level regexp because
// SPEC §8.5 asks for no init() that does work, and this is short enough
// that the loop says what it accepts more plainly than a pattern would.
func isGUID(s string) bool {
	const guidLen = 36
	if len(s) == guidLen+2 && s[0] == '{' && s[len(s)-1] == '}' {
		s = s[1 : len(s)-1]
	}
	if len(s) != guidLen {
		return false
	}
	for _, at := range guidHyphens {
		if s[at] != '-' {
			return false
		}
	}
	for i := 0; i < guidLen; i++ {
		if i == guidHyphens[0] || i == guidHyphens[1] || i == guidHyphens[2] || i == guidHyphens[3] {
			continue
		}
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

func isHexDigit(b byte) bool {
	switch {
	case b >= '0' && b <= '9', b >= 'a' && b <= 'f', b >= 'A' && b <= 'F':
		return true
	default:
		return false
	}
}
