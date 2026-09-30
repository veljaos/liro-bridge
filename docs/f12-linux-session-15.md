# F12 Linux — session 15: code, written on Ubuntu, for Fedora

**What this is:** the handover at the end of session 14, the Fedora VM's
first working sitting (D-406 to D-409). The next step is code, and it
starts fresh. **Written:** 2026-09-30, after `3b19ea4`.
**Working rules:** `docs/f12-linux-session-13.md` §C, with §E below added.
D-304's five questions before believing any check.

---

## A. Where the code is written: the Ubuntu VM

**There is no Go toolchain on the Fedora VM, and none goes on it.** It is the
stock Fedora 44 Workstation that B15 and F8 were measured on, and installing
a toolchain and its dependencies would spoil that. The code is written,
built and tested on the Ubuntu VM (two-run local green, D-368), packaged
there as an `.rpm` — which also keeps F12 §8's glibc floor — and comes back
to Fedora as a package with its hash in this repository, installed by the
owner with `dnf install ./file.rpm`, one change per boot.

## B. First: the plan, to the owner, before any code

The owner decided D-408's options 1 and 4, and that no portal call may wait
for ever. **Bring the plan before writing anything**: it changes how the
program behaves on both distributions. What the plan has to answer, at
least:

1. **The chooser helper** (option 1). A short-lived process of this binary
   that calls `org.freedesktop.portal.FileChooser` itself over D-Bus and
   holds nothing but the dialog's title, labels and filters, and the paths
   chosen. D-409's constraint, **not a preference**:
   - **it starts with an empty environment**, built by the parent with only
     what the portal call needs (the session bus address; say what else and
     why);
   - **it takes everything it is given, and returns the paths chosen, on
     stdin and stdout only** — nothing a person chose and nothing of the
     agent's on its command line or in its environment, because a crash
     under the limit alone puts both into the system journal;
   - it keeps `RLIMIT_CORE` at zero and does **not** set the dumpable flag.
     `ForbidCoreDumps` runs first in `main` for every process of this
     binary; say how the helper is told apart before it, and why that cannot
     be reached by anything else.
   - The parent window: `wayland:<handle>` from `WaylandToplevel.ExportHandle`
     (gotk4 `pkg/gdkwayland/v4`, D-409), and on Xorg `x11:<xid>` — the
     Ubuntu VM's Xorg session is a place to read it.
   - What happens where no portal answers the FileChooser interface at all.
2. **The drop** (option 4). `text/uri-list`, read through
   `GtkDropTargetAsync` and **an asynchronous read**: D-409's first probe hung
   on its own synchronous read on GTK's thread. Files offers it (D-409);
   other sources are not read.
3. **No portal call waits for ever.** Two different waits: the portal
   answering the call (an error or a request handle comes back at once —
   D-408 measured an error in 1 ms), and a person choosing (minutes are
   ordinary). Say which is bounded by what, and what the window **says**
   when a chooser fails — new text for `sr-Latn.json` and its two siblings,
   which is the owner's to read.
4. **What changes on Ubuntu**: the person gets GNOME's portal dialog, not
   GTK's fallback, on Wayland and on Xorg. **Windows is not touched.**
5. **Tests**, including one where a refused portal produces an error in
   bounded time rather than a hang.

**Also open, and also needs code: D-407's first-start gap.** A person who
installs the package and opens it gets a window with no agent behind it;
their web application cannot reach Liro until the next login, and nothing
says so. Options in session 14 §G; **not yet decided**. Bring it with the
plan or say why not.

## C. The Fedora VM as it stands

Fedora 44 Workstation, updated 2026-09-29 (D-405, D-406): kernel
7.2.7-200.fc44, GNOME 50.5, GTK 4.22.5, WebKitGTK 2.54.0, xdg-desktop-portal
1.22.1, SELinux Enforcing, `ptrace_scope` 0, cores to systemd-coredump.
Hostname `fedora`. Wayland only; no GNOME on Xorg (D-405, D-406).

Changed from stock, all recorded:

