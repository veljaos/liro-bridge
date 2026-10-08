# F12 — session 31: check.sh controlled and run, the timeouts and the pin, block 1 closed; next, block 2 — the Windows pass

**What this is:** the handover after the Ubuntu sitting that ran check.sh's
controls and check.sh itself for the first time (D-447), took A33's last two
pieces (D-449), recorded the owner's reordering of E8 (D-448) and closed
block 1 (D-450). **It is written for block 2, the Windows pass.**
**Written:** 2026-10-08. **Working rules:** session 29 §0 first, then
session 13 §C, session 15 §E, sessions 22–25 §E, session 28 §F, session 29
§F, session 30 §F, and §F below. D-304's five questions before believing any
check.

**The list is `docs/open-items.md`**, which opens with the owner's order —
renumbered by D-448: the Windows pass is block 2 and E8 is last.

---

## A. Where things are left

| | |
|---|---|
| **Windows** | **Nothing of the program has been run on Windows since dev.14** (the owner). The record agrees as far as read: no entry since D-354 (2026-09-23) reads as the program run on a Windows machine; D-436 and D-437 used the owner's Windows machine for Git Bash's gpg only. **Which version is installed there is not in anything read tonight.** CI's `windows` job is the only Windows execution since, and it runs tests, not the installed program. |
| Ubuntu VM | Rebooted at 21:25 UTC before this sitting; **the agent was not looked at**. The rc was installed in session 29 (D-442). `~/s31-predictions.md` holds every prediction of this sitting, timestamped. `dist/ci-helpers` removed after each full run; the tree holds no ignored Go file. |
| Fedora VM | As session 30 §A: the rc installed, the running agent not the rc, the offline update pending its own boot. |
| check.sh | **Controlled and run** (D-447): C1 red with CI's two failures exactly, C2–C7 as expected; green on `8dee60a`, `084b4d4`, `b781030`, `73058c4`. |
| CI | Read to their ends: `8dee60a` run 37846617401 and `72e252e` run 37852060680 (`084b4d4` inside it, no run of its own), both `success`, all nine jobs. **`c2dab76` (the timeouts, the pin and D-449, pushed together): run 37854675685, `success`, attempt 1, all nine jobs, concluded 22:54:14 UTC**, read to its end; `a7bfdbf`, `6cf4f52`, `b781030`, `73058c4` pushed under it, no runs of their own (the query on `c2dab76` the control). **The pin, seen in it**: `ci`, `linux-gui` and `sdk-typescript` ran on the label `ubuntu-24.04` (ba01328's `ci`, the control: `ubuntu-latest`), image `ubuntu-24.04` 20261004.327.1 — the same image as before, as predicted until the 19th. The timeouts show in no log, as predicted; a hung mirror is what would show them. |
| This handover | Its own commit is the next sitting's first reading (session 30 §F.5). |

## B. What this sitting did

- **D-447** — check.sh's controls on Ubuntu: **C1, the tree CI failed on,
  failed here at the same two steps with the same output** — the guard's
  list line for line, the lint's two findings at CI's lines and columns, and
  the lint naming a line that exists only in that tree. C3, C4 (and C2, C5,
  C6 again) as expected. check.sh green on `8dee60a` with CI's numbers and
  the probe's 166 results identical to CI's test by test. **F6, predicted
  before the run and then read: a full run leaves `dist/ci-helpers`, and the
  next run swept 54 packages where CI sweeps 52 under "clean"** — fixed
  (`084b4d4`, refused, control C7).
- **D-448** — the owner's decision: E8 to the end.
- **D-449** — the timeouts (`b781030`) and the pin (`73058c4`), each cited
  as check.sh.
- **D-450** — block 1 closed; the Node 20 actions not in it, by decision;
  what the block cost.

## C. The owner's rulings

1. **C1 is the result wanted: the check can fail.** The caveat — CI never
   linted `cf24a25`, so the lint is compared with its parent's run — stands
   beside it and does not weaken it.
2. **F6 is the better finding**: check.sh drifting from CI on its second
   run, the failure it exists to prevent, found the day it was written; worth
   keeping because it was predicted before the run.
3. **G2 is the controls behaving correctly**, not a miss: C3 and C4 refused
   to accept a refusal for the wrong reason.
4. **E8, the Guide, moves to the end** — after macOS, before v1, all three
   platforms in one pass. The reason against (Linux is fresh now, and will
   not be in ten weeks) heard and overruled (D-448).
5. **The Node.js 20 actions are not part of block 1**: ordinary A33 work,
   no date, the owner's decision rather than an omission (D-450).
6. **Block 1 closed. It took three evenings against the owner's half day,
   out by a factor of six — not because the work was underestimated, but
   because the ground under it had not been checked** (D-450). Read the next
   estimate knowing that.

## D. Block 2, the Windows pass

**D-433's estimate: 8–12 working days, the ownership audit 2–3 of it — the
owner's Windows machine and hands throughout.** Read it with §C.6: nothing
of the program has run on Windows since dev.14, so this block's ground is
the least checked of any. Expect the first day to go on finding out what is
there.

### D.0 First, before any item: what is on the machine

Read, not assumed (session 29 §F.1): the installed version and how it was
installed (`liro-bridge --version`; Apps & features; per-user or
per-machine); whether a tray agent is running and its exact PID; the Go
toolchain, if the tests are to run there, against go.mod's 1.26.5; and
master's last `windows` CI job, read to its end. **Then the owner decides
what to install** — the rc's `.msi` from the release page, or a build of
master — and that install is its own step, read back, before any
measurement rests on it (D-391: one change at a time).

### D.1 The owner's eight

Each as `open-items` has it; the item there is the authority.

| item | what | closes when | needs |
|---|---|---|---|
| **A7** | the icon test that fails on newer Go; three answers offered, none chosen (D-308, D-318, D-320) | the owner chooses, then code | the owner's decision |
| **C1** | the two-window fix on Windows: the Explorer verb changes whenever a tray agent runs; never run on Windows (D-355 §3) | run there and watched | Windows, the owner's hands |
| **C7** | the PKCS#11 certificate chooser in the agent on Windows; status uncertain (D-316, D-318) | run with a card | hardware |
| **D10** | `cmd/liro-bridge` tests write HKCU Run, and an audit append nobody explained (D-266, D-313 §5) | read to its cause, fixed or explained | Windows |
| **D15** | the thinned icon has never reached the owner's installed tray (D-321) | seen in the tray | Windows, the owner's eyes |
| **B14** | icon ink at 20 and 24 px (D-320, D-321) | seen at both sizes | Windows, the owner's eyes |
| **D31's render** | the footer's three doors and Izađi, built for every platform (D-421, D-422), watched on Ubuntu and Fedora; **Windows' render untested** | seen on Windows; R7's doors opened | Windows, the owner's eyes |
| **D33's Windows half** | the caller gets its result at the run's end, not at Završi (D-421, D-422); watched on Fedora (D-423); **Windows not run** | the caller's file on disk with the report still up, watched | Windows, a card, the owner's hands |

### D.2 C21, the memory-ownership audit, against WebView2 and Win32

On Linux gotk4 mishandled boxed values twice (D-385, D-412), and D-412 swept
every call's `transfer-ownership` against the generated body. **The Windows
side — WebView2's COM references, Win32 handles and buffers — has had
nothing comparable.** The same sweep: every COM interface obtained (who
calls `Release`, and exactly once), every handle (`CloseHandle`,
`DestroyWindow`, `DeleteObject` … on every path, the error paths included),
every buffer passed across the boundary (who allocates, who frees, with
which allocator — `CoTaskMemFree`, `LocalFree`, `SysFreeString`). Each
finding fixed or recorded. **Mostly code reading, so it can start without
the owner's hands**; a finding that needs a measurement waits for Windows.
2–3 days of D-433's 8–12. The memory note "gotk4 boxed ownership: check the
GIR" is the Linux half's lesson: compare the annotation (here, the API's
documented ownership) with what the code does, never one alone.

