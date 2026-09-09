# Examples

One program, seven times: read the discovery file, pair, list the certificates,
submit a document, watch the job, collect the signature. Each is written against
[`docs/PROTOCOL.md`](../../docs/PROTOCOL.md) alone and has no dependencies beyond
its language's standard library.

| File | Run it with |
|---|---|
| [`sign-with-sdk.mjs`](sign-with-sdk.mjs) | `node sign-with-sdk.mjs ugovor.pdf` — through [`@liro/bridge`](../typescript) |
| [`liro-test-client.ps1`](liro-test-client.ps1) | `powershell -File liro-test-client.ps1 ugovor.pdf` |
| [`sign.py`](sign.py) | `python sign.py ugovor.pdf` |
| [`sign.go`](sign.go) | `go run sign.go ugovor.pdf` |
| [`Sign.cs`](Sign.cs) | `csc /r:System.Net.Http.dll Sign.cs` then `Sign.exe ugovor.pdf` |
| [`Sign.java`](Sign.java) | `java Sign.java ugovor.pdf` (JDK 11+) |
| [`sign.php`](sign.php) | `php sign.php ugovor.pdf` |

`sign-with-sdk.mjs` is the one to read first, and then to compare against any of
the others. It does the same thing in a fifth of the lines, and the difference is
the whole argument for the SDK.

## Which of these have actually been run

An example that does not run is worse than no example: the reader copies it, it
does not work, and they have lost more time than writing it themselves would have
cost. So this says exactly what was executed and what was not.

| Example | Run against a real agent? |
|---|---|
| `sign-with-sdk.mjs` | **Yes.** Paired, listed the certificates, submitted, watched the event stream, and received a 104 888-byte signed PDF at level B-B. |
| `liro-test-client.ps1` | **Yes**, 104 890 bytes. Also the client this protocol was first proven against, with a real MUP card, in phase F7. |
| `sign.py` | **Yes**, 104 888 bytes. |
| `sign.go` | **Yes**, 104 888 bytes. |
| `Sign.cs` | **Yes**, 104 888 bytes, compiled with the C# compiler that ships inside Windows (`%WINDIR%\Microsoft.NET\Framework64\v4.0.30319\csc.exe`). |
| `Sign.java` | **No.** Written against `docs/PROTOCOL.md`; **never executed**, because the machine these were prepared on has no JDK. |
| `sign.php` | **No.** Written against `docs/PROTOCOL.md`; **never executed**, because that machine has no PHP. |

The five that ran did so against a real agent built with the soft token, over a
real loopback socket, with a real pairing window on screen and a person pressing
Approve. Every one of the five documents they received back was then checked with
this project's own independent verifier (`internal/pades/verify`), which reported
`ByteRangeDigestOK`, `SignatureOK` and `SigningCertificateOK` for all five and no
errors. Running them found three defects that reading them did not — all three
are described under [Two things a Windows console will do to
you](#two-things-a-windows-console-will-do-to-you) and the `LOCALAPPDATA`
paragraph below it, and all three are fixed.

**`Sign.java` and `sign.php` carry those same fixes by transcription, not by
having been proven.** They are the same program as the five that ran, in two more
languages, and they are here because a Java or PHP integration is a real audience
— but if one of them does not compile or run first time, that is why. Please
[open an issue](https://github.com/veljaos/liro-bridge/issues).

All of them keep the pairing in `LIRO_APP_ID` and `LIRO_SECRET`, which they print
after the first run. That is the right thing for an example and **the wrong thing
for a real integration**: environment variables end up in process listings, crash
dumps and CI logs. See the SDK's `SecretStore`, or your platform's secrets
manager.

## What each of them is careful about

Writing the first client against this protocol by hand produced four failures in
a row, and every example avoids all four by construction:

1. **A wrong field name.** Every name comes from `PROTOCOL.md` §5.2's own
   example.
2. **`origin` missing from `pair/confirm`.** Both pairing calls carry `origin`
   and the two must be byte-for-byte equal. In each example it is one variable,
   used twice.
3. **A body altered between being hashed and being sent.** The bytes are built
   once, and the same bytes are both hashed and written.
4. **A `Content-Type` on a request with no body.** A request with no body must
   **not** send one — .NET's `HttpClient` puts the header on the content, so a
   `GET` has nowhere to put it, and the agent does not ask for one.

## Two things a Windows console will do to you

**Serbian names.** A certificate's display name is `Zoran Milovanović` or
`ВЕЉКО СТАНОЈЕВИЋ`; that is the normal case for this product, not an edge one. A
Windows console runs on a legacy code page, and Python **raises
`UnicodeEncodeError` and dies** on such a name unless `sys.stdout` is
reconfigured — measured, on the certificate listing, first run. `sign.py`,
`Sign.cs` and `Sign.java` each set their output encoding for this reason.

`sign.go` and `sign.php` write UTF-8 bytes straight out, which a legacy console
renders as mojibake rather than refusing. If you see one, `chcp 65001` before
running fixes it. Neither example carries a platform-specific workaround for
this: the fix belongs to the console, not to the protocol.

**Where `%LOCALAPPDATA%` is.** Read the **environment variable**, which is what
the agent itself writes to. .NET's `Environment.GetFolderPath(LocalApplicationData)`
reads the shell's known folder instead, and the two are not always the same path.
`Sign.cs` used to use it and looked in the wrong place — also measured, also on
the first run.

## Running them

Everything below needs an agent running. For a machine with no card in it, build
one with the soft token (SPEC §16.6 — a release build never contains this code):

```sh
go run ./scripts/gentestkeys ./testdata/softtoken/local
set LIRO_SOFTTOKEN_P12=.\testdata\softtoken\local\test.p12
set LIRO_SOFTTOKEN_PASSWORD=liro-softtoken-test
go run -tags softtoken ./cmd/liro-bridge tray
```

Then run any example. It will open a window on your screen with six digits in it;
type them in when it asks. Approving the signature is a second window, and a
second deliberate act — there is no way to skip either, which is the point.
