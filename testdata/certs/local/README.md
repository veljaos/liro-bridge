# Real certificate fixtures (not committed)

This directory is for **real** certificates extracted from actual MUP,
Pošta Srbije or Halcom cards — not the synthetic fixtures in
`testdata/certs/` (see `../README.md`).

**This directory is deliberately ignored by git** (see `.gitignore`:
`/testdata/certs/local/*` with an exception for this file). Real
certificates identify a real person — a name, a national ID number, an
email address, exactly the personal data SPEC §6.7/§11.6 says must never
be committed to a public repository. Nothing you place here is ever
pushed, and this file is the only thing in this directory that is.

## What to place here

DER-encoded (`.der`) end-entity signing and authentication certificates
exported from a real card, with **no private key material** — public
certificates only. Suggested naming, mirroring `testdata/certs/`:

```
testdata/certs/local/
├── mup_signing.der
├── posta_signing.der
├── halcom_signing.der
├── halcom_auth.der       # same card as halcom_signing.der
└── ...
```

Any subset is fine — the tests that read this directory skip themselves
(with a clear message) when it is absent or contains no matching files,
so a machine with no real certificates on hand still passes `go test
./...` normally. Add whichever real certificates you have; more coverage
just means more of `internal/trust/classify`'s real-fixture tests
actually run instead of skipping.

## Where they come from

Export the public certificate (not the private key) from the Windows
certificate store, e.g. via `certmgr.msc` ("Export..." → "DER encoded
binary X.509 (.CER)", renamed to `.der`), or from the card's own
management tool. Only certificates you are personally authorised to
handle — your own card, or one you have explicit permission to use for
testing — belong here.

## Why this split exists

`testdata/certs/` (committed, synthetic) is enough to prove the *shape*
of the code is right. It cannot prove the code's *understanding* of a
real CA's quirks is right, because a synthetic fixture is built from the
same understanding the code itself embodies — a wrong assumption shared
between the fixture and the implementation still produces a passing
test. Real certificates from an actual issuer are the only fixtures that
can contradict that shared assumption. Because they carry personal data
and this repository is public, they can never be committed — hence a
directory that exists for local development and CI-with-real-hardware
setups, but is invisible to `git`.
