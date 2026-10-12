# Session 33 (cloud) — predictions, written before each measurement

Each block is committed before the run it predicts, so git dates it. The
least certain prediction of each block is named. Results are not written
here; they are in the decision entry that reads the run.

## P1 — C21's Windows measurements, on CI's `windows` job, against the code as it stands (before any fix)

Written 2026-10-11, before the commit carrying these tests was pushed.
The run that measures them is the first CI run on that commit; its
`windows` job is the instrument. No fix is in the commit.

| | test | predicted on the unfixed code |
|---|---|---|
| P1a | `internal/platform` `TestLockingTheSameDirectoryManyTimesKeepsAtMostOneHandle` | **FAIL**: about 200 handles more after 200 cycles (one mutex handle per lock) |
| P1b | `internal/ui` `TestAClosedWindowGivesBackEveryDropTargetItRegistered` | **FAIL**: four of five targets still pinned — those on the browser's windows, which Controller::Close removes before the revoke; the frame's own one released. **Least certain of P1**: whether Close destroys the browser's child windows synchronously, before the revoke runs. If it does not, this passes and the row is a reading only |
| P1c | `internal/ui` `TestNavigatingRegistersTheNewTreesDropTargetsOnTheWindowsOwnThread` | **FAIL**: RegisterDragDrop off the UI thread fails for every window (no OLE on the caller's thread), warnings recorded, no target registered right after Navigate. Second least certain: the calling thread might be in an implicit MTA that RegisterDragDrop accepts |
| P1d | `internal/ui` `TestAWindowThatFailsToOpenGivesBackItsIcons` | **FAIL**: about 30 GDI objects left by 5 failed windows (6 each, FTEST Group 3's figure) |
| P1e | `internal/ui` `TestAPINDialogWhoseOwnerIsDestroyedReturns` | **FAIL**: CollectPIN does not return within 15 s; the dialog's window destroyed with its owner (Windows destroys owned windows across threads) |
| P1f | `cmd/liro-bridge` `TestNothingWrittenToStderrAfterTheConsoleIsGivenBackLandsInAFile` | **FAIL**: the marker is in the file the helper opened after FreeConsole; the file's handle equals the stderr handle read before it |
| P1g | everything else in the `windows` job | passes as on master's last run; the other eight jobs `success` |

If P1b or P1c pass on the unfixed code, the fix still goes in (each is
what the code's own comments say the rule is), and the entry says the
measurement found no defect for that row.

## P2 — the same tests after the fixes

All six pass; nothing else changes.

## P1, read — CI run 38183853425 on 48e53ce (results belong to the entry; listed here only so P3 can be read against them)

P1a red as predicted. P1b red as predicted (4 of 5 pinned). P1c never
asserted: the test hung ten minutes in its own log handler (recordWarnings
forwarded to slog's default handler, which loops through log.Default() back
into itself) and the package timed out, so **P1d and P1e never ran**. The
hang's stack shows `registerDropTargets`, called from Navigate on the test's
goroutine, logging "registered a drop target" — RegisterDragDrop off the UI
thread **succeeded** at least once, which P1c's mechanism said it would not.
**P1f failed as a prediction**: cmd/liro-bridge passed.

## P3 — the same commit plus the harness fix only (no product fix), CI's `windows` job

Written before the commit that carries the harness fix is pushed.

| | test | predicted |
|---|---|---|
| P3a | P1a's | **FAIL**, ~200 handles, as measured |
| P3b | P1b's | **FAIL**, 4 of 5 pinned, as measured |
| P3c | P1c's | **PASS** — no defect found for the row: the stack above shows the off-thread registration succeeding, so the thread Go ran Navigate on was able to take a drop target (an OLE-initialised thread reused by the runtime, or the apartment rule is looser than the comment says). **Least certain of P3**: the test may assert something else that the off-thread path still breaks (a target registered on the wrong apartment never receives a drop, which this test does not drive) |
| P3d | P1d's | **FAIL**, ~30 GDI objects |
| P3e | P1e's | **FAIL**, CollectPIN does not return in 15 s |
| P3f | the P1f reading step | passes; its diagnosis shows `stderrAfter` equal to `stderrBefore` and `file` a different value — Server 2025's FreeConsole leaves the console handles as they were, or their numbers are not the first handed out, and the reuse D-454 reasoned from did not happen here |

## P3, read — CI run 38185042718 on 7cd0a8b

P3a, P3b, P3d, P3e red as predicted (200 handles; 4 of 5 pinned; 30 GDI
objects, 6.0 each; CollectPIN not returned in 15 s). **P3c passed, as
predicted**: off the UI thread the registration succeeded on the runner, so
the row is a reading with no defect measured. P3f half right:
`stderrAfter=0x74` equal to `stderrBefore=0x74`, the marker in no file — but
`file=0xffffffffffffffff` is a closed file's Fd, read after Close by my own
helper, so the file half measured nothing.

## P4 — the fixes and the session's code, pushed together (s33-fixes), every job

Written before the push.

| | what | predicted |
|---|---|---|
| P4a | the five C21 Windows tests | all pass |
| P4b | the stderr test, its helper now reading the handle before Close | passes; not printed (no -v), so its numbers are not read |
| P4c | `TestAFindWindowWalkLosesItsPlaceWhenItsCursorIsDestroyed` (D39's mechanism) | passes — the walk ends early when the cursor is destroyed. **Least certain of P4**: whether the runner's window manager reuses or frees the slot the way the owner's machine did |
| P4d | `TestURLSchemeRoundTrip` (the cloud channel's registration, HKCU scratch key) | passes |
| P4e | `TestSettingsOffersNoTimestampService` | passes |
| P4f | the cloud, B-LT and negotiation tests in `ci`, `linux-gui` and the SDK job | pass |
| P4g | `linux-packages` and `linux-install` with the new desktop entry | pass, the entry found and removed with the package |
| P4h | `packaging` (the MSI with one more RemoveRegistryKey) | passes |

## P4, read — CI run 38186918219 on 55a582e

All P4 rows as predicted **except P4a's PIN-dialog row**: still red,
"CollectPIN did not return within 15 s of its owner being destroyed (the
dialog's window still exists)". P1e's mechanism was wrong — a window
destroyed on one thread does not destroy the windows it owns on another,
so the WM_DESTROY branch the fix added never ran there. (The other jobs:
read in D-457's addendum.)

## P5 — the owner watched on a timer, and F13's first macOS run

Written before the push that carries both.

| | what | predicted |
|---|---|---|
| P5a | `TestAPINDialogWhoseOwnerIsDestroyedReturns` | passes: within ~250 ms of the owner's destruction `IsWindow` is false and the dialog cancels |
| P5b | every other Windows test | as in run 38186918219 |
| P5c | the `macos` job | **red**, and where is the least certain thing in this session: code no compiler has seen. Most likely first failures, in order: an Objective-C compile error in `darwin_objc.go`; a cgo type mismatch between the preamble's declarations and `_cgo_export.h`; a test written for Linux or Windows that is untagged and fails on darwin. **Not** predicted: a window that opens and loads on the first attempt |

## P5, read — runs 38187564063 (`e131e3f`) and after

- **P5a passed** (run 38187564063): the PIN dialog cancels when its owner goes.
- **P5b failed on two rows that were not "as before"**: `internal/cloud`'s own test and the LaunchAgent's path rule, both of which assumed `/`. These were new tests, so P5b's "as in run 38186918219" could not cover them. Fixed in `5e187d9`.
- **P5c failed in its detail.** The job was red, as predicted, but not where predicted. `internal/ui`'s Objective-C and both cgo halves compiled the first time. The first failures were in PKCS#11, a package P5c did not name: darwin forms were missing, and then the link failed (D-463).
- **No prediction block was written for the three macOS runs after this one** (`3f8b65d`, `ac20812`, `38bbcfa`). I treated them as fix-and-retry runs and did not write down beforehand what each would show. This breaks the session's rule, and I am stating it here rather than writing predictions after the fact. What each run showed is in D-463, read from its log. The last run, 38188278943 on `38bbcfa`, was read to its conclusion: all ten jobs green; Windows read from its first test line (every Go package `ok`; the SDK 110 passed, 0 failed); every macOS test named in the `-v` step reported as run, none skipped.

## P6 — D-465 … D-468, every job (written before the push)

| | what | predicted |
|---|---|---|
| P6a | `TestThePostaShape…` (both) | pass on `ci` (Linux, `-race`), `windows`, `macos`, as they did here |
| P6b | `TestDyldsWordsAreReadIntoTheKinds`, now asserting `LoadNoFile` | passes on `macos`. Run 38188278943's log printed LoadNoFile's own sentence ("there is no file at this path"), so the kind was already right; only the assertion is new |
| P6c | `TestNothingIsBothKeptAndRemoved` with `cloud-origins.json` kept | passes on `windows` |
| P6d | `TestSignDocumentBTWithRealTSA` | `ok` everywhere. **The least certain row, and one I cannot read:** on CI it either runs (the server reachable) or now skips, and without `-v` the two look the same. That cost is the one D-465 names |
| P6e | everything else | as in run 38190929428: all ten jobs green |

## P6, read — runs 38225585797 (push) and 38225591952 (PR), `feed7e8`

All ten jobs green in both. P6b passed: the dyld test asserted `LoadNoFile` on the runner, the `-v` step printing the kind's own sentence. P6a and P6c read only as `ok` (no `-v`). P6d could not be read, as predicted.

## P7 — D-469 … D-472 (written before the push)

| | what | predicted |
|---|---|---|
| P7a | `levelshortfall_test.go`, `TestABLTThatFallsShortIsRefusedUnlessTheCallerAcceptsBT`, the reworked fallback tests | pass on `ci`, `linux-gui` (the `cmd` tests need no window), `windows`, `macos` |
| P7b | `TestTSAChoiceScreenRoundTrips` with the button hidden after a failure | passes on `windows`. **The least certain row:** a real WebView2 page, the first time `hidden` is set on that button |
| P7c | `sdk-typescript` and the Windows SDK step, with `LEVEL_NOT_REACHED` | pass; `dist/` matches `src/` |
| P7d | `TestEveryErrorCodeHasAMessageInEveryCatalogue` and the catalogue-parity tests | pass with the new code and two keys |
| P7e | everything else | as in the runs on `feed7e8` |

## P8 — a stand-in Liro Business, end to end (written before it is built; the owner asked for this first)

**What it is.** One Go test in `cmd/liro-bridge`, run in this container and by every CI job that tests `cmd` (Linux, Windows, macOS). It strings together the agent's own code:
- `open-link` with a good link, an agent reported running, so the link is left in the inbox;
- the watcher taking it from the inbox;
- the queue;
- `cloud.Channel.Handle` with the real client, the real store of allowed servers (in a temporary directory) and the real job registry;
- the result posted back.

It runs against a **stand-in server** in the same process: Go's `httptest` TLS server, serving a job of digests and recording what is posted. The parts that are not real:
- a **stand-in for the origin question**, answering allow or deny;
- a **stand-in signer**, standing for the consent window, the PIN and the card: it signs the digests with an RSA key made in the test;
- the server checks the returned signatures against that key's certificate.

**What it would show:**
- that the pieces fit: a link that enters as a command-line argument ends as a signature on the server that served the job, with nothing in between disconnected;
- that the server is asked about once, and remembered;
- that a server the person refuses is never fetched from;
- that the result goes to the job's own origin;
- that what comes back verifies against the signer's certificate.

**What it would not show** (no worklist row moves; the owner, 2026-10-12):
- **real HTTPS**: the server's certificate is trusted by name in the test, not by the system's roots;
- **a browser** handing over the link: no desktop, no scheme registration, no LaunchServices;
- **any window**: not the origin question, not the consent, not the PIN; each is a function standing in;
- **a card or a real signature**: the stand-in signer is not the agent's consent flow, and the signatures are not PAdES;
- **Liro Business itself**: the stand-in is written from F14's description of the protocol, not from the real server's code. Where the two differ, this test cannot see it.

**Predicted:** green in the container and on every CI job that runs `cmd`. Its control: the same test with the queue's watcher never started must fail on the server not receiving a result. If it passes without the watcher, it measures nothing about the path.

## P7, read — runs 38233120579 (push) and 38233125458 (PR), `1abc206`

All jobs green in both. P7b reads only as `ok`: the Windows step is not `-v`.

## P9 — group 0's Windows rows, measured on the old behaviour first (written before the push)

Two runs. **M** carries the new tests, with each fix switched back to its old behaviour by a variable named `p9Unfixed` (internal/ui, cmd/liro-bridge). **F** removes the variable and nothing else.

| | test | predicted on M | predicted on F |
|---|---|---|---|
| P9a | `TestNavigatingRegistersTheNewTreesDropTargetsOnTheWindowsOwnThread` (0.1) | **red**: targets registered on the test goroutine's thread, not the UI thread's | green |
| P9b | `TestNothingWrittenToStderrAfterTheConsoleIsGivenBackLandsInAFile` (0.2) | **red**: `stderrAfterDetach=invalid` (the console's closed number) | green: `nul` |
| P9c | `TestAFailedSubscriptionGivesItsHandlerBack` (0.3) | **red**: 3 handlers kept | green |
| P9d | `TestAQuitReachingThePINLoopIsACancel` (0.3) | **red**: the dialog still up, with 4 characters in its field | green |
| P9e | `TestAPINDialogThatFailsToBuildGivesItsFontsBack` (0.3) | **red**: about 20 GDI objects kept over 10 dialogs | green |
| P9f | `TestTheLinkSchemeIsWrittenAtStartAndRemovedByTheUninstallNotice` (0.5) | **red**: the key left behind | green |
| P9g | the packaging job's install/uninstall step (0.5) | **green on M too**, the least certain row: the first install this project has ever run on CI. If green on M, where `uninstall-notice` does not unregister, it shows the package's own `RemoveRegistryKey` removes the key by itself | green |
| P9h | everything else, including group 0's Linux-run tests (0.4, 0.6, the stand-in Liro Business) | green on both | green |
