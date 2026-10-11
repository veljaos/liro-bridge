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
