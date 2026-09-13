# Handover: 2026-09-13, for whoever picks this up next

One session's worth. Three things were started and none of them is
finished; this says where each one stands, what is blocked on the owner,
and the one thing that is loose in the world rather than in the
repository.

Read `docs/ui-text-review.md` and `docs/decisions.md` D-264 for the
substance. This file is only what is still owed.

---

## 1. The text pass is queued, and only partly specified

`docs/ui-text-review.md` is the whole of `sr-Latn.json` — all 365 keys,
grouped the way the program is arranged rather than the way the JSON
file is, with a "where it appears" line for each. It is a document to be
**written on**, and the owner has not finished reading it. **Nothing in
any catalogue has been changed on the strength of it.**

What it already establishes, so nobody re-derives it:

- The three catalogues are in **exact key parity** (365 each), every
  format verb matches in count and order, and `sr-Cyrl.json` is a
  character-exact transliteration of `sr-Latn.json`. Consequence for the
  pass: once the Latin is settled, Cyrillic can be produced
  mechanically and only `en.json` needs a human.
- **18 findings** are recorded, each naming its keys — six dead keys,
  `place.title` used as a file-chooser title, three names for the
  timestamp authority, "fajl"/"datoteka" 13 keys to 2, "folder"/
  "fascikla" 5 to 2, Serbian's 2–4 plural handled in exactly one place,
  and the rest.
- The **Explorer verb** section carries the real neighbouring verbs read
  out of `C:\Windows\System32\sr-Latn-RS\shell32.dll.mui` by string ID,
  not recalled. The recommendation is `Potpiši`; not decided.

### What is settled, and what is not

Three changes were asked for directly. They are in the document under
**"Changes requested during review"**, C1–C3.

| | State |
|---|---|
| **C1** origin typeface | **Settled.** Settings → Povezane aplikacije only, the `<code>` element: drop `font-family: var(--liro-font-family-mono)` at `settings.css:135`. The pairing window is to be **left alone**. The origin is not a link and must not become one. Not yet made. |
| **C2** pairing success screen | **Partly settled.** `pairing.connected_title` "Povezano" → **"Uspešno povezano"**; remove the application name (`#connected-name`) and its scrolling region; **Zatvori stays** — "nothing else" meant no explanatory paragraph, not no way out. **Blocked on the owner pasting the lucide `circle-check-big` SVG.** It was said to be attached and did not arrive. Do **not** draw a substitute: the instruction was explicitly to use the supplied file as the source. |
| **C3** `place.unavailable` | **Noticed, not asked for, not settled.** One string doing two jobs — see §3 below. |

**The rest of the document is unreviewed.** Do not start making the 18
findings' changes unprompted; the owner is reading top to bottom and
marking it up.

---

## 2. The two demos exist; B has never been run to the end

`sdk/examples/demo-a-application-decides.mjs` and
`demo-b-person-places-stamp.mjs`, sharing
`demo-protocol-common.mjs`. They are not integration examples — they
exist to be **watched**, driving the agent down the protocol path so the
windows an ERP's user would see can be seen.
`sdk/examples/DEMO-WALKTHROUGH.md` is the screen-by-screen, including
which screens are deliberately absent.

**Status, honestly:**

- **Demo A** has been run by the owner: paired with a window on screen,
  listed certificates, reached the approval. Whether it went through to
  a written PDF was never confirmed.
- **Demo B has never completed.** Its only run died at
  `GET /v2/certificates` — see §3. It has not been re-run since the fix.

So the first thing worth doing is running demo B through to the end and
recording what happened, in `sdk/examples/README.md`'s own "which of
these have been run" table, which is already written to say exactly
this.

