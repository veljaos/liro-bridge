# F12 — session 33 (cloud): C21 finished, D39 and D23 read to their causes, D45 inferred only, D6, the cloud channel, B-LT after DSS, F13 written blind

> **Superseded at the session's end by `docs/handover-session-33.md`**: what is done, what has never run, and the worklist with card rows first. Read that first.

**What this is:** the handover after a session run in Anthropic's cloud on a Linux container — **no Windows, no macOS, no card, no reader, no screen** — asked to work through six tasks without asking anything, record every decision, and bring back the whole thing. **Written:** 2026-10-12. Branch `claude/wizardly-clarke-rmvkdt`, not merged; the owner opened PR veljaos/liro-bridge#1 from it. Entries **D-457 to D-472**; predictions in `docs/s33-predictions.md`, each block committed before the run it predicts.

**Read first:** D-457 … D-472, then `docs/phases/F14.md` and `docs/phases/F13.md`, then the open items they touched (A2, A32, C13, C21, C22–C25, D4, D6, D23, D39, D44–D48, B32–B33).

---

## After the owner's review (2026-10-12): D-465 … D-481

**The owner's worklist is `docs/measure-before-release.md`** (D-472): what must be measured before any of this reaches a person, in order. **Then read `docs/s33-what-ran.md`.** It has every change, what has actually run against it, and what has never run. Where this handover says "measured", that table says where, and whether the test was ever seen failing without the change. Where the two differ, the table is the later reading.

- C25 is fixed as the owner ruled (D-465). The B-B finding I reported was wrong; what is true is in `docs/decision-timestamp-failure.md` (C26).
- LDAP, and what B-LT reaches: `docs/decision-ldap.md` (D-466). No Serbian card has reached B-LT.
- The owner's rulings are recorded (D-467): macOS signing waits; D45 stays inferred.
- **Group 0 of the worklist is done** (D-473): each test was seen red on the old behaviour, then green. D-475 fixed a race in one of those tests.
- **The default level is decided: C** (D-474, D-476). It is "the best available", and a level chosen in Settings is a request. SPEC §12.6 is amended and marked not built; building it is worklist 0.7, before card rows 1.2 and 1.3.
- **D-480, read this first: my code wrote a person's name, and on a MUP card their JMBG, into the agent's log** (from `4b7c60c` until `2ec3055`). No test, review or rule found it, and nothing in the project would have. Thirteen pushed heads carry it, and their CI packages can still be downloaded. Whether anyone ran one cannot be known. No message in the product now names a certificate by anything but its thumbprint.
- **Decision C built** (D-478, D-479). **E10 is now "the SPEC has been wrong three times out of three where we checked".** **Row 1.1 with the test timestamp server** (D-481): whether it can be read without a contract depends on the test unit's issuer, which one B-T signature shows.
- **Card row 1.1's preconditions** (D-477): the log names each certificate that got no evidence, by thumbprint. The window and protocol paths log the signer's notes. CI publishes the MSIs as `liro-bridge-windows-<sha>`. Writing them, I found that my own D-462 code logged a certificate's **subject**, against SPEC §18.3. It is fixed, and the test now checks that the subject is absent.

## A. What was done