### D.3 D37 and D39 — the two Windows tests that failed once each

Both **recorded, not re-run away** (D-439, session 28 §F.7), causes not
read:

- **D37** — `TestAConsoleSharedWithACallerIsKept`
  (`console_windows_test.go:189`), run 37669944025: "freed=1 freeErr=The
  handle is invalid. attached=0 attachErr=Incorrect function."; the same
  commit passed it minutes earlier.
- **D39** — `TestTheSignWindowIsOnScreenBeforeTheCertificateListIs`
  (`nocard_windows_test.go:110`), run 37833946737 on a docs-only commit:
  `sign` did not return in 60 s after `WM_CLOSE`, and the next 20 window
  tests failed at once with "window is closed" — **the one-window slot never
  released**. That second part is the more interesting: a slot that outlives
  its window is a product question, not only a test's.

**The count (D-445): since 2026-09-23, 65 `windows` runs, 62 green, one
cancelled, two failures of unknown cause in two different tests** — two
single failures, not one flake seen twice, and not a characterised rate.
Every `windows` job read since has been green; that does not lower it.
Both may share C21's ground (handles, a console, a window's lifetime): read
them with the audit, not after it.

### D.4 Placed in block 2 by me (D-433), the owner's to move

**D3** (`lowerLevel` has no caller: a mixed batch may report a level it did
not reach), **A21** (`certs` on Windows says "Certificates: 0" with no
reason; Linux prints the window's sentence — an owner's decision), **D17**
(a Windows window test pins the singular sentence's wording), **A14**
(whether common Windows backups take the audit log under `%LOCALAPPDATA%` —
the owner's decision for Windows).

## E. What is not done that a reader might assume is

- **Nothing of the program has run on Windows since dev.14**; CI's
  `windows` job is tests, not the installed program.
- **The rc is installed on both VMs and running on neither**; on Ubuntu the
  agent was not looked at after tonight's reboot.
- **check.sh does not check a step's timeout or the host's Ubuntu version**:
  the timeouts and the pin are shown only by CI's runs (D-449).
- **check.sh is the Ubuntu host's two jobs only**: `windows`, `packaging`,
  `linux-packages`, `linux-install` and `sdk-typescript` are CI's alone, and
  its summary says so. A Windows check.sh does not exist.
- **Not controlled in check.sh**: the Go-version refusal; `CI=true`'s
  effect; `/tmp/softtoken`, `/tmp/liro-home`, `/tmp/signed.pdf` persisting
  between runs here where a runner is fresh; `XDG_RUNTIME_DIR`, not passed
  to steps.
- **The Node.js 20 actions are still `@v4`** — by decision (D-450).
- From session 30 §E, still true: what gpg prints in Serbian; the rc's
  published page has the old notes.

## F. Rules added in this sitting

1. **A green run hides what nobody counts; a prediction written before it
   is what reads the count** — F6 was a passing run, found only because 54
   was predicted against CI's 52 (D-447).
2. **"Clean" is git's word, and Go sweeps what git ignores** — a check that
   says what tree it ran on must look where the tool looks, not where git
   does (D-447).
3. **A control wants its own reason, not just its exit code** — C3 and C4
   refused for the leftover files and said NOT as expected; a control that
   accepted any exit 2 would have hidden them (D-447, the owner).
4. **`gh run view --log-failed` can print nothing, exit 0, on a failed
   run; `gh` gives step times as null.** The API's job logs and jobs
   listing have both (D-447, D-449).
5. **A check's green says what it can see**: D-449's check.sh runs prove
   nothing else changed, not that the timeouts or the pin work (D-449).

## G. check.sh, as run here

```
env -i HOME=$HOME PATH=$HOME/go/bin:/usr/local/go/bin:/usr/bin:/bin scripts/check/controls.sh   # seven, ~1 min
env -i HOME=$HOME PATH=$HOME/go/bin:/usr/local/go/bin:/usr/bin:/bin ./check.sh                  # ~25 min cold -race cache, ~7 warm
rm -r dist/ci-helpers                                                                          # after every full run; the next refuses until it is gone
```

golangci-lint v2.13.2 is in `~/go/bin`, not on the login PATH. Run a full
check.sh in the background, announced with its length, bounded by
`timeout`.