**The thing demo B is for.** Choosing "Potpiši birajući poziciju
potpisa" on the method step and pressing Potpiši **cannot open the
placement picker on the protocol path**, and this is deliberate: the
picker draws the page it is stamping, and a protocol batch's documents
arrived over a socket and are nowhere on disk
(`signflow_windows.go`, `signAtAChosenPosition`). It falls back to the
corners with a warning. That warning is **C3**: `place.unavailable`
reads "Ovaj dokument ne može da se prikaže" — *this document cannot be
displayed* — which is true for a local document the renderer cannot
draw, and wrong here, where the document is not a file at all.

**Two side effects of demo B, both real:**

- The method screen calls `saveConfig()` **unguarded for protocol runs**,
  so whatever corner the person picks becomes their standing default for
  local signing too. Not fixed; possibly should be.
- `liro-demo/` in the working directory holds the demos' **device
  secrets** (`pairing-a.json`, `pairing-b.json`, a live `deviceSecret`
  in cleartext) and their signed output. It was not in `.gitignore` and
  now is — `/liro-demo/`, added this session, because `git add -A` would
  otherwise have pushed an application's whole authority to sign on this
  machine to a public repository. Do not remove that line.

**Both demos need an agent built from current source.** A stale one is
what produced §3.

---

## 3. D-264's other half: every installed agent still answers a bare 404

This is the one item that is loose in the world rather than in the
repository, and the one an integrator will meet before the next release.

**What happened.** An SDK asked an agent for `GET /v2/certificates` and
received HTTP 404, `Content-Type: text/plain`, body `404 page not
found`. No JSON, no code — the one answer `PROTOCOL.md` §7 says is
impossible. The cause was not a lost pairing, which is what it looked
like: the agent was a **stale binary** built before that route existed,
so `http.ServeMux` fell through to its own `NotFoundHandler`.

**What is fixed, in this repository.** `ENDPOINT_NOT_FOUND` (404, with
`details.path` and `details.method`) now answers every unmatched path,
and a `recoverPanics` wrapper turns a panicking handler into `INTERNAL`
instead of a dropped connection. Both have tests in
`internal/api/uncoded_test.go`. `PROTOCOL.md` §7 documents the code.

**What is not fixed, and cannot be.** **Every agent already installed
answers a bare uncoded 404, 0.9.1 included.** The fix reaches the next
release and nothing before it. So:

- The SDK does the other half: a 404 with no code now carries the
  sentence the agent could not send — that the agent is probably older
  than the SDK and should be updated (`transport.ts`).
- `PROTOCOL.md` §7 says out loud that a bare uncoded 404 and
  `ENDPOINT_NOT_FOUND` are the same condition, so an integrator meeting
  the old behaviour reads it correctly.

**What this means for whoever is here next.** If an integrator reports
an uncoded 404, the answer is "your agent is older than your SDK, update
it" — not "re-pair". And the 0.9.1 release notes, if any are written,
should say it: the symptom is indistinguishable from a broken pairing
and the wrong diagnosis costs an afternoon. It cost one here.

---

## 4. State of the tree and the machine

- **Nothing is committed.** Everything from this session is in the
  working tree. The exact commands are in the session's closing message;
  if that is gone, `git status` and the four groups below are the whole
  of it: the D-264 fix (`internal/errs`, `internal/api`, the three
  catalogues, `PROTOCOL.md`, `decisions.md`, the SDK `src` and rebuilt
  `dist`), the demos, the review document, and `sdk/examples/README.md`.
- `liro-bridge.exe` at the repository root was **rebuilt** from current
  source with `-tags softtoken`. It was a 7 September build, which is
  what caused §3. It is gitignored, so it is not part of any commit —
  but it is now current, and `.\liro-bridge.exe certs` proves it.
- **No agent is running.** One was at the start of the session (the
  installed 0.9.1, as `tray`); the owner stopped it, and the one started
  briefly to verify the fix over a real socket was stopped and its stale
  `bridge.json` removed.
- The soft-token material at `testdata/softtoken/local/test.p12` exists
  and works; there is also a **real MUP card in the reader**, which is
  why both demos prefer the test key and make you type `yes` before
  using a real certificate.
