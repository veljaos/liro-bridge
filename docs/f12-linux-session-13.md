# F12 Linux — session 13, the last on Ubuntu, and the handover to Fedora

**What this is:** the thirteenth session on the Ubuntu VM and the last one
planned there, and the handover to the first session on the Fedora VM. It
took session 12 §C1's login back to Wayland (D-403). **The owner then closed
Ubuntu and decided to keep the VM** (D-404). Nothing measured across a
boundary; nothing on the system changed.

**Written:** 2026-09-28, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-34-generic.
**Entries:** D-403, D-404. The Fedora VM is built by the owner on 2026-09-29
or later; nothing below about Fedora has been read on a Fedora desktop yet,
and every sentence about it says where it comes from.

---

## A. The Ubuntu VM, kept

**Kept, not deleted, for two things that can only be done on it** (D-404):

1. **E9's Xorg half.** E9 closes when a real signature's audit entry is read
   on each display server. If Fedora has no Xorg session (§D1), this VM is
   the only place the Xorg half can ever be read. It needs E9's code built,
   the reader passed through, and a login where the owner picks **"Ubuntu on
   Xorg"** at the gear. GDM remembers the last choice (D-403): the login
   after that picks "Ubuntu" again, explicitly.
2. **D23's control.** If D23's fix is only ever measured on Fedora and no
   chain survives, it cannot be told whether the fix did that or the
   distribution did. So D23 is counted on Fedora **before** any fix (§E),
   and after the fix it is counted **here** too, on the same machine that
   found it.

**The state it is left in** (D-403): Wayland session 2, tray 2415 by
autostart, `Linger=no`, the reader not attached, `liro-bridge 0.9.9~dev.12`
installed. At its next login take session 12 §C's first read before anything
else.

**The dev.12 packages** are in `dist/linux/`, which git ignores, so they
exist only on this VM and are **not** in the repository. One `build.sh` run
made both (the same mtime, 2026-09-27 21:17). D-396 read that run's binary
as clean from `3a1037a` (`vcs.modified=false`):

```
93b92690691c4652fc5271acb122ee6a52ece307aaa2a4e9c41ae8e7d216fdbb  liro-bridge_0.9.9-dev.12_amd64.deb
9730958307c5e46f0969bbd8cb7622d9054fcc23f04b4115ad1919ab4b1f4542  liro-bridge-0.9.9-dev.12.x86_64.rpm
```

**The owner copies the `.rpm` off this VM before building Fedora's**, by
whatever route they choose. The hash above is the check at the other end.

---

## B. What the owner decided (D-404)

- **Ubuntu is done.** C0, C3a and C3b stay unmeasured by decision (D-400).
- **The VM is kept**, for both of §A's reasons, the second especially:
  *"if D23's fix is only ever measured on Fedora and no chain survives, we
  will not know whether that was the fix or the distribution."*
- **Check on Fedora whether it still ships an Xorg session**, and say so if
  it does not, because that makes this VM the only place E9's Xorg half can
  be read (§D1).
- **The AccountsService miss is recorded as an instrument failure** (§F).

---

## C. Rules that travel, because Claude's memory does not

On the Ubuntu VM, several of these rules lived in Claude's own memory
directory as well as in the record. That directory is on this VM's disk and
the Fedora VM starts without it, so the rules are written out here in full.
Each one is the owner's rule or an owner-confirmed lesson.

- **No synthetic input** (D-094). **Exact PID only**: find the tray with
  `ps -C liro-bridge -o pid,ppid,lstart,args`, never `pgrep -f`/`pkill -f`,
  which match the asking shell (D-393). **Not `ss -p`**, and not
  `/proc/PID/fd`: the agent is not dumpable (D-376, D-396). Descendants with
  `python3 scripts/proctree/proctree.py PID`. No `//nolint`. No trailers on
  commits. The owner pushes by hand.
- **Never prompt for a secret on the owner's desktop.** Anything that could
  ask for a passphrase, PIN or password is run so that it fails instead:
  `sudo -n`, `gpg --batch --pinentry-mode error`. In session 7 a test opened a
  pinentry and the owner typed the real package-signing passphrase into it
  (D-356). Anything needing root is the owner's hands.
