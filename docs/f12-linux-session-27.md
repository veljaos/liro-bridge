# F12 Linux — session 27: D36's revision named on the Ubuntu VM, and a proposed sort of open-items

**What this is:** the handover after one sitting on the Ubuntu VM (D-431,
D-432). **Written:** 2026-10-05. **Working rules:** session 13 §C, session 15
§E, session 22 §E, session 23 §E, session 24 §E, session 25 §E. D-304's five
questions before believing any check.

**Ruled on 2026-10-07 (D-433).** The owner accepted §D as written, with §C's
rulings, and set an order above it; the order is now the head of
`docs/open-items.md`, and that file, not §D, is the list. §D's "deferred past
v1" bucket is dissolved into the order — all v1 work but other issuers' cards.
Four of §D's closures were not applied: A5 and A13 (the order names them), B8
(the owner's choice, not yet made), A14 (D-340 answers it for Linux only).
§E's order is superseded: package signing comes first, A2/E2 is block 6, and
D23's date is re-proposed in open-items D23. §D below is kept as written, as
the proposal that was ruled on.

---

## A. Where the Ubuntu VM is left

| | |
|---|---|
| boot | 2026-10-05 20:59:28 (`uptime -s`) |
| package | `liro-bridge 0.9.9~dev.17`; GTK `4.14.5+ds-0ubuntu0.10`, WebKitGTK `2.52.6-0ubuntu0.24.04.1` |
| agent | 2637 `tray`, autostarted; not touched this sitting, no window opened |
| files | `~/s27-predictions.md` (R0–R7, each written before its fetch, and the readings); the WebKit files and the 2.54.0 tarball were in the session's scratchpad and are not kept — their sha256 are in D-431 |

## B. What this sitting showed (D-431)

The WebKit report's source reading is at **`webkitgtk-2.54.0`, commit
`5220e80b97a253c60ed899361654142ab5021998`**, read through two routes that
agree byte for byte (GitHub at the commit; webkitgtk.org's tarball at its
published sha256). The four functions are the same at **2.54.1** and on
**`main` at `3fc0c58a`** (2026-10-05), so the report is current and nothing
is fixed upstream. R5 failed as written (`WebKitWebViewBase.cpp` changed in
2.54.1 — two unrelated hunks, read). The draft was corrected once: with no
value held, `fontDPI()` falls back to the primary screen's DPI, 96 only
without screen data. **The report is ready; the owner files it.**

## C. The owner's rulings tonight (D-432)

1. **B8 is not solved.** "Solved" rested on my inference from two readings
   taken for other purposes (SELinux `Enforcing` read in an earlier entry;
   `bwrap` chains counted on Fedora for D23), not a measurement aimed at B8.
   **Either left open, or closed by decision with exactly that wording** — the
   owner's choice, not made tonight.
2. **B25 is not closed.** D-094 rules out measuring it, and that stands. But
   the question is whether the accessibility bus can press Odobri, and if it
   can, any program running as the person can approve a signature without
   them. **A source reading answers it, and a reading is not synthetic
   input.** v1 work. See §E.2 for the route.
3. **B29 is not deferred.** The portal refuses our process, so the window gets
   no theme and no fonts from the desktop. It reads as cosmetic only because
   the DPI consequence was the one fixed — and that consequence was invisible
   until it broke every window after the first. v1 work.
4. **A2/E2 goes first tomorrow.**
5. **D23 gets a date**, not an open "v1 work" line (the owner: "a person
   signing fifty documents leaves fifty processes behind"). **The date is not
   set yet** — §E.3 proposes one. One precision for when it is written up:
   what has been counted is **one chain per window**, on both VMs; fifty
   documents in one batch is one window, fifty requests is fifty windows
   (about 150 processes, about 43 MB PSS at D-397's 0.86 MB each). A
   multi-document window has not been counted separately.

## D. PROPOSED sort of open-items — not ruled on

86 items open as of D-431 (48 more are already closed, italic with
"Numbering kept"). Proposed into four buckets, because three did not fit
honestly: 30 items are none of solved, closed or deferred — they are v1
work. Tonight's rulings (§C) are applied; everything else is my proposal
and my reason.

Counts: **solved 2 · closed by decision 28 · deferred past v1 26 · v1 work 30.**

### D.1 Solved (proposed)

| item | reason |
|---|---|
| D29 | Document size removed everywhere a person sees it, built in dev.15 and watched on Fedora (D-423). The Guide's screenshots move into E8, which rewrites the Guide anyway. |
| E7 | The audit entry records the backend, the module and whether the module was the person's own (`signerOrigin`, D-313; `keysources.go:28`). "Why this backend" follows from the fixed source order; that half closes with A6. |

### D.2 Closed by decision (proposed — each needs the owner's ruling)

| item | proposed reason |
|---|---|
| A5 | The 5 s shutdown grace is accepted; no hang has ever been seen. A per-request deadline reopens with B9 (a module that hangs). |
| A9 | `auditModule` keeps a configured module's file name and drops its directories (`keysources.go:58`). A personal name in a file name is one the person gave it; accepted. |
| A10 | Document re-pairing; no migration. |
| A11 | gotk4's `checkptr` failure is upstream's; the race job stays a probe. |
| A13 | No hang observed on Windows; reopens on one. |
| A14 | Write down what D-340 implied as the decision (D-340 not re-read tonight). |
| A19 | Accepted; the Guide (E8) says how to forget pairings before uninstalling. |
| A26 | An upstream courtesy, not v1: the shipped field has not been a `GtkPasswordEntry` since D-385. Filed or dropped by the owner. |
| B3 | Nothing rests on it since D-369. |
| B4 | Conceded in SPEC §6.5.1 the way A3 was (would need its own sentence). |
| B8 | **Owner's choice (§C.1)**: closed with exactly this wording — "my inference from two readings taken for other purposes, not a measurement aimed at B8" — or left open. |
| B10 | Harmless and perhaps unreachable. |
| B17 | Does not affect correctness. |
| B20 | Nothing to check until new UI is written. |
| B21 | CI fuzz flake rate: not a v1 question. |
| B23 | Already the owner's decision (not to instrument Mutter). |
| B24 | The reasoned bound (WinEvents carry no text; `get_accValue` measured refused). |
| B28 | Accepted as not performable on Wayland. |
| B30 | Harmless; the drop delivers. |
| C2 | N = 9, no web process survived its window; what grows is D23. |
| C8 | Unit-tested; no expired card in hand. |
| C18 | Unit-tested; no clean machine needed for v1. |
| C20 | The log is enough. |
| D14 | The document is a record; D-354 holds the corrections. |
| D18 | Accepted as unexplained; dev.17's trigger logging is in place; reopens on a recurrence. |
| D19 | The handler is never connected; the test reads GTK's warning instead. |
| D27 | Closed with the sentence's own path never produced: it is a failure branch not reached on two desktops. |
| D34 | As D18. |

### D.3 Deferred past v1 (proposed — what would reopen each)

| item | reopens with |
|---|---|
| A8 | After D4: once a producer exists for every code that should have one. |
| A18 | A SPEC §6.7 extension; not a v1 promise. |
| A21 | Windows CLI parity; low. |
| A29 + C5 | The window tests, when next needed; until then each release's windows are watched by a person. |
| B5 | v1 is GNOME on Ubuntu and Fedora; another compositor. |
| B6 + F5 | A real GPU; the F12 checklist amended to say so. |
| B9, B11, B13, C9, F4 | Hardware fault cases: a faulting module or card. |
| B12 | Performance; a NetSeT card. |
| B16 | Another machine (no load generators on this VM). |
| B19 | Linger; a machine where a logout can be shown to be one. |
| C6 | A one-PIN-per-signature card. |
| C10 | Code, low. |
| C15 | Halcom hardware; the Guide says untried. |
| D9 | Releases are built by CI from a tag, never from a dirty tree. |
| D11, D12, D13, D16, D17 | Code or Windows, low; none reaches a person. |
| E3 | The owner's decision on third-party code against a national ID; the Guide says MUP is not supported on Linux. |

### D.4 v1 work — none of the three

**Owner's hands:** D36 (file the WebKit report); A16 then C16/F9 (the
`package-signing` environment, then the first tag); A15 (where the
fingerprint is published); F1 (Pošta's root, and `verifypdf` on the Ubuntu
VM — `ugovor-signed-1.pdf` has to be brought from Fedora).

**A decision, then a SPEC edit:** **A2/E2 first tomorrow (§C.4)** — SPEC
§12.6 makes B-LT the default and only B-B is reached; A6 (state the source
order); A12 (a protocol run should not save the corner as the default);
D24 (clause 2's amendment); E9 (build the X11 work, or amend SPEC §6.5.2).

**A reading:** **B25 (§C.2, §E.2)**.

**Code:** D3, D4 (the owner's two, above the rest); D6 (example clients
teach only Windows discovery); **D23 (dated — §C.5, §E.3)**; D30 (the false
comment at `traysignal_other.go:15`, and the owner's call on whether a logout
must run Quit's path); C11 (`signflow.go` has only
`signflow_windows_test.go`, checked tonight); C13 (`Sign.java`, `sign.php`:
run them or remove them); **B29 (§C.3)** — what the window does not get on
Fedora, listed and then dealt with.

**A Windows machine:** A7 (the icon test, status unread since D-320); C1;
C7; D10; D15 with B14; D31's render (its Tab order deferred); D33's Windows
half (Linux done).

**Documents:** E8 (the Guide's Linux section, with D29's screenshots and
A19's sentence); the F12 checklist boxes against D-354 and D-355.

## E. Next

1. **A2/E2** — B-LT on Serbian cards, the owner's decision first.
2. **B25, a source reading — route proposed, not started.** Odobri is a
   button in the web page, not a GTK widget, so the path to read is mostly
   **WebKit's** AT-SPI, with GTK's part being the socket that embeds the web
   process's tree. That is my understanding, to be confirmed by the reading
   itself:
   - WebKit (2.54.0 and 2.52.6): `Source/WebCore/accessibility/atspi/` — the
     Action interface's `DoAction` on a button, and what it calls
     (`AccessibilityObject::press`, read at 2.52.6 in D-425); how the web
     process registers on the accessibility bus.
   - WebKit's sandbox: `BubblewrapLauncher.cpp` — the `xdg-dbus-proxy` policy
     for the accessibility bus; then `xdg-dbus-proxy`'s own source, for
     whether a method call *into* the sandboxed web process from another
     client passes.
   - GTK 4.14.5 and 4.22.5: `gtk/a11y/gtkatspisocket.c` and the context
     code — the embedding; and GTK's own button Action for the window's ×.
   - Ours, read tonight: **Odobri takes any click** (`consent.js:162`; no
     `liroClickFacts`, so an accessibility press would be neither recorded
     as one nor refused). On Linux a PIN screen in our own page follows
     Odobri for a PKCS#11 card; whether that screen can itself be driven
     over the bus is part of the same question.
   About 30–45 minutes of fetching and reading, no runs. Whether it then
   warrants a measurement is D-094's question again.
3. **D23's date — proposed, not set**: begun 2026-10-07, after A2/E2;
   counted on both VMs (a fix measured on one cannot be told from the
   distribution, D-404) by 2026-10-09. The owner sets it.
4. **The owner rules on §D**; then open-items is edited to match, with its
   own entry, in one commit.

## F. Rules added in session 27

None. One correction of mine, recorded in D-432: I proposed B8 "solved" on
an inference — the shape D-416 corrected in D-415.
