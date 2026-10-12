# Session 33 — handover

**Written:** 2026-10-12, at the end of the session. **Branch:** `claude/wizardly-clarke-rmvkdt` (PR veljaos/liro-bridge#1, not merged). **Entries:** D-457 to D-482. **Predictions:** `docs/s33-predictions.md`, P1 to P16, each committed before the run it predicts.

**The build for the card rows:** `liro-bridge-windows-446dcd252bda65a68d46a1b79ab0a28aaedbb1e3`, from push run 38291112853, kept until 2026-11-11. Use the per-user MSI; it is unsigned, so SmartScreen warns. Nothing the product runs has changed since that commit: what came after it is tests and documents.

## Read this first: D-480

My code wrote a person's name, and on a MUP card their JMBG, into `bridge.log`. It was there from `4b7c60c` (2026-10-11) until `2ec3055`.
- **Nothing found it.** No test, review or rule caught it, and nothing in the project would have. I found it while writing up something else.
- **It is fixed.** No message in the product now names a certificate except by thumbprint.
- **There is now a check** (D-482) that reads `bridge.log`, notes and errors for personal data. It was red with the old lines and is green now. What it misses is listed in D-482 and in `internal/logcheck`.
- **Whether anyone ran a build carrying it cannot be known.** No tag contains it.

### The old artifacts: yours to delete

You asked for all thirteen heads' artifacts to be deleted. **I could not do it:** this session's tools cannot delete an artifact, and it has no `gh`. There are twenty runs, 13 push and 7 PR, listed by number in D-482. Where `gh` is signed in, this deletes them all:

```sh
for r in 38186918219 38187564063 38187726349 38187982282 38188278943 38189507427 \
         38190929428 38225585797 38233120579 38242378820 38243584334 38246379093 \
         38248820871 38222413736 38225591952 38233125458 38242385215 38243591029 \
         38246385773 38248827713; do
  gh api "repos/veljaos/liro-bridge/actions/runs/$r/artifacts" --jq '.artifacts[].id' |
    while read -r a; do gh api -X DELETE "repos/veljaos/liro-bridge/actions/artifacts/$a"; done
done
```

Then `gh api "repos/veljaos/liro-bridge/actions/runs/38248820871/artifacts" --jq .total_count` should print `0`.

## What is done

| work | entries | state |
|---|---|---|
| C21, Windows memory ownership: every row a test | D-457, D-473 | done. Each test was red on the old code on CI's Windows runner, then green. |
| D39 (sign window hang) | D-458 | cause read, fixed |
| D45 (two discoveries disagree) | D-458 | **inferred, not measured**; stays so (your ruling) |
| D23 (`xdg-dbus-proxy` chains) | D-460 | fixed, red and green under Xvfb |
| D6 (SDK examples) | D-459 | closed |
| B-LT through the Trusted List | D-462, D-466 | built; **only against stand-ins** |
| The cloud channel (F14) | D-461, D-473 | built; run end to end against a stand-in Liro Business only |
| macOS (F13) | D-463 | written blind, compiled and tested on CI's Mac; signing waits (D-467) |
| C26: a level asked for is reached or refused | D-469 | built |
| The default level is "the best available" (decision C) | D-474, D-476, D-478, D-479 | built; SPEC §12.6 amended |
| What the log says about a level not reached; MSIs published per commit | D-477 | built |
| No certificate named in any message; the log's personal-data check | D-480, D-482 | built |
| Row 1.1 with the test timestamp server | D-481 | answered: one B-T signature decides it (row 1.4) |
| E10: the SPEC, wrong three times out of three where we checked | D-476, D-480 | on the list, before v1 |
| LDAP | D-466 | not to be built (your ruling) |

## What has never run

- **Nothing in this session ran on a real machine of yours, with a card, or in front of a person.** That is groups 1 to 5 below.
- **Against stand-ins only:** all of B-LT, the whole cloud channel, and PKCS#11 on macOS.
- **Read, not run:**
  - the line that carries decision C into a real batch (`mainwindow.go`, `keepBTIfBLTFails: !m.levelRequested`), and the two calls that pass it to the timestamp question (rows 1.2 and 1.3 measure it);
  - the protocol path's call that logs the signer's notes.
- **Linux windows on CI:** every Linux window test skips there (no display, and bubblewrap gets no user namespace). They ran only in this container, under Xvfb.
- **macOS:** the PIN dialog, a file drop, the open panel, alerts and notifications are compiled, never run.
- **`Sign.cs`:** never run.
- **The log check's blind spots** (D-482): the paths it does not drive; names it was not given; a JMBG glued to a letter a–f or split; email addresses, file names and document contents.
- `docs/s33-what-ran.md` has the change-by-change table. Its closing summary predates group 0.

## The worklist as it stands: 20 rows to do, card rows first

Full text, and what would show each row false: `docs/measure-before-release.md`.

### 1. A card, on Windows — 7 rows

| # | what it needs | the claim |
|---|---|---|
| **1.4 — do first** | Pošta card, the test server | B-T verified by DSS and Adobe. **Then read the timestamp unit's issuer out of the PDF: it decides whether 1.1 needs the contract.** |
| 1.1 | Pošta card, an authority the Trusted List anchors | A Pošta card reaches B-LT through the bundled list, with no LDAP. |
| 1.2 | MUP card | At the default, B-T with no warning. With B-LT chosen in Settings, refused with nothing saved. |
| 1.3 | MUP card, a calling program | A named `b-lt` is refused, and nothing comes back. |
| 1.5 | any card, a dead TSA URL | Only *Configure* and *Cancel* are offered mid-batch. |
| 1.6 | any card | Closing the signing window under the PIN dialog ends it as a cancel. |
| 1.7 | the NetSeT machine | D45 is a probe timeout (inferred today). |

### 2. A screen, on Windows — 3 rows
- 2.1: the origin window and the consent step fit, whole, in WebView2.
- 2.2: Settings with saved servers fits, and Remove works.
- 2.3: a failed cloud link is said in a box.

### 3. A server, the cloud channel — 3 rows
- 3.1: a link clicked in a browser reaches the agent.
- 3.2: the origin is asked once, and results go only to the job's own origin.
- 3.3: eight links queue in order.

### 4. A VM, Linux — 3 rows
- 4.1: no `xdg-dbus-proxy` chain is left after a window closes.
- 4.2: the browser hands the link to the agent.
- 4.3: the Linux window measurements hold on a real desktop.

### 5. A Mac (outside v1) — 4 rows
- 5.1: the window CI photographed shows the page.
- 5.2: the bundle opens and runs as a menu-bar agent.
- 5.3: the PIN dialog, drop, open panel, alerts and notifications work.
- 5.4: SafeSign loads and a Pošta card signs.

**Group 0 (CI, mine) — 7 rows, all done.** 0.7's wiring was read, not run.

**Totals:** 27 rows. 7 done; **20 to do**: 7 card, 3 screen, 3 server, 3 VM, 4 Mac.

## Your decisions pending

- **The Pošta contract:** after row 1.4 shows the test unit's issuer.
- **Deleting the old artifacts:** the command above.
- **Whether to add a static rule over every log call:** it is what would cover the check's first blind spot.
- **macOS signing:** still waits on group 5 (D-467).
- **Left for E10:**
  - Settings opened from the timestamp question during a calling program's batch shows that program's level.
  - `consent.level_blt_short` is no longer reachable.
