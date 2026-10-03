# F12 — the phase report

**Date:** 2026-10-03
**Exit condition: not met.** The Ubuntu half holds: the installed package,
approval in the window and a real card in a real reader signed a PDF that the
independent verifier of SPEC §16.4 and OpenSSL both accept ([[D-361]]), and
both demos ran to written, verified PDFs ([[D-397]]). The OS was a VirtualBox
guest with the reader passed through to it. **The Fedora half has never been
taken: no card has been in a reader on Fedora 44** (open-items F1).

F12's own **Report:** line asks for seven things. §§2–7 answer them in its
order; §8 is the provenance table that §7 asks for, and §9 is the other half of
that table: what was never taken. §1 comes first because it governs how
everything after it is read.

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
  "no GPU" ([[D-361]]). **Anything about how a window draws, maps, sizes or
  repaints is a reading of a software renderer.** That covers the 37 px
  header-bar correction ([[D-339]]), the layout checks ([[D-372]]),
  focus-stealing timing ([[D-337]]), the drop outline ([[D-415]], [[D-417]])
  and the chooser geometry ([[D-415]], [[D-417]]). The two variables F12 §3.2
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
  reader, and it was never taken on Fedora at all.
- **Harnesses are not the installed agent.** The PIN-field memory and
  accessibility readings were made with `scripts/pinmem`, which links
  `CollectPIN` into its own process. It is not the tray, and it does not
  measure the installed package ([[D-391]], [[D-392]]). The early window-host
  readings ran with WebKit's sandbox switched off
  (`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`, [[D-326]]–[[D-331]]). The
  window tests run only from a profiled path, never under plain `go test`
  and never in CI (open-items A29, C5).
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
- **Not taken**: any vendor module on Fedora; MUP's card on Linux, which
  neither SafeSign nor OpenSC reads (`token not recognized`, [[D-365]]).

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
- **Not measured**:
  - Fedora 44's GTK with the same script (open-items A26);
  - the keystroke path in front of the field, on either platform. On Linux
    the compositor's sight of every key is conceded in SPEC §6.5.1 clause 3
    ([[D-398]]). On Windows, TSF/IME is unmeasured.

## 7. What a stock GNOME desktop shows when the agent is running

**On Fedora 44: no tray icon, no agent until the next login, and no route
to Settings, the certificates or the audit log.**

- **No tray host**: no `StatusNotifierWatcher` on Fedora's session bus,
  against a control ([[D-405]], [[D-406]]). The agent runs and registers its
  item, and nothing shows it. Its "accepted" line is logged at DEBUG and so
  never appears ([[D-417]]). Ubuntu ships an AppIndicator extension and shows
  the icon ([[D-342]]).
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
  without a tray" is therefore **not met**. What the door should be is the
  owner's to decide, for dev.15.
- **The window itself**:
  - dev.12 could not be given a document at all, because the portals refuse
    the non-dumpable process ([[D-408]]);
  - dev.14 can: through the chooser helper, and by drop ([[D-417]]);
  - the window's process is refused by the Settings portal too, with the cost
    unread (open-items B29);
  - a logout ends the tray inside GDK, not by SIGTERM. That was measured on
    Ubuntu ([[D-415]], [[D-416]]) and never on Fedora.

---

## 8. Provenance: where each finding was taken

Locations, as the entries name them:

| machine | what it is | the entries' own words |
|---|---|---|
| **Ubuntu VM** | VirtualBox guest, Ubuntu 24.04 userspace, kernel 7.0 HWE (6.8.0-142 added in [[D-390]]); GNOME 46; GTK 4.14.5; WebKitGTK 2.52.6; llvmpipe; Wayland, and "Ubuntu on Xorg" for [[D-402]], [[D-415]], [[D-416]] | "the VM every Linux measurement in F12 is taken on" ([[D-346]]), until Fedora |
| **Fedora VM** | VirtualBox guest, Fedora Workstation 44; GNOME 50.5, GTK 4.22.5, WebKitGTK 2.54.0 after the update ([[D-406]]); SELinux Enforcing; Wayland only (no GNOME on Xorg, [[D-405]]) | [[D-405]]–[[D-409]], [[D-417]] |
| **passed-through hardware** | the Realtek `0bda:0165` reader via VirtualBox USB, into the Ubuntu VM only; the owner's Pošta card (SafeSign 4.6) and MUP card | "the OS is a VM, the reader and card are hardware" ([[D-361]]) |
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
| no route to Settings, certificates, audit log on stock GNOME | Fedora VM, and the code | measured + read | [[D-417]] |

