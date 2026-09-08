# Liro Bridge — the local protocol

**Status:** built and maintained from phase F7. This document is what an
integrator implements against.

**Protocol version:** 2 (`/v2/...`).

This document is written in English, like everything else developer-facing
in this project (SPEC §9.2).

---

## 1. What this is

A program running on the same machine as Liro Bridge can ask it to sign.
The agent shows the person its own window, the person presses Approve,
and the signature is made on the card in front of them.

Three things follow from that and shape everything below.

**The agent listens on loopback and nowhere else.** It is not a web API.
It sends no CORS headers, it refuses preflight requests outright, and
every endpoint that carries a body requires
`Content-Type: application/json` — which together mean a page in a browser
cannot call it at all, even from the same machine. That is deliberate: see
§2.5.

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

The person reads the code out; the application submits it — **with the
same `origin` again**:

```
POST /v2/pair/confirm
Content-Type: application/json

{ "requestId": "4f4f...", "code": "681956", "origin": "https://erp.example.com" }
```

**Both calls carry `origin`, and the two must be byte-for-byte equal.**
It is a required field of the confirm body, not only of the request
body: the agent compares what confirm declares against what the request
declared, and a confirm that declares a different origin — or leaves it
out — is answered `PAIRING_ORIGIN_MISMATCH` (403) with the pairing
request still live, so the second call can be corrected without starting
over. This is a comparison of the two strings your application sent, not
of where the calls came from; see §3.4 for the separate rule about an
`Origin` *header*.

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

- **Confirm carries the same `origin` as the request, and it must
  match.** Both bodies have the field; the agent compares them.
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

- Every endpoint that carries a body requires
  `Content-Type: application/json`. A form-shaped POST — the only kind a
  browser can send without asking permission first — is refused with
  `REQUEST_INVALID`. Every authenticated endpoint additionally requires the
  four `X-Liro-*` headers of §3, which a page cannot set without asking
  permission either.
- Asking permission means a CORS preflight, and the agent answers preflight
  with `403` and no `Access-Control-*` headers at all.

`GET /v2/health` is the one call a page can actually make, because it has
no body and needs no authentication. It cannot read a word of the answer:
no `Access-Control-Allow-Origin` header is ever sent, by any endpoint, for
any origin.

So the rule in §2.4 is enforced rather than merely written down.

A note on content types, because it costs an afternoon otherwise: a request
with no body must **not** send `Content-Type`. Several HTTP clients cannot
attach one to a request with no content at all — .NET's `HttpClient` puts
the header on the content, so a `GET` simply has nowhere to put it — and
the agent does not ask for one.

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

### 3.5 Checking your own canonical string — `POST /v2/echo`

Every rejection above is the same answer, deliberately, and that leaves you
with nothing to compare against. This endpoint is the comparison: send a
request the way you would send any other, and the agent tells you the
canonical string it built from it.

```
POST /v2/echo
Content-Type: application/json

{"anything":"you like"}
```

```json
{
  "canonicalString": "POST\n/v2/echo\n1757260800\n9f2c1e0b7a3d4c58\n7f24...",
  "bodySha256": "7f24..."
}
```

Print your own beside it, with the newlines escaped, and the difference is
one line. The four mistakes this exists for are, in the order a first
integration tends to hit them: a header spelled wrong, an `Origin` that does
not match, a body that changed between being hashed and being sent
(`bodySha256` differs), and a `Content-Type` your HTTP client cannot attach
to a request with no body (§2.5).

- It is authenticated exactly like every other endpoint, so a request that
  reaches it has already authenticated — which is why it can say this much
  and no more.
- It returns nothing you did not send: the method, the path, the timestamp,
  the nonce and your own body. **The signature is not returned**, and
  nothing here would let one be derived; that needs the device secret,
  which this endpoint never touches.
- The body is hashed, never parsed. It does not have to be valid JSON, only
  declared as JSON like every other body, so you can send exactly the bytes
  you are about to sign.
- At most 64 KB. Nothing here holds a document.

The canonical string is per-path, so `POST /v2/echo` and `POST /v2/sign`
differ in their second line and nowhere else. That is the point: if the
rest matches, your signing is right.

If it still does not, the agent's own log says which check refused each
request (§3.3). Between the two there is nothing left to guess at.

---

## 4. Finding the agent

