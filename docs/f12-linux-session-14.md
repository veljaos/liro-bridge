# F12 Linux — session 14, the first on Fedora: across the update

**What this is:** the first session on the Fedora 44 VM (D-405). §D's reads
were taken on the machine as built; GNOME Software had staged an update of
835 packages by itself, and the owner decided to apply it before anything is
installed. This document carries the reads across that reboot, because the
Claude session that wrote it ends with it and `/tmp` is a tmpfs.

**Written:** 2026-09-29, before the owner's Restart & Install.
**Working rules:** `docs/f12-linux-session-13.md` §C. D-304's five questions
before believing any check.

---

## A. What the owner does

1. GNOME Software → **Restart & Install**. The machine boots into the update,
   applies it, and reboots again.
2. **Watch the update screen**: say whether it finished or showed an error,
   and roughly how long it took.
3. **At the greeter, before logging in**: choose the user, then the gear, and
   say what it offers. Session 13 §D1's last unread prediction.
4. Log in to **GNOME** (not GNOME Classic), open a terminal, start Claude.

## B. What is read first, before D0 is repeated

Predictions written before the reboot. **Least certain marked.**

| read | how | predicted |
|---|---|---|
| a reboot, two of them | `uptime -s`; `last -x -F \| head`; `journalctl --list-boots \| tail -4` | two boots after 2026-09-29 23:13; the update boot and the login's; `shutdown`/`reboot` records, not `crash` |
| the update ran and succeeded | `journalctl -b -1 -u dnf5-offline-transaction -o cat \| tail` | a completed transaction, no error |
| dnf's record | `dnf history list` | a third transaction, from the offline update |
| nothing left staged | `ls /system-update /usr/lib/sysimage/libdnf5/offline /var/lib/dnf/offline/packages` | all absent or empty |
| **the kernel (least certain)** | `uname -r` | `7.2.7-200.fc44.x86_64` — `GRUB_DEFAULT=saved`, and `/boot/loader/entries` is root's, so this is not read |
| the packages | `rpm -q gnome-shell mutter gtk4 webkitgtk6.0 bubblewrap xdg-dbus-proxy selinux-policy-targeted glibc pcsc-lite pcsc-lite-ccid` | 50.5, 50.5, 4.22.5, 2.54.0, 0.12.0, 0.1.8, 44.10, 2.43-8; pcsc not predicted |
| SELinux | `getenforce` | `Enforcing` |
| automatic downloads stayed off | `gsettings get org.gnome.software download-updates` | `false` |
| nothing staged again | the `offline` paths above, after a few minutes logged in | nothing |
| session | `loginctl show-session <tty2 session> -p Type` | `wayland` |
| the gear | the owner's eye | GNOME and GNOME Classic, no Xorg entry |
| D2 | session 13 §D2's paths; `ps -C liro-bridge` | all absent, no process |

Then **D0 again**, whole (session 13 §D0), and D1's installed half, since an
update could in principle add a session file: `ls /usr/share/xsessions
/usr/share/wayland-sessions`, `rpm -q xorg-x11-server-Xorg`. Predicted
unchanged.

## C. Then the install

Approved by the owner, unsigned, on the hash (D-405). Check the hash again
first: the file is `~/liro-bridge-0.9.9-dev.12.x86_64.rpm`,
`9730958307c5e46f0969bbd8cb7622d9054fcc23f04b4115ad1919ab4b1f4542`.

The owner runs `sudo dnf install ./liro-bridge-0.9.9-dev.12.x86_64.rpm` and
pastes the whole transaction. That transcript is **B15's reading**.

| | predicted |
|---|---|
| what dnf fetches | nothing but `liro-bridge` itself: every declared requirement was provided before the update, and the update removes none (re-check with `rpm -qpR` first) |
| what is typed | the sudo password and `y` |
| **least certain** | whether dnf5 accepts an unsigned **local** rpm without a question or a refusal |
| metadata | a root refresh of all seven enabled repositories, four of them third-party; recorded, not a thing typed |

After the install, **stop and write the next predictions** (session 13 §D3):
what starts the agent on a desktop with no tray, what a person sees, and the
first window are session 13 §E's first item.

