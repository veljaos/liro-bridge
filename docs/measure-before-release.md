# What must be measured before any of this reaches a person

The owner's worklist, from `docs/s33-what-ran.md` (D-472). Work it **top to bottom**: each group is ordered by what a person would meet first. Each row says what is claimed and **what would show it false**, so a run that does not look for that has not measured it.

Group 0 is mine and needs no hardware; it comes first because the rest are read through tests, and a test that cannot fail measures nothing. Everything not on this page can wait behind it.

## 0. CI — mine, before yours — **done (D-473)**

Each test was seen red on the code before its fix, then green after it. For the Windows rows that was on CI's runner (P9: run 38242378820 on the measurement commit `b5b3151`, run 38243584334 on the fix `584e019`); for the rest, in the container.

| # | claimed | what shows it false now | measured |
|---|---|---|---|
| 0.1 | Navigate re-registers drop targets on the window's own thread (C21) | The test reads the thread each target was registered on | Red on the old code: `5 of 5 … on another thread ([7088 …]), not the window's own (8056)`. green after (run 38243584334) |
| 0.2 | After `FreeConsole`, standard error goes to NUL, not into the next file opened (C21) | The child reads standard error before opening anything: it must be NUL, and `os.Stderr` must follow it | Red on the old code: `stderrAfterDetach=invalid`, and this time the marker landed in the file (D44 reproduced). green after (run 38243584334) |
| 0.3 | A failed `add_*` releases its handler; `WM_QUIT` ends the PIN dialog as a cancel; a failed build gives its fonts back (C21) | A stand-in `ICoreWebView2` failing every call; `WM_QUIT` posted to the dialog's thread; GDI count over ten failed builds | Red on the old code: 3 handlers kept; dialog left up with 4 characters; 2.0 GDI objects per dialog. green after (run 38243584334) |
| 0.4 | The origin question, the one-window slot, a good `open-link`, the tray picking up a link (F14) | Tests with a fake window, the real inbox and the real queue; the question's four answers; the slot `Sign` holds | Container, all three CI jobs that test `cmd`. **Still not a real window or a real desktop** (2.x, 3.1) |
| 0.5 | The scheme key is written at start and removed by `uninstall-notice`, and the MSI removes it | The real `HKCU` key on CI's runner; the per-user MSI installed and removed with `/qn` | Red on the old code (key left); green after (run 38243584334). The MSI removes the key **on its own**, even with `uninstall-notice` not unregistering. Never a person's install, with dialogs |
| 0.6 | A link dropped because eight are waiting is said to the person | The outcome reaches the notifier once per burst | Red on the old `offer` (only logged); green after, in the container and on CI |

**Not a row, asked for alongside group 0:** a stand-in Liro Business, run end to end (P8, D-473). It shows the path runs: link → inbox → queue → channel → signer → result. **It does not show** real HTTPS, a browser, any window, a card or the real server, so **no row below moves.**

## 1. A card, on Windows (v1)

| # | claimed | what would show it false |
|---|---|---|
| 1.1 | **A Pošta card reaches B-LT through the bundled Trusted List, with no LDAP** (D-466) | One signature at B-LT. False if the report says B-T, if a named `b-lt` comes back `LEVEL_NOT_REACHED`, or if DSS's validator or Adobe does not read the `/DSS` as B-LT. The notes say which certificate lacked evidence. |
| 1.2 | A MUP card stops at B-T, and the report **says** it fell short, in the warning colour (C26) | False if the level shows in the positive colour, or if the window refuses where Settings (not a caller) set B-LT. |
| 1.3 | A program that names `b-lt` gets `LEVEL_NOT_REACHED` on a MUP card, and nothing is saved (C26) | False if a document comes back at B-T. |
| 1.4 | B-T against Pošta's **test** TSA with the card, verified by both tools (A2 case ii) | False if `achievedLevel` is not B-T, or either verifier rejects the timestamp. |
| 1.5 | When the TSA stops answering mid-batch, the question offers only *Configure* and *Cancel* (C26) | False if *Sign without a timestamp* is on screen. Point Settings at a dead URL to provoke it. |
| 1.6 | The PIN dialog ends as a cancel when the signing window closes under it (C21, measured only with a test owner) | Close the signing window while the PIN dialog is up during a real login. False if the login hangs or the dialog stays. |
| 1.7 | D45 is a probe timeout (**inferred**) | The NetSeT machine, `TestSourcesAreOnePerUsableModule` five times with the log. False if a run takes ~10 s with no probe timing out, or disagrees without one. |

## 2. A screen, on Windows (v1)

| # | claimed | what would show it false |
|---|---|---|
| 2.1 | The origin window (406 points) and the consent step with a 255-character origin fit, whole, in WebView2 | Measured only in WebKitGTK under Xvfb. False if anything is cut or the page scrolls. |
| 2.2 | Settings with a **non-empty** list of allowed servers fits in 880 points, and Remove works | Never rendered with entries. |
| 2.3 | A failed cloud link is said in a box on Windows | Never shown. False if nothing appears. |

## 3. A server — the cloud channel (F14 §6)

| # | claimed | what would show it false |
|---|---|---|
| 3.1 | A `liro-bridge://sign?job=…` link clicked in a browser opens the agent (or reaches the running one), on Windows and Linux | False if the browser does nothing, or a second agent starts. |
| 3.2 | The origin is asked once, a refusal reaches the server as one, and the result goes **only** to the job's own origin | Watch the server's log. False on any request to another host, or a second question for an allowed origin. |
| 3.3 | Eight links queue and are handled one at a time, in order | False if two windows open, or the order changes. (A ninth is dropped and, today, only logged: 0.6.) |

## 4. A VM — Linux (v1)

| # | claimed | what would show it false |
|---|---|---|
| 4.1 | **D23: no `bwrap → bwrap → xdg-dbus-proxy` chain is left after a window closes**, on Fedora and Ubuntu | Count chains before and after N windows, as D-417/D-421 did. False if the count grows. |
| 4.2 | The link desktop entry makes the browser hand `liro-bridge://` to the agent | `xdg-mime query default x-scheme-handler/liro-bridge`, then a click. False if another handler, or none. |
| 4.3 | The Linux window measurements (2.1 and the D23 test) hold on a real desktop, not only under Xvfb | False if a window is cut at the desktop's fonts. |

## 5. A Mac (outside v1; signing waits on this group, D-467)

| # | claimed | what would show it false |
|---|---|---|
| 5.1 | The window the CI photographed shows the page | Look at the `liro-macos` artifact. False if blank or wrong. |
| 5.2 | The unsigned bundle opens (via *Open*), runs as a menu-bar agent, and a link opens it | False at Gatekeeper, at no menu-bar item, or a link doing nothing. |
| 5.3 | The PIN dialog, a drop, the open panel, alerts and notifications work | Each compiled, never run. |
| 5.4 | SafeSign on macOS loads from the recalled path, and a Pošta card signs | False if discovery finds nothing, or the signature does not verify. |