The agent binds **loopback only** (`127.0.0.1`), on the first free port in
**17580–17590**, and writes what it chose to a per-user file:

```
Windows   %LOCALAPPDATA%\Liro\bridge.json
```

```json
{ "port": 17580, "agentVersion": "1.4.0", "protocolVersion": 2 }
```

**Read that file. Never scan ports.** Several people can be signed in to
one machine at once — an accounting firm over RDP is the ordinary case, not
an edge one — and each of them has their own agent on its own port.
Scanning finds somebody else's, which is exactly what the per-user file
prevents.

The file is written when the agent starts and removed when it stops. A file
left behind by a crash points at a port nothing is listening on; your
connection is refused, which means the same thing as no file at all: the
agent is not running.

### 4.1 Health

```
GET /v2/health
```

```json
{ "agentVersion": "1.4.0", "protocolVersion": 2, "minimumClientVersion": "0.0.0" }
```

No authentication, no body, and no `Content-Type` (§2.5). It reveals
nothing else — not certificates, not pairings, not the name of the person
at the machine.

`minimumClientVersion` is the oldest SDK this agent will serve. If your
version is below it, tell the person to update the agent rather than
failing at them later.

### 4.2 Several people on one machine

One agent per user session, not per machine. Sessions do not see each
other's certificates, pairings, jobs or audit entries: the discovery file
is per user, and so is the store the device secret is kept in. An `appId`
paired in one session is answered `NOT_PAIRED` in another, and a job
created in one is `JOB_NOT_FOUND` in another.

---

## 5. Asking for a signature

There are two ways to ask, and which one you use is about who builds the
document.

| | `POST /v2/sign` | `POST /v2/sign/pdf` |
|---|---|---|
| You send | digests | whole documents |
| The agent sees your document | **never** | yes |
| You build PAdES/CMS yourself | yes | no |
| Can be switched off on a machine | no | yes (§5.4) |

Both answer `202 Accepted` with a job (§6), and both put the same window in
front of the same person.

### 5.1 Digests — `POST /v2/sign`

```json
{
  "certificateThumbprint": "7758D4D4B8973EA619B3225185EDE740B3D1ECCE",
  "digestAlgorithm": "SHA256",
  "digests": ["<base64>", "..."],
  "labels": ["ugovor.pdf", "..."]
}
```

| Rule | Value |
|---|---|
| Digests per request | at most 500 |
| Digest length | exactly 32 bytes, base64-encoded |
| `digestAlgorithm` | `SHA256` (or `SHA-256`) |
| `labels` | optional; if present, exactly one per digest |

- **SHA-1 is refused outright**, named as such: `REQUEST_INVALID` with
  `details.field = "digestAlgorithm"`. Nothing in this project produces or
  accepts it.
- **`certificateThumbprint` is required here**, and §5.5 is where you get
  one. It is not a hint. You
  have already built a CMS around one signer certificate, so a signature
  made with any other key produces a document that verifies against
  nothing. The agent offers that certificate to the person and no other;
  they still choose it and still press Approve. If it is not on the
  machine, the job fails `CERT_NOT_FOUND` and no window is opened.
- `labels` are display strings for the window. They are treated as
  untrusted text: control characters and Unicode direction overrides are
  stripped, long ones are truncated with the middle elided, and none of
  them is ever treated as a path or written to a log.

The result, once collected:

```json
{
  "signatures": ["<base64>", null, "<base64>"],
  "failures": [ { "index": 1, "code": "SIGN_FAILED" } ],
  "counts": { "total": 3, "succeeded": 2, "failed": 1 }
}
```

`signatures` has one entry per digest you sent, in the same order. A
document that did not sign is `null` there and has an entry in `failures`
naming its position — the array never closes up over a failure, because
that is how a signature ends up attached to the wrong document.

### 5.2 Whole documents — `POST /v2/sign/pdf`

```json
{
  "certificateThumbprint": "7758D4...",
  "documents": [ { "name": "ugovor.pdf", "content": "<base64>" } ],
  "level": "b-t",
  "stamp": { "visible": true, "position": "bottom-right" }
}
```

| Rule | Value |
|---|---|
| Documents per request | at most 200 |
| One document | at most 100 MB |
| All documents together | at most 500 MB |

The limits are enforced from the declared `Content-Length` before the body
is read, so an oversized request is refused without the agent holding it in
memory: `REQUEST_INVALID` with `details.maxBytes`.