---

## 9. What was never taken

This table is the one to read before quoting anything above. Each row is a
claim someone could reasonably make about F12 and that **no reading
supports**.

| never taken | why it matters | where it stands |
|---|---|---|
| **A real card on Fedora 44** | the exit condition's second half | open-items F1. No reader has been passed to the Fedora VM; Pošta ships RedHat builds, not Fedora ([[D-365]]) |
| **Linux on real hardware** | every Linux reading is a VirtualBox guest | §1 |
| **Any GPU: the DMABUF and NVIDIA variables** | F12 §3.2's two failures "on somebody's machine" | built ([[D-329]]), never exercised; open-items B6 |
| **A fresh stock Ubuntu 24.04 install** | "stock 24.04" in F12 §3.2 and the checklist | the VM is updated userspace on HWE with a 6.8 kernel added ([[D-388]], [[D-390]]) |
| **A real module killing the held worker, on Linux** | F12 §2's box | synthetic SIGABRT only (open-items C9); Windows saw a real module crash a probe child ([[D-315]]) |
| **MUP's card signing on Linux** | the other Serbian card | neither SafeSign nor OpenSC reads it ([[D-365]]); `ubavic/srb-id-pkcs11` not tried (E3) |
| **Halcom** | a third issuer | no signature ever verified (C15) |
| **Settings, Sertifikati and the audit log reached on stock GNOME** | a checklist item | no route exists ([[D-417]], D31) |
| **SELinux's verdict on the sandbox** | Fedora's half of §4 | root's audit log, never read (B8) |
| **Any compositor but GNOME's** | focus, raise, the tray | KDE never run (B5) |
| **The consent window on X11** | E9's half | inferred from Settings ([[D-402]]) |
| **Fedora's GTK against the PIN field and the a11y bus** | the shipped field's guarantee on GTK 4.22 | A26 |
| **The keystroke path before the field, on Windows** | B1's Windows half | TSF/IME unmeasured; the WinEvent method had no control (B24) |
| **A logout on Fedora; any logout under linger; two users** | D30, B19 | Ubuntu measured two logouts; the rest not taken, partly by the owner's decision (B19, D-396, D-400) |
| **A day of requests** | the web-process leak fix | nine windows in forty minutes ([[D-397]]), not a day (C2) |
| **Load of any kind** | WebKit's SIGUSR1 against the Go runtime; the fuzz flake rate | no load generators, by rule (B16, B21, [[D-328]]) |
| **The real signing key on a real tag** | package signing | only a throwaway key in CI (C16) |
| **The chooser's 30-minute ceiling sentence on screen** | the one time it ran, nobody saw it | C20 ([[D-417]]) |
| **R6 on Ubuntu's Wayland with the X uncovered; the outline there** | separates refusal from coverage, backend from versions | B28, D28 |
| **The ported sign flow's tests on Linux** | every test of it is `*_windows_test.go` | covered by compilation and one run, "not the same as tested" ([[D-338]]); C11 |
| **The window tests in CI** | they need a display and a profiled path | they skip on every runner (C5, A29) |
| **Windows since the Linux work** | the two-window fix; what a Windows logoff does to the discovery file | not run on Windows (C1); not looked at ([[D-394]]) |

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
- [ ] **`pkcs11reach_test.go` removed as part of the remedy, and the entry
      says so**: removed in commit `3ae0b86`. I found no entry that says so:
      "pkcs11reach" appears in no F12 entry. The removal is done; the record
      of it is not
- [x] The subcommand signs nothing and cannot be driven into signing: the
      dependency-closure guards ([[D-290]])

**Windows on Linux**

- [x] Every window on GTK4 and WebKitGTK 6.0 ([[D-331]]–[[D-338]]), on the
      Ubuntu VM under llvmpipe. On Fedora only the signing window has been
      opened, because the others have no route (§7)
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

- [ ] **What a stock GNOME user sees, decided rather than defaulted**: decided
      in [[D-342]]; measured on Fedora, and what is seen is no icon, no agent
      until the next login (D27) and no door (D31)
- [ ] **Settings, certificates and the audit log reachable without a tray**:
      **not met** ([[D-417]]; §7)
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
      [[D-397]]); **Fedora never**
- [x] The soft token signs on Ubuntu 24.04, Debian 13 and Fedora 44 in CI,
      both verifiers agreeing ([[D-386]], [[D-387]])
- [x] Which findings came from a VM, stated: this report
