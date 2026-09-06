# Liro Bridge — Master Specification

**Version:** 1.0
**Status:** Authoritative. This document governs every phase.
**Language of this document:** English. All code, comments, identifiers, commit messages and documentation are English. The *user interface* is trilingual (see §9).

---

## 0. How to use this document

You are building **Liro Bridge**, a desktop signing agent, from nothing. There is no previous version. There is no legacy code to be compatible with.

You will receive work in **phases**. For each phase you get exactly two files:

1. **This document** (`docs/SPEC.md`) — the rules that never change.
2. **One phase document** (`docs/phases/FN.md`) — what to build now.

Read this document completely before every phase. It is not background reading; it contains hard constraints that will make your code wrong if you skip them.

### Rules of engagement

- **Do not build beyond your phase.** If a phase says "certificate enumeration", do not also write the PDF engine because it seems convenient. Later phases depend on earlier ones being small and correct.
- **Do not invent requirements.** If something is unspecified and you must choose, choose the simplest option, implement it, and record the choice in `docs/decisions.md` with your reasoning.
- **Do not silently soften a constraint.** If a rule in this document makes your implementation hard, that is usually intentional. Raise it in `decisions.md` rather than working around it.
- **Every phase ends by updating `docs/decisions.md`.** The phase document tells you which decisions must be recorded. A phase with an untouched `decisions.md` is not finished.

### Facts in this document are measured, not assumed

Section 11 and 12 contain specific OIDs, byte-level behaviours and quirks of Serbian certification authorities. These were extracted from **real certificates and real signed PDFs**, not from vendor documentation — vendor documentation was found to be wrong in at least one case (§11.3). Treat them as ground truth. Do not "correct" them against something you read elsewhere.

---

## 1. What Liro Bridge is

Liro Bridge is a small desktop program that lets a person sign PDF documents with a **qualified electronic certificate stored on physical hardware** — a smart card or a USB token.

It does two things:

1. **It signs documents locally.** The user drags PDFs into its window (or right-clicks a file), picks a certificate, enters the card PIN once, and gets signed PDFs back.
2. **It lets other programs ask it to sign.** A web application, an ERP, a script — anything running on the same machine — can call a local HTTP API. The agent asks the human for approval, signs, and returns the result.

It is a **single binary**, written in Go, with no runtime dependencies. It runs as a tray application on the user's own machine. It is open source and free.

### Design centre

Everything in this specification follows from one idea:

> **The private key never leaves the card, the document never leaves the machine, and no signature is ever created without a human clicking a button.**

If a design choice conflicts with any of those three, the design choice is wrong.

---

## 2. Non-goals

These are permanently out of scope. Do not build them, do not leave hooks for them, do not mention them in comments.

| Not this | Why |
|---|---|
| Cloud / remote signing | This product is for physical hardware only. |
| Document archiving or storage | The agent writes signed files where the user says and forgets them. |
| Multi-step signing workflows | No routing, no "waiting for second signer", no state machine. |
| Document sharing or transfer | Nothing leaves the machine. |
| Verification-as-a-service | The agent verifies its own output as a self-check. It is not a validation service for third parties. |
| Unattended / server-side signing | A human approves every batch. There is no daemon mode, no service account, no API key that bypasses consent. |
| Certificate-based authentication (portal login) | Different call surface, different risk. Out of scope. |
| Telemetry of any kind | See §6.6. |

---

## 3. Glossary

Serbian PKI vocabulary in English, so that names in code are unambiguous.

| Term | Meaning |
|---|---|
| **QSCD** | Qualified Signature Creation Device. The smart card or token holding the private key. |
| **Qualified certificate** | A certificate issued by a CA listed in the national Trusted List, meeting eIDAS/Serbian law requirements. |
| **TSP** | Trust Service Provider. The company or body issuing certificates or timestamps. |
| **TSL** | Trusted List. Government-published XML listing every recognised TSP and its service certificates. |
| **TSA / TSU** | Time-Stamping Authority / Time-Stamping Unit. The service that issues RFC 3161 timestamps. |
| **PAdES** | PDF Advanced Electronic Signature — the ETSI profile for signatures embedded in PDF. |
| **CMS / SignedData** | The cryptographic envelope (RFC 5652) that actually holds the signature. |
| **CNG / KSP** | Windows Cryptography API: Next Generation / Key Storage Provider. The modern Windows crypto interface. |
| **PKCS#11** | Cross-platform C API for cryptographic tokens. Used on macOS and Linux. |
| **Batch** | One user approval covering N documents, signed with a single PIN entry. |
| **JMBG** | Serbian national identification number. 13 digits. Personal data — see §6.5. |
| **PIB / MB** | Company tax number / company registration number. |

---

## 4. Architecture

### 4.1 Layers

```
                    ┌─────────────────────────┐
                    │   cmd/liro-bridge       │  entry point, wiring
                    └───────────┬─────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
   ┌────▼─────┐          ┌──────▼──────┐         ┌──────▼──────┐
   │ internal │          │  internal   │         │  internal   │
   │   /ui    │          │    /api     │         │    /cli     │
   └────┬─────┘          └──────┬──────┘         └──────┬──────┘
        │                       │                       │
        └───────────────────────┼───────────────────────┘
                                │
                    ┌───────────▼─────────────┐
                    │  internal/signing       │  orchestration:
                    │  (session, consent,     │  who wants what,
                    │   batch, audit)         │  did the human agree
                    └───────────┬─────────────┘
                                │
              ┌─────────────────┼─────────────────┐
              │                 │                 │
      ┌───────▼──────┐  ┌───────▼──────┐  ┌───────▼──────┐
      │   internal   │  │   internal   │  │   internal   │
      │   /keysource │  │    /pades    │  │   /trust     │
      │  (CNG, soft) │  │ (PDF + CMS)  │  │ (TSL, OCSP)  │
      └──────────────┘  └──────────────┘  └──────────────┘
```

### 4.2 Dependency rules — enforced by lint, not by agreement

These are checked in CI. A violation fails the build.

1. **`internal/pades` MUST NOT import `internal/api`.**
   The PDF engine must be usable without the HTTP server existing. This is the boundary that keeps a compromised web application from reaching documents.

2. **`internal/keysource` MUST NOT import `internal/pades`.**
   Key sources sign hashes. They do not know what a PDF is.

3. **`internal/trust` MUST NOT import anything from `internal/` except `internal/errs`.**
   Trust evaluation (TSL, OCSP, chain building) is pure and independently testable. `internal/errs` is a dependency-free leaf carrying only the error-code vocabulary; importing it does not compromise that property. No other `internal/` package may be imported.

4. **`internal/api`, `internal/ui`, `internal/cli` MUST NOT import each other.**
   Three independent front doors onto the same core.

5. **Nothing in `internal/` may import `cmd/`.**

Implement rule enforcement in CI as a small Go program or a `go list`-based script in `scripts/check-deps.go`. Do not use a third-party linter for this; the rule set is tiny and must not silently stop working when a dependency updates.