- `certificateThumbprint` is **optional** here. The agent builds the CMS,
  so any usable certificate produces a valid document; leaving it out means
  the person chooses, exactly as they do when they sign something
  themselves.
- `level` is `b-b`, `b-t` or `b-lt`. Leaving it out means "whatever this
  agent is configured to produce", which is the person's own standing
  answer to the same question.
- `stamp` is your answer to how the signature should look: `visible` (a
  boolean, required if `stamp` is present at all) and `position`
  (`bottom-right`, `bottom-left`, `top-right`, `top-left`). Supplying it in
  full means the person is not asked — they see the approval and nothing
  else, which is the one-window, one-click case. Leaving `stamp` out means
  they choose.

The result:

```json
{
  "documents": [
    { "name": "ugovor.pdf", "content": "<base64>", "achievedLevel": "B-T" }
  ],
  "failures": [],
  "counts": { "total": 1, "succeeded": 1, "failed": 0 }
}
```

`achievedLevel` is the level the document **actually reached**, never the
one you asked for. A timestamp authority that did not answer produces a
B-B signature and says so; nothing in this project ever claims a level it
did not reach.

### 5.3 The batch fingerprint

Both endpoints answer with `batchFingerprint`: SHA-256, hex, over the
concatenation of the digests, in order. For `/v2/sign/pdf` the digests are
SHA-256 over each document exactly as you sent it.

It is the same value the person sees on the consent window, so a technical
user can compare what was approved against what you say you sent. Compute
it yourself and compare it to the one in the `202`.

### 5.4 The whole-document path can be switched off

A machine can be set up to serve only `/v2/sign` — a deployment where every
caller builds its own CMS does not need the other one. `/v2/sign/pdf` then
answers `DOCUMENT_SIGNING_DISABLED`, and the person at the machine can turn
it back on in the agent's Settings window without restarting anything.

`/v2/sign` has no such switch and never will. There is nothing to turn off
about an endpoint that cannot see a document in the first place.

### 5.5 Which certificates are here — `GET /v2/certificates`

`/v2/sign` requires a thumbprint (§5.1), and this is where you get one. It
is also what an SDK offers the person a choice from.

```
GET /v2/certificates
```

```json
{
  "certificates": [
    {
      "thumbprint": "7758D4D4B8973EA619B3225185EDE740B3D1ECCE",
      "displayName": "ВЕЉКО СТАНОЈЕВИЋ",
      "issuer": "MUPGradjaniCA4",
      "purpose": "signing",
      "qualified": true,
      "usable": true,
      "isTestKey": false
    },
    {
      "thumbprint": "1B2C3D4E5F60718293A4B5C6D7E8F90102030405",
      "displayName": "Zoran Milovanović",
      "issuer": "Halcom CA PO 2",
      "purpose": "signing",
      "qualified": true,
      "usable": false,
      "notUsableReason": "CARD_NOT_PRESENT",
      "isTestKey": false
    }
  ]
}
```

| Field | What it is |
|---|---|
| `thumbprint` | The SHA-1 thumbprint, uppercase hex. Exactly what `certificateThumbprint` takes. |
| `displayName` | The signer's name, built from `givenName` + `surname` — never parsed out of the common name. |
| `issuer` | The issuing CA's common name. |
| `purpose` | `signing`, `authentication` or `unknown`: the role the certificate's KeyUsage gives it. |
| `qualified` | Whether the issuer is a Trusted List service that was granted at the time of asking. |
| `usable` | Whether it can sign **right now**. |
| `notUsableReason` | Absent when it can; otherwise `CARD_NOT_PRESENT`, `CERT_EXPIRED` or `CERT_NOT_USABLE`. |
| `isTestKey` | A test certificate rather than a real one. If you show a certificate to a person, show this too. |

A certificate that cannot sign right now is listed rather than hidden, with
its reason: an absent card and an expired certificate are real choices
temporarily unavailable, and leaving them out is what makes a card look
broken. What is left out is what the agent's own window leaves out — the
machine's internal certificates, and the authentication certificate every
Serbian card carries beside its signing one, which has the same name on it
and cannot sign.

**No certificate is returned. Not the DER, not a PEM, not the public key.**
A Serbian qualified certificate carries the holder's national identity
number and email address inside it, and this listing is answered without any
window, at any moment you choose. It reports what the agent shows a person,
and nothing that was scrubbed out on the way there.

