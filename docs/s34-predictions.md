# Session 34 (Windows, the owner's machine) — predictions, written before each measurement

Each block is committed before the run it predicts, so git dates it. The
least certain prediction of each block is named. Results are not written
here; they are in the decision entry that reads the run.

**Read before any block (2026-10-22, not predictions):**
- **Installed:** v0.9.9-rc1, the per-user MSI of 2026-10-11, its binary built from `4e6fd47` (the tag, `vcs.modified=false`, go1.26.5); its tray is running, PID 13936, from 18:31:10 today; no `liro-bridge://` key.
- **The handover's build is gone.** Run 38291112853 (`446dcd2`), and its PR run 38291116544, have 0 artifacts. So do all twenty runs D-482 listed for deletion. The only Windows package left is `67695f6`'s, run 38297486252, artifact 11770596983, expiring 2026-11-11.

## P1 — is `67695f6`'s package the same product as `446dcd2`? (written before any download or build)

**Between the two commits,** the one non-test Go file that changed is `internal/logcheck/logcheck.go`, and no non-test file imports it. The rest is tests and documents.

**The instrument:** rebuild each commit as `build/msi/build.ps1` does, in a fresh worktree:
- go1.26.5, `CGO_ENABLED=0`, windows/amd64;
- `gensyso -version dev`;
- `go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=… -X main.buildDate=…"`.

Then compare SHA-256.

| | measurement | predicted |
|---|---|---|
| P1a | The downloaded zip's SHA-256 against the API's `digest` | equal |
| P1b | The artifact `.exe`'s build info | go1.26.5, `vcs.revision=67695f68…`, `vcs.modified=false`, `-trimpath` |
| P1c | `67695f6` rebuilt here with the artifact's own commit and build-date stamps, against the artifact `.exe` | **byte-identical**. **Least certain of P1:** Go is reproducible, but `gensyso`'s resource has not been shown deterministic, and a worktree's VCS stamp has not been compared with CI's checkout. If P1c fails, P1d still stands on its own, and the artifact's link to its source rests on P1b alone. |
| P1d | `446dcd2` and `67695f6`, built with identical stamps and `-buildvcs=false` | byte-identical |
| P1e | Control: `67695f6` built twice, the second time with an empty `GOCACHE` | byte-identical. If not, nothing here is reproducible and P1c/P1d mean nothing |
| P1f | Control: `2ec3055^` and `2ec3055` (product code changed: `interactive.go`, `protocolflow.go`, `dss/gather.go`), the same flags | **differ**. If equal, the instrument cannot see a product change |
| P1g | `go list -deps ./cmd/liro-bridge` at `67695f6` for windows, linux, darwin | no `internal/logcheck` |
| P1h | The per-user MSI's `liro-bridge.exe`, extracted with `msiexec /a` (an administrative image, not an install), against the artifact `.exe` | byte-identical |
| P1i | The MSI's `ProductVersion` | `0.0.0`. So installing it over the rc's 0.9.9 hits `DowngradeErrorMessage`: the rc must be uninstalled first, as D-455 found for 0.9.3 |