- **No load generators, busy loops or contention probes** (session 1). A
  measurement that needs a loaded machine is recorded as unmeasurable rather
  than attempted. The rule was given for the Ubuntu VM; take it as holding on
  Fedora unless the owner says otherwise.
- **Local green is two runs**: `go test -count=1 -p 1 ./...` and the same with
  `-tags softtoken` (D-368). A `-run` pattern that matches nothing still
  prints `ok`.
- **One change per boot, checked against the predecessor's own file**, not
  against what is installed (D-391).
- **Before anything "across a logout", prove it was one**: `uptime -s`,
  `last -x -F`, and logind's "Removed session"/"New session" in
  `journalctl -b 0` (D-399, D-400). A `cN` greeter session after a login has
  been a VT switch (D-402): read the journal before calling it anything.
- **Name a button by its label on screen**, from
  `internal/i18n/locales/sr-Latn.json` and the page's HTML: Settings has
  **Zatvori**, not Cancel, and the log does not tell Zatvori from the corner
  X (D-401).
- **Journal searches**: the Ubuntu hostname `Ubuntu-Liro` matched "liro" on
  every journal line. Read Fedora's hostname first. Use `journalctl -o cat`
  either way.
- **Predictions before measurements**, the least certain one marked (F12
  Rules). **"Unreadable" means every route was tried**, or it says which
  route was tried (§F).

---

## D. The first Fedora sitting: reads before anything is installed

The owner builds the VM: Fedora Workstation 44, updated or not as they
choose, **with the updates state recorded**. Claude then reads; nothing is
installed and nothing is changed. Predictions come from the sources named,
not from this machine.

### D0. The machine

```
cat /etc/os-release; uname -r; hostname; getenforce
echo $XDG_SESSION_TYPE $WAYLAND_DISPLAY $DISPLAY
loginctl show-session $XDG_SESSION_ID -p Type
rpm -q gnome-shell gtk4 webkitgtk6.0 bubblewrap xdg-dbus-proxy pcsc-lite pcsc-lite-ccid
sudo -n true; echo "sudo -n: $?"
busctl --user status org.kde.StatusNotifierWatcher
```

| | predicted | from |
|---|---|---|
| GNOME | 50 | F12 §11, not read |
| SELinux | `Enforcing` | Fedora's default, not read |
| session | `wayland`, with `DISPLAY` set by Xwayland | F12 §11 "Wayland only" |
| `sudo -n` | fails (a password is needed) | as on Ubuntu, not read |
| `StatusNotifierWatcher` | **absent**: nothing draws trays on stock GNOME | F12 §6, D-342 |
| `pcsc-lite` | installed or not, recorded as found | not predicted |

### D1. Does Fedora still ship an Xorg session?