That leaves a real gap, stated rather than glossed: if you build your own
CMS you need the signer certificate itself, and you cannot get it here.
Today it has to come from wherever your integration already gets it — a copy
the person exported, or a document they have already signed. Making it
arrive with something the person approved is work this protocol has not done
yet.

**It can be switched off.** On a machine holding several clients' cards — an
accounting firm, which SPEC calls the ordinary case rather than an edge one
— this listing tells an application paired by one client the names on the
other clients' certificates, and nothing in a pairing implies that. The
person at the machine can turn it off in Settings; `/v2/certificates` then
answers `CERTIFICATE_LISTING_DISABLED`. Nothing else changes: on
`/v2/sign/pdf` you never needed a thumbprint, and on `/v2/sign` you have
already built a CMS around a certificate you therefore already know.

Like `/v2/health` and the job endpoints, it is a GET with no body, so it
sends no `Content-Type` (§2.5).

---

## 6. Jobs

### 6.1 Why

A hundred documents takes about 46 seconds after the PIN, plus however long
the person takes to approve them. No HTTP request is held open for that.

Submission answers in milliseconds:

```
202 Accepted
{
  "jobId": "decc3b5bf368d545b2f5c414597b9756",
  "batchFingerprint": "037c78a7...",
  "total": 100,
  "eventsUrl": "/v2/jobs/decc3b5b.../events",
  "resultUrl": "/v2/jobs/decc3b5b.../result"
}
```

**One job per application at a time.** A second submission while one is
running is answered `JOB_IN_PROGRESS` (409). A job belongs to the
application that created it; another application asking about it is
answered `JOB_NOT_FOUND`, exactly as if it did not exist.

### 6.2 Progress — `GET /v2/jobs/{id}/events`

Server-sent events, one per state change, until the job ends and the stream
closes.

```
data: {"state":"signing","completed":37,"total":100,"failed":0,"etaMs":26000}
```

| State | What is happening |
|---|---|
| `queued` | Accepted. Nothing is on screen yet — another job's window may be open. |
| `awaiting_consent` | The agent's window is up and the person has not answered. Carries `consentRemainingMs`. |
| `awaiting_pin` | They approved. The agent is opening the card, where the operating system may ask for a PIN. |
| `preparing_card` | The first signature is in flight. Measured at about 4.0 s on a MUP card and 12.7 s on a Pošta one — expected, not stalled. |
| `signing` | Every signature after the first. |
| `completed` | Finished, with a result waiting to be collected. |
| `failed` | Nothing was signed. Carries `code`. |

`etaMs` appears only once a first signature has actually been measured;
there is nothing honest to put there before then, and it is never computed
from a constant.

Every event carries the whole picture rather than a change to it, so a
client that reconnects mid-batch gets the state as it stands. A client that
falls behind lands on the newest state rather than working through a
backlog — a hundred `signing` events you have not read are worth less than
the one that says where the batch actually is. The terminal state is always
delivered.

**A browser cannot use `EventSource` here**, because `EventSource` cannot
set headers and this endpoint needs the four from §3. Use `fetch` with a
`ReadableStream`, or your language's ordinary HTTP client — which is where
the device secret belongs anyway (§2.4).

### 6.3 The result — `GET /v2/jobs/{id}/result`

| While | Answer |
|---|---|
| the job is running | `202` with the same body an event carries |
| it finished with signatures | `200` with the result of §5.1 or §5.2 |
| it finished with none | the failure's own code — `403` for a refusal, `422` for the card |
| any time after that | `404 JOB_NOT_FOUND` |

**The result is delivered once and the job is then forgotten.** Signatures
are the output of a qualified signing operation; they do not linger in a
process's memory waiting to be collected twice. Read the response, and if
you drop it, submit again.

A job whose result is never collected is discarded after **ten minutes**.

### 6.4 The consent window

The person has **120 seconds** to answer. The window shows a countdown in
the last thirty, and every `awaiting_consent` event carries
`consentRemainingMs` so you can show your own.

The request is not refused because nobody is at the machine: the window
opens and waits. On expiry the job fails `CONSENT_TIMEOUT` (403) and you
may submit again.

The window is the agent's own. It shows the application name **bound at
pairing**, the document count, the file list and the batch fingerprint
behind Details, and the certificate list. Approve is not the initially
focused control and is not pressable until a certificate has been chosen.
There is no flag, header or configuration that skips it.