### 4.3 The four entry points

All four go through the **same** consent screen, the same session, the same audit log. They differ only in what they carry.

| Entry point | Caller | Sees documents | Notes |
|---|---|---|---|
| `POST /v2/sign` | Liro web applications | **No** — hashes only | The application builds the PDF and CMS itself. |
| `POST /v2/sign/pdf` | Third-party ERPs | Yes | Optional module. Can be disabled at install time. |
| Agent window / OS integration | The user directly | Yes | Drag and drop, right-click menu. |
| CLI | Scripts, legacy systems | Yes | `liro-bridge sign --in x.pdf --out y.pdf` |

The hash-only path exists because it is the most secure arrangement available: the agent never possesses the document. It is preserved even though the agent is now capable of handling documents, because a compromised web application must not be able to extract documents through the agent.

---

## 5. Repository layout

```
liro-bridge/
├── cmd/
│   └── liro-bridge/
│       ├── main.go
│       └── rsrc.syso              # Windows icon/manifest (generated)
├── internal/
│   ├── api/                       # HTTP server, pairing, HMAC, jobs
│   ├── audit/                     # append-only log with hash chain
│   ├── cli/                       # command-line interface
│   ├── config/                    # configuration + logging setup
│   ├── consent/                   # approval requests and their lifetime
│   ├── i18n/                      # message catalogue, three locales
│   ├── keysource/
│   │   ├── keysource.go           # the interface (see §5.1)
│   │   ├── windowscng/            # Windows CNG implementation
│   │   ├── pkcs11/                # phase 11+
│   │   └── softtoken/             # test-only, build-tagged out of release
│   ├── pades/
│   │   ├── pdf/                   # parser, incremental update, objects
│   │   ├── cms/                   # SignedData construction
│   │   ├── tsa/                   # RFC 3161 client
│   │   ├── appearance/            # visual stamp
│   │   └── dss/                   # revocation info embedding (B-LT)
│   ├── platform/                  # OS-specific: secrets, readers, autostart
│   ├── signing/                   # orchestration: sessions, batches
│   ├── trust/
│   │   ├── tsl/                   # Trusted List fetch, verify, cache
│   │   ├── revocation/            # OCSP + CRL
│   │   └── classify/              # qualified / not qualified
│   └── ui/
│       ├── assets/                # HTML, CSS, JS, fonts, logo
│       └── ...                    # WebView2 host, tray
├── sdk/
│   ├── typescript/
│   └── dotnet/
├── docs/
│   ├── SPEC.md                    # this file
│   ├── PROTOCOL.md                # generated/maintained from phase F7
│   ├── decisions.md               # append-only decision log
│   └── phases/
├── scripts/
├── testdata/
│   ├── certs/                     # real DER certificates, no private keys
│   ├── pdfs/                      # fixtures, including pre-signed documents
│   └── golden/                    # byte-exact expected outputs
├── go.mod                         # module github.com/veljaos/liro-bridge
└── README.md
```

### 5.1 The key source interface

Every signing backend implements this. Nothing above this layer knows whether the key is on a smart card, a USB token, or a test file.

```go
// Package keysource abstracts over sources of signing keys.
//
// A KeySource enumerates the signing certificates available on this machine
// and produces raw signatures over pre-computed digests. It never sees a
// document.
package keysource

// Source enumerates certificates and opens signing sessions.
type Source interface {
    // Name identifies the backend, e.g. "windows-cng".
    Name() string

    // List returns every certificate this source can sign with, including
    // ones that are currently unusable (expired, hardware absent). The
    // caller decides what to show and what to disable.
    List(ctx context.Context) ([]Certificate, error)

    // Open begins a signing session for one certificate. Opening may
    // prompt the user for a PIN. The session must be closed.
    Open(ctx context.Context, thumbprint Thumbprint) (Session, error)
}

// Session signs digests with one certificate. Not safe for concurrent use.
type Session interface {
    // SignDigest signs a pre-computed digest. The digest must already be
    // the correct length for alg.
    SignDigest(ctx context.Context, alg DigestAlgorithm, digest []byte) ([]byte, error)

    // Certificate returns the signer certificate.
    Certificate() Certificate

    // Chain returns the issuing chain if the source can supply it. May be
    // empty; the caller is responsible for completing the chain (see §11.6).
    Chain() [][]byte

    Close() error
}
```

**Why `SignDigest` and not `Sign`:** the card computes RSA over a digest. Passing whole documents down here would put the document in a layer that must not have it, and would make batching impossible.

---

## 6. Security model

Layered. Each layer is independently useful; none of them is the only thing standing between an attacker and a signature.

### 6.1 Transport binding

The HTTP API listens on **loopback only** (`127.0.0.1`), never on `0.0.0.0`. Binding to a non-loopback interface must be impossible, not merely off by default.

### 6.2 Pairing

An application must be paired before it can request anything. Pairing:

1. The application calls `POST /v2/pair` with its display name and origin.
2. The agent shows a window: *"«My ERP» wants to connect to Liro Bridge."* with the origin shown verbatim.
3. If the user approves, the agent issues a **device secret** — 32 random bytes, returned exactly once.
4. The secret is stored by the agent, bound to the origin, encrypted at rest (§6.4).

The origin is bound at pairing time and checked on every subsequent request. A secret issued to `https://app.example.com` is useless from `https://evil.example.com`.

### 6.3 Request authentication

Every authenticated request carries `X-Liro-Signature`, an HMAC-SHA256 over a **canonical string**:

```
METHOD \n
PATH \n
TIMESTAMP \n
NONCE \n
HEX(SHA256(BODY))
```

- `TIMESTAMP` is Unix seconds. Requests outside **±60 seconds** are rejected.
- `NONCE` is a client-generated random string. The agent keeps seen nonces for **5 minutes** and rejects repeats.
- The body hash is over raw bytes, before any parsing.
- The MAC is computed with the device secret and compared in constant time.

Rejections return `401` with an error code, never with a description of which check failed.

### 6.4 Secret storage

The device secret is encrypted at rest using OS facilities, with **additional entropy** stored alongside — so that copying the encrypted blob to another machine or another user account is not enough.

| OS | Mechanism |
|---|---|
| Windows | DPAPI, `CryptProtectData` with `pOptionalEntropy` |
| macOS | Keychain, item bound to the application |
| Linux | Secret Service (`libsecret`) where available, otherwise an encrypted file with a key derived from machine-id + user, with a clear warning in the log |

This is behind an interface in `internal/platform`. The rest of the code calls `SecretStore.Get`/`Set` and knows nothing about DPAPI.

### 6.5 The consent screen is the only real gate

**This is the most important paragraph in this document, and it is counter-intuitive. Do not optimise it away.**

Measurement on real hardware established: after the user enters the PIN once, **the card keeps the PIN cached in its own state**, independent of which process is talking to it. A second process on the same machine can therefore sign without being asked for a PIN, for as long as the session lives.

