# F12 Linux — session 22: dev.15, after Fedora

**What this is:** the handover at the end of the Fedora sittings of
2026-10-03 (D-417, D-420), for whoever builds dev.15 on the Ubuntu VM.
Everything here is from the record and cited. Nothing in it is to be taken
as anyone's recollection of those sittings. **Written:** 2026-10-03.
**Working rules:** session 13 §C, session 15 §E, session 16 §E, session 17
§F, session 18 §D, session 19 §E, session 20 §E, session 21 §F, and §E
below. D-304's five questions before believing any check.

---

## A. Where things stand

**Fedora is done for dev.14** (the owner, D-420). The Pošta card signs on
Fedora 44 through SafeSign 4.6.0.0 and a protocol request — the PIN in this
program's own field, no SafeSign window — and `pdfsig` and OpenSSL `cms`
both find the signature valid, their controls failing. Two checks were
**not** done and are open in F1: the chain to Pošta Srbije CA Root, and
`scripts/verifypdf`'s Trusted List check (D-420 says why each was not, and
why anchoring at the signature's own CA 1 is not offered in their place).

**The Ubuntu VM is done for dev.14** (D-414–D-416).

**Two defects found on Fedora matter more than the rest**, and both are in
dev.15:

- **D32 — the white window.** After each login the first window paints, and
  every window after it is white — so a person who signs twice in a session
  sees the second one blank. Measured on Fedora; on Ubuntu nine windows
  under one agent all painted (D-397). Seven windows, three agents; both
  halves predicted and held on one agent (D-420).
- **D33 — the Završi wait, on every platform.** The window says Završeno
  and the caller gets nothing until a person presses Završi: 11 min 58 s on
  Fedora. By construction — `runProtocolFlow` gives the API its result only
  when the window closes — and nothing times out after approval. The demo
  walkthrough and `docs/PROTOCOL.md` §6.3 describe behaviour the program
  does not have (D-420).

## B. dev.15: six things, D32 first (the owner, D-420)

1. **D32, the white window.** See §C.
2. **D33, the Završi wait.** The owner decides first when a protocol batch's
   result is given; then code; then the walkthrough's step 5 → 6 and
   PROTOCOL.md §6.3 corrected; then the caller's file arriving with the
   report still on screen, watched.
3. **D31, the Settings button in the main window**, on every platform (D-419).
   The design is shown to the owner before it is built.
4. **D29, the document size removed** everywhere a person sees it (D-415's
   sweep lists the sites).
5. **D28, the drop outline** — cleared by the window's own drop handling,
   whatever WebKit does.
6. **D27, D-407's first start** — the owner's hands on option 1's premise
   first.

D-419's "four things and only these" is superseded by this list. Nothing
else goes into dev.15.

**Linux is not done** until a person on Fedora can reach Settings and export
the audit log (D-419), so dev.15 goes to the Fedora VM.

## C. D32: where to start

**Its cause is unknown.** What is known (D-420):

- The page is built behind the white view: AT-SPI showed the full page —
  the certificate, Otkaži, Odobri, a running countdown — in windows the
  owner saw white. **AT-SPI's `SHOWING` does not track the screen**; the only
  evidence of white or painted is a person's eyes and a screenshot.
- A request window and a launch from Activities were both white, so it is
  not the request's first step.
- The network process outlives each window and is reused; each window gets
  a new web process.
- **The one-time setup happens at the same moment as the window that
  paints**: at a process's first window, and only then, the Settings portal
  refuses the process (`Unable to open /proc/PID/root`, B29, D-376's flag)
  and GDK sets up Vulkan on llvmpipe. **That coincidence is why "the
  agent's first window" and "the process's first-time setup" cannot be told
  apart yet. Separating them is where to start.**
- Fedora 44: GTK 4.22.5, WebKitGTK 2.54.0. Ubuntu: GTK 4.14.5. Which
  variable matters is not read.

*Close* (open-items D32): the cause found, a fix, then on Fedora a pairing
followed by three requests under one agent, each painted, watched; then the
same on Ubuntu.

## D. Left on the Fedora VM

- dev.14 and SafeSign 4.6.0.0-AET.000 installed (dnf transaction 7).
- Agent 12473 running, with its network process, two leftover chains (D23)
  and two `pkcs11-worker`s.
- **The pairing `2afcb9c236e41d7443e572a7c077450d` left deliberately** (the
  owner), in the Login keyring and `~/.config/liro/pairings.json`. **Its
  secret was shredded** with the logs that held it, so no client can use
  it: **dev.15 pairs afresh** — which is D32's first window, and part of its
  test.
- `~/s22-card/ugovor-signed-1.pdf`, the verified file; `~/s21-predictions.md`
  and `~/s22-predictions.md`, the owner's, with every prediction and reading.
- F10 — the VM left as found — waits for the end of Fedora's work, which
  dev.15 is now part of.

## E. Rules added in sessions 22–24

- **Say "ready to send" and stop; send only on the owner's "go".** The
  request of 15:23 was sent in the same turn as saying it would be, with
  nobody watching, and spent an agent's first window on nothing (D-420).
- **When the result goes to a program, read what the program got, not what
  the window says.** The window said Završeno for twelve minutes while the
  caller had nothing.
- **An accessibility state is not a reading of the screen** (D-304 Q2).
- **A scratchpad can hold a named pipe**; a glob read over one blocks. Name
  the files.
