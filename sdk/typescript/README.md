# @liro/bridge

The TypeScript SDK for **Liro Bridge** — the desktop agent that signs PDFs with a
qualified electronic certificate on a smart card or a USB token, on the same
machine as your application.

```ts
import { FileSecretStore, LiroBridge } from '@liro/bridge';

const secretStore = new FileSecretStore('./liro-pairing.json');
const bridge = await LiroBridge.connect({ applicationName: 'Moj ERP', secretStore, onPairingCode });
const [signed] = await bridge.signPdf(pdfBytes);          // signed.content is the signed PDF
```

`onPairingCode` is yours: a function that asks the person for six digits, however
your application asks questions. It is called **once, ever** — on the first run,
while they are looking at the agent's own window. After that the pairing is in
your `secretStore` and it is never called again.

Everything else is this SDK's problem: finding the agent, pairing, storing the
secret, the canonical string, nonces, the job, the event stream, the timeouts,
and turning error codes into typed exceptions. You do none of it and should not
have to know it happens.

---

## Where the device secret goes, and where it must never go

**On your server. Never in a browser.**

The device secret is the whole of an application's authority to ask for a
signature. Not in `localStorage`, not in `sessionStorage`, not in code that is
shipped to a page: a secret in a browser is a secret every visitor has.

This is enforced, not merely written down. The agent sends no CORS headers of any
kind and answers a preflight with `403`, so page JavaScript cannot make a single
authenticated call to it. And **this SDK refuses to construct in a browser** —
`connect()` throws `UNSUPPORTED_ENVIRONMENT` with a sentence naming the reason,
so a developer who tries it is stopped in ten seconds rather than after building
a feature on top of an opaque `TypeError: Failed to fetch`.

If a page needs a signature, give it an endpoint of your own and call Liro Bridge
from your backend.

**Nothing is signed without a person.** There is no flag, header, option or
configuration anywhere in this SDK or in the protocol that skips the agent's own
consent window. The person sees who is asking, how many documents, and which
certificate; they choose it and they press Approve. That is not a formality — it
is the only boundary that actually holds, because a smart card caches its PIN in
its own state, independently of which process is talking to it.

---

## Installing

There is no npm package. Install straight from GitHub:

```bash
npm install github:veljaos/liro-bridge#main
```

or, better for anything you will deploy, pin a tag:

```bash
npm install github:veljaos/liro-bridge#v1.0.0
```