This means the PIN is **not** an access control boundary between applications. The only thing that reliably prevents an unauthorised signature is **the human clicking Approve in the agent's own window**.

Consequences that must be implemented:

- Every batch requires an explicit approval, from every entry point, with no exception and no "remember this application" checkbox.
- The consent window is owned by the agent process. It is never rendered by, styled by, or influenced by the calling application.
- The consent window must be brought to the foreground and must not be suppressible by the caller.
- Session lifetime is bounded. A session is closed when the batch completes, when the window closes, or after a short idle timeout.

### 6.6 What the consent screen shows

Decided: **document count, batch fingerprint, and the list of file names.**

Because file names arrive from a potentially hostile caller, they must be treated as untrusted input:

- **Render as text, never as markup.** The UI is HTML; a file name is inserted with `textContent`, never `innerHTML`.
- **Truncate.** Maximum 120 characters displayed, with the middle elided (`long-start…long-end.pdf`), so a long name cannot push the Approve/Cancel buttons off screen.
- **Strip control characters** and Unicode direction-override characters (`U+202A`–`U+202E`, `U+2066`–`U+2069`). These are used to make `invoice\u202Efdp.exe` display as `invoice exe.pdf`.
- **Cap the visible list.** Show at most 10 names, then "…and N more". The full list is available on a second screen.
- **Never treat the name as a path.** Display only; the agent must not open, resolve, or act on a name supplied over the API.
- The batch fingerprint (SHA-256 over the concatenated digests) is shown so a technical user can verify what was approved against what the calling application says it sent.

The application's display name shown on the screen is the one bound at pairing time, **not** one supplied in the signing request. Otherwise an application could pair as "Test" and later present itself as "Liro".

### 6.7 Audit log

Append-only, local, never transmitted.

Each entry contains: timestamp, certificate SHA-1 thumbprint, document count, requesting application, outcome, and the hash of the previous entry. The hash chain means an entry cannot be removed or altered without breaking every entry after it.

**Never written to the audit log:** JMBG, email addresses, file names, document contents, personal names.

Rotation by size. Export on user request. There is no upload path in the code.

#### When a chain cannot be continued

The hash chain's defining property cuts both ways. Because each entry
carries the hash of the one before it, an entry cannot be removed or
altered without breaking every entry after it — and **a corrupt final
line can never be appended to.** A power cut mid-write, a full disk, a
killed process: from that moment the last entry cannot be read, the next
`PrevHash` cannot be computed, and every signature after it goes
unrecorded. Measured: one truncated last line made every subsequent
append fail, permanently.

Refusing to append onto a chain nobody can read is correct — a hash
chain continued by guessing is not a hash chain — but it is not the end
of what has to happen. The rule is:

- **The broken file is left exactly as it is.** Never overwritten, never
  truncated, never renamed, never deleted. It is evidence up to the
  point it broke.
- **A new chain is started beside it**, in the same directory. The two
  are distinguished by file name; the original chain keeps the names it
  already had.
- **The new chain's first entry records the discontinuity**: which file
  preceded it, at which sequence and line it stopped, and why, as far as
  that is known. That record is part of what the entry hashes, so it
  cannot be altered without breaking the chain it starts.
- **The person is told once**, on the report screen, as a notice rather
  than an error: the log continued in a new file, and where it is. Only
  the entry that opened the new chain carries the record, so no
  subsequent signature repeats it.
- **The same applies when the log cannot be read at all** — a
  permissions change, a network drive that has gone away. A new chain is
  started whose first entry says the previous one was unreachable. The
  alternative is refusing to sign, and blocking a bookkeeper's afternoon
  over a log is worse than recording that the log moved.
- **Verification and export understand several chains.** Verification
  walks each chain separately — a new chain's first entry has no
  `PrevHash` by construction, so walking the whole store as one sequence
  would report the discontinuity itself as tampering — and reports each
  chain's own result alongside the breaks between them: "three chains,
  each intact, breaks on 12.03. and 04.09., with reasons". Export writes
  every chain.

The break is itself a fact worth recording rather than a state to escape
quietly. That is what append-only logs do.

### 6.8 Telemetry

**None.** The agent makes exactly four kinds of outbound network request, all of them functional and all of them explicable to the user:

1. TSA — obtaining a timestamp
2. OCSP / CRL — checking whether a certificate is revoked
3. TSL — refreshing the national Trusted List
4. Update check — GitHub Releases, once per day, disableable

Any other outbound connection is a bug.

---

## 7. Error model

**Errors are codes, not sentences.** The agent returns a stable machine-readable code; the SDK or the UI translates it.

```go
type Error struct {
    Code    Code           `json:"code"`    // "CARD_NOT_PRESENT"
    Details map[string]any `json:"details"` // optional, machine-readable
}
```

Rules:

- Codes are `SCREAMING_SNAKE_CASE`, stable across versions, and documented.
- Codes are never removed or repurposed. New situations get new codes.
- No human-readable message crosses the API. Not even in English. Not even "for debugging".
- `Details` carries structured facts (`{"reader": "Generic Smart Card Reader"}`), never prose.

Initial code set — extend as phases require, and document every addition:

| Code | Meaning |
|---|---|
| `NOT_PAIRED` | No valid pairing for this origin |
| `AUTH_FAILED` | HMAC, timestamp or nonce check failed |
| `CONSENT_DENIED` | The user pressed Cancel |
| `CONSENT_TIMEOUT` | The user did not respond in time |
| `NO_READER` | No smart card reader attached |
| `CARD_NOT_PRESENT` | Reader present, no card in it |
| `PIN_REQUIRED` | The card needs a PIN and none was supplied |
| `PIN_INCORRECT` | Wrong PIN |
| `PIN_LOCKED` | Card blocked, PUK required |
| `CERT_NOT_FOUND` | No certificate with that thumbprint |
| `CERT_EXPIRED` | Certificate outside its validity period |
| `CERT_NOT_USABLE` | Certificate exists but cannot sign (wrong key usage) |
| `CERT_REVOKED` | Revocation check says revoked |
| `TSA_UNAVAILABLE` | Timestamp service did not respond after retries |
| `TSA_REJECTED` | Timestamp service refused the request |
| `PDF_INVALID` | Input is not a parseable PDF |
| `PDF_ENCRYPTED` | Input is password-protected |
| `SIGN_FAILED` | The card refused or failed to sign |
| `STAMP_GLYPH_MISSING` | The visual stamp needs a character the embedded font subset does not contain |
| `VERSION_TOO_OLD` | Client requires a newer agent |
| `INTERNAL` | Anything unclassified — always accompanied by a local log entry |

---

## 8. Coding conventions

### 8.1 Language

Go. Module path `github.com/veljaos/liro-bridge`. Latest stable Go.

### 8.2 Naming

- Packages: short, lowercase, singular, no underscores — `keysource`, `pades`, `trust`.
- Files: `snake_case.go`. Platform-specific files use build constraints and the suffix convention: `secrets_windows.go`, `secrets_darwin.go`, `secrets_linux.go`.
- Exported identifiers are documented with a comment beginning with the identifier's name.
- Acronyms keep their case: `PDFParser`, `CMSBuilder`, `TSLClient`, not `PdfParser`.

