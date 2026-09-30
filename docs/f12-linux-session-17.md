# F12 Linux — session 17: dev.14, the drop and `load-failed`

**What this is:** the handover at the end of session 16's first sitting
on the Ubuntu VM with dev.13 (D-412). The next step is code, begun fresh,
by the owner's ruling. **Written:** 2026-09-30.
**Working rules:** session 13 §C, session 15 §E, session 16 §E, and §F
below. D-304's five questions before believing any check.

---

## A. Where things stand

**dev.13 is installed on the Ubuntu VM and must not go to the Fedora VM.**
Its drop kills the agent (D-412), and the rpm carries the same code. The
rpm in `dist/linux/` stays where it is.

On the Ubuntu VM: dev.13 installed, **not running** — the tray (2467) died
of SIGSEGV at 22:02:04 on R5's drag and nothing restarted it;
`/run/user/1000/liro/bridge.json` is stale. **The next login's autostart
starts dev.13's tray.** `config.json` `outputFolder` is `""` again (R4c).
Nothing else changed on the system.

On the Fedora VM: unchanged since session 15 §C — dev.12 installed, no Liro
process running.

## B. dev.14: what it carries, the owner's ruling "both"

1. **The drop (open-items D25).** `internal/ui/drop_linux.go`:
   `gtk.NewDropTargetAsync(nil, gdk.ActionCopy)` then `target.SetFormats(…)`.
   `gtk_drop_target_async_set_formats` is `transfer none` in the GIR and
   gotk4 keeps the Go object alive across the call (`gtk/v4/gtk.go`
   49106–49111), so GTK takes its own reference.
   - **The test**, in a child process under `MALLOC_PERTURB_` (so freed
     memory is overwritten, not left readable by luck): build the target
     through the same function the window uses, drop the Go reference to
     the formats, force the collector and finalizers, ask
     `target.Formats()` for `text/uri-list`. Predicted: on dev.13's code the
     child crashes or answers false; with the fix, true. Say first whether
     a GTK event controller can be made without `gtk_init` here, or what
     the test needs instead.
   - **Mutation**: the constructor given the formats again must turn it
     red. If it cannot, the test is not evidence, and D-412's cause stays
     a reading of the source.
2. **`load-failed` (open-items D26).** Connected in
   `internal/ui/webkitjs_linux.c` beside the JavaScript evaluation: a C
   handler that copies the error's domain, code and message and hands only
   those to Go. No binding touches WebKit's `GError`. Its test needs a load
   that fails; if it cannot run under `go test` here (open-items C5), it is
   recorded as unrun, not claimed. **Also read, before writing**: when
   `load-failed` fires in this program — a failed first load, a load
   interrupted by closing the window, a navigation refused by
   `decide-policy`?
3. **D-407's first-start change: D-410 held it "for dev.14".** Whether it
   still rides in dev.14 beside two crash fixes, or waits for dev.15, is the
   owner's — ask before writing it. One change per build is the reason it
   was held.

Local green in both views (D-368), each run under `go test -timeout` below
the tool's limit, output to a file. Lint in both GOOS views. Then
`build/linux/build.sh 0.9.9-dev.14 dist/linux` from a clean tree, the hashes
recorded here in the next handover, `NEEDED` compared with dev.13's.

## C. Where R1–R7 stand (session 16 §C1)

| | dev.13, Ubuntu, Wayland (D-412) | on dev.14 |
|---|---|---|
| R1 Izaberi… | **held** — the portal's dialog, modal, no refusal, `blank.pdf` in the list | once, to show the chooser is unchanged by the build |
| R2 helper while open | **held** — one-line environment, cwd `/`, the window's `root` refused | with R1 |
| R3 cancel | **held** — silent, helper gone | — |
| R4 Promeni… | **held**, in three parts (a folder chosen, started on, restored) | — |
| R5 drag from Files | **failed: the agent died** | **first**, the reason for dev.14 |
| R6 corner X with R1's dialog open | **not taken** | yes; needs the tray running, which the reboot after the install gives |
| R7 Settings → audit export | **not taken** | yes |
| C2 Xorg (R1, R2, R6) | not taken | after C1 |
| §D Fedora | not taken — **not with dev.13** | with dev.14's rpm, its hash checked there, `rpm -K` read first |

**R5's predictions for dev.14** (carry and revise): `blank.pdf` in the list,
no sentence; one `signing window: documents added` line in `bridge.log`;
no `Failed to receive drop data`; no `FileTransfer`, `Documents` or
`FileChooser` call from the window's process; the tray alive after the drop
and after a second drag. **Least certain**: whether Files itself calls
`FileTransfer.StartTransfer` because it offers
`application/vnd.portal.filetransfer` — a call from Files is not a call from
us; map the sender. **Let the window stay open a while before dragging, and
drag more than once** — dev.13's crash needed the collector to have run.

## D. The monitor, kept this time

D-411's command was not kept; this is D-412's, run by the owner in their
own terminal, to a file:

```
dbus-monitor --session \
  "type='method_call',interface='org.freedesktop.portal.FileChooser'" \
  "type='method_call',interface='org.freedesktop.portal.Request'" \
  "type='signal',interface='org.freedesktop.portal.Request'" \
  "type='method_call',interface='org.freedesktop.DBus.Properties',destination='org.freedesktop.portal.Desktop'" \
  "type='method_call',interface='org.freedesktop.portal.FileTransfer'" \
  "type='method_call',interface='org.freedesktop.portal.Documents'" \
  "type='error'" \
  "type='method_return',sender='org.freedesktop.portal.Desktop'" \
  > /tmp/s16-bus.log 2>&1
```

Its control for an absence on `Documents`/`FileTransfer`: one
`gdbus call … org.freedesktop.portal.Documents.GetMountPoint` (read-only),
seen by the monitor, before the drag.

## E. The first reads after dev.14's install and reboot

As D-412's, which worked: `uptime -s`, `last -x -F`, logind in `-b 0`; the
install in `apt/history.log` and nothing after it in `dpkg.log`;
`/usr/bin/liro-bridge`'s sha256 against the binary inside dev.14's `.deb`
(`dpkg-deb -x` into the scratchpad) and dev.13's; the tray by
`ps -C liro-bridge`. `/proc/PID/exe` is refused (non-dumpable), so the PID
is tied to the file by start time after the file's ctime, and by
`bridge.log`'s start line followed by `StatusNotifierItem-PID-1`.
**`liro-bridge --version` writes a start line to `bridge.log`**
(`main.go:176`); say so when reading the log after it.

## F. Rules added in session 16

- **gotk4's ownership, the class (D-412).** For any gotk4 call with a boxed
  argument, a boxed return, or a signal carrying one, read the GIR's
  `transfer-ownership` (`/usr/share/gir-1.0`) against the generated body
  before relying on it. Objects are handled by construction in v0.3.1;
  boxed values are where it went wrong, twice (D-385, D-412).
- **Pause with the dialog open to read the helper by PID.** R4a's helper
  was not read because no pause was given, and its connection was gone
  before it was mapped.
- **Read a log from a line number, not a time pattern.** R1's `21:4[3-9]`
  could not match 21:50 and a line was reported absent.
- **An absence on the bus needs a positive control on the same rule** —
  shown this session for `Documents` and `FileTransfer`.
- **After a crash, read before touching**: processes, `bridge.log`, the
  journal by `_PID`, `journalctl -k`, `/var/crash`, the discovery file,
  orphans. Go's crash output goes to the journal, not `bridge.log`.