**F12 §11 already assumes it does not** ("Wayland with X11 still available
against Wayland only"). That was never read on Fedora. So this check also
tests the plan's own premise. There are two questions, and the answer
depends on which one:

- **installed**: `ls /usr/share/xsessions /usr/share/wayland-sessions`;
  `rpm -q gnome-session-xsession xorg-x11-server-Xorg`; and whether the
  greeter shows a gear at all, by the owner's eye.
- **installable**: `dnf repoquery --available gnome-session-xsession
  gnome-classic-session-xsession xorg-x11-server-Xorg xfce4-session`.
  This reads repository metadata and needs no root.

| | predicted |
|---|---|
| `/usr/share/xsessions` | absent or empty; `wayland-sessions` holds GNOME's |
| `gnome-session-xsession` | not installed and **not available** |
| the gear | no gear, or one with no Xorg entry |
| **least certain**: `xorg-x11-server-Xorg` and an X11 desktop such as Xfce in the repositories | available |

**What each outcome means, written before it is read:**

- **Nothing X11 is installable.** This VM is the only place E9's Xorg half
  can be read. Say so plainly (the owner's instruction), and add it to E9 in
  open-items.
- **No GNOME on Xorg, but an X11 desktop is installable.** An Xorg reading on
  Fedora would mean installing a second desktop. That changes the system and
  is not what a Fedora Workstation user meets, so it is the owner's
  decision. Until the owner rules, this VM is the only place **GNOME on
  Xorg** can be read. Report it in exactly those words.
- **GNOME on Xorg is installable.** F12 §11's premise was wrong. Record it
  and bring it to the owner. Nothing is installed to find out.

### D2. No Liro state

The snapshot list from session 1 §4: `~/.config/liro`, `~/.local/share/liro`,
`~/.local/state/liro`, `$XDG_RUNTIME_DIR/liro`, `~/.config/autostart`.
Predicted: all absent. This is the baseline that later measurements are
compared against. **Record when it stops being true.**

### D3. dev.12's `.rpm`, before it is installed

- `sha256sum` against §A's hash. Predicted: equal.
- `rpm -K`. Predicted: **`digests OK` with no `signatures`**. dev builds are
  not signed. The real key has never run (C16), and CI's signing uses a
  throwaway key.
- **The README says an rpm with no signature "should not be installed".**
  That is written for a release. For the owner's own dev build, the check is
  the hash in this repository. Whether to install it anyway is the owner's
  decision, so ask before the install.
- **Why dev.12 and not a new build**: it is the binary Ubuntu ran. The D23
  count, the sandbox and the stock-GNOME reads then differ from Ubuntu's
  only by the distribution. A build made on Fedora would also break F12 §8's
  glibc floor.
- **The install is the owner's hands**: `sudo dnf install ./liro-bridge-0.9.9-dev.12.x86_64.rpm`.
  Record the whole transaction as printed: what dnf fetched, and whether
  anything else had to be typed. That transaction **is** B15's reading.

After the install, stop and write the next predictions. What starts the
agent on a desktop with no tray, what a person sees, and the first window
are §E's first item, not this sitting's.

---

## E. What Fedora is for, from open-items

Each item below needs predictions written against the machine before it is
measured.

1. **What a stock GNOME user sees** (F8; F12 §6; D-342). D-342 decided that on
   a desktop with no watcher the agent runs and says nothing. It has never
   run on such a desktop. Reach it through the desktop entry.
2. **B15**: `dnf install ./file.rpm` needs nothing else typed (§D3).
3. **B8**: WebKitGTK's sandbox under SELinux, with no profile.
   `scripts/sandboxcheck/run.sh` is Ubuntu-shaped: its `dpkg-query` and
   AppArmor sysctl lines will print errors on Fedora, and its window part is
   portable. Adapt it or read around those lines, and say which. SELinux
   denials are in the audit log, which needs root: owner's hands.
4. **D23 before any fix**: dev.12, N windows, `proctree.py` on the tray's exact
   pid. Take a control reading with a window open, as D-397 did. Record the
   WebKitGTK version beside the count.
5. **B26**: `scripts/a11yprobe` against Fedora 44's GTK. Its report to GTK is
   still the owner's to file.
6. **B5**: focus and raise on GNOME 50, repeating D-337's reading. KDE stays a
   separate image.
7. **F1**: a real card on Fedora 44. SafeSign's `redhat10` `.rpm` ships
   `/usr/lib64/libaetpkss.so` (D-359, listed and not installed). Whether an
   RHEL 10 build installs and loads on Fedora 44 is a prediction for then.
   Needs the reader passed through.
8. **E9's Xorg half**: §D1 decides whether Fedora has one.

---

## F. Instruments that failed

| the instrument | what it reported | what was wrong |
|---|---|---|
| D-400's reading of GDM's per-user memory | "not read here: `/var/lib/AccountsService/users/` is root's" | the same value is a property on AccountsService's system-bus interface, readable as the user (`busctl get-property org.freedesktop.Accounts /org/freedesktop/Accounts/User1000 org.freedesktop.Accounts.User Session`). The file is root's, so the store was not unreadable; one route was tried |

The owner on it: *an "unreadable" that meant "I tried one way" is the same
shape as this week's other instrument failures, and it is **the first one
found by looking again rather than by a control**.* (D-404)

---

## G. This machine

Nothing on the system changed. No apt packages, no configuration, no linger.
Claude's memory gained one line: GDM remembers the last session, and the
AccountsService read above, in the note on greeter sessions.