### 8.3 Comments

Comments explain **why**, not **what**. `// increment i` is noise. `// The card caches the PIN independently of our process, so a second session would not re-prompt — see SPEC §6.5` is the reason this file exists.

Every non-obvious constant carries the measurement or the standard that produced it:

```go
// maxClockSkew is the window within which a request timestamp is accepted.
// Chosen at 60s: long enough for an unsynchronised desktop clock, short
// enough that a captured request is not replayable for long.
const maxClockSkew = 60 * time.Second
```

### 8.4 Errors

- Wrap with `fmt.Errorf("...: %w", err)` to preserve the chain.
- Sentinel errors for conditions callers branch on: `var ErrCardNotPresent = errors.New("card not present")`.
- The API layer maps internal errors to codes (§7). Internal error strings never reach a client.

### 8.5 Concurrency

- Every blocking operation takes a `context.Context` as its first parameter.
- A `Session` is not safe for concurrent use. The orchestration layer owns serialisation.
- No global mutable state. No `init()` that does work.

### 8.6 Dependencies

Minimal. Prefer the standard library. Before adding a dependency, record in `decisions.md` what it does, why the standard library is insufficient, and what the licence is.

Expected acceptable dependencies: a WebView2 binding, a systray binding, `golang.org/x/sys`, and a PKCS#11 binding in phase 11. **ASN.1 and PDF handling are written by hand** — see §12.1.

### 8.7 Formatting and CI

`gofmt` and `go vet` clean. `golangci-lint` with a configuration committed to the repo. CI runs: build for all target platforms, vet, lint, dependency-rule check, and the full test suite. A red CI blocks the phase.

---

## 9. Internationalisation

### 9.1 Locales

Exactly three, always with the script subtag:

- `sr-Latn` — Serbian, Latin script. **Default.**
- `sr-Cyrl` — Serbian, Cyrillic script.
- `en` — English.

**Never use a bare `sr`.** In CLDR, bare `sr` resolves to Cyrillic, which silently gives the wrong script to users who expect Latin. Treat a bare `sr` in any code path as a bug.

### 9.2 Mechanics

- One message catalogue per locale, in `internal/i18n/`, embedded in the binary.
- Message keys mirror error codes where they correspond: the code `CARD_NOT_PRESENT` has the key `error.card_not_present`.
- The UI selects a locale from user configuration, falling back to OS locale, falling back to `sr-Latn`.
- The split is by audience, not by "CLI vs window": code, comments, documentation, and every command's `--help`/usage text are English, like everything else developer-facing — a person invoking `liro-bridge --help` to learn what the program does is reading it the same way they'd read a comment. Everything a non-developer reads at runtime — CLI *output* (a certs listing, an error message, a progress line) and every window's text — is localised in all three catalogues. Log files are English regardless, for the reason already given: they are read by developers, not signers (D-092).
- A missing translation falls back to `en` and logs a warning. It never renders the raw key to the user.

### 9.3 Text that is not localised

Certificate subject fields, file names, error codes, and log lines are data. They pass through unmodified, in whatever script they arrive in. A Cyrillic name from a MUP certificate is displayed in Cyrillic even when the interface is in English.

---

## 10. Visual design

The agent's windows are HTML rendered in an embedded browser view. They must look like Liro applications, but the agent has **no dependency on the Liro Design System** — that is a React monorepo and cannot run here.

The bridge between them is **design tokens as CSS custom properties**.

### 10.1 The token rule

The agent embeds a generated `tokens.css` containing every colour, spacing and typography value as CSS variables. Agent stylesheets use `var(--liro-*)` exclusively.

**A hex colour literal anywhere in the agent's CSS is a lint failure.** Not a warning. This is the same rule the design system enforces on itself, and it is the only thing that keeps the two looking identical over time.

If a value is needed that does not exist as a token, the answer is to add a token — not to write the hex.

### 10.2 Windows

All small, all keyboard-navigable, all trilingual:

| Window | Purpose |
|---|---|
| Signing | One window whose content changes: the documents, the certificate picker and the approval, how to sign, where the signature goes, then the progress and the report. |
| Placement | The page of a document with the stamp on it, dragged where it belongs. Opened from the signing window and returning to it. |
| Pairing | "«App» wants to connect." Approve / Deny. |
| Settings | Language, TSA, output naming, audit export, update preferences. |

Plus a tray icon with a short menu.

**The signing window is one window.** It was several — a window for the
documents, a window for the approval, a window for the signing method —
each opening on top of the last, each taking the foreground, each
arriving somewhere else on the screen. Every step now replaces the
content of the window already open, and the window resizes to what that
step needs. The placement picker is the one exception, because it has to
show a page of the document at a size worth dragging on.

**No step of that window is ever visible on the way to another.** A step
is a screen a person is being asked something on; showing one for a
moment while the program is on its way somewhere else tells them the
flow went backwards. A page that has just been navigated to shows
nothing until the step that navigated to it says which screen it wants,
and the screen that covers a wait is the one that describes the wait —
"Preparing card…" from the moment Sign is pressed, not from the moment
the first signature completes.

**A step that asks nothing is not a step.** If a screen has no question
on it, its content belongs on the screen before it or in the thing it
was about to open, and the step comes out.

**A window's content is served from a virtual host, and that host's name
must not end in `.local`.** `.local` is reserved for multicast DNS (RFC
6762): Windows resolves such a name through mDNS before the virtual-host
mapping is consulted, and every page load pays a fixed ~2 s for it —
measured. Use a name under `.invalid` (RFC 2606), which is permanently
reserved and can never resolve.

### 10.3 Accessibility

Every interactive element reachable by keyboard. Focus visible. Approve is never the default focused button — the user must move to it deliberately.

---

## 11. Certificate rules — ground truth

Everything in this section was verified against real certificates from MUP, Pošta Srbije and Halcom. It is the part of the project you are most likely to get wrong.

### 11.1 Qualification is decided by the Trusted List, not by heuristics

The Republic of Serbia publishes an ETSI TS 119 612 XML Trusted List. It lists every recognised provider, its service certificates, and each service's status.

Primary rule: **a certificate is qualified if its chain terminates at a service that was `granted` in the TSL at the time of signing.**

Supporting evidence (used to enrich display, never as the primary test): the eIDAS policy OID and the `qcStatements` extension.

The agent fetches the TSL periodically, verifies its XML signature against the published signer certificates, caches it locally, and **works offline against the last known list**, displaying its age. It never fails closed because the network is down, and never fails open by pretending an unknown certificate is qualified.

Note: the TSL contains **no `ServiceSupplyPoint` elements**. OCSP and TSA endpoints cannot be derived from it. They come from the certificate's own AIA extension or from configuration.

### 11.2 The eIDAS policy OID is present on all three issuers

Every end-entity signing certificate examined carries:

