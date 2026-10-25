# Handover after session 36 (2026-10-25), for tomorrow

Written at the owner's request at the end of the session. `docs/measure-before-release.md` is the worklist and is authoritative; this file says where it stands, what moved, and what to do first. The entries are D-504 to D-510.

---

## 1. Where the worklist stands

**Group 0 (CI, mine) is now complete.** 0.1–0.6 were done in D-473. **0.7's last unrun part closed today**: its rule — "the default is the best available, and a chosen level is a request" — had been built and tested in a container but never carried into a batch on a card. Both sides have now run: **row 1.2** (D-491, a level chosen in Settings) and **row 1.3** (D-505, a level named by a caller). Each was refused `LEVEL_NOT_REACHED` with nothing saved, and the default stopped at B-T with no warning.

**Group 1 (a card, on Windows) is finished except for two rows, and neither is mine to finish.**

| row | state |
|---|---|
| 1.1 | **Blocked on the owner.** Cannot be read with Pošta's test server at all (D-484, D-486). See §4 |
| 1.2 | **Done** (D-491) |
| 1.3 | **Done** (D-505) |
| 1.4 | **Done** (D-486) |
| 1.5 | **Done** (D-506) |
| 1.6 | **Done** (D-507), carrying two limits written into the row |
| 1.7 | **Not read.** Run five times on the NetSeT machine with no slow run and no probe timing out (D-497); D45 stays inferred |

**Group 2 (a screen) is where the work now is.** 2.1's window half is measured and **false** — that is open item D54. 2.2 has never been rendered with entries at all. 2.3 is half read. **The unrun half of all three is the same missing step** — see §3.

**Groups 3, 4 and 5** are untouched since their last entries: the cloud channel end to end (3.1–3.3) has never run on a real machine; Linux (4.x) waits on a VM; 5.1 is read and 5.2–5.4 are not.

---

## 2. What closed today, and what it cost

**Four measurements, three rows and one leftover:**

- **P9e, Esc on the origin window (D-504).** Esc does nothing — not with another window in front, and **not after a click inside the page**. `GetGUIThreadInfo` says why: the click focuses the host window, never WebView2, so `origin.js`'s listener is never reached. The X was the only thing that worked. **Wider than it looks: eight of our pages register the same "Escape cancels" listener that F5 §5.6 promises, and none had ever been pressed by a person on Windows.** Open item D55.
- **Row 1.3 (D-505).** A paired program named `b-lt`; the card signed, the signature was discarded, nothing was written. The caller is answered **`422 {"code":"LEVEL_NOT_REACHED"}`** — not the `documents`/`failures`/`counts` shape PROTOCOL.md §5.2 shows, because when every document fails no body is built at all. §5.2 needs a sentence.
- **Row 1.5 (D-506).** Two buttons only, so the row holds — **and the screen offering them is titled "Bez vremenskog žiga" and explains the B-B signature it withholds.** Open item D57.
- **Row 1.6 (D-507).** Against a real NetSeT card login, a posted close destroyed the signing window in 76 ms and the PIN dialog cancelled itself **126 ms later**, one watchdog tick. The process exited; nothing was written.

**Two numbers that change earlier records:**

- **D52's per-document cost is wrong after the first document.** Measured: 38.7 s for the first, **18.7 s for the second**. The 20 s difference is MUP's dead OCSP, remembered per batch by `revocationMemory`; the 29.9 MB CRL is not remembered and is fetched again. **E11's and D52's arithmetic for a long batch should use ~19 s, not ~40**, which moves where `MaxLifetime` falls.
- **MUP's root certificate is fine; we were wrong about it** (D-508, correcting D-505). `ca.mup.gov.rs` serves both its CA certificates as **PEM**; we accept only DER. Nothing depends on it — `CompleteChain` discards self-signed roots anyway — but the warning blames the far end, which is how a false sentence about a national CA reached the record. Open item D60. **It also answers a question D-038 left open** and that `CompleteChain`'s own comment still quotes: MUP's listed CA is an intermediate under a separate, unlisted root.

**One defect fixed, not just recorded: D59.** `verifypdf`, `cngprobe` and `p11worker` all printed the signer's subject — on a MUP card, a person's name and their JMBG — to standard output. All three now withhold it and say so; each already printed the thumbprint that identifies the certificate. The rule is a guard (`scripts/guards/subjects_test.go`), not three fixes. D-508, D-509.

