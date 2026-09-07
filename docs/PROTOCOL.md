# Liro Bridge — the local protocol

**Status:** built and maintained from phase F7. This document is what an
integrator implements against.

**Protocol version:** 2 (`/v2/...`).

This document is written in English, like everything else developer-facing
in this project (SPEC §9.2).

> **What is here so far.** F7 is being built in four groups. This document
> currently covers **pairing and request authentication**. Discovery
> (`bridge.json`, the port range, `/v2/health`), jobs and the two signing
> endpoints are added as those groups land, and are marked below where a
> forward reference is unavoidable.

---

## 1. What this is

A program running on the same machine as Liro Bridge can ask it to sign.
The agent shows the person its own window, the person presses Approve,
and the signature is made on the card in front of them.

Three things follow from that and shape everything below.

**The agent listens on loopback and nowhere else.** It is not a web API.
It sends no CORS headers, it refuses preflight requests outright, and
every endpoint requires `Content-Type: application/json` — which together
mean a page in a browser cannot call it at all, even from the same
machine. That is deliberate: see §2.5.

**Every request is signed.** Not with a bearer token in a header, but
with an HMAC over a canonical string that covers the method, the path,
the time, a nonce and the body. A captured request cannot be replayed and
an altered one cannot be passed off as the original.

**Nothing is signed without a person.** There is no flag, header or
configuration that skips the consent window. A request that arrived over
HTTP is not more trusted than a person dropping files on the agent; it
gets the same window (SPEC §6.5).

---

## 2. Pairing

An application must be paired before it can ask for anything. Pairing
happens once, and the person doing it has to be at the machine.

### 2.1 The two calls

```
POST /v2/pair/request
Content-Type: application/json

{ "applicationName": "My ERP", "origin": "https://erp.example.com" }
```

```
200 OK
{ "requestId": "4f4fd8af92f5e8bee9d1c8f5d5167ce0", "expiresInSeconds": 300 }
```

The agent opens its own window showing a **six-digit code**. **The code is
not in the response**, and never will be: if it were, an application could
pair itself with nobody watching, and the window would prove nothing. The
code travels through a person, which is the whole mechanism.

The person reads the code out; the application submits it:

```
POST /v2/pair/confirm
Content-Type: application/json

{ "requestId": "4f4f...", "code": "681956", "origin": "https://erp.example.com" }
```

```
200 OK
{
  "appId": "e1a263168556ef06d1490dd26b247ec0",
  "deviceSecret": "bGlyby1icmlkZ2UtZXhhbXBsZS1zZWNyZXQtMzJieXQ=",
  "applicationName": "My ERP",
  "origin": "https://erp.example.com"
}
```

`deviceSecret` is 32 random bytes, base64-encoded. It is returned **once**.
Store it; there is no way to ask for it again.

### 2.2 The rules

| Rule | Value |
|---|---|
| The code is valid for | 5 minutes |
| Wrong codes that void the request | 5 |
| Pairing requests open at one time | 1 |
| Pairing requests per minute, per origin | 3 |

- **Confirm must come from the same origin as the request.**
- **A request is spent once confirmed.** A second confirm is answered
  `PAIRING_EXPIRED`.
- **The person can refuse.** The window has a Deny button, and closing it
  is the same answer; confirm is then answered `PAIRING_DENIED`.

### 2.3 What is bound

The display name and the origin are bound at pairing time and are what the
agent shows above **every** later signature request. An application cannot
supply a different name in a signing request — otherwise it could pair as
"Test" and present itself as "Liro" (SPEC §6.6).

The name is sanitised before it is bound: control characters and Unicode
direction-override characters are stripped and it is truncated to 120
characters with the middle elided. What is bound is the sanitised form, so
what the person approved at pairing time and what they see above a
signature are the same bytes.

The origin is **not** sanitised — it is refused. It has to be shown
verbatim, with no prettifying and no stripping of the scheme, because a
person who sees `http://` where they expected `https://` must be able to
notice. A value that has been altered on its way to the screen is not
verbatim, so an origin containing a control character, whitespace or a
direction override is answered `REQUEST_INVALID`, and so is one longer
than 255 bytes.