```
0.4.0.194112.1.2      QCP-n-qscd  (qualified, natural person, on a QSCD)
```

and `qcStatements` containing:

```
1.3.6.1.5.5.7.11.2    PKIXQCSyntax-v2
    0.4.0.194121.1.1  semantics: natural person
0.4.0.1862.1.1        QcCompliance
0.4.0.1862.1.4        QcSSCD
0.4.0.1862.1.6        QcType
    0.4.0.1862.1.6.1  esign
0.4.0.1862.1.5        QcPDS         (Halcom only; MUP and Pošta omit it)
```

`0.4.0.1862.1.6.2` is `eseal` — a legal person's seal rather than a natural person's signature. Recognise it; the UI for it comes in a later phase.

### 11.3 Issuer-specific policy OIDs — and a warning

| Issuer | Own policy OID in the end-entity certificate |
|---|---|
| MUP | `1.3.6.1.4.1.33589.1.1.0` |
| Pošta Srbije | `1.3.6.1.4.1.15672.10.142.1.0` |
| Halcom (legal-person branch) | `1.3.6.1.4.1.5939.10.1.6` |

**Warning:** Halcom's published Certificate Policy document states `1.3.6.1.4.1.5939.11.2.6`. The actual certificate contains `1.3.6.1.4.1.5939.10.1.6`. Different branch.

The lesson generalises: **populate the OID allow-list from real certificates, never from a PDF policy document.** Vendor documents are frequently stale or wrong. The same PKS document was found to label the single OID `0.4.0.194112.1.2` three different ways in three places.

### 11.4 Key usage differs between issuers — this breaks the obvious filter

| Issuer | KeyUsage on the signing certificate |
|---|---|
| MUP | `digitalSignature` + `contentCommitment` |
| Pošta | `digitalSignature` + `contentCommitment` |
| **Halcom** | **`contentCommitment` only** |

A filter requiring `digitalSignature` rejects every Halcom certificate.

> **Rule: a certificate is usable for signing if `contentCommitment` (a.k.a. `nonRepudiation`) is set. Never require `digitalSignature`.**

### 11.5 Two certificates with identical subjects

Serbian CAs issue **two** certificates per card: one for signing, one for authentication/encryption. On Halcom cards their **Subject DN is byte-for-byte identical** — same CN, same email, same serial number attributes. Only KeyUsage and the policy OID differ.

Consequences:

- The `contentCommitment` filter is not stylistic. It is the only thing that separates them.
- **A listing shows only signing certificates by default.** The authentication certificate is not a choice: it cannot sign, and on a Halcom card it carries the same name as the one that can. Showing it disabled — which F1 §6.1 asked for, on the reasoning that hiding it would make a user think their card was broken — shows the person their own name twice, the second time struck through, and makes every list twice as long for nothing. `certs --all` still shows everything. See docs/decisions.md.
- **A *signing* certificate that cannot be used right now stays visible, disabled, with its reason.** An absent card and an expired certificate are real choices temporarily unavailable, and hiding *those* is what would make a card look broken.
- **The certificate list must never rely on CN alone.** Every row shows: the name, the role derived from KeyUsage ("for signing" / "for login"), and the last four characters of the SHA-1 thumbprint.
- Certificate choice is explicit per session. There is no "remember last used" — on a bookkeeper's machine, several clients' certificates may be installed at once, and a remembered default becomes a wrong-signer incident.

### 11.6 Subject DN parsing — three traps

**Trap 1: multi-valued RDNs.** Attributes appear twice inside a *single* RDN, joined with `+`:

```
2.5.4.5  = CA:RS-246275 + 2.5.4.5  = PNORS-2003964710097
2.5.4.97 = MB:RS-17491857 + 2.5.4.97 = VATRS-103026255
```

A parser that returns "the serialNumber attribute" returns one of the two, non-deterministically. **Iterate every value of every attribute.** Getting this wrong is the easiest way to leak a JMBG into a field meant for an internal reference number.

**Trap 2: personal identifiers everywhere.** All three issuers embed the national ID number as `PNORS-<13 digits>`. Halcom additionally uses `IDCRS-<ID card number>` and `PAS<CC>-<passport number>` for non-residents. Strip **all** semantic identifier prefixes, not only `PNORS`.

**Trap 3: the email address is in two different places.**

| Issuer | Email location |
|---|---|
| Halcom | `emailAddress` **inside the Subject DN** |
| MUP, Pošta | `rfc822Name` in **SubjectAlternativeName** |

Both must be scrubbed before anything is displayed publicly or logged.

### 11.7 CN is not the person's name

```
MUP:    "ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign"      (Cyrillic, CA number, literal " Sign")
Halcom: "Zoran Milovanović 246275"
Pošta:  "Redžvel Mešković 200094362"
```

Do not write a regex for this. All three certificates carry `givenName` (2.5.4.42) and `surname` (2.5.4.4) as separate attributes. **Build the display name and the visual stamp from those.** Leave CN untouched for technical display.

### 11.8 Chain completion

| Issuer | Certificates embedded in the CMS of a signed document |
|---|---|
| MUP | **1** — the signer certificate only |
| Halcom | 3 — root + intermediate + signer |
| Pošta | 3 — root + intermediate + signer |

For MUP the chain must be built by the agent, from AIA `caIssuers` or from a bundled trust store. The chain must be present in the output regardless of what the source supplied.

**Halcom AIA defect:** Halcom's `caIssuers` points at a `.crl` file where RFC 5280 requires a certificate. Code that follows AIA blindly receives a CRL where it expects a certificate. Always verify the content type of what you fetched before parsing it.

### 11.9 Revocation endpoints

All three support OCSP, so B-LT is achievable for all of them.

| Issuer | OCSP | CRL |
|---|---|---|
| MUP | `http://ocsp.mup.gov.rs/MUPGradjaniCAocsp` | `http://ca.mup.gov.rs/MUPGradjaniCA4.crl` |
| Pošta | `http://ldap-ocsp.ca.posta.rs/ocsp` | `http://repository.ca.posta.rs/crl/PostaSrbijeCA1.crl` |
| Halcom | `http://ocsp.halcom.rs` | `http://domina.halcom.rs/crls/...` |

Note that MUP's OCSP URL appears in the **end-entity** certificate, not in the CA certificate. Read AIA from the certificate being checked.

### 11.10 Hardware presence — decided per certificate, never once for the whole machine

Certificates remain listed in the Windows certificate store **after the card is removed**. Verified: with the reader empty, both Halcom certificates still enumerate normally.

> **Rule: presence is determined from `SCardListReaders` and reader state, or by attempting to open the key container. Never from the certificate being enumerable.**

The original wording above was ambiguous enough to permit computing presence once — "is there a card in any reader" — and applying that single answer to every hardware-backed certificate on the machine. Verified this is wrong: on a machine holding several clients' certificates (a bookkeeper's normal case, not an edge case — see §14.1), inserting one card marked *every* hardware-backed certificate as usable, including ones whose card was not present anywhere.