| what | since | owed |
|---|---|---|
| `org.gnome.software download-updates` `false` | 09-29 23:13 (D-405) | back on at the end (open-items F10) |
| `gh` 2.97.0 | 09-29 23:21, the owner's, unrecorded at the time (D-406) | the owner's to keep |
| `liro-bridge` 0.9.9~dev.12 | 09-30 20:23:57 (D-406) | removed at the end (F10) |
| `~/.config/autostart/liro-bridge.desktop` (`Exec=… tray`, enabled) | 09-30 20:29, written by the first `open` (D-407) | **it starts `liro-bridge tray` at the next login** — a login is a measurement; see §D |
| `~/.local/state/liro/logs/bridge.log` | 09-30 20:29 | — |
| system journal rotated and vacuumed | 09-30 21:44, the owner, to remove Claude's token (D-409) | records before 21:44 are gone; every one the record relies on is quoted in D-406 to D-409 |
| `/var/lib/systemd/coredump` empty | already at 21:40:46, remover unknown (D-409) | — |
| Claude's memory | one note pointing to session 13 §C | — |

No Liro process running at the end. dev.12 has run five times: the owner's
first open (5714, ended by SIGQUIT for its stacks), I1 (7675), I2 (8022,
`GDK_DEBUG=no-portals`), and the two D-376 crash runs (8935, 9081).

## D. What is measured on Fedora, and what is left

Measured: B15, closed (D-406); the first start's process tree, sandbox
processes, autostart entry and no-agent state (D-407); **no way to give the
program a document**, its cause and its controls (D-408, open-items D25);
D-376 against systemd-coredump, holding (D-409); what Files offers on a drop,
and that `text/uri-list` is read with no portal call (D-409).

Left, from session 13 §E, each needing predictions first:

1. **F8, what a stock GNOME user sees**: the window is read (D-407). The
   agent started by autostart at a login — no icon anywhere, nothing said
   (D-342) — is not. It needs a logout proven to be one; the autostart entry
   is already in place, so the next login on this VM starts `tray` whether
   or not that is the plan.
2. **B8, the sandbox under SELinux**: `bwrap` runs with the web process in it
   (D-407); the audit log is root's and unread.
3. **D23**: one window read (WebKitWebProcess 1, xdg-dbus-proxy 1, 7
   descendants); the N-window count is not.
4. **A26, `scripts/a11yprobe` against GTK 4.22** (session 13 §E called it
   B26; it is A26). Not run.
5. **B5**, focus and raise on GNOME 50. Not run.
6. **F1**, a real card. Not run.
7. **E9's Xorg half**: not on Fedora; the Ubuntu VM is the only place.

Recorded and not pursued: the portal's Realtime refusal of WebKit's
sandboxed web process ("pid 2", D-408); GTK's settings read refused and
falling back to GSettings (D-408, a log line).

## E. Rules added in session 14

To session 13 §C's, each paid for this session:

- **A crash stand-in runs under `env -i`.** systemd-coredump puts the whole
  environment of a crashed process into the system journal even when it
  writes no core; D-409's stand-ins put Claude's session token there.
- **A probe never blocks GTK's thread.** D-409's first drop probe read
  synchronously in a callback and hung itself; the hang was nearly
  recorded as the portal's.
- **Choose a deciding sign that can appear.** Go turns SIGABRT into a SIGQUIT
  death, which the kernel's audit hook skips; `ANOM_ABEND` could not have
  shown either way (D-409).
- **Read the boot you mean.** The update's journal was first read at the
  wrong boot, because the boots were counted by a prediction (D-406).
- **"Already gone" is not "removed", and a record does not attribute an act
  to anyone it cannot show did it** (D-409).

## F. What Fedora has cost and bought

**Cost**: two sittings on one VM; an 835-package update applied first so that
every reading is of the same machine; the journal before 2026-09-30 21:44,
vacuumed to remove a session token Claude's own stand-ins put there; and five
instrument failures of Claude's, each recorded where it happened — the wrong
boot, a crash check that could not fail, a probe that hung itself, the token,
and the finding filed as an unmeasured claim.

**Bought**: B15 closed. D-407, a gap in how the agent first comes to be
running that no tray desktop would have shown. D-376 measured on the second
core handler it will meet, and a constraint it never stated — no core is not
no record. A GTK defect with a reproducer. And D-408: **the one thing that
would have made this program unusable on Fedora — no way to give it a
document — found in a day, by the owner pressing a button**, after every
measurement that week had said the dialog was fine. Ubuntu shows the same
refusal and survives it only because its GTK still falls back; the GTK that
does not has shipped. Without this VM the program would have met that on a
person's machine first.
