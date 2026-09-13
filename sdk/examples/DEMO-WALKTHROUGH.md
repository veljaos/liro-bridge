# Watching the protocol path

Two runnable demonstrations of the path an ERP actually uses, meant to be
*watched* rather than copied. Read the demo you are about to run before
you run it, so that a screen which does not appear is noticed as an
absence rather than assumed to be something you missed.

- [`demo-a-application-decides.mjs`](demo-a-application-decides.mjs) — the application answers every question.
- [`demo-b-person-places-stamp.mjs`](demo-b-person-places-stamp.mjs) — the same request with `stamp` removed, so you answer it.

The two files are identical apart from that one option. Everything else
they share lives in
[`demo-protocol-common.mjs`](demo-protocol-common.mjs).

---

## Before either demo: which agent is running

**This matters more than anything else on this page.** On this machine
there are two different agents:

| Binary | Soft token? | What a signature costs |
|---|---|---|
| `%LOCALAPPDATA%\Programs\Liro Bridge\liro-bridge.exe` (the installed 0.9.1) | **No** | A **real** signature with your MUP card, and a real PIN prompt |
| `.\liro-bridge.exe` (repo root, built with `-tags softtoken`) | **Yes** | A test signature, no card, no PIN |

The installed one is a release build, and a release build does not
contain the soft token at all (SPEC §16.6). If it is the one running,
both demos will find only your real card and will ask you to sign with
it for real. The demos refuse to do that silently — they stop and make
you type `yes` first — but the simplest thing is to run the right agent.

There is also a **real MUP card in your reader right now**
(`ВЕЉКО СТАНОЈЕВИЋ`, `…B3D1ECCE`), which is why this is worth spelling
out.

### Starting the soft-token agent

Stop the installed agent first — its tray icon → **Izađi** — because two
agents cannot both own the discovery file.

```sh
set LIRO_SOFTTOKEN_P12=.\testdata\softtoken\local\test.p12
set LIRO_SOFTTOKEN_PASSWORD=liro-softtoken-test
.\liro-bridge.exe tray
```

`testdata\softtoken\local\test.p12` already exists on this machine. If it
ever goes missing, `go run ./scripts/gentestkeys ./testdata/softtoken/local`
writes it again.

You can confirm the soft token is visible before starting anything:

```sh
.\liro-bridge.exe certs
```

The third entry should be `Liro Bridge Soft Token … (TEST KLJUČ) ✓ upotrebljiv`.

**So: no card needed.** Put your card in only if you want to watch the
real thing, in which case pass `LIRO_THUMBPRINT=…B3D1ECCE` — or just say
`yes` at the prompt.

---

## Demo A — the application decides everything

```sh
node sdk/examples/demo-a-application-decides.mjs
```

The request names the documents, the certificate thumbprint, the level
(`b-b`) and the stamp (`{ visible: true, position: 'bottom-right' }`).
Three documents, all of them this project's blank A4 page under three
names, so the stamp is the only thing on the page and where it lands is
unmistakable. Pass your own paths as arguments to use real documents.

### Screen by screen

**1 — Terminal.** The certificate listing, and which one the application
picked. It prefers the soft token whenever one is present.

**2 — Pairing window.** Title **Zahtev za povezivanje**, always on top.

- **Zahtev**: `Liro Business (demo A)`
- **Poreklo**: `https://erp.example.rs`, verbatim — the demo declares
  this, nothing verifies it, and showing it unaltered is the whole point
  of the field.
- **Kod**: six digits.
- One button: **Odbij**. There is no Allow, by design — the code is the
  approval, and it travels through you.

Type the six digits into the terminal. The window becomes **Povezano**
with a tick; press **Zatvori**.

**3 — The approval.** Title bar reads **Liro Bridge**.

- **No step header and no dots.** With the method question already
  answered there is only one step, and a header saying "1 of 1" would be
  noise on the one screen that must be nothing but the approval.
- `3 dokument(a) za potpisivanje`
- **Exactly one certificate row**, because the request named a
  thumbprint. The agent narrows the list rather than choosing for you.
- **You must click that row.** `Odobri` stays disabled until you do —
  there is no auto-selection even with a single row. This is deliberate
  (SPEC §6.5, §18.15): the person chooses the certificate and presses
  the button, and naming a thumbprint only removes the wrong answers.
- **Aplikacija**: `Liro Business (demo A)`
- **Detalji** hides the batch fingerprint (with a **Kopiraj** button) and
  the file list — `Ugovor-001.pdf`, `-002`, `-003`.
- Buttons: **Otkaži**, **Odobri**.

You have 120 seconds. A countdown line appears only for the **last 30**,
so if you answer normally you never see a clock.

**4 — Progress.** `Priprema kartice...`, then `Potpisivanje 1 od 3` with
a bar, an ETA and a **Zaustavi** button. Against the soft token this is
close to instantaneous and you may see it only as a flash — that is the
demo being fast, not a step being skipped. A real card spends about 4 s
(MUP) or 12.7 s (Pošta) on the first signature alone.

**5 — Report.** `Završeno`, `3 potpisano`.

- **Sačuvano u**: `vraćeno aplikaciji koja je tražila` — a protocol batch
  writes nothing to disk, so there is no folder.
- **Otvori folder** is therefore **disabled** (greyed, not hidden).
- **Nivo potpisa**: `B-B`, with the warning line `Nivo B-B — bez
  vremenskog žiga` under it.
