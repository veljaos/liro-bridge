# Soft token test key material (not committed)

This directory holds the soft token's generated PKCS#12 file and
certificate — never committed, because a private key, even a throwaway
test one, ends up copied into something real eventually (F2 §3.3).

**This directory is deliberately ignored by git** (see `.gitignore`:
`/testdata/softtoken/local/*` with an exception for this file).

## Generating

```sh
go run ./scripts/gentestkeys ./testdata/softtoken/local
```

This writes `test.p12` (password `liro-softtoken-test`, unless you pass
a different one as a second argument) and `test-cert.pem`.

## Using it

```sh
export LIRO_SOFTTOKEN_P12=./testdata/softtoken/local/test.p12
export LIRO_SOFTTOKEN_PASSWORD=liro-softtoken-test
go run -tags softtoken ./cmd/liro-bridge certs
```

The soft token is only ever compiled in with the `softtoken` build tag
(SPEC §16.6) — a release binary never has this code path at all, so
setting these variables against a release build does nothing.
