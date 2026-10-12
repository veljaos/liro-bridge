# Session 33 — what has actually run against each change

**Asked by the owner before anything merges:** "How would I tell, reading this, which parts are measured and which are only compiled?" This is the answer, one row per change.

It was built from four read-only inventories, one per area: the test files, `ci.yml` and the CI logs. I then checked the claims each row rests on myself: test names, skip conditions, what `ci.yml` runs. I take no row on trust from a summary, including my own earlier ones; where an earlier record of mine overstated something, the row says so.

**Where things can run, and what each place can show:**
- **CI.** GitHub's runners: Windows, macOS and Linux. No card, no reader, no person.
  - **Windows** opens real WebView2 windows.
  - **macOS** opens a real WKWebView window and talks to SoftHSM.
  - **Linux opens no window at all.** `ci.yml` starts no display, and bubblewrap gets no user namespace there, so every Linux window test **skips** on CI. This was read in run 38188278943's `-race -v` probe.
  - Most steps run without `-v`, so in their logs a skip and a pass both print `ok`.
- **Container.** This session's Linux machine: Xvfb, WebKitGTK 2.52.6, no card. It is where the Linux window measurements were made, and nothing there is repeated by CI.
- **Real machine.** One of the owner's, or one with a card, a reader or a person looking at a screen. **Nothing in this session ran on one.**

**Reading the columns:**
- "Seen failing without it" means a test was watched to go red on the code before the change: the control. Without one, a passing test may not be testing the change.
- "Never run" lists the parts of the change that no test, anywhere, has executed. They are compiled, and that is all.

## C21 — Windows memory ownership (D-457)

| change | test | where it runs | seen failing without it | never run |
|---|---|---|---|---|
| One mutex handle per directory per process, not one per audit record | `TestLockingTheSameDirectoryManyTimesKeepsAtMostOneHandle` | CI Windows | **yes**: 133 → 333 handles in 200 locks, two runs | the Global→Local fallback; that keeping the handle preserves the abandoned-writer signal is covered only incidentally |
| Drop targets revoked before the controller closes | `TestAClosedWindowGivesBackEveryDropTargetItRegistered` | CI Windows | **yes**: 4 of 5 pinned, twice | the "revoke refused" log branch |
| A failed window destroys its icons | `TestAWindowThatFailsToOpenGivesBackItsIcons` | CI Windows | **yes**: 15 → 45 GDI objects | — |
| The PIN dialog cancels when its owner goes (250 ms `IsWindow` timer) | `TestAPINDialogWhoseOwnerIsDestroyedReturns` | CI Windows | **yes**: red unfixed; still red with my first fix (`WM_DESTROY`); green with the timer | the `WM_DESTROY` branch (needs an owner on the same thread; no test has one); that the field is cleared on cancel |
| Navigate re-registers drop targets on the UI thread | `TestNavigatingRegistersTheNewTreesDropTargetsOnTheWindowsOwnThread` | CI Windows | **no**: it passed on the unfixed code. The test does not tell fixed from unfixed | whether a drop arrives after a navigation |
| Standard handles re-pointed at NUL after `FreeConsole` | `TestNothingWrittenToStderrAfterTheConsoleIsGivenBackLandsInAFile` | CI Windows | **no**: it passed on the unfixed code (the prediction failed). The file half of its reading has not been re-read since the helper was fixed | nothing asserts NUL; `retiredStdFiles` and `log.SetOutput` |
| A failed `add_*` releases its handler (three sites) | none | — | — | **all of it** |
| A failed PIN dialog deletes its fonts | none for the failure path | — | — | the failure path |
| `WM_QUIT` reaching the PIN loop cancels it | none | — | — | **all of it** |
| The tray's own icon destroyed at exit | none observes it | — | — | the release |
| CNG certificate context kept while `fCallerFree` is FALSE | `TestCloseHonoursCallerFreeFalse` (fake connection) | CI all three | — | **the real retain path**: needs a provider that answers FALSE |