> **The rule stated precisely: presence is a property of one certificate, not of the machine. Determine it per certificate by attempting to open that certificate's own key — this never prompts for a PIN, only signing does — and treat `NTE_BAD_KEYSET` or `SCARD_W_REMOVED_CARD` from that attempt as "this certificate's card is not present." The reader-state check (`SCardListReaders`/`AnyCardPresent`) remains — it answers a different, coarser question, "is there a reader attached at all," and keeps its own message.**

Getting either check wrong produces the worst possible user experience: the agent offers to sign, the user clicks, enters a PIN, and fails five seconds later.

### 11.11 Windows provider reality

Verified on a machine with both middlewares installed:

- MUP and PKS both use **TrustEdgeID** (NetSeT). One middleware, two issuers.
- Halcom uses **Nexus Personal**, which registers a minidriver, so its certificates surface through `Microsoft Smart Card Key Storage Provider` — a CNG KSP.
- No vendor-specific KSP appears in the provider list. Everything goes through Microsoft's.

> **On Windows, CNG is sufficient for MUP, PKS and Halcom. PKCS#11 is not needed on Windows at all.** It is required only for macOS and Linux, in phase 11 and later.

Pošta's middleware (SafeSign) is the only one shipping macOS and Linux builds. On those platforms the agent must tell the user which issuers are actually supported rather than reporting "no certificates found".

---

## 12. PAdES rules — ground truth

Verified by taking apart three real signed PDFs, one from each issuer, and independently re-verifying their signatures.

### 12.1 Write the PDF and CMS layers by hand

Do not use a third-party PDF-signing library.

The `/ByteRange` calculation is the single place in this project where a bug produces a signature that *looks* correct, opens without error, and is invalid. That code must be ours, small, and covered by byte-exact tests. An imported library's bug here is a bug we cannot see.

ASN.1 encoding uses `encoding/asn1` from the standard library where it fits, with hand-written structures where it does not (CMS attributes, `qcStatements`, TSTInfo).

### 12.2 Signature dictionary shape

This is what a correct Serbian qualified PDF signature looks like, taken from working documents:

```
/Type      /Sig
/Filter    /Adobe.PPKLite
/SubFilter /ETSI.CAdES.detached
/ByteRange [0 offsetA offsetB lengthB]
/Contents  <hex ... zero-padded ...>
/M         (D:20260324155552+01'00')
```

The document timestamp added afterwards is a second revision with `/SubFilter /ETSI.RFC3161`.

### 12.3 CMS SignedData

```
version: 1
digestAlgorithm: SHA-256
signerInfo:
  sid: issuerAndSerialNumber
  signedAttrs:
    1.2.840.113549.1.9.3    contentType
    1.2.840.113549.1.9.4    messageDigest
    1.2.840.113549.1.9.16.2.47  signingCertificateV2
  signatureAlgorithm: sha256WithRSAEncryption
  unsignedAttrs:
    1.2.840.113549.1.9.16.2.14  signatureTimeStampToken
```

**There is no `signingTime` attribute, and there must not be one.** PAdES takes the time from the timestamp, not from the signer's clock. All three reference documents omit it.

**The signature is computed over the DER encoding of the signed attributes re-tagged as `SET OF`** (tag `0x31`), not over the implicit `[0]` as it appears in the structure. This is a classic error; it is why an independently written verifier is part of the test suite.

`signingCertificateV2` binds the signature to a specific certificate. Without it a signature can be replayed against a different certificate with the same key. Not optional.

### 12.4 Digest algorithms

| Where | Algorithm |
|---|---|
| Document digest (`/ByteRange`) | SHA-256 |
| Signature timestamp imprint | SHA-256 or better |
| Document timestamp imprint | SHA-256 or better |

**SHA-1 must never be produced anywhere.** The reference documents were signed by the state's own tool, which uses SHA-1 for the document-timestamp imprint. That is a defect to be aware of, not a pattern to copy — Pošta's TSA rejects SHA-1 outright.

### 12.5 Timestamp tokens use BER, not DER

Document timestamp tokens in real documents are **BER with indefinite length** (`30 80 ...`), not DER. A strict DER parser fails on them; `openssl pkcs7 -inform DER` fails on them.

> **The parser must accept BER for embedded tokens.** Documents signed through the state portal are common; a DER-only parser breaks on a large share of real-world input.

Output that we generate is DER.

### 12.6 Signature levels

| Level | Contents | Our position |
|---|---|---|
| B-B | Signature only | Fallback only, on explicit user choice |
| B-T | + signature timestamp | **Minimum** |
| B-LT | + `/DSS` with certificates, OCSP responses, CRLs | **Default** |
| B-LTA | + archive timestamp | Optional |

The state's own signing tool stops at B-T with a document timestamp and **no `/DSS`**. Producing B-LT puts our output ahead of it.

### 12.7 Timestamp authorities

Configurable, several supported. All are Serbian qualified providers.

| Provider | Notes |
|---|---|
| Office for IT and eGovernment | Policy `1.3.6.1.4.1.55016.1.1.0`. Used by the state portal itself. Suggested default. |
| Pošta Srbije | Contract-based, prepaid or postpaid. |
| PKS | |
| Inception | |
| Telekom Srbija | Registered from March 2026. |

**Development and CI use Pošta's public test TSA:**

```
https://test-tsa.ca.posta.rs/timestamp1     user Test.Korisnik / password 123456
https://test-tsa.ca.posta.rs/timestamp2     client certificate (PFX), password 1234
```

RFC 3161 version 1, `certReq: true`, nonce optional, **SHA-1 forbidden**. The second endpoint exercises client-certificate authentication, which is what production TSAs require — use it, do not only test the easy one.

### 12.8 TSA failure must never block

Three attempts with backoff. Then a clear choice presented to the user: save without a timestamp (B-B, visibly marked) or cancel.

**Never silently downgrade the level. Never hang.** A TSA outage must not leave the user staring at a frozen window.

### 12.9 Timing and progress

Measured on real hardware: the first signature takes about **4.9 seconds**; every subsequent signature in the same session takes about **0.41 seconds**, with variation under 2 ms across 90 operations. The first-signature cost is card initialisation, and there is no way to avoid it.

Requirements:

- A distinct UI state, **"Preparing card…"**, covers the first signature. Five seconds of an unmoving progress bar reads as a hang.
- **The ETA is computed from the measured first signature, not from a constant.** Pošta is migrating to RSA-4096, which will be slower; hard-coded constants will become wrong.
- The agent must support **both PIN policies**: one PIN per batch, and one PIN per signature (`ALWAYS_AUTHENTICATE`). Detect at the first signature of a batch and adjust the ETA and the message. Only MUP is confirmed as one-PIN-per-batch; other issuers are untested.

### 12.10 Batch failure policy

If document 47 of 100 fails, **skip it and continue**, then present a report listing what succeeded and what did not. Aborting would force the user to re-enter the PIN for the remaining 53.

### 12.11 Output naming

