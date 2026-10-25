# Session 36 (Windows, the owner's machine) — predictions, written before each measurement

Each block is committed before the run it predicts, so git dates it. The
least certain prediction of each block is named. Results are not written
here; they are in the decision entry that reads the run.

**Read before any block (2026-10-25 — measurements, not predictions.
Nothing here is carried over from session 35; every line was read today.)**

- **Display scale 100 %.** DPI-aware before any GDI call (D-501's rule):
  `GetDpiForMonitor` (MDT_EFFECTIVE_DPI) 96, `GetDpiForSystem` 96,
  aware screen metrics 1920×1080 against WMI's physical 1920×1080 — **a
  ratio of 1.0, which is what makes the 96 trustworthy.**
  - **`HKCU\Control Panel\Desktop\LogPixels` still reads 192,** stale from
    session 35's 200 % run, with `Win8DpiScaling` 0 and
    `PerMonitorSettings\…\DpiValue` 0. D-502 left this trap written down;
    it is still armed, and it is ignored here.
- **Text size 100 %:** `TextScaleFactor = 100` under
  `HKCU\Software\Microsoft\Accessibility` — present and explicit, where
  D-501 found it absent and D-502 found it 150. The owner put it back.
- **The agent: the tray.** PID **12808**, command line
  `"…\AppData\Local\Programs\Liro Bridge\liro-bridge.exe" tray`, parent
  `explorer.exe`, started **17:42:55**. It owns the listener on
  127.0.0.1:17580, `bridge.json` names 17580 and was written by it at
  17:42:56, and `/v2/health` answers 200. **Not the Start menu's `open`**,
  which D-502 explains is a window with no agent behind it.
  - **It has signed nothing in its life yet.** That is E11's clock: the
    first card operation in this process costs a PIN, later ones do not.
- **Installed:** `67695f6`'s per-user MSI. `--version` gives
  `dev (commit 67695f6, built 2026-10-12T20:06:40Z, go1.26.5, windows/amd64)`.
  Unchanged since session 35.
- **`config.json`** (written 2026-10-24 23:36:35) — **not** what session
  35's baseline recorded. `signatureLevel` is **`""`** and
  `signatureLevelChosen` is **absent**: the machine is on the default,
  "the best available", not on a chosen `b-t`. TSA
  `https://test-tsa.ca.posta.rs/timestamp1`, user and password set.
  `preferPKCS11` absent, i.e. false.
- **The card:** the MUP card, in the reader, with the new certificate —
  issuer `MUP Gradjani CA 4`, valid 2026-09-22 to 2031-09-22, thumbprint
  `EE8EC0F36917D9E5EC4A05A52FE93645FA7671BD`, Non-Repudiation. The owner
  says so and the reader shows one card; the certificate store is not
  evidence of which card is in, because it keeps stale card certificates
  (the expired 2021 pair is still in it). The Pošta card is out.
- **Where the files are:** `%LOCALAPPDATA%\Liro` — `platform.ConfigDir`,
  not `…\Liro Bridge`, which is the installed program's folder and holds
  none of this. My first read of this machine looked in the wrong one.
- **Baselines:** `bridge.log` **2826** lines. The audit chain's last entry
  is sequence **330**, 2026-10-24T21:37:14Z, `approved`, `B-T`,
  `windows-cng`. `cloud-origins.json` **absent**. No window of class
  `LiroBridgeWindow` on screen.
- **The documents:** `Desktop\liro-s36\row-1.3.pdf`, `row-1.5-a.pdf`,
  `row-1.5-b.pdf`, `row-1.6.pdf`, all copies of `testdata/pdfs/blank.pdf`
  (`b9749fdb…`).
- **The clock:** the machine left summer time today. Session 35's log lines
  are `+02:00`; today's are `+01:00`.

## P10 — P9e, Esc on the origin window: three keystrokes at 100 % scale, 100 % text

**What is being finished.** Row 2.1's window half has been measured at
100 % (D-499), at 200 % scale (D-501) and at 150 % text (D-502, D-503).
**Esc was never pressed** — the owner closed with the X both times — so
P9e is unmeasured, and D-503 left it as the next sitting's first job.
D54's *Not measured* list names it: "whether **Esc** refuses when the
window was never the foreground (the window does not take the foreground
when it opens, measured twice)".

**Why 100 % text, and the same 255-character origin.** The keystroke is
the only new variable. Text size changes what the window *shows* and
nothing about where a keystroke goes, and changing it costs a logon
session. The origin is session 35's, `https://` + 244 × `a` + `.rs`, so
these three runs are directly comparable to D-502's and D-503's.
**At 100 % text the name fills its region in six lines (D-499), so there
is no white space in it to click** — run (c) clicks the name text.

**The three runs. My hands send the link and watch; the owner's hands do
one thing each.**

| | the owner does | what I am asking |
|---|---|---|
| **R6a** | **presses Esc, having never touched the window** — no click, no Alt-Tab | Does Esc refuse a window that was never the foreground? |
| **R6b** | **clicks the title bar**, then presses Esc | Does activating the frame, without touching the page, give the page the key? |
| **R6c** | **clicks the server's name** (inside the page, no button there), then presses Esc | Does Esc refuse once the page has the key? |

**What the code says, read for this block:**

- `origin.js:20` — `document.addEventListener("keydown", …)`, and
  `ev.key === "Escape"` → `window.liroSend("cancel")`. **The listener is
  inside WebView2's document.** So Esc works only if the page has the
  keyboard, and for nobody else.
- `askOrigin` (`cloudlink.go`) — `MessageTypeCancel` and `OnClosed` both
  reach `decide(false)`. **Esc and the X are the same answer and the same
  log line**, `cloud: the person did not allow this server`
  (`internal/cloud/channel.go:90`). The 120-second timeout is the one that
  is told apart: it logs `cloud: nobody answered whether a server may send
  requests` (`cloudlink.go:323`) **first**, and then the refusal.
- `showAndFocusWindow` (`win32_windows.go:258`) — every window of ours
  calls `ShowWindow`, then topmost, then **`SetForegroundWindow(hwnd)`**
  and **`SetFocus(hwnd)`**. So the program does ask for the foreground.
  D-502 and D-503 measured that it does not get it: the tray is not the
  foreground process, and Windows refuses foreground to a process that
  does not own it. `SetFocus` is on the **host** window, not on the
  WebView2 child.
- **Nothing in `internal/ui` ever calls `ICoreWebView2Controller::MoveFocus`,
  and `window_windows.go` has no `WM_ACTIVATE` or `WM_SETFOCUS` case and
  no `WM_KEYDOWN` case.** Nothing in our code hands the keyboard to the
  page, and nothing in our code answers a key that reaches the host.
  Whatever R6b does, WebView2 does of its own accord.

| | measured | predicted |
|---|---|---|
| P10a | The window, before any keystroke | Client **420×406** at `GetDpiForWindow` 96; title "Dozvoliti ovaj server?"; `WS_THICKFRAME` absent; topmost; **no scroll bar anywhere**, and the whole 255-character origin in **six lines** — D-499 and D-501's page at this machine's scale. The frame size is not predicted firmly: D-499 recorded 422×438, D-502's 150 % frame implies about 436×445 for this client, and the two do not agree |
| P10b | Whether the window is the foreground when it opens | **No.** Measured twice (D-502 frame 00, D-503 frame 00). The foreground stays with whatever the owner was in |
| P10c | The focus chain while the window sits untouched | `GetForegroundWindow` is **not** the origin window; the focused window of the foreground thread belongs to **another process**. Our window's own thread has focus on the **host** `LiroBridgeWindow`, not on a WebView2 child |
| P10d | **R6a — Esc, never touched** | **Esc does nothing.** The keystroke goes to whatever has the foreground, the page never sees a `keydown`, and the window stays up. It then **waits out the full 120 seconds** and ends as D-500's accidental R5 did: `cloud: nobody answered whether a server may send requests`, then `cloud: the person did not allow this server`. **Nothing is told to the owner, and the server is never contacted.** This is the prediction I am most confident of, and it is the one that matters: the key a person reaches for to say no does not say no |
| P10e | **R6b — title bar, then Esc** | **Least certain of P10.** I predict **Esc still does nothing**, and the window again waits out the 120 seconds. The title bar is non-client area: clicking it activates the frame, and our code moves focus to the host window and never into the page. For the page to get the key, WebView2's controller would have to take focus on activation by itself — which nothing in our code asks for and nothing has measured. **If the window closes on this Esc, I am wrong about WebView2, not about our code,** and the answer is worth more than the prediction |
| P10f | **R6c — the name, then Esc** | **Esc refuses, within a few hundred milliseconds.** A click inside the page gives the WebView2 child the keyboard; `origin.js` fires; `liroSend("cancel")` → `MessageTypeCancel` → `decide(false)`. The log gets **only** `cloud: the person did not allow this server` — **no "nobody answered" line**, which is how the record will tell this from a timeout |
| P10g | What the owner is told, in all three | **Nothing, in all three.** No box, no notification, no line on any window. A refusal by Esc, by the X and by the clock are silent alike (P9f, held three times) |
| P10h | `cloud-origins.json` | **Still absent** after all three. A refusal writes nothing (P9g, held twice) |
| P10i | `open-link`, each run | `open-link: an agent is running, so the link was left for it`, exit 0; the window opens in **12808**, the tray. The agent is resolved from the window at the moment it appears and checked against the listener, never carried over (D-500's correction) |

**What would make this unreadable rather than false:**

- **The owner touching the window before pressing Esc in R6a** — an
  Alt-Tab, a taskbar click, anything that gives it the foreground. Then
  R6a is R6b.
- **The agent going between runs,** as it did in D-500. The owner's
  correction stands: the window's owner is read every time.
- **A keystroke landing in one of my own windows.** I open none while a
  run is live, and the watcher only reads.

**What I will not do:** close any of the three windows myself while the run
is live. R6a and R6b are predicted to end on the clock, and letting them
is the measurement. The watcher stops reading at 135 seconds; if a window
is somehow still up then, I will say so before touching it.
