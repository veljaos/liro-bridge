# F12 — the phase report

**Date:** 2026-10-03; **updated 2026-10-05**, through [[D-430]].
**Exit condition: not ticked here.**
- **The Ubuntu half holds.** The installed package, approval in the window
  and a real card in a real reader signed a PDF that the independent
  verifier of SPEC §16.4 and OpenSSL both accept ([[D-361]]). Both demos
  ran to written, verified PDFs ([[D-397]]). The OS was a VirtualBox guest
  with the reader passed through to it.
- **The Fedora half was taken on 2026-10-03** ([[D-420]]). The Pošta card,
  through SafeSign 4.6.0.0's RHEL build, signed a PDF from a protocol
  request on the Fedora VM, with the reader passed through the same way.
  `pdfsig` and OpenSSL `cms` both accept the signature, each with a control
  that fails.
- **What Fedora's half lacks**: the chain to Pošta Srbije CA Root, and
  `scripts/verifypdf`'s Trusted List check (open-items F1). The owner ruled
  Fedora done for F12 on that signature ([[D-420]]). **The owner decided
  ([[D-429]]) to leave the exit condition's Fedora half unticked, and this
  is deliberate, not an oversight.** "Verified twice" means two independent
  verifiers reaching [[D-361]]'s bar. Fedora's check was anchored at the
  signature's own copy of CA 1, not at Pošta's root, and had no Trusted List
  check. F1 stays open.

F12's own **Report:** line asks for seven things. §§2–7 answer them in its
order; §8 is the provenance table that §7 asks for, and §9 is the other half of
that table: what was never taken. §1 comes first because it governs how
everything after it is read. §7 now also carries D32 (the white second
window, its cause and its close) and what dev.15 to dev.17 changed on a
stock GNOME desktop.

