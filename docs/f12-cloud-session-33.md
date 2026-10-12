# F12 — session 33 (cloud): C21 finished, D39/D45/D23 read to their causes, D6, the cloud channel, B-LT after DSS, F13 written blind

**What this is:** the handover after a session run in Anthropic's cloud on a Linux container — **no Windows, no macOS, no card, no reader, no screen** — asked to work through six tasks without asking anything, record every decision, and bring back the whole thing. **Written:** 2026-10-12. Branch `claude/wizardly-clarke-rmvkdt`, not merged, no PR. Entries **D-457 to D-463**; predictions in `docs/s33-predictions.md`, each block committed before the run it predicts.

**Read first:** D-457 … D-463, then `docs/phases/F14.md` and `docs/phases/F13.md`, then the open items they touched (A2, A32, C13, C21, C22–C24, D4, D6, D23, D39, D44–D48, B32–B33).

---

## A. What was done

| task | entry | state |
|---|---|---|
| **C21**, the memory-ownership audit, every remaining row | D-457 | **done.** Six rows made into tests and measured on CI's Windows runner before any fix; fixes under them. |
| **D39** (sign window hung 60 s) | D-458 | **cause read** from CI's log: the test's `FindWindowExW` walk lost its place; fixed, and the mechanism is a test. |
| **D45** (two discoveries disagreed) | D-458 | **cause inferred** (10.4 s = the 10 s probe timeout + a probe); the test fixed. Needs the owner's machine to confirm. |
| **D23** (`xdg-dbus-proxy` chains) | D-460 | **cause read in WebKit's source and fixed**, red/green under Xvfb; new finding: gotk4 finalizes nothing. |
| **D6** (examples' discovery path) | D-459 | **closed**; `sign.php` and `Sign.java` run for the first time (against the stand-in). |
| **A2/E2** (timestamp and B-LT) | D-462 | **built** after reading DSS 6.5; `CERT_REVOKED` has its producer; presets removed; SPEC §12.6 rewritten **as a draft for the owner**. |
| **A32** (the cloud channel) | D-461 | **built to F14**, with protocol version negotiation; SPEC §2/§4.3 amended. |
| **F13** (macOS) | D-463 | **written blind**, with a `macos` CI job as its compiler. Still outside v1 (SPEC §19). |

## B. CI, read to the end of each run

| run | commit | what it was for | result |
|---|---|---|---|
| 38183853425 | `48e53ce` | P1: C21's tests, no fix | windows **red as designed**: P1a, P1b red; Navigate's test hung 10 min in **my** log handler; cmd green (P1f's prediction failed). Every other job green. |
| 38185042718 | `7cd0a8b` | P3: harness fixed, still no fix | windows red as designed: P3a/b/d/e red, **P3c passed (no defect)**, P3f half-read. Every other job green. |
| 38186918219 | `55a582e` | P4: the fixes, cloud channel, B-LT | @@P4@@ |
| 38187564063 | `e131e3f` | P5: the PIN dialog's owner watch; F13's first macOS run | @@P5@@ |

**check.sh, cited before each push** (run as an ordinary user; root fails one test for an environmental reason, D-460):
- `48e53ce`: all of `ci` except the real-TSA test (DNS to `test-tsa.ca.posta.rs` blocked here; CI reaches it).
- `7cd0a8b`: `ci:2,4–8` green.
- `0e3935a`: **lint red in both views** — an unused test helper of mine — fixed before pushing (`55a582e`).
- `55a582e`: every `ci` and `linux-gui` step green except the DNS-blocked test; fuzz and the three builds re-run separately, green.
- `61b78ca`/`e131e3f`: the same, the same one environmental failure; `ci:2,4–8,11–14` green on `e131e3f`.

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

## F. Rules this session adds

1. **A measurement commit carries no fix, and a harness defect gets its own commit before the fix does** — otherwise the rows a broken harness never ran can only be measured on fixed code (D-457).
2. **A result the test cannot see is not measured** — read the handle before closing the file; wait until the page has the size you asked for; ask whether the thing you are checking is still on screen (D-457, D-461).
3. **`go f(g())` evaluates `g()` before the goroutine starts** — a control made by adding `go` in front of a call can change nothing (D-461).
4. **A Windows window does not take another thread's owned windows with it** (D-457, P4).

## G. The tree

Worktree `/home/user/wt-fix` (branch `s33-fixes`) was where the work was done; the pushed branch is the same commits, fast-forwarded. The main tree holds a stash `d6-work` from before the work moved — superseded by D-459's commits; dropped at the end of the session.