## D39, D45 (D-458)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| D39: the window finder takes a snapshot instead of walking with a cursor | `TestAFindWindowWalkLosesItsPlaceWhenItsCursorIsDestroyed` shows the **old mechanism** failing | CI Windows | the mechanism, yes; **the old helper failing and the new one passing, no** | that D39's intermittent hang is gone is inferred, not shown |
| D45: the test asks one discovery, guarded | `TestSourcesAreOnePerUsableModule` | CI all three; real modules likely only on macOS (Homebrew OpenSC), unread | **no** | **the cause is inferred, not measured** (the owner's ruling: it stays so) |

## D23 — the leftover D-Bus proxy (D-460)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| The stat-line parser and the pairing rule | two pure tests | CI Linux | — | — |
| Releasing the orphaned proxy when a window closes | `TestClosingAWindowLeavesNoDBusProxyBehind` | **container only**. It skips on CI (no display, no user namespace) | **yes, in the container**: 1 proxy left 10 s after close without the release, none with it, three windows in a row | **the owner's Fedora and Ubuntu VMs**, the count D23 was opened for |

## D6 — the SDK examples (D-459)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| `sign.php`, `Sign.java` read the discovery file on Linux | executed against the **stand-in agent** | container; CI's Linux SDK job probably (php/java present on the image; its log unread) | **yes, in the container** | their macOS and Windows branches; the `~/.local/state` fallback; the real agent |
| `Sign.cs`, `liro-test-client.ps1` | a text check only | CI | — | **`Sign.cs` has never been compiled; the PowerShell client never executed** |

## Protocol version negotiation (D-461)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| Agent: `protocolVersions`, `X-Liro-Protocol`, 426 | `negotiation_test.go` (health route) | CI all three | not stated | the header on authenticated routes (same middleware, untested); the agent writing it into its own `bridge.json` |
| TypeScript SDK negotiates | `negotiation.test.mjs` | CI Linux and Windows | **yes**: red on the old SDK | **the SDK has never talked to the real agent**; the stand-in does not send 426 |

## The cloud channel (D-461, F14)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| Reading a link, fetching a job, posting a result to the same origin only, refusing other servers, redirects, untrusted TLS | `internal/cloud` tests against **loopback TLS servers** | CI all three | — | **any real server**; the digests kind end to end; retries; 404/5xx; oversize bodies |
| The queue: one at a time, in order; drop when full | two tests with **hand-built** queues and fakes | CI all three (`cmd` tests run on Linux CI too; these need no window) | **yes**: the order test, once the control was made real | `newCloudLinks`, the real wiring; a queue of 8 (the test uses 1) |
| `open-link` refusing a bad link | `TestOpenLinkRefusesABadLinkAndLeavesNothing` | CI all three | — | **the good-link path**: `TestOpenLinkLeavesAGoodLinkForTheAgent` does not call `runOpenLink`. It tests the inbox directly. "Becomes the agent" has never run |
| The origin question (`askOrigin`) and the one-window slot (`holdSlot`) | none of either | — | — | **all of both** |
| The origin window's page | loads and sends a message: CI macOS (WKWebView). Payload strings: CI all three | | | in WebView2; the real Allow/Deny path; the buttons and Escape |
| The origin window's size (406) and the consent step with a 255-character origin | two layout tests | **container only** (skip on CI) | the consent one: **a one-off 200-point run in the container, not in the committed test** | WebView2; the consent test covers one locale |
| Saying an outcome: notification, or a box on Windows | two tests with the notifier and the box **faked** | CI all three | — | a real notification on any platform; a real box |
| The tray and the watcher picking up a link | none | — | — | **all of it** |
| The Windows scheme in the registry | `TestURLSchemeRoundTrip`, under a **scratch** key | CI Windows | — | the real `Software\Classes\liro-bridge` key; registering at start; removal by `uninstall-notice`; **a browser opening a link** |
| The MSI removing the key | built and opened by Windows Installer | CI packaging | — | **never installed or uninstalled** |
| The Linux link desktop entry | installed and removed on Ubuntu, Debian and Fedora; two lines checked | CI linux-install | — | `desktop-file-validate`; the MIME cache; a desktop session; a browser |
| Settings: the list of allowed servers, Remove | rendered **empty** in WebView2 | CI Windows | — | a non-empty list; the Remove action |
| `cloud-origins.json` on uninstall | it was in neither list, so the notice did not name it. **Found by the inventory and fixed now (D-468)** | CI Windows (the list test) | — | the notice shown to a person |

## B-LT (D-462)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| Evidence for the signer's chain and the TSA's; Unknown not evidence; delegated responders; CRL checked against its issuer; revoked signer → `CERT_REVOKED`, nothing saved; revoked TSA keeps B-T | `blt_test.go`, `dss/gather_test.go` — **stand-in OCSP/CRL/TSA servers on loopback** | CI all three | not recorded per test | **any real Serbian responder, CRL or TSA**; stale evidence "embedded and reported" (only "asked once more" is tested); a revoked intermediate; revocation by CRL end to end; a second B-LT signature merging `/DSS` end to end |
| The Trusted List as anchor, Pošta's shape (no intermediate on the token, LDAP AIA) | `trustlist_blt_test.go`, **new (D-466)** | container; CI from the next run | **yes**: the control without the list stops at B-T | **the Pošta card** |
| The real-TSA test | `TestSignDocumentBTWithRealTSA` reaches Pošta's **test** TSA. It produces B-T and runs no B-LT code | CI, where a green job before this change meant the server was reached (inferred: no `-v`); here it now skips | its two paths: **yes** (D-465) | — |
| Presets removed from Settings | `TestSettingsOffersNoTimestampService` | CI Windows | — | Settings' height not re-measured |

## C26 — a level asked for is reached or refused (D-469)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| The library refuses by default: no timestamp → `TSA_*`; B-LT short → `LEVEL_NOT_REACHED` | `TestSignDocumentAbortsOnTSAFailureByDefault` (now really the default), `TestABLTThatFallsShortIsRefusedUnlessTheCallerAcceptsBT` (with an unrevoked control) | container; CI all three from the next run | the B-LT refusal: control in the same test | — |
| B-T below a set B-LT shown as a warning, with a sentence; B-B not offered after a failure or against a named level; a named level refused below | `levelshortfall_test.go` (pure rules); the Windows TSA-choice test now checks the button is hidden | container; CI all three; the button on CI Windows | **yes**: the old rules fail the new tests | **with a card or a screen**: worklist 1.2, 1.3, 1.5 |

## F13 — macOS (D-463)

| change | test | where | seen failing without it | never run |
|---|---|---|---|---|
| The main thread, the window, `liro://` assets, messages both ways, navigation refused | `TestAMacWindowLoadsAPageAndCarriesMessagesBothWays` | CI macOS (`-v`: ran) | — | the person closing the window; owner windows; load errors. Resize and navigate are checked only for "no error" |
| The window's photograph | taken in the suite step | CI macOS | — | **nobody has looked at it**; the test passes even if no picture is taken |
| The Keychain | `TestTheKeychainStoreRoundTrips` (a test service name) | CI macOS (`-v`: ran) | — | the production constructor; the real service name; the prompt after an update |
| The LaunchAgent file | two tests; `plutil` lint on macOS | CI all three | — | `launchd` ever reading it; the darwin constructors |
| PKCS#11, shared with Linux | four real-module tests against **SoftHSM 2.7** | CI macOS (`-v`: ran, none skipped) | — | **SafeSign and a card** |
| dyld's load-failure words | `TestDyldsWordsAreReadIntoTheKinds` | CI macOS | — | four of the five wordings are recalled, not seen. The one real case was **logged, not asserted**, while I called it measured. **Asserted from the next run (D-468)** |
| Module paths for SafeSign and OpenSC | discovery tests | CI macOS | — | SafeSign's path is recalled, never read off an install |
| The bundle: plist, scheme, universal binary, `--version`, system frameworks only | the bundle step | CI macOS | yes: the Homebrew link was caught by the check | Gatekeeper; LaunchServices; the x86_64 half ever running |
| File drop, NSAlert, open panel, **PIN dialog**, status-bar item, osascript notifications, an Apple Event link | none | — | — | **all of them** |

## What this table says, in one paragraph

**Measured with a control** (red on the old code, green on the new):
- four of C21's Windows fixes;
- the D23 proxy release (in the container only);
- the SDK's negotiation;
- the D6 examples on Linux;
- the cloud queue's order;
- Pošta's B-LT shape (stand-ins).

**Passing, with no control:** two C21 fixes (Navigate, `FreeConsole`). Their tests pass with and without the change.

**Exercised only against stand-ins:** all of B-LT, all of the cloud protocol, all of PKCS#11 on macOS.

**Compiled and never executed:**
- in the cloud channel: the origin question, the one-window slot, the tray's link pickup, the good-link `open-link` path, Windows scheme registration at start and removal at uninstall;
- three C21 error paths;
- most of macOS's dialogs;
- `Sign.cs`.

**Nothing at all on a real machine of the owner's, with a card, or in front of a person.**