### 2.4 Where the device secret lives

**On your server. Never in a browser.** Not in `localStorage`, not in
`sessionStorage`, not in code that is shipped to a page. A secret in a
browser is a secret every visitor has.

The agent stores its own copy encrypted at rest, bound to the Windows user
account and to a second file of random bytes kept beside it, so copying
the encrypted blob to another machine or another account is not enough
(SPEC §6.4).

### 2.5 Why a browser cannot call this

Two things together:

- Every endpoint requires `Content-Type: application/json`. A form-shaped
  POST — the only kind a browser can send without asking permission first
  — is refused with `REQUEST_INVALID`.
- Anything else needs a CORS preflight, and the agent answers preflight
  with `403` and no `Access-Control-*` headers at all.

So the rule in §2.4 is enforced rather than merely written down.

### 2.6 Managing pairings

The person can see every paired application in the agent's Settings
window — its name, its origin, when it was paired and when it last asked
for anything — and disconnect any of them. Disconnecting is immediate: the
device secret is deleted, and the next request that presents it is
answered `NOT_PAIRED`.

---

## 3. Authenticating a request

Every call to a protected endpoint carries four headers:

```
X-Liro-App-Id      the appId from pairing
X-Liro-Timestamp   Unix seconds, as a decimal string
X-Liro-Nonce       a value your client has not used before
X-Liro-Signature   lowercase hex HMAC-SHA256
```

### 3.1 The canonical string

The signature covers this exact text, and nothing else:

```
METHOD \n PATH \n TIMESTAMP \n NONCE \n SHA256HEX(BODY)
```

The separator is a single `\n` (U+000A). There is no trailing newline, no
spaces around the parts, and no query string on the path.

- `METHOD` is upper-cased.
- `PATH` is the request path as it appears in the request line, **without**
  the query string.
- `TIMESTAMP` and `NONCE` are the header values, character for character.
  `"1757260800"` and `"01757260800"` are the same instant and different
  canonical strings; sign what you send.
- `SHA256HEX(BODY)` is SHA-256 over the raw request body, lowercase hex.

**An empty body still hashes to something**, and this is the something:

```
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

Every integrator gets that wrong once.

The signature is `HMAC-SHA256(deviceSecret, canonicalString)`, hex-encoded
in lowercase. The key is the 32 raw bytes, i.e. the base64 from
`/v2/pair/confirm` decoded — not the base64 text itself.

### 3.2 A worked example

Check your implementation against this before you check it against a
running agent. Every value is fixed.

```
deviceSecret (base64)  bGlyby1icmlkZ2UtZXhhbXBsZS1zZWNyZXQtMzJieXQ=
deviceSecret (bytes)   the 32 ASCII bytes "liro-bridge-example-secret-32byt"

method                 POST
path                   /v2/sign
X-Liro-Timestamp       1757260800
X-Liro-Nonce           9f2c1e0b7a3d4c58

body (120 bytes, exactly as sent, no trailing newline)
{"certificateThumbprint":"7758D4","digestAlgorithm":"SHA256","digests":["47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="]}

