# Real signed PDF fixtures (not committed)

This directory is for the **three real signed PDFs** F3 §2.1, §10.1 and
§10.2 are built and tested against — one from MUP, one from Pošta
Srbije, one from Halcom — not the synthetic fixtures the parser and
incremental-update tests use (built in Go, in
`internal/pades/pdf/fixtures_test.go`; see that package's tests).

**This directory is deliberately ignored by git** (see `.gitignore`).
A real Serbian qualified signature embeds the signer's name, national ID
number and email address in the signer certificate — exactly the
personal data SPEC §6.7/§11.6 says must never be committed to a public
repository — so, mirroring `testdata/certs/local/` (see its README),
nothing you place here is ever pushed.

## What to place here

Real signed PDFs, one from each issuer, matching F3 §2.1's naming intent:

```
testdata/pdfs/local/
├── mup.pdf       # classic base + one cross-reference-stream revision (4 object streams)
├── posta.pdf     # classic base + one cross-reference-stream revision (1 object stream)
└── halcom.pdf    # classic xref tables throughout, no streams
```

**Corrected structural measurement** (F3 §2.1's own table, and this
file's previous revision, understated the nuance — see `docs/decisions.md`
D-050 for the full account, including a real parser bug this measurement
exposed). Measured directly against the three files now present, by
walking each document's actual `/Prev` chain rather than assuming from a
summary table:

| Fixture | Revisions | Classic xref sections | Cross-reference stream sections | Object streams |
|---|---|---|---|---|
| halcom.pdf | 3 | 3 | 0 | 0 |
| mup.pdf | 4 | 4 (incl. one hybrid stub) | 1 | 4 |
| posta.pdf | 4 | 4 (incl. one hybrid stub) | 1 | 1 |

Neither mup.pdf nor posta.pdf is "purely stream-based": each is a
classic-table base revision (the original document), then **one**
revision that upgrades to a genuine cross-reference stream — reached via
a backward-compatible hybrid `xref 0 0` + `/XRefStm` pointer (PDF
32000-1 §7.5.8.4), exactly the hybrid case F3 §2.1 calls out — and then
two further plain classic-table revisions appended on top of that. Only
that one stream revision carries object streams: four distinct ones for
mup.pdf, one for posta.pdf.

Each document also contains **two** signature slots, not one: the main
PAdES signature (`/SubFilter /ETSI.CAdES.detached`) and a separate
document-timestamp revision (`/SubFilter /ETSI.RFC3161`, SPEC §12.2)
appended afterwards. The document-timestamp token is BER-encoded with
indefinite length (`30 80 …`, SPEC §12.5). The main signature's own
*embedded* timestamp (a different, unsigned CMS attribute) uses a SHA-1
imprint — a defect in the state's signing tool that SPEC §12.4 already
documents; this project's independent verifier correctly rejects it and
is not expected to report it as OK.

Any subset is fine. The real-fixture tests this project's test suite
runs against this directory (real-document parsing, cross-reference
mechanism measurement, incremental-update byte preservation, existing-
signature extraction and Trusted List classification, independent
verification of the existing signature, BER document-timestamp parsing,
and the already-signed-document test) skip themselves, with a clear
message, when a given file is absent — so `go test ./...` on a machine
with none of these present still passes normally, exercising only the
synthetic fixtures. Add whichever real documents you have signing
authority over or explicit permission to use for testing; more coverage
means more of this phase's real-fixture tests actually run instead of
skipping.

## Why this matters, and what is now verified

SPEC §12 and F3 were written against, and every measured fact in them
(cross-reference mechanism per issuer, CMS blob sizes, BER timestamp
tokens, the MUP-single-certificate chain, the Halcom AIA defect) comes
from, three real signed PDFs. The parser, incremental-update writer, CMS
layer and verifier are still built and unit-tested primarily against
**synthetic** fixtures (`internal/pades/pdf/fixtures_test.go`) that
reproduce the structural mechanisms (classic xref table; xref stream +
object stream) — those prove the *shape* of the code cheaply and
deterministically — but with the three real files now present, this
project's understanding of each real CA's actual output is exercised
directly, not assumed. See `internal/pades/pdf/real_test.go`,
`internal/pades/verify/real_test.go` and `internal/pades/real_test.go`.

The following F3 exit-condition items are exercised against all three
real documents and pass:

- Parsing all three real fixtures completely — every object resolves,
  including objects compressed inside object streams, across the full
  `/Prev` chain (`TestRealFixturesParseCompletely`).
- The cross-reference mechanism each fixture actually uses, measured
  independently of `Document`'s own bookkeeping
  (`TestRealFixturesCrossReferenceMechanismMatchesMeasurement`) — see the
  corrected table above.
- Extracting the signature dictionary, `/ByteRange`, CMS blob and signer
  certificate, and classifying the signer as qualified against the real,
  bundled Trusted List (`TestRealFixturesExtractSignatureAndClassifyQualified`).
- The independent verifier's acceptance of each document's existing,
  genuine signature — the `/ByteRange` digest matches `messageDigest` and
  the RSA signature over the re-tagged `SET OF` verifies
  (`TestRealFixturesIndependentVerificationAcceptsExistingSignature`).
- Parsing the real, BER-encoded (indefinite-length) document-timestamp
  token present in every fixture
  (`TestRealFixturesDocumentTimestampIsBERIndefiniteLength`).
- The already-signed-document test against a genuine prior signature —
  the single most important test in this phase (F3 §10.2) — now proven
  cryptographically, not only structurally: signing each real document
  again leaves the original CA's signature verifying and adds a second,
  independently verifying signature
  (`TestRealFixturesAlreadySignedDocumentBothSignaturesVerify`).
- Incremental update preserves the real documents' original bytes
  literally (`TestRealFixturesIncrementalUpdatePreservesOriginalBytes`).

Still not exercised without further access: manual acceptance in Adobe
Reader, the PKS/Inception/eUprava validation services, and signing a
document already signed with a real card (SPEC §16.7, F3 §10.3) — these
remain pending, as they require the actual applications/services, not
just the files.
