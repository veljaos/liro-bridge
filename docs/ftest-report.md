# FTEST — unattended test report

**Date:** 2026-09-06
**Base commit:** `0deb494` (F6b: visual stamp placement, single-window flow, method selection)
**Machine:** the owner's own Windows 11 Pro (10.0.26200), AMD Ryzen 5 4500, Go 1.27.1, real MUP e-ID card in the reader.
**Run:** unattended. Nobody was at the machine.

Written as the work happened, in the order it happened. Numbers are measured
on this machine unless it says otherwise.

---

## 0. The boundaries, and what was actually done about them

| Boundary | What was done |
|---|---|
| No PIN, no signing with the real card | Nothing in this session opened a signing session against the card. Every signature came from the soft token (`testdata/softtoken/local/test.p12`). No PIN dialog appeared at any point. |
| No synthetic input | No `SetCursorPos`, `mouse_event`, `SendInput` or `keybd_event` anywhere. Windows were driven through `Window.Eval` inside their own DOM and through window messages posted to this session's own windows, and photographed with `PrintWindow`. |
| Never kill by image name | Every process this session started was stopped by its own exact PID. |
| Leave the machine as found | Snapshot taken before anything ran (below). Restoration verified at the end, not assumed. |

### 0.1 The snapshot taken before anything ran

```
config.json          locale sr-Latn, level b-b, visibleStamp true, stampPosition custom,
                     stampX 185.37141393442624, stampY 478.0389344262295, stampPlacedPage 1,
                     outputFolder "", explorerMenuEnabled true, startWithWindows true
HKCU\...\Run
  LiroBridge         "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe"
HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign
  (default)          Потпиши користећи Liro Bridge
  MultiSelectModel   Player
  Icon               C:\Users\Veljko\AppData\Local\Liro\icon-13717.ico
  \command           "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe" --shell-verb "%1"
audit log            %LOCALAPPDATA%\Liro\audit\2026-09-001.jsonl, 101 320 bytes,
                     SHA-256 f3d7a5fb917cf336acabed2e011e93e95ce1f9e3662a7425822f1d694d930fe1
```

Copies of all of it are in this session's scratch directory, and the
restoration check is at the end of this report.

---

## What broke

### B-1 — `go test ./...` rewrites the owner's real autostart entry (fixed)

**What it was.** `internal/platform.TestWindowsAutostartRoundTrip` recorded
only *whether* autostart was enabled (`a.IsEnabled()`, a bool) and restored it
with `a.SetEnabled(before, testExePath)`. `testExePath` is
`C:\test\liro-bridge.exe`, a path deliberately chosen not to exist. So on any
machine where autostart was on — this one — one `go test ./...` replaced the
real value with a path to nothing, and the agent would silently stop starting
with Windows.

Measured, not inferred. Before the baseline suite run:

```
LiroBridge = "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe"
```

After it:

```
LiroBridge = "C:\test\liro-bridge.exe"
```

D-134 already recorded this as a known defect in a package that pass did not
touch. FTEST §0.4 asks for it to be fixed, and it is.

**The fix.** `keepAutostartValue(t, keyPath)` snapshots the value *verbatim*
plus whether it was present at all, and puts exactly that back — deleting the
value again if there was none before. `TestWindowsAutostartRoundTrip` uses it.

**The tests that now cover it.** `TestAutostartTestsPutBackWhatTheyFound` seeds
a scratch key with a value that looks like a real installation, runs the
round-trip inside a nested `t.Run` (the only way to observe what a `t.Cleanup`
actually restores), and asserts the seeded string comes back byte for byte.
`TestAutostartTestsDoNotInventAnEntry` covers the other direction: a machine
with no entry must not have one afterwards.

Confirmed to fail against the old shape — the bool-and-`SetEnabled` restore
reintroduced by hand produced

```
autostart value = "\"C:\\test\\liro-bridge.exe\"" after a test run,
want it untouched at "\"C:\\Users\\Somebody\\Desktop\\liro-bridge\\liro-bridge.exe\""
```

— and to pass after. The owner's real value was restored by hand as soon as
the corruption was measured.

**Not in scope of this fix, and checked:** `internal/platform`'s shell-menu
tests already use a scratch key of their own and never touch the real Explorer
verb; `cmd/liro-bridge`'s `keepThisMachinesAutostartAndMenu` already restores
both registrations verbatim. Only this one test was writing to the real one.

---

## What held

### Baseline suite

`go test ./... -count=1 -tags softtoken` on this machine, before any change:
**28 packages, all green.** Slowest: `cmd/liro-bridge` 47.0 s,
`internal/pades` 19.6 s, `internal/ui` 16.5 s, `internal/jobs` 11.9 s,
`internal/pades/tsa` 9.7 s (it contacts Pošta's real test TSA).

---

*(this report is written as the work happens; sections below are appended)*