---

## 7. Errors

Every error body is:

```json
{ "code": "PAIRING_CODE_INCORRECT", "details": { "attemptsRemaining": 4 } }
```

and nothing else. **No human-readable message crosses this boundary, in
any language** (SPEC §7). `details` carries structured facts, never prose,
and may be absent.

Codes are stable. They are never removed or repurposed; new situations get
new codes.

### 7.1 Every code a caller can receive

`INTERNAL` is the only one that is a `5xx`. Everything else means either
"fix your request" or "understood, and it did not happen" — a card that is
not there is not the agent failing, and answering `500` for one would say
it was.

**Pairing and authentication**

| Code | HTTP | What it means | What to do |
|---|---|---|---|
| `REQUEST_INVALID` | 400 | Wrong method, a body that is not JSON, a missing or unacceptable field, a body over the limit. `details.field` or `details.maxBytes` says which. | Fix the request. |
| `NOT_PAIRED` | 401 | No live pairing for this `appId`. | Pair again. |
| `AUTH_FAILED` | 401 | The request did not authenticate. | See §3.3. |
| `PAIRING_CODE_INCORRECT` | 401 | Wrong six-digit code; the request is still live. `details.attemptsRemaining`. | Ask the person to read the code again. |
| `PAIRING_DENIED` | 403 | The person refused the pairing, or closed the window. | Stop. This is an answer. |
| `PAIRING_ORIGIN_MISMATCH` | 403 | The `origin` in the confirm body is not the one the request declared. | Send the same `origin` field in both calls (§2.1). |
| `PAIRING_IN_PROGRESS` | 409 | Another application's pairing window is open. | Wait and try again. |
| `PAIRING_EXPIRED` | 410 | The pairing request is gone: five minutes passed, five wrong codes voided it, it was already confirmed, or there is no such request. | Start a new pairing request. |
| `RATE_LIMITED` | 429 | Too many pairing requests from this origin. `details.retryAfterSeconds`. | Wait that long. |

**Jobs**

| Code | HTTP | What it means | What to do |
|---|---|---|---|
| `DOCUMENT_SIGNING_DISABLED` | 403 | `/v2/sign/pdf` is switched off on this machine (§5.4). | Use `/v2/sign`, or ask the person to turn it on. |
| `CERTIFICATE_LISTING_DISABLED` | 403 | `GET /v2/certificates` is switched off on this machine (§5.5). | Ask the person for the thumbprint, or ask them to turn it on. Signing is unaffected. |
| `JOB_IN_PROGRESS` | 409 | This application already has a job running. | Wait for it. |
| `JOB_NOT_FOUND` | 404 | No such job: it never existed, it belongs to another application, its result has been collected, or it expired. | Submit again. |

**Signing**

| Code | HTTP | What it means | What to do |
|---|---|---|---|
| `CONSENT_DENIED` | 403 | The person pressed Cancel, closed the window, or stopped the batch. | Stop. This is an answer. |
| `CONSENT_TIMEOUT` | 403 | Nobody answered within 120 seconds. | Submit again when somebody is there. |
| `NO_READER` | 422 | No card reader is attached. | Ask the person to connect one. |
| `SMART_CARD_SERVICE_DOWN` | 422 | The operating system's smart card service is not running. | A service to start, not hardware to plug in. |
| `CARD_NOT_PRESENT` | 422 | The certificate's card is not in a reader. | Ask the person to insert it. |
| `PIN_REQUIRED` | 422 | The card needs its PIN and none was given. | The operating system asks; there is nothing to send. |
| `PIN_INCORRECT` | 422 | Wrong PIN. | **Never retry automatically.** See §7.2. |
| `PIN_LOCKED` | 422 | The card is blocked and needs its PUK. | Stop. |
| `CERT_NOT_FOUND` | 422 | No certificate with that thumbprint is available here. | Check the thumbprint, or leave it out on `/v2/sign/pdf`. |
| `CERT_EXPIRED` | 422 | The certificate is outside its validity period. | Nothing you can do from here. |
| `CERT_NOT_USABLE` | 422 | It exists but cannot sign. | Choose another. |
| `CERT_REVOKED` | 422 | Revocation says revoked. | Stop. |
| `PDF_INVALID` | 422 | A document is not a readable PDF. | Check what you sent. |
| `PDF_ENCRYPTED` | 422 | A document is password-protected. | Decrypt it first; the agent never will. |
| `TSA_UNAVAILABLE` | 422 | The timestamp authority did not answer after three attempts. | Retry later, or ask for `b-b`. |
| `TSA_REJECTED` | 422 | It refused the request. | Check the agent's timestamp settings. |
| `TSA_CLIENT_CERT_UNREADABLE` | 422 | The configured TSA client certificate could not be read. | The person's Settings, not your request. |
| `TSA_CLIENT_CERT_INVALID` | 422 | It was read and would not open — almost always a wrong password. | The person's Settings. |
| `STAMP_GLYPH_MISSING` | 422 | The visible stamp needs a character the embedded font does not have. `details.character`, `details.codePoint`. | Change the text. |
| `SIGN_FAILED` | 422 | The card refused or failed to sign. | Retry once; then stop. |
| `INPUT_UNREADABLE` | 422 | A document could not be read. Local batches only; you cannot cause this. | — |
| `OUTPUT_EXISTS`, `OUTPUT_WRITE_FAILED`, `OUTPUT_IN_USE` | 422 | About writing a file. Local batches only — a protocol batch writes nothing. | — |
| `VERSION_TOO_OLD` | 426 | This client is older than `minimumClientVersion`. | Update. |
| `INTERNAL` | 500 | Unclassified. The agent has logged it locally. | Report it. |