**And one self-inflicted red build, D-510.** `51b7f8c` was pushed with seven unchecked `fmt.Fprintf` calls and turned both CI jobs red. Worth carrying forward for the method, not the fault: **three checks in two days had to be re-proved after being changed, and two of them were passing for the wrong reason** — a scripted revert that silently did not match, a guard that went blind when the calls it watched were renamed, and a test that walked its own source file and so could never fail. Every one was caught by reading the file or deliberately breaking the thing under test; none by a green suite.

---

## 3. The first thing tomorrow

**P8's step 3 — allow a server, then read Settings and the box. One link and one click closes the rest of 2.2 and 2.3, and it needs no card.**

Every origin question so far has been **refused**, so `cloud-origins.json` has never existed, and both remaining screen rows are waiting on that one file:

- **2.2** — "Settings with a non-empty list of allowed servers fits in 880 points, and Remove works" — has **never been rendered with entries**.
- **2.3's second half** — "a server that cannot be reached is said in a box" — needs the list written first, because an origin must be allowed before the job is ever fetched (`channel.go`: `Origins.Allow` runs, then `FetchJob`).

**So: send a link whose origin does not resolve, and click *Dozvoli*.** The list is written (2.2's precondition), the fetch then fails, and the box appears (2.3's other half). Then open Settings: the entry is there to be read and removed (2.2). **One sitting, two rows, no card, no display change.**

**Fold into the same window, both owed from today and both free:**

- **Tab** — whether it moves focus into the page is the one cheap thing that might still reach `origin.js`, and D-504 leaves Esc dead for every click that is not on a button. The owner's instruction was to fold it into the next origin window rather than open one for it.
- **The silent 120-second timeout** — still only ever seen by accident (D-500). D-504's window was closed with the X at 116 s, so `originAskTimeout` has never been watched deliberately. Letting one window sit costs two minutes of nobody's attention.

**Write P14 before any of it**, as every block this session was.

**After that, in order of what a person would meet first:** D54's fix needs the owner's design choice (§4 is not it — D54 is listed under §5 below); then group 3, the cloud channel end to end, which has never run on a real machine at all.

---

## 4. The decisions that are the owner's and still unmade

These four were named by the owner for this handover. None is mine to make, and nothing below is a recommendation about which way they go.

### 4.1 Row 1.1 — a timestamp authority the Trusted List anchors

**The blocker is a contract, and it was established by measurement, not assumed.** Row 1.1 claims a Pošta card reaches B-LT through the bundled Trusted List with no LDAP. D-481 set out the two cases; D-486 signed with the card and read the timestamp unit's certificate out of the PDF; **D-484 is the answer: Pošta's test timestamp unit is issued by a test CA the Trusted List does not carry** ("TEST 2018 CA 1 - Novo", seen again today in both row 1.5 outputs as `TimestampChainTrusted false`).

So **row 1.1 cannot be read with the test server at all** — not "is harder", cannot. It needs a timestamp authority whose chain the list anchors; Pošta's production units are listed. **The decision: whether to obtain one, and on what terms.** Until then 1.1 is not a measurement waiting on time, it is a row waiting on a commercial decision — and it is the only row in group 1 left open that is not inferred.

### 4.2 D-025 — the PIN decision, and what it rests on now (open item A38)

**Undecided since 2026-10-24.** D-025 (2026-09-01) ruled that `windowscng` never sets `NCRYPT_PIN_PROPERTY`, nothing in `keysource`/`softtoken`/`signing` carries a PIN, and Windows' own dialog asks — **rejecting** collecting the PIN in our own UI. Its two reasons have both moved since:

- **Reason 1 is refuted through CNG** (D-493): while the agent held the PIN, a process that is not the agent could not sign. So the PIN *is* a boundary between processes on that route. **Its other half — whether the card keeps a verified state a program talking to it directly could use — is the PKCS#11 half, and is still untested** (E10).
- **Reason 2 is half in question** (D-495): our code never holds the PIN, so "cannot leak from our logs" stands; but Windows' audit log shows every PIN-less signature as a verification sent from our own process, so what verifies the card lives in our process after all.

**Today adds one thing worth knowing for it:** D-507 reached a **real PKCS#11 login on this machine** — `preferPKCS11: true`, the tray down, `liro-bridge open`, and our own dialog raised by a NetSeT module for the MUP card. Nothing was typed, so it answers nothing about E10; **but it is the harness E10's measurement would need, and it is now written down and repeatable** (D-507, with `config.json` copied and restored byte-for-byte).

**The decision:** whether D-025 stands on reason 2 alone, changes, or waits for the PKCS#11 half. Then SPEC §6.5, §6.5.1, F2 §2.3 and D-025's own text are brought into line.

