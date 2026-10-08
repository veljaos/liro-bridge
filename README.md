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

## Installing on Linux, and checking the package first (F12 §8)

The release page carries a `.deb` for Ubuntu 24.04 and Debian and an `.rpm`
for Fedora, and beside them `SHA256SUMS`, its signature `SHA256SUMS.asc`, and
the public key `liro-bridge-packages.asc`. The key is an OpenPGP ed25519 key,
used only for these packages:

```
Liro Bridge Linux packages (Konfirs d.o.o. Beograd)
37D3 C56D 5F26 F2C1 F887  429F 0FC7 D69C DDDD 8D65
```

That is `37D3C56D5F26F2C1F887429F0FC7D69CDDDD8D65` in one piece. It expires on
2029-10-06. It is not the key that signs the update manifest (SPEC §15.2) and
cannot be derived from it.

*Debian and Ubuntu*

```
gpg --import liro-bridge-packages.asc          # "key 0FC7D69CDDDD8D65: public key "Liro Bridge Linux packages …" imported"
gpg --verify SHA256SUMS.asc SHA256SUMS         # "Good signature from "Liro Bridge Linux packages (Konfirs d.o.o. Beograd)""
                                               # "Primary key fingerprint: 37D3 C56D …" — compare this line with the one above
sha256sum --check --ignore-missing SHA256SUMS  # "liro-bridge_<v>_amd64.deb: OK"
sudo apt install ./liro-bridge_<v>_amd64.deb
```

The import prints only the last sixteen digits of the key's fingerprint; the
whole of it appears under `gpg --verify`, on the line that begins
"Primary key fingerprint". That is the line to compare, all forty digits,
with the fingerprint above. Above it gpg also says "WARNING: This key is not
certified with a trusted signature!" — expected for a key you have just
imported and not signed yourself; the fingerprint is what tells you whose key
it is. "BAD signature" means the checksum file is not the one that was
signed: stop there. The quoted words are gpg's in English; in another
language they differ, and the digits do not.

The `.deb` carries no embedded signature: `apt install ./file.deb` would not
check one. The signed checksum file is the check, so run it before installing.

*Fedora*

```
gpg --show-keys liro-bridge-packages.asc       # forty digits under "pub" — compare them with the one above
sudo rpm --import liro-bridge-packages.asc
rpm -K liro-bridge-<v>.x86_64.rpm              # "digests signatures OK"
sudo dnf install ./liro-bridge-<v>.x86_64.rpm
```

Compare the fingerprint before `rpm --import`, not after: rpm trusts every
key it has imported, for every package, and `rpm -K` says only that the
package was signed by one of them. `gpg --show-keys` reads the file without
importing it anywhere; the first time gpg runs it also says it created
`~/.gnupg` and a trustdb, and still imports nothing.

Nothing at install time checks who signed the package, so `rpm -K` is the
check. `dnf` does not check the signature of a package installed from a file
(`localpkg_gpgcheck` is off by default) and says so — "skipped OpenPGP checks
for 1 package from repository: @commandline" — for a signed package and an
unsigned one alike; `rpm` underneath it checks only digests
(`%_pkgverify_level` is `digest`). "digests OK" with no "signatures" means the
package is not signed, and should not be installed. Any word in capitals, such
as "DIGESTS SIGNATURES NOT OK", means stop; that line does not say which part
failed, and `rpm -Kv` does.

**What this does and does not protect against.** The packages are signed;
installing them does not check the signature, on either distribution. The
check protects only a person who runs it before installing, and a package
installed without it is protected exactly as much as an unsigned one would be.
Run, it catches a file swapped or altered on its way to you, from a mirror or
anyone between. It does not catch
a compromise of this repository itself, because the key and the fingerprint
above come from the same place as the packages. Comparing the fingerprint with
one published somewhere else would close that gap; no such place exists yet.

## Contact

Maintained by Veljko Stanojević. Questions, defects and reports go to the
repository's issue tracker: <https://github.com/veljaos/liro-bridge/issues>.
The Linux packages name the same maintainer and carry no address of their own
(D-355).