## D. What changed on this machine in session 14

- `org.gnome.software download-updates` `true` → `false`, 23:13:10, as the
  user, the owner's approval. Owed back at the end: open-items F10.
- Claude's memory on this VM: one note pointing to session 13 §C.
- The staged update: applied by the owner (§A), not by this session.
- `gh` 2.97.0, `dnf history` #3, 2026-09-29 23:21 — the owner's, for `gh auth
  login`; not recorded here when it was made (D-406).
- dev.12 installed by the owner, 2026-09-30 20:23:57, `dnf history` #5
  (D-406, B15).

---

## E. After the reboot (2026-09-30)

Read and recorded in D-406. In short: three boots, not two, and the third is
the owner's; the transaction completed and its unit exited 1 in a race with
the reboot; 2002 of 2003 packages carry Fedora's signature, and whether dnf
checked them can no longer be established; `gh` was an unrecorded change;
every other §B read held; D0 and D1 unchanged. B15 answered: nothing fetched,
nothing typed but the password and `y`.

## F. The first start: predictions

Written 2026-09-30 after the install, before anything of this program has
run. From the package (`rpm -ql`, `rpm -q --scripts`) and the code
(`cmd/liro-bridge/open.go`, `startup_other.go`, `mainwindow.go`,
`internal/platform/autostart_linux.go`), not from this machine.

**What starts the agent: nothing, yet.** The package installs no autostart
entry and runs no scriptlet. The agent is `liro-bridge tray`, and the only
thing that will ever start it by itself is the autostart entry the program
writes on its first run. So at this login nothing of Liro is running (read:
20:24:57, `ps -C liro-bridge` empty).

**The route a person takes**: Activities, type "Liro", click **Liro Bridge**.
That runs `liro-bridge open`. The owner's hands (D-094).

| read | how | predicted |
|---|---|---|
| the search finds it | the owner's eye | **Liro Bridge** with its icon, from `liro-bridge.desktop` |
| one process | `ps -C liro-bridge -o pid,ppid,lstart,args` | one, `liro-bridge open`; its parent recorded, not predicted |
| the window | the owner's eye | one window, **Liro Bridge**, in Serbian Latin (default locale `sr-Latn`), at the empty document list |
| Wayland-native | `xlsclients`/`xwininfo -root -tree` if installed, else say which route | no X11 window of this program (SPEC §6.5.2) |
| WebKit under it | `python3 scripts/proctree/proctree.py PID` | `WebKitNetworkProcess`, `WebKitWebProcess`, `bwrap`, `xdg-dbus-proxy` below the `open` pid; D23's count taken with the version beside it (2.54.0) |
| **the sandbox under SELinux (least certain; B8)** | the web view draws; `proctree` shows `bwrap`; denials need `sudo ausearch -m avc -ts <time>` — owner's hands | starts; `unconfined_t`, and Fedora's own browser uses the same sandbox. No policy for this program exists to confirm it |
| autostart written | `cat ~/.config/autostart/liro-bridge.desktop` | appears: `Exec=/usr/bin/liro-bridge tray`, `X-GNOME-Autostart-enabled=true` (`startWithWindows` defaults on). **D2 stops being true here** |
| other Liro paths | D2's list, plus `~/.cache` | recorded as found; which appear is not predicted |
| **no agent behind this window** | `ls $XDG_RUNTIME_DIR/liro`; `ps` | no discovery file, nothing listening: `open` runs a window, not the agent (`runMainWindow` passes no stop, and `liveAgent` found none to hand over to) |
| closing it | the corner X; then `ps`, `proctree` | the process and its WebKit children end; nothing of Liro left running; the autostart file stays |

**What that last row means, written before it is read.** On a desktop with no
tray, a person who installs the package and opens it from Activities gets a
window, and **no agent until their next login** — so a web page cannot reach
Liro in that first session. D-342 says the agent runs and says nothing on
such a desktop; it did not say how it comes to be running the first time.
If the row holds, bring it to the owner as a question about the product
(F12 §6, §8), not as a measurement.

**After that, and not in the same boot's change**: a logout and login, proven
as one (session 13 §C), should start `liro-bridge tray` from the autostart
entry, with no icon anywhere and nothing said (D-342, `StatusNotifierWatcher`
absent). That is its own prediction sheet, for the owner to schedule.

## G. How the agent first comes to be running: the options (D-407)

Brought to the owner, not decided. What is known and what is only read from
the code is said for each.

**1. The window starts the agent itself.** When `open` finds no live agent,
it becomes the agent: `runTray`'s path, opening the agent's own window at
once instead of waiting for a handover. The pieces exist and run on Ubuntu:
the agent already opens its own window (`openAgentWindow` →
`runAgentWindow` with `quitAgent`), and a second launch already hands over
to it (F12 §7.1). Closing the window then leaves the agent running, quiet
(D-342) — the state a person is in after their next login anyway, one login
sooner. *Read from the code, not measured:* that a `tray` started from a
GNOME Shell launch stays up after its window closes, inside the app scope
gnome-shell made for it. Closest to the Windows installer's `LaunchAgent`.
*Cost:* code in `open`; the "Quit" route on a desktop with no tray is the
window's, as it is today after a login.

**2. The package starts it.** In substance, it cannot. An rpm scriptlet runs
as root, from `dnf` under `sudo`, in no one's graphical session, possibly
over ssh, possibly for a machine nobody is logged into. Starting a GUI agent
in the right user's session from there is the "GUI process started before
the graphical session exists" F12 §8 rejected, in a worse form. The most a
package can do is a system-wide `/etc/xdg/autostart` entry, which still
waits for the next login — **it does not close the gap** — and moves the
per-user off switch that `EnsureAutostart` was written to respect.

**3. The person is told.** The window says, when it finds no agent, that web
applications can reach Liro from the next login (or offers "start now",
which is option 1 behind a button). Cheapest and honest; the gap stays, and
a person on a desktop with no tray has no other place the message could go.

**Not options, recorded so they are not re-derived:** a systemd user unit
(F12 §8 rejected it, for the reason in option 2); starting the agent from
the web application's side (nothing on the page can start a local process;
that is what the agent is for).

My recommendation is **1**, with **3**'s sentence kept for the one case it
cannot cover: an agent that failed to start. It is the Windows behaviour
reached by the only route Linux has. Before it is written, one read would
settle its main unknown: start `liro-bridge tray` by hand from a Shell
launch and close its window — owner's hands, its own sheet.

## H. Next reads in this boot, owner's hands

- The window as seen: drawn, Serbian Latin, the empty list. Whether it
  follows the desktop's appearance cannot be told with `color-scheme`
  `'default'` (D-407).
- **The file chooser (new, from D-407's portal line)**: **Izaberi...**
  (`main.browse`), whose dialog is titled **Izaberite PDF dokumente**, if
  that is what the empty list shows — name it as it appears. GTK 4 asks the portal for it on Wayland, and the portal
  has just refused this process once. **Predicted, least certain: the
  dialog opens** — the settings read and the file chooser are different
  portal interfaces, and only one refusal is read.
- Then close the window by its corner X: `ps -C liro-bridge` empty,
  `proctree` gone, the autostart file stays.

## I. The chooser that does not open: controls, and the two reads left

Measured so far (to be recorded in full as D-408):

- **What the owner saw**: **Izaberi…** and **Promeni…** did nothing; the
  corner X closed the window and **pid 5714 did not exit** (8 minutes later,
  alive, WebKit web process gone, network process and proxy left).
- **Where it was stuck**: SIGQUIT to 5714 (exact PID; the only route left,
  since the flag also refuses ptrace and `/proc` stacks). Main goroutine at
  `internal/ui.runChooser` (`filedialog_linux.go:125`), waiting for
  `OpenMultiple`'s callback from the first **Izaberi…** press. The window's
  action loop was blocked from that press on, which is why **Promeni…** did
  nothing and why closing the window did not end the process.
- **Control A** (PyGObject, GTK 4.22.5, `Gtk.FileDialog.open_multiple`, two
  runs differing only in `prctl(PR_SET_DUMPABLE, 0)`): the non-dumpable run's
  `FileChooser.OpenFile` got `AccessDenied: … Unable to open /proc/6647/root`
  from the portal within 1 ms, and **GTK never called back** until the
  script cancelled at 25 s. The dumpable run's `OpenFile` was accepted and
  the portal's dialog process started.
- **Why Ubuntu differed**: the portal's check is the same in 1.18.4
  (Ubuntu, `1.18.4-1ubuntu2.24.04.3`) and 1.22.1. GTK changed: 4.14.5
  (Ubuntu, `4.14.5+ds-0ubuntu0.10`) passed a `portal_error_handler` that
  fell back to GTK's own dialog; GTK commit `d515311b59` ("filechoosernative:
  Make portals not fall back", first in 4.17.1) removed it, and 4.22.5 frees
  the request with `/* FIXME: Show an error dialog here ? */` and never
  answers.

### I1. A drop from Files, portals on (owner's hands)

Open **Liro Bridge** from Activities. In Files, go to
`~/liro-bridge/testdata/pdfs/` and drag `blank.pdf` onto the window. Then
close it with the corner X.

| | predicted |
|---|---|
| **the drop (least certain)** | **not predicted either way.** GTK's file-list drop can go through the Documents portal (`FileTransfer.RetrieveFiles`) when Files offers it, and whether xdg-document-portal refuses a non-dumpable caller as the desktop portal does, and whether GTK then tries `text/uri-list`, is not read |
| the corner X | the process exits (no chooser was opened); `ps -C liro-bridge` empty |

### I2. The chooser with GTK's portals off (owner's hands)

Claude starts `GDK_DEBUG=no-portals liro-bridge open` from its shell, with
the dumpable flag untouched. Press **Izaberi…**, then Cancel; **Promeni…**,
then Cancel; then the corner X.

| | predicted |
|---|---|
| **Izaberi…** | a dialog titled **Izaberite PDF dokumente** opens: GTK's own chooser, not GNOME's (4.22.5's `gdk_display_should_use_portal` returns false first on `GDK_DEBUG_NO_PORTALS`) |
| no refusal | no `Failed to read portal settings` line in `bridge.log` for this run |
| **Promeni…** | a folder chooser opens |
| the corner X | the process exits |

This is the control on the program itself, not a fix: GTK documents the
equivalent call as "apps must not call it".

### I1, read

**The drop did not work.** `bridge.log`, 20:50:17, from GTK's
`gtkdroptarget.c`: `Failed to receive drop data: GDBus.Error:org.gtk.GDBus.UnmappedGError.Quark._g_2dio_2derror_2dquark.Code0:
Unable to open /proc/7675/root`. **Same cause, a different portal service**:
xdg-document-portal's `FileTransfer` handler (1.22.1,
`document-portal/file-transfer.c:530`) looks the caller up through the same
app-info check and returns its GError raw (`G_IO_ERROR_FAILED`, hence
`Code0`); the desktop portal wraps the same failure as `AccessDenied`. No
`dbus-monitor` ran during I1, so the sender is identified by the error's
shape and the source, not by a bus trace. **The drop did not hang**: GTK
handles the error and logs it; the process exited on the corner X (`ps`
empty at 20:50:49). Nothing is shown to the person, and the program logged
nothing of its own.

**So on Fedora 44 there is no way at all to give this program a document**:
the button hangs the window, and a drop does nothing. Not "only dragging
works".

### I2, read

Both choosers opened with `GDK_DEBUG=no-portals`, and the process exited on
the corner X (`ps` empty at 20:54:11). No portal method calls on the bus; the
one error in the trace went to WebKit's sandboxed web process (the Realtime
"pid 2" line), not to this program. Recorded in D-408.

## J. Documents on Fedora: the options (D-408)

For the owner; none taken, and **D-376 is not changed** until they are read.
"Costs D-376" means: does any process of this program that holds a PIN, a
document or a signing session become dumpable, for how long, and against
which core handler. Two facts sit under every row: D-376 measured the
**limit alone** as enough against Ubuntu's apport, with the flag as the
defence against a handler that ignores the limit; and **Fedora's handler is
systemd-coredump, against which nothing has been measured**, with
`ptrace_scope = 0`, where the flag is also what keeps other processes of the
same user out of the agent's memory.

| | what | costs D-376 | GTK's position | Windows | Ubuntu (GTK 4.14) |
|---|---|---|---|---|---|
| **1** | **A chooser helper.** The window re-runs this binary as a short-lived `choose` process that does not set the flag, holds nothing but the dialog's title and the chosen paths, and calls `org.freedesktop.portal.FileChooser` itself over D-Bus (the program already speaks D-Bus for notifications), parented to the window by its exported Wayland handle. Paths come back on its stdout. | **Nothing for the agent or the window**: they keep the flag and the limit. The helper keeps the limit and not the flag, so it is covered by the limit alone, measured against apport and not against systemd-coredump. What a helper core would hold: file paths the person chose. | Uses the portal as GTK intends. | Nothing: a Linux-only file. | Fixed for real: the person gets GNOME's portal dialog, not GTK's fallback, and a later GTK does not break it. |
| **2** | **GTK's own dialog, called directly** (`GtkFileChooserDialog`), not `GtkFileDialog`. | Nothing. | **Deprecated since GTK 4.10**, gone in GTK 5. The binding has the type but no constructor (the C one is variadic), so it is built from its GType or a few lines of cgo. | Nothing. | The dialog Ubuntu shows today, then by design rather than by accident. |
| **3** | **`GDK_DEBUG=no-portals`**, set by the program before GTK starts. Measured in I2: both choosers work. | Nothing. | **GTK documents the equivalent call, `gtk_disable_portals()` (4.18), as: "This should only be used in portal implementations, apps must not call it."** Setting the debug variable is that call by a side door, and a debug variable's meaning is not a promise. It also turns off every other portal, is inherited by every child (WebKit's processes, `xdg-open`), and cannot work in a sandbox. | Nothing. | The same flag exists in 4.14. |
| **4** | **Drops from `text/uri-list`**, read directly (`GtkDropTargetAsync` and `gdk_drop_read_async`, both in the binding), not through GTK's file-list conversion, which chose the portal. Needed beside 1 or 2; 3 would cover it. | Nothing. | Supported API; the portal transfer exists for sandboxed apps. | Nothing (OLE drop target). | Same code; Ubuntu's drop works today. |
| **5** | **Weaken D-376.** (a) Drop the flag and keep the limit. (b) Clear the flag only while a chooser is open, or around a drop. | **(a)** asks the owner to give up the defence against a handler that ignores the limit, unmeasured on Fedora's handler, and on Fedora to let any process of the same user ptrace the agent. **(b)** a hole for as long as a person looks at the dialog; a drop cannot be foreseen at all. | — | (a) Windows has its own mechanism; not affected. | Same cost. |

**Whichever is chosen, two things go with it:**

- **The window must not hang on a callback that may never come.** GTK 4.22
  drops a refused chooser silently (the FIXME above). `runChooser` waiting
  for ever is how one refusal became a window that could not be closed.
- **A report to GTK**, the owner's to file as with B26: `d515311b59`
  promised "show an error instead of falling back", and 4.22.5 shows nothing
  and never answers the caller.

**My recommendation: 1 and 4.** Together they are the only pair that fixes
Fedora and Ubuntu for real, costs D-376 nothing for any process that holds a
secret, and uses GTK and the portal as their authors intend. 2 and 4 is the
smaller change if 1 is too much for this phase, at the price of a deprecated
API. I would not take 3: it works, but GTK says apps must not do it. 5 is on
the list because the owner asked which options ask for D-376; none needs to.

**Before 1 is written, three reads**: that Files offers `text/uri-list`
beside the portal format (for 4); that the binding exposes the Wayland
handle export (without it the helper's dialog is not modal to the window);
and **D-376 against systemd-coredump on this VM**, the limit alone and the
flag alone, as D-376 measured apport. That last one is owed whichever
option is taken.