**How this was compiled, and its limit.** The F12 entries, [[D-287]] to
[[D-417]] (interleaved with F11's [[D-311]], [[D-313]], [[D-315]]–[[D-317]]),
were read in full by five read-only extraction passes, each covering about
twenty-six entries and listing for every finding where it was taken, what
kind of evidence it is, and what the entry says was not taken. **Every
"measured on" in this report was then checked against the lines of its own
entry by me**, not against the extraction; where an entry does not say where
something ran, this report says so rather than inferring the machine. What
the entries do not record, this report cannot recover: a reading whose entry
did not mention the machine is listed as unstated.

**The update of 2026-10-05.** I read [[D-418]] to [[D-426]] in full myself,
with no extraction pass; nine entries are small enough to read whole. Every
"measured on" added was checked against its entry's lines, as before. Where
a finding changed after 2026-10-03, the section says what it was and what it
is now, rather than rewriting it as though it had always been so. [[D-427]]
recorded that update. [[D-428]], taken the same evening on the strength of
what the update found, was added after it.

---

## 1. What a VM cannot prove: limits on this phase's claims

F12 §0.1 asked for this split from the first day. Here it is as a limit on
what this phase claims, not as a caveat on it. **Every Linux reading in F12
was taken in a VirtualBox guest.** Not one was taken on Linux running on
real hardware.

- **Rendering is llvmpipe, everywhere.** On the Ubuntu VM Mesa cannot get a
  hardware driver ("VMware: No 3D enabled", "ZINK: failed to choose pdev"),
  so everything runs on llvmpipe ([[D-324]]). The VM is described as "no
  working GPU driver" ([[D-346]]), and the first real-card signature says
  "no GPU" ([[D-361]]). The Fedora VM is the same: GSK's default there is
  its Vulkan renderer, and llvmpipe is its only device ("Not using Vulkan:
  device is CPU", [[D-424]] K0). **Anything about how a window draws, maps,
  sizes or repaints is a reading of a software renderer.** That covers the 37 px
  header-bar correction ([[D-339]]), the layout checks ([[D-372]]),
  focus-stealing timing ([[D-337]]), the drop outline ([[D-415]], [[D-417]])
  and the chooser geometry ([[D-415]], [[D-417]]). So are D32's white
  windows ([[D-420]]). Its cause, though, is a number, not a picture: GTK's
  `gtk-xft-dpi` −1 and a second view's zoom NaN. The same defect showed under
  GTK's cairo renderer ([[D-424]], §7). The two variables F12 §3.2
  sets for real GPUs (`WEBKIT_DISABLE_DMABUF_RENDERER`,
  `__NV_DISABLE_EXPLICIT_SYNC`) have never been exercised; their screenshots
  with and without came out byte-identical, which is evidence of nothing
  ([[D-329]]).
- **Timing on a VM is weaker than timing on a machine.** Every duration
  measured on a guest is a property of that guest's scheduler and host load:
  - the 0.53 s notification raise ([[D-355]] §6);
  - the 1.10 s and 0.14 s focus times ([[D-337]]);
  - 3.5 ms for the portal to answer `OpenFile` ([[D-417]]);
  - 0.02 s from SIGTERM to exit ([[D-394]]);
  - 6.2 s for `certs` on the Pošta card ([[D-361]]);
  - the hundredfold serial/parallel test difference ([[D-346]]).

  A guest's clock can also move under it. In one Fedora sitting it was
  stepped twice, once after stopping for about 41 minutes, and `ps` start
  times read after that were off by the gap ([[D-423]]).

  This machine's clock floor was characterised only on the Windows developer
  machine ([[D-304]]), never on either VM. **No duration from a VM should be
  quoted as the program's speed.** It is the order of magnitude on that guest.
  The project's rule against load generators means nothing was measured
  under load ([[D-328]]).
- **The card and reader were real; the path to them was not a machine's.** The
  Realtek `0bda:0165` reader was passed through VirtualBox's USB to the Ubuntu
  guest, on the generic CCID driver ([[D-361]], [[D-365]]). VirtualBox did
  not re-attach it across a guest reboot once ([[D-364]]), did once
  ([[D-399]]), and it was absent at another boot for no known reason
  ([[D-400]]). So every real-card reading on Linux is a real card behind a
  virtual USB stack. That is weaker than the same reading on a laptop's own
  reader. The same reader was passed through to the Fedora VM for
  [[D-420]], so Fedora's one real-card signature has the same limit.
- **Harnesses are not the installed agent.** The PIN-field memory and
  accessibility readings were made with `scripts/pinmem`, which links
  `CollectPIN` into its own process. It is not the tray, and it does not
  measure the installed package ([[D-391]], [[D-392]]). The early window-host
  readings ran with WebKit's sandbox switched off
  (`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`, [[D-326]]–[[D-331]]). The
  window tests run only from a profiled path, never under plain `go test`
  and never in CI (open-items A29, C5). D32's cause was found in harnesses
  too: `scripts/d32probe` (Go, through `internal/ui`) and
  `scripts/ctxprobe/ctxprobe.py`, a Python host with none of our Go code
  ([[D-423]], [[D-424]]). The workaround's effect was then read on the
  installed agent ([[D-426]]).
- **CI's images are containers.** They show that the package installs and that
  the soft token signs, verified, on clean Ubuntu 24.04, Debian 13 and Fedora
  44 ([[D-355]], [[D-386]], [[D-387]]). The workflow's own comment says what
  a container cannot show: a window, a sandbox, a card.
- **One instance of each machine.** One Ubuntu VM, which was resized over the
  phase: 4 CPUs and about 4 GB in [[D-326]]–[[D-327]], 6 CPUs and 6 GB by
  [[D-346]]. One Fedora VM. One Pošta card, one MUP card, one reader. One
  Windows 11 machine for [[D-395]].
- **The Ubuntu VM was never stock.** Its userspace is 24.04, but it ran the HWE
  7.0 kernel ([[D-324]]: "not stock 24.04's 6.8"). The sandbox was later
  measured on 6.8.0-142 as well ([[D-390]]), on the same updated userspace.
  No fresh 24.04 install was measured.

---

## 2. What `CGO_ENABLED` became, and on which platform

**On Linux the binary is dynamically linked and built with cgo; on Windows
nothing changed.** SPEC §1 gained §1.1 by the owner's ruling ([[D-287]]).

- **`CGO_ENABLED=0` for Linux was measured to be possible, and to be no
  answer.** The project's own agent built that way is fully static and runs.
  That was measured under the `docker-desktop` WSL distro (musl) on the
  Windows developer machine, not on a Linux machine ([[D-287]]). purego builds
  at `CGO_ENABLED=0` but yields a dynamically linked binary that fails on
  musl for want of the glibc loader, measured on the same WSL distro
  ([[D-287]]). GTK and WebKitGTK need cgo regardless.
- **What ships links thirteen libraries.** Through gotk4, pinned at v0.3.1
  because v0.4.x needs GLib functions 24.04's 2.80 does not have:
  - `PT_INTERP` `/lib64/ld-linux-x86-64.so.2`, 13 `DT_NEEDED`, 10 094
    imported symbols ([[D-327]], on the Ubuntu VM);
  - `dpkg-shlibdeps` gives eleven packages ([[D-327]]);
  - the same thirteen were re-read from the packaged binary ([[D-354]]);
  - CI asserts the list on every push (`ci.yml`, "the agent links exactly the
    libraries SPEC §1.1 and F12 §8 account for").

  [[D-326]]'s "five libraries, a property of the platform" was wrong and is
  corrected in [[D-327]].
- **Windows**: CI builds `windows/amd64` with `CGO_ENABLED: 0` on every push
  (`ci.yml`, "build windows/amd64").
- **Cost of the Linux build**: 14 min 52 s cold, 1.93 GiB peak, on the
  4-CPU VM ([[D-327]]). That is a VM timing, under §1.

## 3. What the struct layout measured out at

**On Linux the layout is not modelled; the compiler takes it from the header.**
The PKCS#11 layer is cgo against p11-kit's `pkcs11.h`. `CK_ULONG` is eight
bytes against Windows' four, and the structures are naturally aligned rather
than packed, so every offset in `module_windows.go` would be wrong on Linux
([[D-349]]).

- **What [[D-287]] could not confirm**: the Linux branch of the packing rule.
  The model that reproduced F11's eleven hardware-proven Windows numbers was
  never checked on Linux against a header, because there was none on that
  machine ([[D-287]]). The cgo route removed the need to model it.
- **What shows the layout is right is a verified signature, not a call
  returning `CKR_OK`.** That was F11's lesson: three of four wrong
  `CK_ATTRIBUTE` layouts returned `CKR_OK`. In order:
  - SoftHSM 2.6 on the Ubuntu VM: a signature verified with `crypto/rsa`
    against the token's own certificate, and a wrong PIN refused with
    `CKR_PIN_INCORRECT` ([[D-349]]);
  - the Pošta card through SafeSign 4.6: verified by `scripts/verifypdf` and
    OpenSSL, with controls that fail ([[D-361]]);
  - four more real-card signatures across both demos, verified by
    `verifypdf` and `pdfsig` ([[D-397]]).

  All on the Ubuntu VM, with the reader passed through.
- **On Fedora** ([[D-420]]): SafeSign 4.6.0.0's RHEL 10 build
  (`libaetpkss.so`) signed with the Pošta card. `pdfsig` and OpenSSL `cms`
  verified the signature, each with a control that fails. That carries the
  same evidence for the layout onto GTK 4.22's platform. It is not anchored
  to Pošta's root (§9).
- **Not taken**: MUP's card on Linux, which neither SafeSign nor OpenSC
  reads (`token not recognized`, [[D-365]]).

## 4. Whether the sandbox worked on a stock Ubuntu 24.04

**It does not start without a profile, and it starts with one, on the 6.8
and 7.0 kernels.**

- **Without a profile**: WebKitGTK aborts with "bwrap: setting up uid map:
  Permission denied" ([[D-324]]). The cause is
  `apparmor_restrict_unprivileged_userns = 1` with a non-setuid `bwrap`.
  The probe was PyGObject with the distribution's own `gir1.2-webkit-6.0`,
  not this program.
- **With the package's AppArmor profile** (Ubuntu's epiphany profile renamed,
  [[D-354]]), the installed agent's sandbox starts. The unprofiled control is
  refused. The table in [[D-390]], read from the files the sitting left,
  covers:
  - kernels 7.0.0-31 and 7.0.0-34 (the latter with 22 updates);
  - kernel 6.8.0-142;
  - with the two development profiles removed one by one.

  A python3.12 profile left from session 1 coloured the first baseline. It
  was caught by its own control and removed ([[D-388]]–[[D-390]]).
- **The limits**:
  - the VM is an updated 24.04 userspace on an HWE kernel with a 6.8 kernel
    added, not a fresh stock install;
  - one boot per kernel;
  - `aa-status` was noted by the owner and not written to a file
    ([[D-390]]).
- **Fedora**: `bwrap` runs with the web process inside it under SELinux
  Enforcing ([[D-407]], and every window since). **Whether SELinux denied
  anything along the way is in root's audit log and has never been read**
  (open-items B8).

## 5. What the consent window does when it cannot raise itself, demonstrated

**A new window per request takes focus; an existing window cannot be brought
forward by itself.** Measured on the Ubuntu VM, GNOME 46, Wayland, GTK
4.14.5, software rendering, with no synthetic input ([[D-337]]):

- a new window took focus, including from a process idle for 45 s;
- `present()` on an existing, unfocused window with a foreign window active
  did not raise it — still inactive 3 s later.

The five window rows were re-taken on a GTK window holding a WebKitGTK view
showing `consent.html`, and held ([[D-337]], pointer added later). **So the
design opens a new window per request** ([[D-341]], guarded by a test).

**The notification half reversed.** [[D-337]] measured that a clicked
notification raised nothing and brought no token. [[D-355]] §6, with the
package installed, measured the opposite: a notification with no action
raised the window 0.53 s after the click, and one with an action delivered a
token that `present()` honours. **Why the two readings differ is not
established.** SPEC §6.5.2's table was removed ([[D-369]]). That nothing
rests on the window being in front was established by reading the code, not
by running it ([[D-288]]).

**Not measured**:
- KDE or any compositor but GNOME 46's (open-items B5);
- GNOME 50 on Fedora;
- the consent window on X11. On "Ubuntu on Xorg" the Settings window was an
  ordinary X client carrying the tray's pid ([[D-402]]); that the consent
  window does the same is inferred.

## 6. Which PIN dialog, and why

**The program's own field, in a locked page, with no toolkit text control**
([[D-385]]). It was chosen because the native `GtkPasswordEntry` sent the
typed text to the accessibility bus in `TextChanged` events. Any process of
the same user can read that bus, and it is on by default on stock GNOME with
no screen reader ([[D-384]]). The arc that led there is in the record,
including a withdrawn conclusion: [[D-350]] was withdrawn by [[D-351]], and
D-351's number withdrawn in turn by [[D-352]].

- **Measured on the shipped field**, with `scripts/pinmem` (a harness, §1) on
  the Ubuntu VM, the owner typing a random needle by hand, eight valid runs
  ([[D-392]]):
  - one copy while typed, in a mapping locked at 4 kB;
  - 0 after OK and 0 after the wipe;
  - no `TextChanged`;
  - the process never connected to `ibus-daemon`.

  By the stopping rule ([[D-391]]) this closes the narrow claim: the old
  field's behaviour did not survive into the new one. It does not establish
  that nothing ever survives.
- **Windows** ([[D-395]], Windows 11 Pro 10.0.26200, typed by hand): the
  shipped dialog hands the typed text to no UIA, MSAA or `WM_GETTEXT`
  client. The WinEvent method had no working control on the typed path and
  is not counted (open-items B24).
- **On Fedora** the PIN for [[D-420]]'s signature went into this program's
  own field, with no SafeSign window at any point (the owner). That records
  which field was used. It is not a measurement of the field.
- **Not measured**:
  - Fedora 44's GTK with the same script (open-items A26);
  - the keystroke path in front of the field, on either platform. On Linux
    the compositor's sight of every key is conceded in SPEC §6.5.1 clause 3
    ([[D-398]]). On Windows, TSF/IME is unmeasured.

## 7. What a stock GNOME desktop shows when the agent is running

**As found on Fedora 44 under dev.12 to dev.14** ([[D-405]]–[[D-417]]): no
tray icon, no agent until the next login, and no route to Settings, the
certificates or the audit log. **Since dev.15** ([[D-422]]–[[D-426]]): still
no tray icon; a launch from Activities becomes the agent; the three doors
are in the main window's footer; and an agent's windows draw, where under
dev.14 and dev.15 every web window after a process's first was white (D32).
Each is below, with where it was watched. The first subsection is §7 as
written on 2026-10-03, its tails brought up to date.

### What was found, dev.12 to dev.14

- **No tray host**: no `StatusNotifierWatcher` on Fedora's session bus,
  against a control ([[D-405]], [[D-406]]). The agent runs and registers its
  item, and nothing shows it. Its "accepted" line is logged at DEBUG and so
  never appears ([[D-417]]). Ubuntu ships an AppIndicator extension and shows
  the icon ([[D-342]]). Unchanged since.
- **No agent after install**: the package installs nothing that starts. The
  first `open` writes the autostart entry, so the agent arrives at the next
  login. Until then a web application cannot reach it, and nothing says so
  ([[D-407]], open-items D27).
- **No door** ([[D-417]], open-items D31):
  - **Podešavanja** (and in it **Izvezi dnevnik revizije**), **Sertifikati**
    and **Prikaži dnevnik revizije** open only from the tray's menu;
  - the signing window, reached from Activities, offers none of them.

  [[D-342]] decided F12 §6 with the sentence that a person on a trayless
  desktop reaches them from "the main window, which is where Settings, the
  certificate list and the audit log are reached from anyway". **That
  sentence was never measured, and on dev.14 it is false.** It is D22's
  shape: a mechanism stated as fact and left where the next reader meets it.
  F12's checklist item "Settings, certificates and the audit log reachable
  without a tray" was therefore **not met** at [[D-418]].
- **The window itself**:
  - dev.12 could not be given a document at all, because the portals refuse
    the non-dumpable process ([[D-408]]);
  - dev.14 can: through the chooser helper, and by drop ([[D-417]]);
  - the window's process is refused by the Settings portal too (open-items
    B29). Its cost was unread on 2026-10-03; it was D32, below;
  - a logout ends the tray inside GDK, not by SIGTERM, on Ubuntu
    ([[D-415]], [[D-416]]). **On Fedora's Wayland two logouts went through
    SIGTERM and Quit's path ran** ([[D-420]]), so which ending a logout gets
    depends on the machine, and the code has to handle both (D30).

### D32: every window after the first, its cause, and its close

**What a person met** ([[D-420]]; Fedora, dev.14): after each login the
first window paints and every window after it is white, so a person who
signs twice in a session sees the second one blank. Seven windows under
three agents; both halves were predicted before agent 12473's two requests,
and both held. The evidence is the owner's eyes and screenshots. AT-SPI's
`SHOWING`, read as "painted" for two windows, is not a reading of the screen
(D-304 Q2).

**What it was not** ([[D-421]], [[D-423]], [[D-424]]; Fedora, probe hosts).
Each of these was ruled out by a probe run:
- GTK's first-time setup;
- the first view being the only one with no terminate before it;
- our own `WEBKIT_DISABLE_DMABUF_RENDERER=1`. Its limit: device EGL was
  refused under `=0` too, and the fallback path was not read;
- the shared web context with `liro://` registered once;
- Skia's GPU rasteriser;
- GTK's renderer (cairo, G0).

A Python host with none of our Go code reproduced it (C0).

**The cause, as far as this program reaches** ([[D-424]]; Fedora,
`scripts/ctxprobe/ctxprobe.py`):
- GTK 4.22.5 asks the Settings portal even outside a sandbox. The portal
  refuses our non-dumpable process ("Unable to open /proc/PID/root", B29),
  and GtkSettings keeps `gtk-xft-dpi` at −1.
- With −1, WebKitGTK 2.54.0 builds a process's first web view at zoom 1.0
  and every later one at NaN. The page lays out in a 0×0 viewport and is
  drawn at an enormous scale, which on the agent's `#ffffff` page is white.
- The DPI value alone decides it, both ways. K5: non-dumpable with 98304
  set, window 2 full. K6: dumpable with −1, window 2 broken.
- So [[D-376]]'s protection stays. On this Fedora `ptrace_scope` is 0, so it
  is the only thing between a same-user program and the agent's memory.
- **Why the first view survives −1 is inside WebKit, and not read**
  (open-items D36).

**Ubuntu** ([[D-425]]). GTK 4.14.5 never asks the Settings portal outside a
sandbox: it reads GSettings and has 98304 (X1). WebKitGTK 2.52.6 has the
same defect with −1 (X2, sandbox off; window 2's geometry was read, the
window was not watched). **Ubuntu is unexposed, not safe.** A GTK that asks
the portal unsandboxed would bring −1 while D-376 stands, and GTK 4.14 has
−1 too where the schema is missing.

**The workaround** (dev.16; chosen in [[D-424]], built in [[D-425]]):
- when GTK's `gtk-xft-dpi` is −1, it is set from
  `org.gnome.desktop.interface text-scaling-factor`, read directly through
  GSettings, as GDK 4.22.5 computes it, and followed live;
- with no schema it stays −1, and the agent logs a WARN that says so;
- one log line when it acts;
- four unit tests, each with a control that fails.

**The close** ([[D-426]]; Fedora, the installed dev.17, an autostarted
agent). A pairing and three requests under one agent: four windows, each
drawn, watched by the owner. The agent's one line was "GTK had no
gtk-xft-dpi, so it is taken from the desktop's text-scaling-factor", 1 →
98304. No prediction failed. On Ubuntu the workaround logged "left as it
is" on every window ([[D-425]]).

**What the close does not reach**:
- the defect itself (D36);
- a desktop without `text-scaling-factor`;
- a change of text scaling followed on screen (tested, not watched);
- the windows [[D-420]] to [[D-423]] called white. On a white page a
  magnified corner could not be told from an undrawn view ([[D-424]]).

### What dev.15 to dev.17 changed, and where each was watched

| build | carries | watched |
|---|---|---|
| **dev.15** (`9950197`, [[D-422]]) | D33: the caller answered when the run ends, not at Završi · D31: Podešavanja, Sertifikati and Prikaži dnevnik revizije in the main window on every platform · D29: no document size anywhere a person sees it · D28: the drop target tells the page the drag is over · D27: on Linux, `open` with no agent becomes the agent | Fedora ([[D-423]]), each held: D33 with the Pošta card, the caller's file on disk 0.86 s after the run ended, the report still up and Završi not pressed; D31's three doors opened their windows, content white (D32); D29; D27; D28, which cannot fail there for dev.15's reason |
| **dev.16** (`df27b59`, `33ae58d`, [[D-425]]) | D32's workaround · footer A: the doors and Izađi on one quiet line under the batch's buttons | Ubuntu, Wayland and Xorg ([[D-425]]): "left as it is"; footer A confirmed by the owner; R7 performed; D27 on GNOME 46; D28 fixed on Xorg against dev.14. **Never installed on Fedora** |
| **dev.17** (`dd87a71`, built at `e87974a`, [[D-425]]) | D35: the audit export's entries file 0600 · D34: Izađi and Zatvori say what fired them (`pointer`, `key`, `neither-pointer-nor-key`, `script`), a close of Settings from outside is logged apart, and the quit line no longer says "the person" | Ubuntu's Xorg ([[D-425]]): pointer and key, Zatvori and the ×, the export 0600. Fedora ([[D-426]]): D32's close; ([[D-428]]): footer A, the three doors drawn, the export 0600, Settings' × logged as a close from outside |

**So, against what was found:**
- **The door** has existed on every platform since dev.15.
  - **Performed on Ubuntu** (R7, [[D-425]]): Podešavanja → Izvezi dnevnik
    revizije, with the export verified; Sertifikati and Dnevnik revizije
    drawn.
  - **On Fedora** the doors opened their windows under dev.15, and the
    windows were white ([[D-423]]). **Under dev.17** ([[D-428]]), with the
    same agent that drew [[D-426]]'s four windows, each was drawn and
    watched by the owner: Podešavanja; Izvezi dnevnik revizije through the
    portal's folder chooser to `~/Documents`, both files 0600, nine
    entries, "provera ispravnosti: u redu"; Sertifikati, with the Pošta
    certificate; and Dnevnik revizije, with the same nine entries.
  - So [[D-419]]'s condition, that a person on Fedora can reach Settings
    and export the audit log, **is met**. That makes eight web windows
    under one agent, every one drawn.
- **The first start**: `open` becomes the agent, watched on GNOME 50 and 46.
  The sentence for an agent whose protocol did not start has never been
  produced (D27).
- **The tray icon**: still none on stock GNOME, and none is intended.
- **Footer A** has been seen on Ubuntu ([[D-425]]) and on Fedora
  ([[D-428]]).

---

## 8. Provenance: where each finding was taken

Locations, as the entries name them:

| machine | what it is | the entries' own words |
|---|---|---|
| **Ubuntu VM** | VirtualBox guest, Ubuntu 24.04 userspace, kernel 7.0 HWE (6.8.0-142 added in [[D-390]]); GNOME 46; GTK 4.14.5; WebKitGTK 2.52.6; llvmpipe; Wayland, and "Ubuntu on Xorg" for [[D-402]], [[D-415]], [[D-416]], [[D-425]] | "the VM every Linux measurement in F12 is taken on" ([[D-346]]), until Fedora; then [[D-421]], [[D-422]], [[D-425]] |
| **Fedora VM** | VirtualBox guest, Fedora Workstation 44; GNOME 50.5, GTK 4.22.5, WebKitGTK 2.54.0 after the update ([[D-406]]); kernel 7.2.7 by [[D-426]]; SELinux Enforcing; Wayland only (no GNOME on Xorg, [[D-405]]); llvmpipe; no Go toolchain | [[D-405]]–[[D-409]], [[D-417]], [[D-420]], [[D-423]], [[D-424]], [[D-426]] |
| **passed-through hardware** | the Realtek `0bda:0165` reader via VirtualBox USB, into the Ubuntu VM and, from [[D-420]], the Fedora VM; the owner's Pošta card (SafeSign 4.6) and MUP card | "the OS is a VM, the reader and card are hardware" ([[D-361]]) |
| **Windows, native hardware** | the developer machine; the "home machine" with a reader and the Pošta card; Windows 11 Pro 10.0.26200 for [[D-395]] | [[D-291]]–[[D-310]], [[D-318]]–[[D-323]], [[D-395]] |
| **WSL** | the `docker-desktop` (musl) and an Ubuntu WSL distro on the Windows machine | [[D-287]], [[D-319]] |
| **CI** | GitHub `ubuntu-latest` (`ci`, `linux-gui`), `windows-latest`; containers `ubuntu:24.04`, `debian:trixie`, `fedora:44` | job and run numbers in each entry |

**What was measured, and where.** "Measured" means run and read. "Read"
means source, binary, package or documentation, with nothing run.

| finding | where | kind | entry |
|---|---|---|---|
| static `CGO_ENABLED=0` agent runs; purego is not static | WSL (musl) | measured | [[D-287]] |
| gotk4 v0.3.1: 13 `DT_NEEDED`, 11 packages; v0.4.x needs newer GLib | Ubuntu VM | measured | [[D-327]] |
| PKCS#11 layout via cgo; SoftHSM signature verified | Ubuntu VM | measured (SoftHSM) | [[D-349]] |
| out-of-process worker: a real module's crash in a probe child survived, listing complete | Windows, real Pošta card, by accident | measured | [[D-315]] (F11 phase) |
| a module killing its worker becomes a `Failure` on Linux | Ubuntu VM | **synthetic SIGABRT only** | [[D-349]] |
| WebKit sandbox needs a profile; starts with it on 6.8 and 7.0 | Ubuntu VM | measured | [[D-324]], [[D-390]] |
| sandbox runs under SELinux | Fedora VM | measured; denials **not read** | [[D-407]] |
| focus and raise: new window yes, `present()` on an existing one no | Ubuntu VM, Wayland, llvmpipe | measured | [[D-337]] |
| clicked notification raises; an action brings a token | Ubuntu VM | measured, cause of the reversal unknown | [[D-355]] §6 |
| the shipped `GtkPasswordEntry` sends typed text on the a11y bus | Ubuntu VM, pinmem | measured (harness) | [[D-384]] |
| the own locked field: 0 copies after the wipe, no `TextChanged`, eight runs | Ubuntu VM, pinmem | measured (harness) | [[D-392]] |
| Windows dialog hands text to no UIA/MSAA/`WM_GETTEXT` client | Windows 11 | measured | [[D-395]] |
| core dumps forbidden: no core, no apport report | Ubuntu VM, installed dev.10 | measured, synthetic crash | [[D-376]], [[D-377]] |
| the same against systemd-coredump | Fedora VM, installed dev.12 | measured, synthetic crash | [[D-409]] |
| Secret Service on GNOME Keyring; `plain` transfer; any same-user process reads the secret | Ubuntu VM | measured | [[D-347]] |
| audit lock across two processes | the entry names `ext4, /dev/sda2` and no machine | measured | [[D-367]] |
| discovery file: SIGTERM removes it; SIGKILL leaves it | Ubuntu VM | measured | [[D-394]], [[D-396]] |
| a logout ends the tray inside GDK by `_exit(1)`, both backends | Ubuntu VM (Wayland, Xorg) | measured, and read in machine code | [[D-415]], [[D-416]] |
| packages: install on three clean images | CI containers | measured | [[D-355]] |
| package signature: Fedora's `rpm -K` accepts Ubuntu's ed25519 signature (throwaway key) | CI containers | measured | [[D-360]] |
| soft token signs, two verifiers agree, on three images | CI containers | measured; logs read | [[D-386]], [[D-387]] |
| first real-card signature, verified twice with controls | Ubuntu VM + passed-through reader, Pošta card | measured | [[D-361]] |
| both demos to written, verified PDFs | Ubuntu VM + passed-through reader, Pošta card | measured | [[D-397]] |
| `certs` states: card in/out, reader attached/detached | Ubuntu VM + passed-through reader | measured | [[D-365]], [[D-370]], [[D-379]] |
| pcscd stopped: the Linux sentence | Ubuntu VM | measured | [[D-360]] |
| remove and purge | Ubuntu VM | measured | [[D-363]] |
| the portal refuses the non-dumpable process | Fedora VM ([[D-408]]); Ubuntu VM ([[D-411]]) | measured, with controls | [[D-408]], [[D-411]] |
| chooser helper and `text/uri-list` drop work | Ubuntu VM (both backends), Fedora VM | measured | [[D-414]], [[D-415]], [[D-417]] |
| the drop target's GTK ownership defect | Ubuntu VM | measured (test red 3/3), read in machine code | [[D-412]], [[D-413]] |
| the attached dialog's geometry; R6 performable on Xorg only | Ubuntu VM Xorg; Fedora VM | measured | [[D-415]], [[D-417]] |
| no route to Settings, certificates, audit log on stock GNOME (dev.14; doors built in dev.15, §7) | Fedora VM, and the code | measured + read | [[D-417]] |
| the Pošta card signs through SafeSign's RHEL build; `pdfsig` and OpenSSL `cms` accept it, each control failing; the chain to the root not checked | Fedora VM + passed-through reader | measured | [[D-420]] |
| the caller waits for Završi after Završeno (D33), by construction on every platform | Fedora VM with the card; the code | measured + read | [[D-420]] |
| D33 fixed: the caller's file on disk with the report still up | Fedora VM with the card, dev.15 | measured | [[D-423]] |
| a logout on Fedora's Wayland ends the tray by SIGTERM, twice | Fedora VM | measured | [[D-420]] |
| every web window after a process's first is white (D32) | Fedora VM, three agents, dev.14 | measured, the owner's eyes and screenshots | [[D-420]] |
| D32's candidates ruled out: GTK's setup, terminate, our DMABUF variable, the context, Skia, GSK's renderer | Fedora VM, `d32probe` and `ctxprobe.py` | measured (probe hosts) | [[D-423]], [[D-424]] |
| D32's cause: `gtk-xft-dpi` −1 → view 2 zoom NaN; the DPI decides it both ways | Fedora VM, `ctxprobe.py` | measured (Python host) | [[D-424]] |
| GTK 4.14 never asks the Settings portal unsandboxed and has 98304 | Ubuntu VM, a Python host; GTK's sources | measured + read | [[D-425]] (X1) |
| WebKitGTK 2.52.6 has the defect with −1 | Ubuntu VM, `ctxprobe.py`, sandbox off | measured (geometry); window 2 **not watched** | [[D-425]] (X2) |
| the workaround: four windows drawn under one agent | Fedora VM, installed dev.17 | measured, the owner's eyes | [[D-426]] |
| a tray launched by the Shell outlives its window (D27's premise) | Ubuntu VM (GNOME 46); Fedora VM (GNOME 50) | measured | [[D-421]], [[D-423]] |
| dev.15's five: D29, D27, D31 (doors open, content white), D28, D33 | Fedora VM | measured | [[D-423]] |
| D28 fixed on Xorg against dev.14; footer A; R7 performed; D27 on GNOME 46 | Ubuntu VM, dev.16 | measured | [[D-425]] |
| every window leaves an `xdg-dbus-proxy` chain (D23) | Fedora VM and Ubuntu VM | measured | [[D-420]], [[D-421]], [[D-423]] |
| the audit export's entries file 0664, then 0600 | Ubuntu VM, dev.16 and dev.17 | measured | [[D-425]] |
| the log tells a mouse from a key, and Zatvori from a close from outside | Ubuntu VM Xorg, installed dev.17 | measured | [[D-425]] |
| the doors from the main window: Settings, the export (0600, chain intact), Sertifikati, Dnevnik revizije — each drawn | Fedora VM, installed dev.17, the agent of [[D-426]] | measured, the owner's eyes | [[D-428]] |
| the chooser helper's folder mode: the portal's dialog | Fedora VM | measured | [[D-428]] |
| Settings' title-bar × logged as a close from outside | Fedora VM, dev.17 | measured | [[D-428]] |
| Settings' Zatvori by mouse logged as `Zatvori sent cancel`, `trigger: pointer` | Fedora VM, dev.17 | measured | [[D-430]] |
| the upstream report's reproducer: −1 → view 2 nan; `GDK_DEBUG=default-settings` the same; the control both 1.0 | Fedora VM, PyGObject under `env -i` | measured, the owner's eyes | [[D-430]] |

---

## 9. What was never taken

This table is the one to read before quoting anything above. Each row is a
claim someone could reasonably make about F12 and that **no reading
supports**.

**Updated 2026-10-05.** Two rows are replaced. A real card on Fedora was
taken in [[D-420]], and its row is now the chain to the root. "No route"
was built in dev.15, and its row is now the doors' windows on Fedora. Five
rows are narrowed. The rows added cover what D32's close does
not reach and what dev.15 to dev.17 were not watched doing. Two of the
rows added that day, the doors' windows on Fedora and footer A on Fedora,
were taken in [[D-428]] and removed. The row D-428 added, Settings' Zatvori
on Fedora, was taken in [[D-430]] ("Zatvori sent cancel", `trigger: pointer`)
and removed.

| never taken | why it matters | where it stands |
|---|---|---|
| **The Pošta chain to its root, on Fedora's signature; `verifypdf`'s Trusted List check there** | Ubuntu's "verified twice" had both; Fedora's shows integrity and the signer, not whose CA 1 it is | the root is on the card (`F2E88F59…`), `verifypdf` is on the Ubuntu VM; neither done (F1, [[D-420]]) |
| **Linux on real hardware** | every Linux reading is a VirtualBox guest | §1 |
| **Any GPU: the DMABUF and NVIDIA variables** | F12 §3.2's two failures "on somebody's machine" | built ([[D-329]]), never exercised; ruled out as D32's cause on llvmpipe, with its limit ([[D-423]]); open-items B6 |
| **A fresh stock Ubuntu 24.04 install** | "stock 24.04" in F12 §3.2 and the checklist | the VM is updated userspace on HWE with a 6.8 kernel added ([[D-388]], [[D-390]]) |
| **A real module killing the held worker, on Linux** | F12 §2's box | synthetic SIGABRT only (open-items C9); Windows saw a real module crash a probe child ([[D-315]]) |
| **MUP's card signing on Linux** | the other Serbian card | neither SafeSign nor OpenSC reads it ([[D-365]]); `ubavic/srb-id-pkcs11` not tried (E3) |
| **Halcom** | a third issuer | no signature ever verified (C15) |
| **Why WebKit's first view survives `gtk-xft-dpi` −1** | the defect under D32, in 2.52.6 and 2.54.0 | not read; the upstream report drafted, its reproducer not run on Fedora as written, not filed (D36) |
| **A desktop with no `text-scaling-factor`** | the workaround cannot act there, and later windows may be blank | met on neither VM; the WARN is tested, not seen (D36) |
| **Text scaling changed while the agent runs, on screen** | the workaround follows it live | unit-tested under GSettings' memory backend ([[D-425]]); never watched |
| **SELinux's verdict on the sandbox** | Fedora's half of §4 | root's audit log, never read (B8) |
| **Any compositor but GNOME's** | focus, raise, the tray | KDE never run (B5) |
| **The consent window on X11** | E9's half | inferred from Settings ([[D-402]]) |
| **Fedora's GTK against the PIN field and the a11y bus** | the shipped field's guarantee on GTK 4.22 | A26 |
| **The keystroke path before the field, on Windows** | B1's Windows half | TSF/IME unmeasured; the WinEvent method had no control (B24) |
| **A logout under linger; two users; a logout on Ubuntu's Wayland under dev.16 or later** | D30, B19 | Ubuntu: two logouts, both ending inside GDK ([[D-415]], [[D-416]]); Fedora's Wayland: two, both by SIGTERM ([[D-420]]); under dev.16 the agent had quit before the logout (D34); linger and two users not taken, partly by the owner's decision (B19, D-396, D-400) |
| **D33 on Ubuntu, and on Windows** | the caller answered when the run ends | watched on Fedora with the card only ([[D-423]]) |
| **D27's sentence on screen** | what a person is told when the protocol did not start | never produced ([[D-423]], [[D-425]]) |
| **An accessibility press on Izađi or Zatvori** | dev.17's `neither-pointer-nor-key`, the third route into D34's line | not produced ([[D-425]]); whether a same-user program can press at all is B25 |
| **A day of requests** | the web-process leak fix | nine windows in forty minutes ([[D-397]]), not a day (C2); every window leaves a proxy chain on both VMs ([[D-420]], [[D-421]], [[D-423]]; D23) |
| **Load of any kind** | WebKit's SIGUSR1 against the Go runtime; the fuzz flake rate | no load generators, by rule (B16, B21, [[D-328]]) |
| **The real signing key on a real tag** | package signing | only a throwaway key in CI (C16) |
| **The chooser's 30-minute ceiling sentence on screen** | the one time it ran, nobody saw it | C20 ([[D-417]]) |
| **R6 on Ubuntu's Wayland with the X uncovered** | separates refusal from coverage, backend from versions | B28; the outline itself fixed (D28, [[D-425]]) |
| **The ported sign flow's tests on Linux** | every test of it is `*_windows_test.go` | covered by compilation and one run, "not the same as tested" ([[D-338]]); C11 |
| **The window tests in CI** | they need a display and a profiled path | they skip on every runner (C5, A29) |
| **Windows since the Linux work** | the two-window fix; what a Windows logoff does to the discovery file; D31's footer, D33 and dev.17's trigger there | not run on Windows (C1); not looked at ([[D-394]]); the footer's render untested ([[D-422]]) |

---

## 10. Corrections a reader of this phase must not miss

The record corrects itself in place. These are the corrections that change
what a sentence elsewhere seems to say:

- **[[D-342]]'s "the main window, which is where Settings, the certificate
  list and the audit log are reached from anyway"** is false (§7, [[D-417]]).
- **[[D-337]]'s notification row** is contradicted by [[D-355]] §6, with the
  cause unknown.
- **[[D-393]]'s "a logout ends the agent this way" (SIGTERM)** is false on both
  backends ([[D-415]], [[D-416]]). The comment at `traysignal_other.go:15`
  still says it (open-items D30).
- **[[D-350]]'s PIN conclusions** were withdrawn by [[D-351]] and
  [[D-352]], and narrowed by [[D-355]].
- **[[D-380]]'s "likeliest sender"** was withdrawn by [[D-382]].
- **[[D-326]]'s five libraries** were corrected to thirteen by [[D-327]].
- **D21's "the Windows edit control's memory was measured"** was an
  invention; no entry measured it ([[D-397]]).
- **[[D-386]]'s "the CI logs could not be read"** was a statement about a
  tool ([[D-387]]).

Added on 2026-10-05:

- **[[D-421]]'s "Ubuntu's clean record … is evidence that 2.52 does not
  have the defect"** is false. WebKitGTK 2.52.6 has it (X2). Ubuntu is
  unexposed only because GTK 4.14 has a DPI value ([[D-425]]).
- **"White", in [[D-420]] to [[D-423]]**, is what the owner saw, not what
  was drawn. On the agent's `#ffffff` page a magnified white corner and an
  undrawn view look the same ([[D-424]]).
- **[[D-420]]'s AT-SPI `SHOWING` read as "painted"** was withdrawn in the
  same entry: the tree shows the page was built, not what is on the screen.
- **[[D-424]]'s premise that Ubuntu's portal refuses the process "too"**
  was about the file chooser's `OpenFile` ([[D-411]]). GTK 4.14 never asks
  the Settings portal ([[D-425]], open-items B29).
- **The log asserted a person when it did not know** (D34, [[D-425]]).
  Before dev.17 the quit line said "the person quit the agent from its own
  window", and Settings logged "the page sent cancel" for a close by Zatvori
  and for one from outside alike. One such line was written when the owner
  says nobody acted. dev.17's lines say what fired them and no more. The
  21:36:47 quit is unexplained.
- **[[D-423]]'s "agent B died of SIGHUP from a closed terminal"** is
  contradicted in the same entry. How B ended has not been read.
- **This report's §7 as written on 2026-10-03** ("no door", "Fedora
  never") describes dev.14. §7 now says what changed.

---

## The exit checklist, item by item

**The amendment**

- [x] SPEC §1 amended, conditioned on the platform ([[D-287]], §1.1)
- [x] Windows still builds with `CGO_ENABLED=0` — CI's "build windows/amd64"
      step on every push
- [x] `-race` on `cmd/liro-bridge` noted as newly possible ([[D-287]]); first
      run on Linux in [[D-330]]

**The worker**

- [x] PKCS#11 out of process, same binary, subcommand ([[D-297]];
      `pkcs11-worker` processes read in [[D-397]])
- [x] `RTLD_LOCAL` ([[D-349]]); one OS-locked goroutine ([[D-298]]);
      `CKF_OS_LOCKING_OK` (`module_linux.go`, `openModuleLocking`)
- [ ] **A module that kills its worker becomes a `Failure`, demonstrated with
      the module that does it**:
      - Windows: a real module crashed a discovery probe child and the listing
        survived ([[D-315]], by accident);
      - Linux: synthetic SIGABRT only;
      - the held worker killed by a real module: never (C9)
- [x] **`pkcs11reach_test.go` removed as part of the remedy, and the entry
      says so**: removed in commit `3ae0b86`. [[D-418]] is the entry that
      says so, written because this report found none on 2026-10-03
- [x] The subcommand signs nothing and cannot be driven into signing: the
      dependency-closure guards ([[D-290]])

**Windows on Linux**

- [x] Every window on GTK4 and WebKitGTK 6.0 ([[D-331]]–[[D-338]]), on the
      Ubuntu VM under llvmpipe. On Fedora: the signing, pairing and consent
      windows. Every one after a process's first was white until D32's
      workaround, and four were drawn under one agent in [[D-426]]. The main
      window and the three doors' windows were drawn under the same agent
      ([[D-428]]): eight in all (§7)
- [ ] The DMABUF and NVIDIA variables: set before GTK initialises, user values
      respected ([[D-329]], unit-tested); **never exercised** (§9)
- [x] The sandbox measured rather than assumed, on 6.8 and 7.0
      ([[D-390]]), with §4's limits

**Consent**

- [x] A new window per request ([[D-337]], [[D-341]])
- [x] A notification alongside; clicking it activates legitimately through
      the action's token ([[D-355]] §6)
- [x] Nothing in the design depends on the window being in front, by reading
      ([[D-288]])
- [ ] **The X11 fallback refused, with §4.1's reason recorded**: not refused.
      [[D-402]] measured that §4.1's reason was "true of xdotool and false of
      this attacker". The owner decided on `GDK_BACKEND=wayland` in Wayland
      sessions plus an amended clause; **decided, not built** (E9)

**The PIN**

- [x] Native, not in the page: the program's own locked field, and why
      ([[D-384]], [[D-385]])
- [x] Clause 2's requirements hold on the shipped field, on Ubuntu
      ([[D-392]]); Fedora's GTK not measured (A26)
- [x] `ulMinPinLen`/`ulMaxPinLen` read from the token (`module_linux.go`;
      [[D-349]], [[D-353]])
- [x] Nothing retries (SPEC §6.5.1 clause 5; [[D-301]]'s allow-list)

**The desktop**

- [x] **What a stock GNOME user sees, decided rather than defaulted**: decided
      in [[D-342]] and measured on Fedora. Since dev.15 a launch becomes the
      agent (D27) and the doors are in the main window (D31). There is no
      icon, and none is intended. Open-items F8 closed in [[D-428]] (§7)
- [x] **Settings, certificates and the audit log reachable without a tray**:
      on Ubuntu (R7, [[D-425]]) and on Fedora ([[D-428]]): from the main
      window's footer, the export made and verified, every window drawn and
      watched by the owner. [[D-419]]'s condition (§7)
- [x] Secret Service where present, a protected file where not, PIN in
      neither ([[D-343]], [[D-347]])

**Packaging**

- [x] `.deb` and `.rpm` from one nFPM configuration; Flatpak and Snap refused
      with the reason ([[D-354]])
- [x] Dependencies declared per distribution; installed on three clean images
      ([[D-354]], [[D-355]])
- [x] Built in `ubuntu:24.04`; the binary needs GLIBC 2.34 at most ([[D-354]])
- [x] Autostart as an XDG `.desktop` file ([[D-354]]; gnome-session's handling
      measured, [[D-355]] §8, [[D-364]])
- [x] The app-grid entry and "Open with", seen by the owner ([[D-355]])
- [x] `MimeType=application/pdf`; no context-menu verb ([[D-354]])

**Exit condition**

- [ ] **On Ubuntu 24.04 and on Fedora 44, a real card signs, verified twice**:
      Ubuntu yes, in a VM with the reader passed through ([[D-361]],
      [[D-397]]). **Fedora: signed, and verified by `pdfsig` and OpenSSL
      with controls** ([[D-420]]). The chain to the root and `verifypdf` are
      not checked (F1). The owner ruled Fedora done for F12 on it
      ([[D-420]]). **Left unticked by the owner's decision** ([[D-429]]):
      that reading does not reach [[D-361]]'s bar, so F1 stays open
- [x] The soft token signs on Ubuntu 24.04, Debian 13 and Fedora 44 in CI,
      both verifiers agreeing ([[D-386]], [[D-387]])
- [x] Which findings came from a VM, stated: this report, updated through
      [[D-430]]