- Buttons: **Sačuvaj izveštaj...**, **Otvori folder** (disabled),
  **Potpiši još dokumenata**, **Završi**.

**6 — Terminal.** Three signed PDFs written under
`liro-demo\out\demo-a\`, with their sizes and the level each actually
reached. Open one: the stamp is in the **bottom-right** corner, because
the application said so.

### What you will NOT see in demo A, and why

Every one of these is an absence to notice, not a step you missed:

| Missing | Why |
|---|---|
| The **documents** step (drop zone, file list, Izaberi) | The application supplied the documents. |
| The **signing-method** step | The application supplied `stamp`. |
| The **step header** entirely | One step is not a sequence. |
| The **timestamp** question (`Bez vremenskog žiga`) | Only asked when the level needs a timestamp and no authority answers. `b-b` needs none. |
| **Fajl već postoji** | A protocol batch writes no files, so there is nothing to overwrite. |
| **Neki od ovih su već potpisani** | Same reason — the suffix check is about files on disk. |
| A **Windows PIN dialog** | The soft token has no PIN. With a real card you would get one here, owned by Windows, not by this program. |

---

## Demo B — you answer the stamp question

```sh
node sdk/examples/demo-b-person-places-stamp.mjs
```

The same request with `stamp` left out, and one document instead of
three. Nothing else differs.

### Screen by screen

**1, 2 — Terminal and pairing** as in demo A, except the application is
`Liro Business (demo B)` and it pairs separately, under its own stored
pairing. You will see the pairing window a second time. Afterwards both
demos appear as separate rows under **Povezane aplikacije** in Settings.

**3 — The approval**, with two differences:

- A **step header with two dots** at the top. You are on step 1 of 2.
- The primary button still says **Odobri**, not **Dalje** — the
  certificate step's label is not swapped out when another step follows
  it, unlike the method step's. Worth noticing; it is arguably the one
  screen where the label and the step count disagree.

**4 — Metod potpisivanja.** Step 2 of 2, with **Nazad** now live in the
header.

Three options, and on this machine the second is preselected with
**Gore desno** highlighted, because that is your saved standing
preference (`config.json` currently holds `visibleStamp: true`,
`stampPosition: "top-right"`):

- `Potpiši birajući poziciju potpisa`
- `Potpiši sa definisanim pozicijama` ← preselected, with the four
  corners under it as a 2×2 grid laid out as they sit on a page
- `Potpiši bez vizuelnog prikaza`

The page, reference-line and identity-document fields are **not** here —
those belong to the Settings role of this same window, not to a step of
signing. The primary button says **Potpiši**.

**5 — The thing actually worth watching.** Choose the **first** option,
`Potpiši birajući poziciju potpisa`, and press **Potpiši**.

**The placement picker will not open.** The screen snaps back to
`Potpiši sa definisanim pozicijama` and a warning line appears:

> Ovaj dokument ne može da se prikaže, pa se pečat postavlja po uglu.

This is deliberate, not a bug. The picker draws the page it is placing a
stamp on, and a protocol batch's documents arrived over a socket and are
nowhere on disk — there is no page to open
(`signflow_windows.go`, `signAtAChosenPosition`). The alternative the
code explicitly rejects is opening a file chooser and letting you place
the stamp by looking at some *other* document.

**So, stated plainly: over the protocol you can choose the method, but
you cannot place a stamp by eye.** Placing by eye is something only a
local batch — a drag, or the Explorer verb — can do. If you expected to
place it yourself in this demo, this is the absence to notice.

(The warning's wording is worth a line in the text pass: it says the
document "cannot be displayed", when the real reason is that it is not a
file at all.)

**6 — Then pick a corner** and press **Potpiši**. Progress and report
follow exactly as in demo A, with one document, and the signed PDF lands
under `liro-demo\out\demo-b\` with the stamp in the corner you chose.

### One side effect to know about

Choosing anything on the method screen writes your choice to
`config.json` — `saveConfig()` is called there and is not guarded for
protocol runs. So if you pick a corner other than top-right in this
demo, **that becomes your standing default** for local signing too.
Settings → **Vidljivi pečat…** puts it back.

Nothing else in either demo writes to your configuration.

---

## Running them again

The pairing is stored, so a second run goes straight to the approval and
you will not see the pairing window. To see it again:

```sh
node sdk/examples/demo-a-application-decides.mjs --fresh
```

That deletes this demo's stored pairing only. The row it leaves behind
in Settings → **Povezane aplikacije** can be removed there with
**Prekini vezu**.

## Everything the demos write

All under `liro-demo\` in the working directory, and nothing else:

- `liro-demo\pairing-a.json`, `liro-demo\pairing-b.json` — the device
  secrets. Fine for a demo, **wrong for a real integration**: see the
  SDK's `SecretStore`.
- `liro-demo\out\demo-a\`, `liro-demo\out\demo-b\` — the signed PDFs.

Delete the directory when you are done. It is untracked.

## Environment

| Variable | Effect |
|---|---|
| `LIRO_THUMBPRINT` | Sign with this certificate instead of the auto-chosen one. |
| `LIRO_REAL_CARD=1` | Skip the "this is a real certificate" confirmation. |
| `LIRO_LEVEL` | `b-b` (default), `b-t`, `b-lt`. Anything but `b-b` with no timestamp authority configured adds a screen — see demo A's table. |
| `LIRO_ORIGIN` | The origin shown on the pairing window. Default `https://erp.example.rs`. |
| `LIRO_DEMO_DIR` | Where the pairings and output go. Default `.\liro-demo`. |
