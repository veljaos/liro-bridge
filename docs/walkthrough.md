# The stranger's walkthrough

F10's exit condition is that a stranger installs Liro Bridge and signs a
document. Neither the person who built it nor the owner can be that
stranger. This document is the part that can be prepared in advance:
exactly what that person will do, from the download link to a signed
PDF, and — at each point where it can go wrong — what they will see and
what they should do about it.

It is written for whoever runs that test: the owner watching over a
shoulder, or the stranger reading it alone. `docs/guide/Uputstvo.html`
is the document that ships to users; **this one is for the test**, and
it says things a user guide would not — what is known to work, what has
never been tried, and what to write down.

---

## Who the stranger should be

Not a developer. The whole point is somebody who has an invoice to sign.

The three things they must genuinely have, because the test is
meaningless without them:

- **Their own qualified certificate on their own card**, from MUP, Pošta
  Srbije or Halcom. Not the owner's card.
- **A Windows machine that is theirs**, not this development machine.
  Everything in `docs/decisions.md` D-243 was measured on a machine that
  has had this program on it for a month; a machine that has not is a
  different test and the one that matters.
- **No Go toolchain**, and ideally no administrator rights. F10's rules
  name both.

They should be told one thing before they start and nothing else: *here
is a link; install it and sign a document; tell me everything that
confused you.*

---

## What to have ready before they start

- The release page URL, with the `.msi` on it.
- Somewhere to write down what happened. The interesting output of this
  test is not "did it work" but **where they hesitated**.
- Their own PDF to sign. Not a fixture. A real invoice or contract of
  theirs, because file names, page sizes and foreign producers are all
  things this program has been wrong about before.

---

## Step 0 — before the download

**What they do.** Nothing yet.

**What can go wrong.** Their card middleware may not be installed. This
is the single likeliest reason the whole thing fails, and it fails
*late* — at the certificate step, after they have already got past
SmartScreen and the installer, which is the worst possible place to
discover it.

| Issuer | What must already be there |
|---|---|
| MUP | a reader + **TrustEdgeID** (NetSeT) |
| Pošta Srbije | a reader + **TrustEdgeID** (NetSeT) |
| Halcom | a reader or USB token + **Nexus Personal** |

**Worth doing deliberately:** ask them, before anything, whether they
have ever used this card on this machine for anything — signing a PDF
elsewhere, logging into a portal. If the answer is no, expect the
middleware to be missing and treat that as a finding about the guide
rather than about the program.

---

## Step 1 — the download

**What they do.** Open the release page, download the file ending in
`-x64.msi`.

**What they see.** A release page with several files on it: two MSIs and
a plain EXE, plus `release.json` and `release.json.sig`.

**What can go wrong.**

- **They pick the wrong file.** There are three. `-per-machine.msi` will
  demand administrator rights they may not have; the `.exe` runs without
  installing anything, which works but gives them no Start menu entry
  and no right-click menu. If they hesitate here, the release notes are
  not clear enough — write down which one they reached for first.
- **Their browser blocks it.** Chrome and Edge warn about unsigned
  executables and sometimes about MSIs. They may need "Keep" in the
  download bar. This is the first of three separate security warnings
  they will meet, and the guide names only two.

---

## Step 2 — the warning

**What they do.** Double-click the downloaded file.

**What they see.** One of two dialogs, depending on how they got it:

- **"Open File - Security Warning"**, with `Publisher: Unknown
  Publisher`, and **Run** / **Cancel**.
- **"Windows protected your PC"** — a blue window with only **Don't
  run** visible. **More info** has to be clicked before **Run anyway**
  appears at all.

Both are expected: there is no code-signing certificate (SPEC §15.1).

**What can go wrong.**

- **They stop here, and they are right to.** A person who cancels
  because "Unknown Publisher" alarmed them has behaved correctly. That
  is not a failed test — it is the most valuable single result this
  whole exercise can produce, because it says the guide's reassurance
  did not reach them before the dialog did. Write down whether they had
  read anything first.
- **They cannot find "Run anyway".** It is behind **More info** and
  invisible until then. This is the step the guide has a screenshot for.
- **Antivirus quarantines it** rather than warning. Then the file simply
  disappears and nothing explains why. The guide's §4 covers it,
  including checking the SHA-256 against the release page first.

**Note for the observer:** the guide's screenshot of this dialog is from
an **English** Windows. If the stranger's Windows is Serbian, the dialog
is in Serbian and will not match the picture. The text names every
button in both languages for that reason — check whether that was
enough.

---

## Step 3 — the install

**What they do.** Let it run. It takes about a second.

**What they see.** A brief progress bar, then nothing. The agent starts
itself and its icon appears near the clock.

**What can go wrong.**

