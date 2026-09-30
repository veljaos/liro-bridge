# F12 Linux — session 16: dev.13 across both VMs

**What this is:** the handover at the end of session 15, which built
D-408's options 1 and 4 on the Ubuntu VM (D-410). The next step is
measurement, with the owner's hands, on both VMs. **Written:** 2026-09-30.
**Working rules:** session 13 §C, session 15 §E, and §E below. D-304's
five questions before believing any check.

---

## A. dev.13

**What it carries: D-410 and nothing else.** The chooser helper, the
`text/uri-list` drop, the bounded waits, the four sentences. D-407's
first-start change is held for dev.14 (D-410, the owner's ruling).

Built on this VM with `build/linux/build.sh 0.9.9-dev.13 dist/linux`
from a clean tree at the commit named below; `dist/linux/` is ignored by
git, so the packages exist only on this VM:

```
322f732998b13cf05b06c950e9e45ef303772fb96481758023a03b4b8471afc1  liro-bridge_0.9.9-dev.13_amd64.deb
666193b35ab41be1a658b643d2660f1dca86f3f8742c5524103483b84bd66aa5  liro-bridge-0.9.9-dev.13.x86_64.rpm
```

One `build.sh` run made both (2026-09-30 20:53), at `2a46931`: the binary
says `liro-bridge 0.9.9-dev.13 (commit 2a46931, …)` and records
`vcs.modified=false`. What was read of it here is §F.

**The rpm reaches Fedora on a USB stick**, the owner's route, and its hash
above is the check at the other end. No Go toolchain on the Fedora VM.

**One change per boot on each machine** (D-391). The install is the
change; the readings below are taken in the boot after it, checked against
dev.12's own file, not against what is installed.

---

## B. The Ubuntu VM, first, while dev.12 is still installed: B27

**The question:** does Ubuntu's portal refuse the non-dumpable window
process today, and which chooser does Ubuntu then show? D-408 predicted
"refused, then GTK 4.14's own dialog". Its log prediction failed (0 of 93
starts, D-410), and an absence in a log that keeps only some levels is not
an answer. **The owner's condition: a control that can show the line when
there is one.**

**The instrument is the bus, not GTK's log.** A monitor sees the portal's
`AccessDenied` reply whether or not GTK logs it:

```
dbus-monitor --session \
  "type='method_call',interface='org.freedesktop.portal.FileChooser'" \
  "type='error'" "type='method_return',sender='org.freedesktop.portal.Desktop'"
```

**Its control, first, and it can fail:** a dumpable caller's `OpenFile`
seen by the same monitor, answered with a request handle, and a real
portal dialog on screen (the owner cancels it):

```
gdbus call --session --dest org.freedesktop.portal.Desktop \
  --object-path /org/freedesktop/portal/desktop \
  --method org.freedesktop.portal.FileChooser.OpenFile '' 'B27 control' '@a{sv} {}'
```

Then the reading: the owner opens Liro Bridge (with the dev.12 tray
running, `open` hands over and the tray's window opens) and presses
**Izaberi…**, then cancels whatever appears.

| | predicted |
|---|---|
| control: `gdbus`'s `OpenFile` | a `method_return` with an object path; a GNOME dialog titled "B27 control" |
| the tray's `OpenFile` | **an `error` `org.freedesktop.DBus.Error.AccessDenied`, "Unable to open /proc/2402/root" or the tray's PID then**, within the same second |
| what the owner sees | GTK 4.14's own file dialog, not the portal's (by D-408's reading of `portal_error_handler`) |
| **least certain** | that GTK 4.14 calls `OpenFile` at all: it may decide against the portal earlier (the settings read at start is refused too) and show its own dialog without asking — **then the monitor sees no call from the tray, which is an answer, not a failed instrument, because the control showed the monitor seeing one** |

## C. The Ubuntu VM: dev.13

The owner installs: `sudo apt install ./liro-bridge_0.9.9-dev.13_amd64.deb`,
then **reboots**, so that the tray autostart starts is dev.13 and the
boot's only change is the install. After the reboot: session 12 §C's first
read (the tray's exact PID with `ps -C liro-bridge -o pid,ppid,lstart,args`;
`--version` says dev.13).

### C1. Wayland (the "Ubuntu" session)

The same readings on each machine; the table is written once, here, and
§D points at it.

| | what | predicted |
|---|---|---|
| R1 | **Izaberi…** | the **portal's** dialog, titled "Izaberite PDF dokumente", "PDF dokumenti" chosen; modal to the window (the window behind cannot be clicked); choosing `blank.pdf` puts it in the list; nothing in `bridge.log` about the chooser |
| R2 | while R1's dialog is open | `ps -C liro-bridge -o pid,ppid,lstart,args` shows `liro-bridge file-chooser` whose parent is the window's process; `tr '\0' '\n' < /proc/HELPER/environ` is **one line**, `DBUS_SESSION_BUS_ADDRESS=…`; `readlink /proc/HELPER/cwd` is `/`; `readlink /proc/WINDOW/root` is **refused** (D-376 holds for the window) |
| R3 | cancel | nothing said; the helper gone (`ps`, exact PID) |
| R4 | **Promeni…** | a folder dialog starting on the current output folder; choosing one changes it |
| R5 | a PDF dragged from Files | in the list; nothing logged |
| R6 | **close the window with its corner X while R1's dialog is open** | **least certain**: the dialog goes away (`Request.Close`), the helper is gone within 2 s; the window's process: on Ubuntu the tray stays, on Fedora `open` exits |
| R7 | Settings → export the audit log | a folder dialog; the export as before |

### C2. Xorg (the "Ubuntu on Xorg" session)

The owner picks **"Ubuntu on Xorg"** at the gear; GDM remembers it, so the
login after C2 picks **"Ubuntu"** again, explicitly (D-403). R1, R2 and R6
again. Predicted: the same, with the dialog modal to the window through
`x11:` and the X id; **least certain here is the modality**, because
the X id is found through `dlsym` rather than the binding (D-410).

## D. The Fedora VM: dev.13

The owner copies the rpm (§A's hash checked at the other end) and installs:
`sudo dnf install ./liro-bridge-0.9.9-dev.13.x86_64.rpm`. **Then the
reboot question is the owner's**: the autostart entry D-407 wrote starts
`liro-bridge tray` at the next login, so a login after the install is also
F8's reading (session 15 §D1) and needs a logout proven to be one.
Proposed: no reboot; no Liro process is running (session 15 §C), so the
owner opens Liro Bridge from GNOME Shell and `open` runs dev.13 with no
agent behind it, as in D-407.

R1–R7 as in §C1. **What D-408 found, reversed, is the prediction**: the
dialog opens, Promeni… opens, the corner X ends the process, a drop from
Files arrives. And one line D-408 read is expected to stay: "Failed to
read portal settings … Unable to open /proc/PID/root" from the window's
process, which still asks for settings and still falls back to GSettings.

## E. Rules added in session 15

- **Output that may be needed is written to a file, never piped into
  `tail`.** A hang's stacks went into `tail -15` and were lost (D-410).
- **Every test run carries `go test -timeout` below the tool's limit**, so
  a hang prints its stacks and ends instead of being moved to the
  background.
- **A daemon a test starts carries `Pdeathsig`**; `t.Cleanup` does not run
  when a timeout kills the test binary, and twice a private `dbus-daemon`
  outlived it.

## F. What was read of dev.13 before it leaves this VM

| | predicted | read |
|---|---|---|
| glibc | within the floor, 2.39 | needs 2.34 (`build.sh`'s own check) |
| **`NEEDED`** (least certain: `gtk4-wayland`'s pkg-config puts `-lwayland-client` on the link line) | **the same 13 libraries as dev.12**, because Ubuntu's linker defaults to `--as-needed` | **identical to the installed dev.12's**, 13 entries; no `libwayland-client`, no `libX11` |
| new dynamic symbols | all defined by GTK 4.14.5 and its GObject, the floor | 20 new, all defined; a made-up name is not found (the control) |
| `rpm -K` | digests OK, no signature | **not read**: `rpm` is not installed on this VM. Read on Fedora before the install, as D-405 read dev.12's |

**An instrument failure of mine, in this table's third row**: the first
symbol check matched names with `nm -D` over `libgtk-4`, which lists the
library's own undefined references as well as its definitions, so a
symbol GTK only uses (`g_type_check_instance_is_a`, GObject's) reported
"defined". That check could not fail. Re-read with `--defined-only`
against both libraries, with a made-up name as the control.

What GTK 4.22.5 on Fedora exports is not read here. Predicted: all 20;
the first sign otherwise would be dev.13 refusing to start there at all
(`liro-bridge --version`), which is the first thing to read after the
install.