| task | entry | state |
|---|---|---|
| **C21**, the memory-ownership audit, every remaining row | D-457 | **done.** Six rows made into tests and measured on CI's Windows runner before any fix; fixes under them. |
| **D39** (sign window hung 60 s) | D-458 | **cause read** from CI's log: the test's `FindWindowExW` walk lost its place; fixed, and the mechanism is a test. |
| **D45** (two discoveries disagreed) | D-458 | **cause inferred, not measured.** The arithmetic (10.4 s = the 10 s probe timeout + a probe) fits a timeout. It is not a reading of one, and nothing here could observe it. The test is corrected. It stays open until the owner's machine shows it (the owner, D-467). |
| **D23** (`xdg-dbus-proxy` chains) | D-460 | **cause read in WebKit's source and fixed**, red/green under Xvfb; new finding: gotk4 finalizes nothing. |
| **D6** (examples' discovery path) | D-459 | **closed**; `sign.php` and `Sign.java` run for the first time (against the stand-in). |
| **A2/E2** (timestamp and B-LT) | D-462 | **built** after reading DSS 6.5; `CERT_REVOKED` has its producer; presets removed; SPEC §12.6 rewritten **as a draft for the owner**. |
| **A32** (the cloud channel) | D-461 | **built to F14**, with protocol version negotiation; SPEC §2/§4.3 amended. |
| **F13** (macOS) | D-463 | **written blind**, with a `macos` CI job as its compiler. Still outside v1 (SPEC §19). |
| (unasked) a racy API test | D-464 | a docs-only push went red on `TestTheResultAnswers202WhileTheJobIsStillRunning`: D-252's defect at a site its sweep missed. Reproduced 7/1200, fixed, 0/1200. |

## B. CI, read to the end of each run

| run | commit | what it was for | result |
|---|---|---|---|
| 38183853425 | `48e53ce` | P1: C21's tests, no fix | windows **red as designed**: P1a, P1b red; Navigate's test hung 10 min in **my** log handler; cmd green (P1f's prediction failed). Every other job green. |
| 38185042718 | `7cd0a8b` | P3: harness fixed, still no fix | windows red as designed: P3a/b/d/e red, **P3c passed (no defect)**, P3f half-read. Every other job green. |
| 38186918219 | `55a582e` | P4: the fixes, cloud channel, B-LT | eight jobs green (packaging with the MSI's new key; the desktop entry installed and removed on three distributions). windows red on **two** rows: the PIN dialog (P1e's mechanism wrong — see D-457) and `internal/cloud`'s own test, which cut a path at `/` — **I first reported one row**, having read the log only from `internal/pades` down. |
| 38187564063 | `e131e3f` | P5: the PIN owner watch; F13's first macOS run | windows: **`internal/ui` green, the PIN row included (P5a)**; red on the cloud test and the LaunchAgent's path rule (both fixed in `5e187d9`). macos: red at vet in PKCS#11 only — `internal/ui`'s Objective-C compiled first time. |
| 38187726349 | `3f8b65d` | macOS, second | macos: **`internal/ui` and `internal/platform` green** (the window, the Keychain, the LaunchAgent); PKCS#11 failed to link (`pkcs11_linux.c`'s `liro_*` symbols missing — in its own test binary and in the three that import it, `cmd/liro-bridge`, `pkcs11/worker`, `pinscreen`; and `-lp11-kit` from Homebrew, found here). windows red on the same two test bugs as `e131e3f` (`5e187d9`). The Linux jobs and the three `linux-install` green. |
| 38187982282 | `ac20812` | macOS, third | **macos green**: the suite, and the bundle — universal, `--version`, only system frameworks. windows red on the two test bugs fixed in `5e187d9` (read from its log: `internal/cloud`'s `filepathDir` and the LaunchAgent's two tests, nothing else). The Linux jobs and the three `linux-install` green. |
| 38188278943 | `38bbcfa` | everything, and the macOS tests with `-v` | **all ten jobs green**: `ci` (the real-TSA test cannot skip on an unreachable server — `SignDocument` saves at B-B and the test fails, as it does in this container — so green there means Pošta's test TSA was reached and a B-T signature verified; inferred from the test's code and the result, the log being not `-v`), `linux-gui` with its race probe, `linux-packages` and the three `linux-install`, `packaging`, `sdk-typescript`; **`windows` green from its first test line** — every Go package `ok`, the SDK 110 passed, 0 failed, 3 skipped by design (the discovery-file examples place the file on Linux and macOS only); **`macos` green, and the `-v` step says every named test ran, none skipped**: the window, the Keychain, both LaunchAgent tests, and SoftHSM 2.7 — token read, a 256-byte signature verified against the certificate, one wrong PIN one `CKR_PIN_INCORRECT`. |
| 38189507427 | `a5a8e45` | the docs | **ci red** on `TestTheResultAnswers202WhileTheJobIsStillRunning`, a test race: the job was still `queued` when read. D-464; reproduced locally 7/1200, fixed. The other nine jobs green. |
| the next | D-464's commit | the fix | **not in this file**: a file cannot carry the result of the run its own push starts. It is read to its end in the session's closing report. |

**check.sh, cited before each push** (run as an ordinary user; root fails one test for an environmental reason, D-460):
- `48e53ce`: all of `ci` except the real-TSA test (DNS to `test-tsa.ca.posta.rs` blocked here; CI reaches it — see run 38188278943's row for how that is known). That test's `Skip` branch is dead: an unreachable TSA is not an error from `SignDocument` but a B-B result, so the test fails rather than skips (C25).
- `7cd0a8b`: `ci:2,4–8` green.
- `0e3935a`: **lint red in both views** — an unused test helper of mine — fixed before pushing (`55a582e`).
- `55a582e`: every `ci` and `linux-gui` step green except the DNS-blocked test; fuzz and the three builds re-run separately, green.
- `61b78ca`/`e131e3f`: the same, the same one environmental failure; `ci:2,4–8,11–14` green on `e131e3f`.
- `a5a8e45` (docs): `ci:2,4–8,12–14` green.
- D-464's commit: `ci:2,4–8,12–14` green, recorded in the closing report; `internal/api` under `-race` green, and the test 0/1200 at `GOMAXPROCS=4`.
- `3f8b65d`, `ac20812`, `38bbcfa`: `ci:2,4–8,12–14` (and `linux-gui:4,5` where the PKCS#11 package changed) green; on `38bbcfa` `ci:9` red only on the DNS-blocked test.

## C. Decided, and on what basis (each in its entry)

- **C21:** a fix stays in when its measurement found no defect, if it is the rule the code states (said before the run, in P1). — D-457
- **Cloud:** no device secret for a server; no new audit field (the application *is* the origin); the origin question waits for the one-window slot; links one at a time, eight queued; a link that fails is said — a notification, or the native box where there is no notifier (Windows); the origin window 406 points, measured; `internal/cloud` held to the API door's dependency rule. — D-461
- **B-LT:** DSS's rules where they are sound: the timestamp's chain is required for B-LT; an anchor is self-signed or in the Trusted List; Unknown is not evidence; stale evidence embedded and reported; a revoked signer saves nothing; a revoked TSA or responder keeps B-T. **LDAP not built** — a new outbound protocol is the owner's (SPEC §6.8, D-281). — D-462
- **SPEC:** §2's cloud-signing row reworded as permanent and the channel distinguished from it; §4.3 five entry points; §4.2 rule 4 names `internal/cloud`; §12.6 rewritten, marked as a draft. — D-461, D-462
- **F13:** notifications through `osascript`; the login keychain; unsigned; no PDF filter; LaunchAgent written not loaded; a bare bundle launch is the agent; module paths recalled, not read. — D-463

## D. Never run — and what each needs

| what | needs |
|---|---|
| The cloud channel against a real server, a link clicked in a browser, a job signed with the card | **a server (F14 §6), each platform, the card, a person** |
| B-LT against a real Serbian OCSP/CRL/TSA; a B-LT signature on a Pošta card; Pošta's test TSA end to end | **network that reaches them, the card** |
| SPEC §12.6's new text | **the owner's eyes** — it is marked a draft |
| The CNG branch where `fCallerFree` is FALSE | a provider that answers FALSE (a card's KSP, perhaps) |
| D23's fix on the owner's Fedora and Ubuntu VMs, counting chains | **the owner's hands, the VMs** |
| D45's inference | **the owner's machine** (NetSeT's slow probe) |
| The origin window and Settings in WebView2; the consent window with a 255-character origin | **Windows with a screen**; the last also measurable under Xvfb |
| Everything in F13 §4's right-hand column: Gatekeeper, a link opening the agent, the menu bar item, a drop, the PIN dialog, SafeSign with a Pošta card, Approve | **a Mac, a card, a person** |
| The macOS window's photograph | **the owner's eyes** — it is in the `liro-macos` artifact of runs 38187726349, 38187982282 and 38188278943 (the last: artifact 11720308536, two files — the photograph and the zipped bundle); this container's proxy refused the artifact host, so I have not seen it |
| Signing the macOS bundle | **the owner's Apple Developer ID**, or a decision to ship unsigned |
| `Sign.cs` compiled | a C# compiler (Windows) |
| The stderr test's file half (P3f), with the helper fixed | one CI run read with `-v` |

## E. Defects of mine this session, said

- `recordWarnings` looped through `log.Default()` and hung CI's test ten minutes (D-457).
- The stderr helper read a closed file's handle (D-457).
- A control whose code change did nothing — `go f(g())` still calls `g` synchronously — reported as a control until I looked at it again (D-461's queue test, redone).
- The origin window's first measurement read the page before the resize had reached it, then accepted a region that hid the origin (D-461; both found by the test, both fixed in it).
- P1e's mechanism (an owner destroys owned windows across threads) was wrong; P4 showed it, and the fix that relied on it did nothing there (D-457, P4/P5).
- An unused helper left in the B-LT tests — caught by check.sh's lint before the push.
- Two tests that assumed `/` (the origin list's, the LaunchAgent's) — red on Windows; and **I read run 38186918219's Windows log from the middle and reported one failure where there were two** (D-457's P4 paragraph says so).
- The consent-layout test's first form passed its own control, because it never asked whether the page scrolled (D-461).
- **I wrote up runs before their last jobs had finished.** `linux-gui`'s race probe, `linux-packages` and the three `linux-install` jobs run ten to fifteen minutes after the others. Three runs' rows said "every other job green" before those jobs ended. They all ended green, and I read each to its conclusion at the end of the session, but the rows were written before that was known. In the predictions file I also wrote "all seven jobs green" for the last run while two were still running; I caught it before committing.
- **Three runs had no prediction written before them** (the macOS runs after P5). Said in `docs/s33-predictions.md`, not written afterwards.
- The second macOS run's link failure was recorded as PKCS#11's alone. Its log shows the same missing symbols in the three packages that import it (D-463, corrected).

## F. Rules this session adds

1. **A measurement commit carries no fix, and a harness defect gets its own commit before the fix does** — otherwise the rows a broken harness never ran can only be measured on fixed code (D-457).
2. **A result the test cannot see is not measured** — read the handle before closing the file; wait until the page has the size you asked for; ask whether the thing you are checking is still on screen (D-457, D-461).
3. **`go f(g())` evaluates `g()` before the goroutine starts** — a control made by adding `go` in front of a call can change nothing (D-461).
4. **A Windows window does not take another thread's owned windows with it** (D-457, P4).
5. **Read a failed job's log from its first test line, not its tail** — a 130-line tail began at `internal/pades` and hid a failure above it (D-457).
6. **In a suite, a skip and a pass both print `ok`** — a measurement that matters gets a `-v` run of its own (D-463).
7. **On macOS, a cgo link line can carry a Homebrew library into the product** — the bundle is checked with `otool -L` (D-463).
8. **A run is read when its last job has ended, not when the jobs I was waiting for have.** In this workflow `linux-install` starts only after `linux-packages`, about fifteen minutes in (this session).

## G. The tree

Worktree `/home/user/wt-fix` (branch `s33-fixes`) was where the work was done; the pushed branch is the same commits, fast-forwarded. The main tree held a stash `d6-work` from before the work moved. It was compared file by file with the branch: four files identical, and two (`Sign.cs`, the README) older drafts that the committed version improves on (`OperatingSystem.IsWindows` there, which .NET Framework 4's `csc` lacks). It was dropped. The worktree was removed at the end; nothing is left outside the branch.
