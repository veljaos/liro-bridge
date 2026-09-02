# Liro Bridge

Liro Bridge is a desktop signing agent that lets a person sign PDF documents
with a qualified electronic certificate stored on a smart card or USB token,
and lets other local programs request signatures through a local HTTP API.
The private key never leaves the card, the document never leaves the
machine, and no signature is ever created without a human clicking a button.

This project is under construction. See `docs/SPEC.md` for the full
specification and `docs/phases/` for the phased build plan.

## Verifying a signature independently (F2)

`sign-digest` signs a pre-computed digest with a certificate already
present on this machine (a real smart card, or — for development and
CI, with no hardware at all — the soft token described below). The
following proves the whole chain — CNG plumbing, padding, digest
algorithm, byte order — end to end, using a verifier that shares no code
with this project: OpenSSL.

```bash
# 1. digest of some file
openssl dgst -sha256 -binary input.txt > digest.bin
xxd -p -c 256 digest.bin

# 2. sign it
liro-bridge sign-digest --thumbprint ABCD... --digest <hex> --out sig.bin

# 3. extract the public key from the certificate
# (liro-bridge certs --json nests the list under "certificates", not at
# the top level, and — with more than one certificate installed — .[0]
# is not necessarily the one you just signed with, so select by
# thumbprint rather than by position)
liro-bridge certs --json | jq -r --arg tp "ABCD..." \
  '.certificates[] | select(.thumbprint == $tp) | .pem' > cert.pem
openssl x509 -in cert.pem -pubkey -noout > pub.pem

# 4. verify with a tool that is not ours
openssl dgst -sha256 -verify pub.pem -signature sig.bin input.txt
# expected output:  Verified OK
```

Replace `ABCD...` with the thumbprint from `liro-bridge certs`. This
exact sequence is what CI runs against the soft token on every push (see
`.github/workflows/ci.yml`); running it against a real card is the
manual acceptance step for this phase.

### The soft token — signing with no hardware at all

`internal/keysource/softtoken` is a test-only signing backend over a
PKCS#12 file. It is compiled in only when the binary is built with the
`softtoken` Go build tag — a normal build never contains it at all, and
every certificate it hands out is marked as a test key everywhere it is
displayed (`(TEST KEY)` in `certs`, `isTestKey: true` in `certs --json`,
and a `TEST SIGNATURE` line on every `sign-digest` run).

```sh
# Generate a throwaway self-signed test certificate and PKCS#12 file
# (written to a gitignored directory — see testdata/softtoken/local/README.md).
go run ./scripts/gentestkeys ./testdata/softtoken/local

export LIRO_SOFTTOKEN_P12=./testdata/softtoken/local/test.p12
export LIRO_SOFTTOKEN_PASSWORD=liro-softtoken-test

go run -tags softtoken ./cmd/liro-bridge certs
go run -tags softtoken ./cmd/liro-bridge sign-digest --thumbprint <hex> --digest <hex>
```

`LIRO_SOFTTOKEN_P12`/`LIRO_SOFTTOKEN_PASSWORD` are environment variables
only — never settable from the config file, so a user cannot turn the
soft token on by editing JSON.
