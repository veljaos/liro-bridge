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

**`sign-digest` is in a build made with the `softtoken` tag, and never in
a release binary.** It signs whatever digest it is handed and shows
nobody anything first, and a digest is not a lesser thing than a
document: the digest a PAdES signature is computed over is a SHA-256 of
a `/ByteRange`, so a signature over an attacker-chosen digest is a
signature over an attacker-chosen document. SPEC §18.2 admits no
signature without human approval, and SPEC §6.6's consent screen cannot
be built in front of a digest — it shows a document count, a batch
fingerprint and file names, and a digest has none of the three. An
application that wants the hash-only path uses `POST /v2/sign`, which
has pairing, origin binding and a real consent screen. See
`docs/decisions.md` D-227.

**The tag adds the soft token; it does not take the card away.** A build
made this way still enumerates and opens CNG certificates first, so the
recipe below is the manual acceptance step against real hardware exactly
as it was — just run from a tagged build.

```bash
# 0. build one that has the command (see "the soft token" below if you
#    want to run this with no hardware at all)
go build -tags softtoken -o liro-bridge.exe ./cmd/liro-bridge

# 1. digest of some file
openssl dgst -sha256 -binary input.txt > digest.bin
xxd -p -c 256 digest.bin

# 2. sign it
./liro-bridge sign-digest --thumbprint ABCD... --digest <hex> --out sig.bin

# 3. extract the public key from the certificate
# (liro-bridge certs --json nests the list under "certificates", not at
# the top level, and — with more than one certificate installed — .[0]
# is not necessarily the one you just signed with, so select by
# thumbprint rather than by position)
./liro-bridge certs --json | jq -r --arg tp "ABCD..." \
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

The same tag is what carries the two signing commands that show nobody
anything — `sign-digest` and `sign-no-consent`. A release binary has
neither, proved on every push by reading the binary's symbol table
rather than by trusting the tag.

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

## Smart card middleware, and which issuers are actually verified (F11)

On Windows the agent reaches cards through the operating system's own
Cryptography API, and that is the default and stays the default. A PKCS#11
backend exists beside it, speaking to an issuer's middleware directly, for the
cards CNG does not see — and because it is the only route on macOS and Linux,
where CNG does not exist.

**What is verified, and what is not, stated at the line the verification stops
at.** An untested issuer claimed as supported is the defect; naming the line is
the honest form.

| Issuer | Middleware | Module loads and answers | A signature this project verifies |
|---|---|---|---|
| **Pošta Srbije** | SafeSign | yes | **yes** — a real card, signed and independently verified |
| **MUP e-ID** | NetSeT (TrustEdgeID, MUP RS) | yes, both builds | not through PKCS#11; MUP signs through CNG today |
| **Halcom** | Nexus Personal | yes | **no — there is no Halcom card** |

**Halcom is written and unverified, and the line is exactly here:** the module
loads, `C_GetFunctionList` answers, `C_Initialize` succeeds, and the slot,
token and mechanism lists read correctly. What has never been shown is that a
Halcom card produces a signature this project can verify, because no Halcom
card has ever been in the reader.

Two further things worth knowing before relying on this backend:

- **A signature made with a Serbian card through PKCS#11 reaches B-T at best,
  not the B-LT the specification defaults to.** Neither card carries its own
  issuing certificate, so the chain the token can supply is empty and there is
  nothing for a `/DSS` to carry. Completing the chain is the caller's work and
  is not built yet.
- **Adobe Reader will say "signature validity unknown" for these documents.**
  That is a statement about the trust anchor — Adobe does not carry the Serbian
  CAs in its own store — and not about the signature. Any program signing with
  these cards produces the same verdict.
