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