### 4.3 D43 — the 0.9.3 release, built and not tagged

**This is the one that is loose in the world rather than in the repository.** Every Windows release since v0.9.1 — v0.9.1, v0.9.2 and v0.9.9-rc1 — **takes the tray down after about half an hour of main window** (watched on the rc: dead 33 min 10 s), and the program writes no line saying it stopped.

**Decided already (D-454):** a 0.9.3 from the `v0.9.2` tag with the callback fix and `debug.SetCrashOutput`, nothing else. **Built (D-455):** `release/0.9.3`, five files, green on its own Windows CI step. **The notes have their line (D-456, `0df5719`).**

**What is unmade is the owner's: the tag and the push** — the tag publishes at once as latest — **and then the watch**, which needs the rc uninstalled first and reinstalled after, on this machine. Nothing blocks it but the decision to do it.

### 4.4 A7 — the icon test on newer Go

`go` on PATH here is **1.27.1 with `GOTOOLCHAIN=auto`**; go.mod wants 1.26.5, and `~/go/bin/go1.26.5.exe` is installed alongside it. On 1.27.1 **the whole suite's only failure is this one test**, with D-308's message to the byte (18579 against 18880). Three answers were offered in D-308 and **none has been chosen**. Installing 1.26.5 got past `check.py`'s refusal and **decided nothing** (D-489).

It is small, and it is the thing standing between this machine and running the suite on the Go that is actually installed.

---

## 5. The open items this session opened

All six are numbered by me and are the owner's to renumber or collapse. D59 is already closed.

| | |
|---|---|
| **D55** | Escape reaches none of our windows on Windows — the page never gets the keyboard. Eight pages promise it (F5 §5.6). **Tab is the cheap unmeasured question** |
| **D56** | The report says where a document went when nothing was written: "0 potpisano" above "Sačuvano u — vraćeno aplikaciji koja je tražila". The output row is unconditional on anything having been signed |
| **D57** | The timestamp question is titled with the one option it withholds, and explains the B-B signature it will not make |
| **D58** | A batch tells `bridge.log` nothing about what it refused, what authority failed, what a person was asked, or **which of two NetSeT modules asked for a PIN**. Three instances in one day |
| **D59** | *Closed.* The three tools in `scripts/` that printed the signer's JMBG, and a guard so a fourth cannot |
| **D60** | We refuse a valid CA certificate because it is PEM; the warning blames the server. Also answers D-038 |

**And A37 gained the cheapest half of its own answer:** golangci-lint v2.13.2 — the version both CI jobs pin — **is already installed on this machine** and needs none of A37's blockers; it runs no tests, writes nothing, touches no registry, opens no window. A lint-only local gate in the windows view is available today. Two caveats are written into the item: the linux view cannot run whole here, and any local gate must pass `--max-same-issues=0` or it under-reports (this session's failure was seven issues reported as three).

---

## 6. The machine, as it was left

- **Tray running: PID 12252**, listening on 17580, `/v2/health` 200. **Its parent is `powershell.exe`, not `explorer.exe`** — I started it, where a logon normally does. D-502's rule stands: **read how the agent was started, every time; do not assume it from here.**
- **Display scale 100 %, text size 100 %.** `HKCU\Control Panel\Desktop\LogPixels` still reads **192**, stale from session 35's 200 % run — it would record this machine as 200 % on its word alone. Read the ratio, not the registry.
- `config.json` is on the default: `signatureLevel` `""`, `signatureLevelChosen` absent, TSA back at `https://test-tsa.ca.posta.rs/timestamp1`, `preferPKCS11` absent. Digest `06FE0F4E…`, 728 bytes.
- **The Row 1.3 pairing was revoked** through the agent's own `api.Pairings.Revoke`; the other six are untouched. Worth noting for whoever needs it: **there is no command-line way to revoke a pairing** — only the Settings window.
- `cloud-origins.json` **absent**, which is exactly what §3 is about. Audit chain at **332**. `bridge.log` carries the session.
- **`Desktop\liro-s35` and `liro-s36` are deleted** — 25 files, 14 inputs (all copies of `testdata/pdfs/blank.pdf`) and **11 real card signatures** carrying the owner's name and JMBG. Checked first: no row rests on any of them, and every row that used one has its reading written out in its entry (D-491 reads row 1.2's `/DSS` out in full). **One gap, stated rather than hidden:** the `/DSS` contents of the two row-1.5 outputs were never read directly — D-506 records the shortfall from the signer's log note instead. Nothing claims it, and it is one signature away.
- **Nothing is pushed by me, ever.** The owner pushes by hand.