- **The install refuses with a message about WebView2.** The machine
  does not have the Microsoft Edge WebView2 Runtime. Windows 11 has it;
  a Windows 10 machine kept off the internet by policy may not. The
  installer refuses deliberately rather than installing a program that
  cannot draw a single window — but the person then has to install
  something else from Microsoft, and may not be allowed to.
  **This path has never been exercised on a machine that genuinely
  lacks the runtime.** See "What is still unmeasured" below.
- **Nothing appears to happen.** The agent is a tray icon, not a window.
  If they are watching the middle of the screen they will think it
  failed. Whether they find the Start menu entry unprompted is worth
  knowing.
- **They were sent the per-machine MSI by mistake** and have no
  administrator rights: they get a UAC prompt they cannot answer.

---

## Step 4 — the first signature

**What they do.** Start **Liro Bridge** from the Start menu, drag their
PDF onto the window, press **Dalje**, pick their certificate, press
**Odobri**, enter the PIN, choose a signing method, press **Potpiši**.

**What they see, in order:** the document list → the certificate list →
the method screen → the PIN prompt (Windows' own) → a progress screen
→ the report.

**What can go wrong, in the order they will meet it:**

| What the screen says | What it means | What they should do |
|---|---|---|
| "Nije pronađen nijedan čitač kartica" | no reader | plug it in; try another USB port |
| "Ubacite karticu u čitač" | reader, no card | seat the card properly |
| "Windows servis za pametne kartice nije pokrenut" | the Smart Card service is stopped | restart the machine |
| "Na ovoj kartici nije pronađen sertifikat za potpisivanje" | no signing certificate visible | the middleware from step 0 is missing |
| the list shows their name **twice** | it should not — only signing certificates are listed (D-149) | a finding; write it down |
| the PIN is refused | **stop** | three wrong entries block the card; a national ID card then needs a visit to a MUP counter |

**The one to watch for.** After they press **Potpiši**, the first
signature takes about five seconds while the card initialises. The
screen says "Priprema kartice…" for exactly this reason. If they think
it has hung, the wording is not doing its job.

**And afterwards:** ask where they expect the signed file to be. It is
beside the original with `-signed` added. If they look in Downloads, or
in a Documents folder, that is a finding about the report screen.

---

## Step 5 — did it actually work?

The stranger cannot answer this and should not be asked to. The observer
checks:

- The file exists, beside the original, with `-signed` in the name, and
  the **original is unchanged**.
- Opening it in Adobe Reader shows a signature bar.
  **Expect "identity unknown" for a MUP certificate** — Adobe's trust
  list does not include MUP's CA. That is documented, is not a defect,
  and cannot be fixed in code (SPEC §16.7).
- `go run ./scripts/verifypdf <file>` says every check passed. This is
  the project's own independent verifier (SPEC §16.4) and it is the
  answer that actually settles it.
- The audit log has an entry: tray → **Prikaži dnevnik revizije**.

---

## What is still unmeasured, and must not be reported as if it were

Stated here rather than left to be discovered, because a walkthrough
that implies everything has been tried is worse than none.

**A machine with no WebView2 runtime.** The installer's launch condition
and the agent's own run-time check are both written and both have code
paths that have been read. Neither has been run on a machine that
genuinely lacks the runtime. Pointing
`WEBVIEW2_BROWSER_EXECUTABLE_FOLDER` at an empty directory reproduces
the *agent's* side of it, and that is how the run-time message was
found; it does **not** exercise the installer's `RegistrySearch`, which
reads three registry locations that exist on this machine. A machine
without the runtime is being arranged. Until it exists:

- the agent's behaviour with the runtime absent is **measured**
- the installer's refusal is **not**

**A machine that has never had a Go toolchain.** Everything in D-243 was
measured on the development machine. F10's rules ask for a machine that
has never had one, and this is the stranger's own machine — which is
precisely why the stranger's test is the exit condition and this
document is not.

**A reader with no driver, and a card the middleware does not see.**
Named in F10's exit condition as failure points to describe. Both are
described above from the code's own error paths; neither has been
produced on real hardware, because doing so means uninstalling a working
middleware from a working machine.

**The SmartScreen screen itself.** The zone warning is photographed. The
blue "Windows protected your PC" screen is described but not
photographed: it is driven by a reputation lookup on a file somebody
actually downloaded, and a locally built binary with a hand-written
Mark-of-the-Web does not produce it. It needs a real published release,
fetched through a browser.

---

## What to write down

In order of how much it is worth:

1. **Every point where they hesitated**, even for two seconds, and what
   they were looking at.
2. **Anything they clicked that they did not mean to click.**
3. Whether they found the signed file without being told where it was.
4. Whether they read anything before double-clicking, and if so what.
5. Whether it worked.

The last one is the least interesting. A stranger who succeeds having
been confused four times has found four defects; one who fails at a
point the guide covers has found a defect in the guide, which is the
same thing.