### 7.2 `PIN_INCORRECT` is never retried

When a signing call answers `PIN_INCORRECT`, **do not retry it
automatically. Ever.** Three wrong PIN entries block the card, and
for a national identity card unblocking means a visit to the Ministry. Show
the person what happened and let them decide.

---

## 8. Limits

| Limit | Value |
|---|---|
| Pairing requests open at one time | 1 |
| Pairing requests per minute, per origin | 3 |
| Jobs per application at one time | 1 |
| Digests per `/v2/sign` request | 500 |
| Documents per `/v2/sign/pdf` request | 200 |
| One document | 100 MB |
| All documents in one request | 500 MB |
| Timestamp skew | ±60 seconds |
| A nonce is remembered for | 5 minutes |
| Nonce length | 128 characters |
| One `/v2/echo` body | 64 KB |
| Consent | 120 seconds |
| An uncollected result is kept for | 10 minutes |

A refused pairing request is counted too: a limiter that only counts the
requests it allowed does not limit a caller in a loop.

---

## 9. A whole integration, start to finish

```
1. Read %LOCALAPPDATA%\Liro\bridge.json for the port. Never scan.

2. GET /v2/health
   -> 200 {"agentVersion":"...","protocolVersion":2,"minimumClientVersion":"0.0.0"}

Once, on the first run:

3. POST /v2/pair/request  {"applicationName":"My ERP","origin":"https://erp.example.com"}
   -> 200 {"requestId":"4f4f...","expiresInSeconds":300}

4. The agent opens a window on the person's screen showing six digits.
   You cannot see them. That is the point.

5. The person reads them to you. Your application submits them:

   POST /v2/pair/confirm  {"requestId":"4f4f...","code":"681956","origin":"https://erp.example.com"}
   -> 200 {"appId":"e1a2...","deviceSecret":"...","applicationName":"My ERP","origin":"..."}

   The origin is the same string as in step 3, and it has to be.


6. Store appId and deviceSecret on your server, and never anywhere else.

Every time after that:

7. GET /v2/certificates, if you need a thumbprint or want to offer a choice
   -> 200 {"certificates":[{"thumbprint":"7758D4...","displayName":"...",
            "usable":true, ...}]}

8. POST /v2/sign  (or /v2/sign/pdf), with the four headers from §3
   -> 202 {"jobId":"...","batchFingerprint":"...","total":100,
           "eventsUrl":"...","resultUrl":"..."}

9. Compare batchFingerprint against the one you computed. It is what the
   person is looking at.

10. GET the eventsUrl and read the stream, or poll the resultUrl. The
    person approves in the agent's own window; you cannot skip that and
    there is no header that does.

11. GET the resultUrl once the job has finished, and keep what it gives
    you. It is delivered once.

If step 8 is answered AUTH_FAILED and you cannot see why: POST the same
body to /v2/echo and compare the canonical string it returns against
your own (§3.5), then read the agent's log (§3.3).
```