`document.pdf` → `document-signed.pdf` by default. Suffix configurable. **The original is never silently overwritten.**

---

## 13. Visual signature stamp

Specified in detail because the geometry and the font handling are already proven and must be reproduced exactly.

### 13.1 Geometry

| Property | Value |
|---|---|
| Width | 190 pt (fixed) |
| Margin from page edge | 24 pt |
| Logo | 36 × 36 pt |
| Internal padding | 4 pt |
| Height by line count | index = lines: `[44, 44, 46, 56, 72]` |

Height grows with the number of lines actually drawn, so there is no empty space. Default corner `bottom-right`; explicit coordinates via an optional position parameter. A visual position picker comes in a later phase.

### 13.2 The font must be embedded

The PDF base-14 Helvetica cannot render Cyrillic, nor č ć đ š ž. Serbian names almost always contain at least one of these.

The stamp embeds a **subsetted NotoSans** as `Type0` / `CIDFontType2` with `Identity-H` encoding. The subset contains exactly the characters needed, which keeps the embedded file small.

Text is converted to two-byte CIDs for `Tj`. **If a character is not in the subset, throw a clear error naming the character and its code point.** Silently drawing nothing is worse than failing.

### 13.3 Page box resolution

Read `/MediaBox` from the target page. If it is not on the Page object, follow the `/Parent` chain — `MediaBox` is frequently inherited from the `Pages` node. Fall back to A4 only if it is genuinely absent anywhere. Works on any page, not only the first.

### 13.4 Separation from the invisible path

Stamp generation is a separate module, invoked **only** when a visual stamp is requested. The default behaviour — an invisible signature with `/Rect [0 0 0 0]` — must remain untouched when no stamp is requested.

### 13.5 Content

Up to four lines: label, signer name, optional identifier line, certificate serial and time.

**The personal identity document number is not shown by default.** It is personal data appearing on a document that will be sent to third parties. Available as an option, never the default.

The signer name is built from `givenName` + `surname` (§11.7), not by parsing CN.

---

## 14. Networking and ports

The HTTP server binds to `127.0.0.1` on the first free port in the range **17580–17590**.

The agent writes the actual port, its version, and its protocol version to:

```
Windows:  %LOCALAPPDATA%\Liro\bridge.json
macOS:    ~/Library/Application Support/Liro/bridge.json
Linux:    $XDG_RUNTIME_DIR/liro/bridge.json  (fallback ~/.local/state/liro/)
```

SDKs read this file to find the agent. **They must never scan ports.**

### 14.1 Multi-session machines

Accounting firms commonly run several users on one machine over RDP or terminal services. This is a supported configuration, not an edge case.

- One agent instance **per user session**, not per machine.
- The discovery file lives in the per-user directory, so each session finds its own agent.
- Sessions must not see each other's certificates, audit entries or pairings.
- The port range makes concurrent instances possible; the discovery file makes them findable.

---

## 15. Distribution

- **Open source**, public repository, permissive licence.
- Windows artefacts: **MSI** (per-user, no administrator rights; plus a per-machine option for GPO deployment) and a plain **EXE** for users who prefer it.
- Published on GitHub Releases.

### 15.1 Code signing

There is currently **no code-signing certificate**. Windows SmartScreen will warn on first run. This is a known, accepted condition, documented for users with a screenshot in the installation guide.

The build pipeline contains a **signing step that is currently a no-op**. When a certificate is obtained, it is filled in; nothing else changes. Do not design around its permanent absence.

### 15.2 Updates

Because there is no code-signing certificate, "signed package" means our own signature, not a Windows one:

- The agent embeds a **public key**. Every release is signed with the corresponding private key.
- The agent checks GitHub Releases **once per day**, verifies the signature of any new release before doing anything with it, and **asks the user** before installing. It never installs by itself.
- The check is disableable in settings.
- The protocol carries a **minimum supported version** so an SDK can tell the user to update the agent rather than failing obscurely.

---

## 16. Testing

Five levels. A phase is not complete until its level of the pyramid is green.

### 16.1 Unit tests

ASN.1 structures, DN parsing, PDF object parsing, `/ByteRange` arithmetic, HMAC canonicalisation. Table-driven. Include the malformed cases: multi-valued RDNs, missing `MediaBox`, BER indefinite lengths, unusual whitespace in PDF dictionaries.

### 16.2 Golden files

Sign with a fixed test key and a fixed timestamp, compare the output **byte for byte** against a stored reference in `testdata/golden/`. This catches accidental changes to output structure that still happen to verify — the class of bug that is otherwise invisible until a validator somewhere else rejects it.

### 16.3 Real-document fixtures

`testdata/pdfs/` contains three real signed PDFs, one from each issuer. Required tests:

- Parse all three; extract certificates, signatures and timestamps correctly.
- **Sign an already-signed document** and confirm that both the old and the new signature verify afterwards. Incremental update must not disturb the earlier revision. This is the single most important test in the project; if it fails, nothing else matters.

### 16.4 Independent verification

A verifier written independently of the signing code, which:

- recomputes SHA-256 over the `/ByteRange` and compares it to `messageDigest`
- verifies the RSA signature over the re-tagged `SET OF` signed attributes
- validates the timestamp token and its chain

Every generated signature is checked with it in CI. It has already been used successfully against all three reference documents, so its correctness is established independently of ours.

### 16.5 Fuzzing

The PDF parser accepts files from outside. Fuzz it. It must never panic, never allocate unboundedly, and never loop forever on malformed input.

### 16.6 The soft token

A test key source backed by a PKCS#12 file, implementing the same `keysource.Session` interface. It exists so the entire pipeline can be developed and tested with no hardware at all.

Two hard requirements:

1. **It is excluded from release builds by a build tag.** It must be impossible to enable in a shipped binary.
2. Any signature it produces is **visibly marked as a test signature** in the UI and in the audit log.

### 16.7 Manual acceptance

Before any release, output is checked in all four of: Adobe Acrobat Reader, the PKS qualified validation service, the Inception validation service, and the eUprava validator.

Documented expectation: **Adobe will report "identity unknown" for MUP-signed documents**, because MUP's CA is not in Adobe's AATL trust list. This is not a defect and cannot be fixed in code. It is documented for users.

### 16.8 The validator landscape

The validators this project checks output against are not interchangeable — each proves a different thing, and a document rejected by one of them while accepted by the rest is not automatically this project's bug. Record what each one actually establishes:

