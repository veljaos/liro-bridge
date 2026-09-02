# Test certificates

These certificates are **synthetic**, not extracted from a real card.

Genuine Serbian qualified certificates belong to a real, identifiable
person (a name, a national ID number, an email address — exactly the
personal data SPEC §6.7/§11.6 says must never be logged or stored
carelessly). This implementation environment had no access to real
issued cards, so committing "real" fixtures was never an option. Instead
`scripts/gencerts/main.go` builds certificates that reproduce the
structure SPEC §11 documents as measured fact — issuer-specific policy
OIDs, per-issuer KeyUsage (including Halcom's `contentCommitment`-only
signing certificate), multi-valued RDNs, the personal-identifier and
email-address traps, and two certificates sharing one identical
Subject — with fabricated names, national IDs and email addresses.

**The three CA trust anchors are genuine**, though: `mup_signing.der`,
`posta_signing.der` and `halcom_signing.der` (and `halcom_auth.der`,
`expired_signing.der`) carry an `Issuer` field whose bytes are copied
verbatim from the real "MUP Gradjani CA 4", "Posta Srbije CA 1" and
"Halcom BG CA FL e-signature" certificates published in the actual
Republic of Serbia Trusted List bundled at
`internal/trust/tsl/seed/TSL-RS.xml`. This makes
`classify`'s "qualified against the bundled TSL" test exercise real
issuer-name bytes, not a fabricated trust anchor. The synthetic
certificates are not cryptographically signed by those CAs — a
throwaway key is used instead — which is harmless because F1 classifies
by issuer-name matching only; chain/signature verification arrives in
F3 (see docs/decisions.md).

| File | Mimics | Purpose |
|---|---|---|
| `mup_signing.der` | MUP e-ID signing certificate | qualified, signing, Cyrillic CN with trailing " Sign" |
| `posta_signing.der` | Pošta Srbije signing certificate | qualified, signing |
| `halcom_signing.der` | Halcom signing certificate | qualified, signing, KeyUsage = contentCommitment only (SPEC §11.4) |
| `halcom_auth.der` | Halcom authentication certificate | same Subject as halcom_signing.der (SPEC §11.5), KeyUsage = digitalSignature+keyEncipherment |
| `selfsigned_unrelated.der` | the GUID-subject software certificates noted in SPEC §11.10/F1 §3.5 | not qualified, no issuer match |
| `expired_signing.der` | an otherwise-valid MUP-style certificate | NotAfter in the past, for the expiry test |

Regenerate with:

```
go run ./scripts/gencerts internal/trust/tsl/seed/TSL-RS.xml testdata/certs
```

Do not add real certificates extracted from an actual card to this
directory, even public ones with no private key — the point of this
approach is that no personal data from a real document ever needs to
enter the repository at all.