That removes a class of risk — nobody can squat a similar name or take over an
abandoned package — at the cost of one thing you should know: the built
JavaScript is **committed to the repository**, because installing from GitHub
runs no build step. See [Rebuilding](#rebuilding-the-committed-output).

- **ESM and CommonJS.** `import` and `require` both work.
- **Types are generated from the source** and ship with the package. You do not
  need `@types/node` to compile against it: nothing in the public surface is a
  Node-specific type, and a document is a `Uint8Array` rather than a `Buffer`.
- **Zero runtime dependencies.** Node's own `crypto`, `fs` and `fetch` cover
  everything. Every dependency here would be a dependency in your application,
  in a package that handles a signing secret.
- **Node 18 or later**, enforced with a message that says so rather than failing
  at some random later point with `fetch is not defined`.

---

## Where the secret lives: `secretStore`

**`secretStore` is required and has no default.** Not a file path with a sensible
fallback, not `~/.liro`. Somebody has to decide where a signing secret is kept,
and it is not this library — so the type system makes you decide.

If you have no strong opinion yet, this is the answer:

```ts
import { FileSecretStore } from '@liro/bridge';

const secretStore = new FileSecretStore('./liro-pairing.json');
```

One JSON file at a path you name. Its directory is created if it does not exist,
and the file is locked down as tightly as the platform allows — and a failure to
lock it down is an error, not a warning:

- **POSIX** — mode `0600`, owner read/write and nothing else.
- **Windows** — inheritance is broken and the Everyone, Authenticated Users,
  Users and Interactive groups are taken off it, so no other ordinary account on
  the machine can read it. (SYSTEM and Administrators may remain; an
  administrator can take ownership of any file regardless of its permissions, so
  removing them buys nothing.)

`FileSecretStore` exists so a first integration works. **For a server, implement
`SecretStore` against whatever your application already trusts with
credentials** — a secrets manager, an encrypted config service, a database column
your operations team knows about:

```ts
interface SecretStore {
  get(appName: string): Promise<StoredPairing | null>;
  set(appName: string, pairing: StoredPairing): Promise<void>;
  clear(appName: string): Promise<void>;
}
```

One rule when you write your own: **persist `pairing.serialise()` and rebuild
with `StoredPairing.deserialise()`.** `JSON.stringify(pairing)` deliberately
leaves the secret out, so that a pairing cannot reach a log by accident — which
means a store that persists the stringified form stores no secret at all. That
mistake does not go unnoticed: the next `connect()` throws
`SECRET_STORE_INVALID` with a sentence naming `serialise()`.

```ts
class MyStore implements SecretStore {
  async get(appName: string) {
    const row = await db.getSecret(appName);
    return row === null ? null : StoredPairing.deserialise(JSON.parse(row));
  }
  async set(appName: string, pairing: StoredPairing) {
    await db.putSecret(appName, JSON.stringify(pairing.serialise()));
  }
  async clear(appName: string) {
    await db.deleteSecret(appName);
  }
}
```

The secret is not on the pairing object in any form a log can reach:
`console.log(pairing)`, `JSON.stringify(pairing)`, `{...pairing}`,
`Object.keys(pairing)` and `util.inspect(pairing, { showHidden: true })` all show
everything except it. So does `console.log(bridge)`. There is a test that pairs,
walks every error path the SDK has, and greps every string it produced.

---

## Pairing

An application must be paired before it can ask for anything. Pairing happens
once, and the person doing it has to be at the machine.

```ts
const bridge = await LiroBridge.connect({
  applicationName: 'Moj ERP',
  origin: 'https://erp.example.com',
  secretStore,
  onPairingCode: ({ attemptsRemaining }) => promptTheUser(attemptsRemaining),
});
```

**What the person sees.** A small window of the agent's own, in their own
language, with three things on it:

```
  Zahtev
  Moj ERP

  Poreklo
  https://erp.example.com

                 Kod
             9 3 4 5 9 2
      Unesite ovaj broj u aplikaciju.
         Ovaj kod važi 5 minuta.

                                  [ Odbij ]
```

They read the six digits out to your application. **The code is in no response
and never will be**: if it were, an application could pair itself with nobody
watching and the window would prove nothing. There is no Allow button either —
the code *is* the approval, and a button beside it would be a second, weaker one.

- **`applicationName`** is bound at pairing and is what the person sees above
  every later signature. It cannot be changed in a signing request, so an
  application cannot pair as "Test" and later present itself as "Liro". It is
  also the key your `secretStore` is asked under, so two different applications
  on one machine must not share one.
- **`origin`** is shown **verbatim**, with no prettifying and no stripping of the
  scheme, so that a person who sees `http://` where they expected `https://` can
  notice. It defaults to `"local"`, which is the truthful answer for a program on
  the machine with no web origin of its own; if your application has a real one,
  pass it. The agent refuses an origin containing whitespace, a control character
  or a Unicode direction override, and one longer than 255 bytes — refuses rather
  than cleans up, because a value altered on its way to the screen is not
  verbatim.
- **The same `origin` goes in both calls.** Getting that wrong is what costs an
  afternoon writing a client by hand; here it is one variable used twice, and
  there is no way to pass a different one.

**A wrong code does not start over.** The pairing request stays live, the agent
says how many attempts remain, and `onPairingCode` is called again with
`attemptsRemaining` filled in. Five wrong codes void the request.

Each of the other things that can go wrong is its own typed error, with its own
message saying what to do: `PAIRING_EXPIRED` (five minutes, five wrong codes, or
already confirmed — start a new one), `PAIRING_DENIED` (they refused — stop, this
is an answer), `PAIRING_IN_PROGRESS` (another application's window is open —
wait), `RATE_LIMITED` (`details.retryAfterSeconds`).

**`disconnect()` is local only.** It forgets the stored pairing so the next
`connect()` pairs again. The protocol has no way to unpair, by design: a pairing
is revoked by the person, in the agent's own Settings window, where they can see
every application that is paired.

---

## The API

```ts
class LiroBridge {
  static connect(options: ConnectOptions): Promise<LiroBridge>;

  health(): Promise<Health>;
  certificates(): Promise<Certificate[]>;

  signPdf(documents, options?): Promise<SignedDocument[]>;
  signDigests(digests, options): Promise<Signature[]>;

  onProgress(handler: (p: Progress) => void): void;
  disconnect(): Promise<void>;
}
```

### `signPdf` — bytes in, bytes out

The common case is one document, and it needs no wrapping:

```ts
const [signed] = await bridge.signPdf(await readFile('ugovor.pdf'));
await writeFile('ugovor-signed.pdf', signed.content!);
```

Several, with the names the person will see on the consent window:

```ts
const results = await bridge.signPdf([
  { name: 'ugovor.pdf', content: a },
  { name: 'račun.pdf', content: b },
]);
```

Options, all optional:

```ts
await bridge.signPdf(pdf, {
  certificateThumbprint: '7758D4…',    // or leave it out and the person chooses
  level: 'b-lt',                        // or leave it out for the agent's own setting
  stamp: { visible: true, position: 'bottom-right' },
  signal: controller.signal,
  timeoutMs: 5 * 60_000,
});
```

**`stamp` supplied in full means the person is not asked** — they see the
approval and nothing else, which is the one-window, one-click case. Leaving it
out means they choose how the signature should look, exactly as they do when they
sign something themselves.

`SignedDocument.achievedLevel` is the level the document **actually reached**,
never the one you asked for. A timestamp authority that did not answer produces a
`B-B` signature and says so; nothing in this project ever claims a level it did
not reach.

**A failed document is `content: null` with a `failure`, in place.** The array
never closes up over one: if you sent document 47 you can find entry 47, because
a list that silently closes up is how a signature ends up attached to the wrong
document. When *every* document fails, the job itself failed and `signPdf`
throws instead.

### `certificates` — what this machine can sign with

```ts
for (const c of await bridge.certificates()) {
  console.log(c.thumbprint, c.displayName, c.usable ? 'usable' : c.notUsableReason);
}
```

A certificate that cannot sign right now is listed rather than hidden, with its
reason (`CARD_NOT_PRESENT`, `CERT_EXPIRED`, `CERT_NOT_USABLE`): an absent card is
a real choice temporarily unavailable, and leaving it out is what makes a card
look broken. What is left out is what the agent's own window leaves out — the
machine's internal certificates, and the authentication certificate every Serbian
card carries beside its signing one, which has the same name on it and cannot
sign.

**If a certificate reaches a person's screen, show `isTestKey` too.** A soft-token
signature is a test signature and has to be visibly marked everywhere it appears.

This can throw `CERTIFICATE_LISTING_DISABLED`. **That is a setting on the
machine, not a fault in your request**: on a machine holding several clients'
cards, this listing would tell an application paired by one client the names on
the other clients' certificates, so the person at the machine can switch it off.
Signing is unaffected.

**No certificate is returned — not the DER, not a PEM, not the public key.** A
Serbian qualified certificate carries the holder's national identity number and
email address inside it, and this listing is answered without any window, at any
moment you choose.

### `signDigests` — when you build the CMS yourself

The path on which the agent never possesses your document: you send hashes, you
get signatures.

```ts
const [signature] = await bridge.signDigests([sha256OfMySignedAttributes], {
  certificateThumbprint: '7758D4D4B8973EA619B3225185EDE740B3D1ECCE',
  labels: ['ugovor.pdf'],
});
```

**`certificateThumbprint` is required here, and it is not a hint.** You have
already built a CMS around one signer certificate, so a signature made with any
other key produces a document that verifies against nothing. The agent offers
that certificate to the person and no other; they still choose it and still press
Approve.

**And there is a hole here, which this SDK will not paper over.** To build a CMS
you need the signer certificate itself — for `issuerAndSerialNumber`, for
`signingCertificateV2`, and for the certificates set. The protocol will not give
it to you, for the reason in the previous section. What you can do today:

1. call `certificates()`, and let the person choose one;
2. build the CMS around the copy of that certificate your integration already
   has — one they exported, or one taken from a document they have already
   signed;
3. send the digest here, naming that certificate's thumbprint.

If you have no such copy, use `signPdf` instead and let the agent build the CMS.
Making the certificate travel with something the person approved is work this
protocol has not done yet, and nothing here pretends otherwise.

### `onProgress` — optional, and everything works without it

Somebody signing one document should not have to think about a stream.

```ts
bridge.onProgress((p) => {
  if (p.state === 'awaiting_consent') {
    showCountdown(p.consentRemainingMs);   // milliseconds left to answer
  } else if (p.state === 'preparing_card') {
    show('Preparing the card…');
  } else if (p.state === 'signing') {
    showBar(p.completed, p.total, p.etaMs);
  }
});
```

| State | What is happening |
|---|---|
| `queued` | Accepted. Nothing is on screen yet — another job's window may be open. |
| `awaiting_consent` | The agent's window is up and the person has not answered. Carries `consentRemainingMs`; they have 120 seconds. |
| `awaiting_pin` | They approved. The agent is opening the card, where the operating system may ask for a PIN. |
| `preparing_card` | **The first signature is in flight. This is expected, not a stall** — measured at about 4.0 s on a MUP card and 12.7 s on a Pošta one. It is card initialisation and there is no way to avoid it, which is why it has a state of its own: a progress bar that does not move for twelve seconds reads as a hang, and this is what to show instead. |
| `signing` | Every signature after the first — about 0.41 s each. |
| `completed` / `failed` | Done. |

`etaMs` is `null` until a first signature has actually been measured. There is
nothing honest to put there before then, and it is never computed from a
constant.

Every event carries the whole picture rather than a change to it, so a handler
that misses one has lost nothing. Anything your handler throws is swallowed: a
batch nobody can get back is not failed by a logging call.

---

## Errors

Every error is a `LiroError` carrying a `code` you can branch on and the agent's
structured `details`. The union is exhaustive, so a `switch` with a `never`
default is checked by the compiler.

```ts
import { LiroError } from '@liro/bridge';

try {
  await bridge.signPdf(pdf);
} catch (e) {
  if (e instanceof LiroError && e.code === 'CARD_NOT_PRESENT') {
    // tell the person to insert their card
  }
}
```

**The agent never sends a human-readable message, in any language.** An error
body is a stable code and optional structured facts, and nothing else — because
the agent serves three interface languages and an open set of callers it does not
control, and only you know what language the person in front of you speaks.
`LiroError.message` is an English sentence for a developer's log; write your own
for anything a person reads.

Two of the messages are worth reading before you need them.

### `AUTH_FAILED` — and what to do about it

Every way authentication can fail answers `401 AUTH_FAILED` with no detail, and
always will: a code that said "your nonce was reused" would tell whoever is
probing that the timestamp and the signature were both fine. Two things say more,
and between them there is nothing left to guess at:

1. **The agent's own log** records which check refused each request —
   `signature_mismatch`, `clock_skew`, `unparseable_timestamp`, `nonce_reused`,
   `origin_mismatch`, `missing_header`, `nonce_too_long`. It is on the machine
   you are already sitting at, at `%LOCALAPPDATA%\Liro\logs\bridge.log`.
2. **`POST /v2/echo`** returns the canonical string the agent built from your
   request. Print your own beside it and the difference is one line.

If you are using this SDK you should never see `AUTH_FAILED` at all — it builds
the canonical string, the nonces and the timestamps for you. If you do, the
likeliest causes are a machine clock more than 60 seconds out, or a pairing the
person has disconnected in Settings (which is `NOT_PAIRED`, not this).

`bridge.echo(body)` is there for when you are debugging another client:

```ts
const { canonicalString, bodySha256 } = await bridge.echo('{"anything":"you like"}');
```

### `PIN_INCORRECT` — never retry this

**Never retry a `PIN_INCORRECT` automatically. Ever, under any circumstance.**
Three wrong entries block the card, and for a Serbian national identity card
unblocking it means a visit to the Ministry. Show the person what happened and
let them decide what to do next. This SDK never retries it, and neither should
anything you build on top.

### What is retried, and what is not

Only a failure that left no response at all, or a `5xx`, and only on a request
that is safe to repeat: `health`, `certificates`, `echo`. Every attempt gets a
new nonce and a new timestamp.

Never retried, and each for its own reason:

- **A `401`.** A retry of a request that did not authenticate does not
  authenticate either.
- **A submission.** A `POST /v2/sign` that timed out may already have created a
  job. Sending it again is a hundred documents signed twice, with a second window
  in front of a person for a batch they have already approved.
- **`POST /v2/pair/confirm`.** Every confirm spends one of five code attempts.
- **The result.** It is delivered once and the job is then forgotten.

---

## Finding the agent

The agent binds loopback only, on the first free port in 17580–17590, and writes
what it chose to a per-user file — `%LOCALAPPDATA%\Liro\bridge.json` on Windows.
This SDK reads it. **It never scans ports, and there is no option that would turn
some on**: several people can be signed in to one machine at once, an accounting
firm over RDP being the ordinary case rather than an edge one, and scanning finds
somebody else's agent.

When the agent is not running you get `AGENT_NOT_RUNNING`, with a sentence saying
so. A file left behind by a crash points at a port nothing is listening on, and
that means the same thing.

---

## Rebuilding the committed output

`sdk/typescript/dist/` is committed, because installing from GitHub runs no build
step: what is in the repository is what an integrator gets.

```bash
cd sdk/typescript
npm install          # the TypeScript compiler and the Node types; neither ships
npm run build        # rewrites dist/
npm run check-build  # builds again into a scratch directory and diffs
npm test
```

`npm run check-build` runs in CI. A committed artefact nobody regenerates is a
committed artefact in name only — this project has already had one generated file
drift from its generator, and running the generator would have deleted a block
five screens depended on.

---

## Other languages

Liro Bridge's protocol is plain HTTP with an HMAC over a canonical string. There
is one first-class SDK, this one; everything else is about twenty lines. The
complete, runnable versions of each are in
[`sdk/examples/`](../examples), and the fragment below each heading is the part
that matters — reading the discovery file, and signing a request.

Each avoids, by construction, the four mistakes writing a client by hand actually
produces: a wrong field name, `origin` missing from `pair/confirm`, a body
altered between being hashed and being sent, and a `Content-Type` on a request
with no body. That last one is not a detail: **a request with no body must not
send `Content-Type`.** Several HTTP clients cannot attach one to a request with
no content at all — .NET's `HttpClient` puts the header on the content, so a
`GET` has nowhere to put it — and the agent does not ask for one.

### C#

Full version: [`sdk/examples/Sign.cs`](../examples/Sign.cs). Compiles with the C#
compiler that ships inside Windows and with modern .NET alike.

```csharp
private static readonly HttpClient Http = new HttpClient();
private static string _base;

/// Reads the per-user discovery file. Never scan ports: several
/// people can be signed in to one machine, each with their own agent.
///
/// From the LOCALAPPDATA *environment variable*, because that is what
/// the agent itself writes to. Environment.GetFolderPath reads the
/// shell's known folder instead, and the two are not always the same
/// path — under folder redirection, or in a process that inherited a
/// different environment, they disagree and this looks in the wrong
/// place. Measured: it did.
private static void Discover()
{
    var local = Environment.GetEnvironmentVariable("LOCALAPPDATA")
                ?? Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
    var path = Path.Combine(local, "Liro", "bridge.json");
    if (!File.Exists(path))
    {
        throw new FileNotFoundException("Liro Bridge is not running: no " + path, path);
    }
    var port = Regex.Match(File.ReadAllText(path), "\"port\"\\s*:\\s*(\\d+)").Groups[1].Value;
    _base = "http://127.0.0.1:" + port;
}

/// One request, signed when a secret is given.
private static async Task<Tuple<int, string>> Call(
    string method, string path, string json, string appId, string secret)
{
    var raw = json == null ? new byte[0] : Encoding.UTF8.GetBytes(json);  // hashed AND sent
    var request = new HttpRequestMessage(new HttpMethod(method), _base + path);
    if (raw.Length > 0)
    {
        request.Content = new ByteArrayContent(raw);                     // only when
        request.Content.Headers.ContentType =                            // there is a body
            new MediaTypeHeaderValue("application/json");
    }
    if (secret != null)
    {
        var ts = ((long)(DateTime.UtcNow - new DateTime(1970, 1, 1)).TotalSeconds).ToString();
        var nonce = Guid.NewGuid().ToString("N");
        string bodyHash;
        using (var sha = SHA256.Create()) bodyHash = Hex(sha.ComputeHash(raw));
        var canon = string.Join("\n", new[] { method.ToUpperInvariant(), path, ts, nonce, bodyHash });
        using (var mac = new HMACSHA256(Convert.FromBase64String(secret)))
        {
            request.Headers.Add("X-Liro-App-Id", appId);
            request.Headers.Add("X-Liro-Timestamp", ts);
            request.Headers.Add("X-Liro-Nonce", nonce);
            request.Headers.Add("X-Liro-Signature", Hex(mac.ComputeHash(Encoding.UTF8.GetBytes(canon))));
        }
    }
    var response = await Http.SendAsync(request);
    var body = await response.Content.ReadAsStringAsync();   // {"code":"...","details":{...}}
    return Tuple.Create((int)response.StatusCode, body);
}

private static string Hex(byte[] b) { return BitConverter.ToString(b).Replace("-", "").ToLowerInvariant(); }

```

### Python

Full version: [`sdk/examples/sign.py`](../examples/sign.py). Standard library only.

```python
BASE = "http://127.0.0.1:%d" % json.load(
    open(os.path.join(os.environ["LOCALAPPDATA"], "Liro", "bridge.json"))
)["port"]                                     # read the file; never scan ports


def call(method, path, body=None, app_id=None, secret=None):
    raw = b"" if body is None else json.dumps(body).encode("utf-8")   # hashed AND sent
    headers = {}
    if raw:
        headers["Content-Type"] = "application/json"   # only when there is a body
    if secret:
        ts, nonce = str(int(time.time())), uuid.uuid4().hex
        canon = "\n".join([method, path, ts, nonce, hashlib.sha256(raw).hexdigest()])
        headers.update({
            "X-Liro-App-Id": app_id, "X-Liro-Timestamp": ts, "X-Liro-Nonce": nonce,
            "X-Liro-Signature": hmac.new(base64.b64decode(secret),
                                         canon.encode("utf-8"), hashlib.sha256).hexdigest(),
        })
    req = urllib.request.Request(BASE + path, data=raw or None, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, json.loads(r.read() or b"null")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"null")     # {"code": "...", "details": {...}}
```

### Java

Full version: [`sdk/examples/Sign.java`](../examples/Sign.java). JDK 11 or later,
no build file needed: `java Sign.java ugovor.pdf`.

```java
private static final HttpClient HTTP = HttpClient.newHttpClient();
private static String base;

/** Reads the per-user discovery file. Never scan ports. */
static void discover() throws IOException {
    Path file = Paths.get(System.getenv("LOCALAPPDATA"), "Liro", "bridge.json");
    Matcher m = Pattern.compile("\"port\"\\s*:\\s*(\\d+)").matcher(Files.readString(file));
    if (!m.find()) throw new IOException("no port in " + file);
    base = "http://127.0.0.1:" + m.group(1);
}

/** One request, signed when a secret is given. Returns {status, body}. */
static Object[] call(String method, String path, String json, String appId, String secret)
        throws Exception {
    byte[] raw = json == null ? new byte[0] : json.getBytes(StandardCharsets.UTF_8); // hashed AND sent
    HttpRequest.Builder b = HttpRequest.newBuilder(URI.create(base + path))
            .method(method, raw.length == 0
                    ? HttpRequest.BodyPublishers.noBody()
                    : HttpRequest.BodyPublishers.ofByteArray(raw));
    if (raw.length > 0) b.header("Content-Type", "application/json");  // only with a body
    if (secret != null) {
        String ts = Long.toString(System.currentTimeMillis() / 1000);
        String nonce = UUID.randomUUID().toString().replace("-", "");
        String bodyHash = hex(MessageDigest.getInstance("SHA-256").digest(raw));
        String canon = String.join("\n", method.toUpperCase(), path, ts, nonce, bodyHash);
        Mac mac = Mac.getInstance("HmacSHA256");
        mac.init(new SecretKeySpec(Base64.getDecoder().decode(secret), "HmacSHA256"));
        b.header("X-Liro-App-Id", appId).header("X-Liro-Timestamp", ts).header("X-Liro-Nonce", nonce)
         .header("X-Liro-Signature", hex(mac.doFinal(canon.getBytes(StandardCharsets.UTF_8))));
    }
    HttpResponse<String> r = HTTP.send(b.build(), HttpResponse.BodyHandlers.ofString());
    return new Object[] {r.statusCode(), r.body()};   // {"code":"...","details":{...}}
}

static String hex(byte[] bytes) {
    StringBuilder s = new StringBuilder(bytes.length * 2);
    for (byte x : bytes) s.append(String.format("%02x", x));
    return s.toString();
}
```

### PHP

Full version: [`sdk/examples/sign.php`](../examples/sign.php). No extensions
beyond the standard build.

```php
$discovery = getenv('LOCALAPPDATA') . DIRECTORY_SEPARATOR . 'Liro' . DIRECTORY_SEPARATOR . 'bridge.json';
if (!is_file($discovery)) {
    fwrite(STDERR, "Liro Bridge is not running: no $discovery\n");   // never scan ports
    exit(1);
}
$base = 'http://127.0.0.1:' . json_decode(file_get_contents($discovery), true)['port'];

/** One request, signed when a secret is given. Returns [status, decoded body]. */
function liro_call(string $method, string $path, ?array $body = null,
                   ?string $appId = null, ?string $secret = null): array {
    global $base;
    $raw = $body === null ? '' : json_encode($body, JSON_UNESCAPED_SLASHES);  // hashed AND sent
    $headers = [];
    if ($raw !== '') {
        $headers[] = 'Content-Type: application/json';        // only when there is a body
    }
    if ($secret !== null) {
        $ts    = (string) time();
        $nonce = bin2hex(random_bytes(16));                   // never mt_rand()
        $canon = implode("\n", [strtoupper($method), $path, $ts, $nonce, hash('sha256', $raw)]);
        $sig   = hash_hmac('sha256', $canon, base64_decode($secret));
        $headers[] = "X-Liro-App-Id: $appId";
        $headers[] = "X-Liro-Timestamp: $ts";
        $headers[] = "X-Liro-Nonce: $nonce";
        $headers[] = "X-Liro-Signature: $sig";
    }
    $context = stream_context_create(['http' => [
        'method'        => strtoupper($method),
        'header'        => implode("\r\n", $headers),
        'content'       => $raw,
        'ignore_errors' => true,        // read 4xx and 5xx bodies rather than throwing
        'timeout'       => 30,
    ]]);
    $text   = file_get_contents($base . $path, false, $context);
    $status = (int) explode(' ', $http_response_header[0])[1];
    return [$status, $text === '' ? null : json_decode($text, true)];  // {"code":…,"details":…}
}
```

### Go

Full version: [`sdk/examples/sign.go`](../examples/sign.go). Standard library
only; the agent itself is written in Go, so somebody always asks.

```go
var base string // http://127.0.0.1:<port>, from the discovery file

func discover() error {
	b, err := os.ReadFile(filepath.Join(os.Getenv("LOCALAPPDATA"), "Liro", "bridge.json"))
	if err != nil {
		return fmt.Errorf("Liro Bridge is not running: %w", err) // never scan ports
	}
	var info struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return err
	}
	base = fmt.Sprintf("http://127.0.0.1:%d", info.Port)
	return nil
}

// call sends one request, signing it when a secret is given.
func call(method, path string, body any, appID, secret string) (int, []byte, error) {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil { // hashed AND sent: one slice
			return 0, nil, err
		}
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) > 0 {
		req.Header.Set("Content-Type", "application/json") // only when there is a body
	}
	if secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := hex.EncodeToString(randomBytes(16))
		sum := sha256.Sum256(raw)
		canon := strings.Join([]string{method, path, ts, nonce, hex.EncodeToString(sum[:])}, "\n")
		key, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			return 0, nil, err
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(canon))
		req.Header.Set("X-Liro-App-Id", appID)
		req.Header.Set("X-Liro-Timestamp", ts)
		req.Header.Set("X-Liro-Nonce", nonce)
		req.Header.Set("X-Liro-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return resp.StatusCode, out, err // an error body is {"code":"...","details":{...}}
}
```

### PowerShell

Full version: [`sdk/examples/liro-test-client.ps1`](../examples/liro-test-client.ps1)
— the client this protocol was first proven against, with a real MUP card.

---

## The specification

[`docs/PROTOCOL.md`](../../docs/PROTOCOL.md) is what this SDK implements against,
and what to read if you are writing a client of your own. It is the authority; if
this README and that document disagree, that document is right and this is a bug.

---

## Licence

Apache License 2.0.