SHA256HEX(body)
7f249dfc3164e80275772db6207b973f95c984b57356a6489cea975c5a706acc
```

The canonical string is then, with `\n` shown as a literal escape:

```
POST\n/v2/sign\n1757260800\n9f2c1e0b7a3d4c58\n7f249dfc3164e80275772db6207b973f95c984b57356a6489cea975c5a706acc
```

and

```
X-Liro-Signature
e976ad93b0f66044ecaf6d831b3f9794efe3276186a51f1260fef875b469f238
```

If you get a different signature, print your canonical string with the
newlines escaped and compare it to the line above character by character.
It is almost always the body hash or a stray newline.

### 3.3 What is checked, and what you are told

| Check | Value |
|---|---|
| Timestamp within | ±60 seconds of the agent's clock |
| A nonce is remembered for | 5 minutes |
| Nonce length | at most 128 characters |
| Signature comparison | constant time |

The signature is verified **before** the nonce is spent, so a request that
fails to authenticate cannot consume a nonce your client is about to use.

Rejections are `401` with a code, and **the code never says which check
failed** — that would tell whoever is probing how far they got.

- `NOT_PAIRED` — there is no live pairing for that `appId`. It was never
  paired, or it has been disconnected. Pair again.
- `AUTH_FAILED` — everything else: a missing header, a stale or
  unparseable timestamp, a reused nonce, a wrong signature, an `Origin`
  header that does not match the one bound at pairing.

If you are stuck, look at the agent's own log. It records which check
refused each request — `signature_mismatch`, `clock_skew`,
`unparseable_timestamp`, `nonce_reused`, `origin_mismatch`,
`missing_header`, `nonce_too_long` — for exactly this reason. It is the
one place that is allowed to say.

### 3.4 The Origin header

If your client sends an `Origin` header, it must equal the origin bound at
pairing. A server-side client normally sends none, and that is fine — it is
where the device secret belongs.

---

## 4. Errors

Every error body is:

```json
{ "code": "PAIRING_CODE_INCORRECT", "details": { "attemptsRemaining": 4 } }
```

and nothing else. **No human-readable message crosses this boundary, in
any language** (SPEC §7). `details` carries structured facts, never prose,
and may be absent.

Codes are stable. They are never removed or repurposed; new situations get
new codes.

### 4.1 Codes this part of the protocol can return

| Code | HTTP | What it means | What to do |
|---|---|---|---|
| `REQUEST_INVALID` | 400 | Wrong method, body that is not JSON, a missing or unacceptable field. `details.field` names it where there is one. | Fix the request. |
| `NOT_PAIRED` | 401 | No live pairing for this `appId`. | Pair again. |
| `AUTH_FAILED` | 401 | The request did not authenticate. | See §3.3. |
| `PAIRING_CODE_INCORRECT` | 401 | Wrong six-digit code; the request is still live. `details.attemptsRemaining` says how many tries are left. | Ask the person to read the code again. |
| `PAIRING_DENIED` | 403 | The person refused the pairing, or closed the window. | Stop. This is an answer. |
| `PAIRING_ORIGIN_MISMATCH` | 403 | Confirm came from a different origin than the request. | Send the same origin in both calls. |
| `PAIRING_EXPIRED` | 410 | The pairing request is gone: five minutes passed, five wrong codes voided it, it was already confirmed, or there is no such request. | Start a new pairing request. |
| `PAIRING_IN_PROGRESS` | 409 | Another application's pairing window is open. | Wait and try again. |
| `RATE_LIMITED` | 429 | Too many pairing requests from this origin. `details.retryAfterSeconds`. | Wait that long. |
| `INTERNAL` | 500 | Unclassified. The agent has logged it locally. | Report it. |

Signing adds its own codes (`CARD_NOT_PRESENT`, `PIN_INCORRECT`,
`CONSENT_DENIED`, …); they are documented with the signing endpoints.

### 4.2 `PIN_INCORRECT` is never retried

When a signing call is added and it answers `PIN_INCORRECT`, **do not
retry it automatically. Ever.** Three wrong PIN entries block the card, and
for a national identity card unblocking means a visit to the Ministry. Show
the person what happened and let them decide.

---

## 5. Rate limits

| Limit | Value |
|---|---|
| Pairing requests open at one time | 1 |
| Pairing requests per minute, per origin | 3 |

A refused pairing request is counted too: a limiter that only counts the
requests it allowed does not limit a caller in a loop.

---

## 6. A worked pairing, start to finish

```
1. POST /v2/pair/request  {"applicationName":"My ERP","origin":"https://erp.example.com"}
   -> 200 {"requestId":"4f4f...","expiresInSeconds":300}

2. The agent opens a window on the person's screen showing six digits.
   You cannot see them. That is the point.

3. The person reads them to you. Your application submits them:

   POST /v2/pair/confirm  {"requestId":"4f4f...","code":"681956","origin":"https://erp.example.com"}
   -> 200 {"appId":"e1a2...","deviceSecret":"...","applicationName":"My ERP","origin":"..."}

4. Store appId and deviceSecret on your server, and never anywhere else.

5. Every later request carries the four headers from §3.
```