| Validator | What it proves |
|---|---|
| **eUprava validator** | Legal validity under Serbian law. This is the validator that matters most for Liro Bridge's users: it is the one a Serbian qualified signature is legally judged against, and it is more authoritative for our users than Adobe's opinion. |
| **DSS Demo, European Commission** | Conformance to ETSI's PAdES specifications themselves. It is the ETSI reference implementation and produces the most detailed diagnostic report of any validator in this list — the first place to look when a signature is structurally wrong, not just rejected. |
| **PKS** and **Inception** | Registered qualified validation services under Serbian law. Their acceptance is independent evidence of legal validity, alongside the eUprava validator. |
| **pyHanko** | The most rigorous open-source PAdES implementation available. It enumerates every signature in a document, reports byte-range coverage per signature, and diffs revisions to classify exactly what an incremental update changed (form filling, DSS, LTA, or something else). Useful as a fast, local, offline check during development — see the diagnosis below for what it is not a substitute for. |
| **Adobe Acrobat** | What the user actually sees and judges the product by — that alone earns it a permanent place in this list, regardless of how it ranks against the others below. It is also demonstrably **stricter than the specification** in places: Adobe does not read `/ByteRange` through its general PDF object parser but scans it from raw bytes with its own dedicated scanner, before the rest of the document is even parsed (PDF 32000-1 §7.7.5 permits, but does not require, this). That scanner accepts only the exact shape real producers emit and rejects PDF that is otherwise entirely spec-conformant. |

**Measured case that motivated this table.** A document this project signed was rejected by Adobe Acrobat ("At least one signature is invalid", an empty Signature Panel) while **four independent implementations — pyHanko, pypdf, PDFium, and this project's own from-scratch verifier (§16.4) — accepted the same bytes as valid.** pyHanko in particular enumerated all signatures as `SignatureCoverageLevel.ENTIRE_REVISION` and reported no structural defect. The actual fault (documented in `docs/decisions.md`) was real, but specific to how Adobe's raw-byte `/ByteRange` scanner and its signature-widget renderer are stricter than the general PDF object model every other tool here parses through — not a case of Acrobat being wrong and everything else being right, nor the reverse.

The lesson this table exists to fix in place: **do not let a future change "simplify" a formatting decision back to a form Acrobat dislikes** on the reasoning that four other validators already accept it. Four acceptances are not proof a fifth, stricter reader will too — that is exactly the gap this section was written to close.

---

## 17. `docs/decisions.md`

The most valuable file in the repository.

It records what was decided, why, and **what was tried and rejected** — the last of which cannot be reconstructed from the code afterwards.

### Format

```markdown
## D-007 — Presence detection via reader state, not the certificate store

**Date:** 2026-09-01
**Phase:** F1

**Decision.** Hardware presence is determined by SCardListReaders and reader
state, never by whether a certificate enumerates.

**Why.** With the reader empty, certificates remain listed in the Windows
store; both Halcom certificates enumerated normally with no card inserted.
Deciding presence from enumeration would let the agent offer to sign,
accept a click, request a PIN, and only then fail.

**Rejected.** Attempting a signature to test presence — it triggers a PIN
prompt as a side effect, which is unacceptable for a liveness check.
```

### Rules

- Every phase appends at least one entry.
- Entries are numbered sequentially and **never edited or deleted**. A superseded decision gets a new entry that references the old one.
- Rejected alternatives are recorded even when the reason feels obvious at the time. It will not feel obvious in six months.
- Each phase document lists the decisions that phase must record. Recording more is fine; recording fewer means the phase is not finished.

---

## 18. Hard prohibitions

Violating any of these is a defect regardless of what else the code does.

1. **No network traffic** other than TSA, OCSP/CRL, TSL and the update check.
2. **No signature without human approval.** No flag, no configuration, no header bypasses the consent screen.
3. **No JMBG, email address, personal name, file name or document content in the audit log or in any log file.**
4. **No `digitalSignature`-only key usage filter.** Use `contentCommitment`.
5. **No hex colour literals** in agent CSS. Tokens only.
6. **No bare `sr` locale.** Always `sr-Latn` or `sr-Cyrl`.
7. **No human-readable error messages crossing the API.** Codes only.
8. **No SHA-1** produced anywhere.
9. **`internal/pades` never imports `internal/api`.**
10. **No silent overwrite** of a user's original file.
11. **No silent downgrade** of signature level.
12. **The soft token is never present in a release build.**
13. **No presence detection from the certificate store.**
14. **No third-party library computes `/ByteRange`.**
15. **No "remember this certificate"** default across sessions.

---

## 19. Phase map

Three groups, fourteen phases. Each has a verifiable exit condition.

### Group 1 — signs on my own machine (Windows only)

| Phase | Content | Done when |
|---|---|---|
| **F0** | Repository, Go module, configuration, logging, i18n skeleton, `decisions.md`, CI, dependency-rule linter | `liro-bridge --version` runs; CI green |
| **F1** | Certificates: CNG enumeration, classification, TSL fetch/verify/cache/offline, reader presence | `liro-bridge certs` lists a real e-ID card and marks it qualified |
| **F2** | Hash signing: session, PIN, batch, timing measurement; soft token | Signature over a digest verifies with OpenSSL; CI passes with no hardware |
| **F3** | PAdES engine: parser, incremental update, placeholder, CMS, TSA, B-T, DSS, B-LT | CLI signs a PDF; Adobe reports the document unmodified; PKS validator accepts it |
| **F4** | Visual stamp: logo, subsetted font, geometry, Cyrillic | Stamp renders on all three fixtures with a Cyrillic name correct |
| **F5** | Agent windows: WebView2, tray, tokens, three locales, consent screen, audit log | A human clicks Sign, a signature appears, an audit entry is written |
| **F6** | Local signing: drag and drop, batch, ETA, output naming, shell integration | 100 documents, one PIN, from the agent window |

*First point at which shipping makes sense.*

### Group 2 — any program can call it

| Phase | Content | Done when |
|---|---|---|
| **F7** | Protocol: pairing, HMAC, nonce, port discovery, multi-session and RDP, jobs, error codes | Two sessions on one machine operate independently |
| **F8** | TypeScript SDK, documentation, examples | Integration into a third-party project in three lines of code |
| **F9** | CLI for legacy systems; .NET SDK | A Delphi program signs via `exec` |
| **F10** | Packaging: MSI and EXE, update channel, GitHub Releases, user guide | A stranger installs it and signs a document |

### Group 3 — runs everywhere

| Phase | Content |
|---|---|
| **F11** | PKCS#11 layer (cgo, cross-compilation) |
| **F12** | macOS: Keychain, WKWebView, SafeSign |
| **F13** | Linux: Secret Service, WebKitGTK, PDF preview limitation notice |

**F3 is the hardest and riskiest phase and deliberately precedes all UI work.** If `/ByteRange` is wrong, everything built on top of it is wasted. F2's soft token exists so that F3 through F6 can be developed without a card.

---

## 20. SDK design principle

The SDK is thick. The README is thin.

Target integration, complete:

```ts
import { LiroBridge } from '@liro/bridge';

const bridge = await LiroBridge.connect({ appName: 'My ERP' });
const signed = await bridge.signPdf(pdfBytes);
```

The SDK is responsible for: reading the discovery file, pairing on first use, storing the device secret, computing HMACs, generating nonces, polling the job, handling consent timeouts, mapping error codes to typed exceptions, and checking the minimum agent version.

The integrator does none of it and should not know it happens.

> **If the README example needs more than five lines, the SDK is wrong — fix the SDK, not the README.**
