# F12 Linux — session 18: dev.14 on the Ubuntu VM

**What this is:** the handover at the end of session 17's build of dev.14
(D-413), written before the owner installs it, because the reboot after the
install ends the session that wrote it. **Written:** 2026-10-01.
**Working rules:** session 13 §C, session 15 §E, session 16 §E, session 17
§F, and §D below. D-304's five questions before believing any check.

---

## A. Where things stand

**dev.14 is built and installed nowhere.** At `27c59d7`, `vcs.modified=false`,
by `NFPM=$HOME/go/bin/nfpm build/linux/build.sh 0.9.9-dev.14 dist/linux`
(`nfpm` is not on this shell's PATH; the first attempt stopped there, before
any package was written).

| | sha256 |
|---|---|
| `dist/linux/liro-bridge_0.9.9-dev.14_amd64.deb` | `f41006ed8a4873641953cef3b10f7661f95198af3b844d0325268c58b0b4900f` |
| `dist/linux/liro-bridge-0.9.9-dev.14.x86_64.rpm` | `f5b1ecdbc22f63673af18ee2b3d13f0864705bceee2ad45aecf8274f6e88c022` |
| `/usr/bin/liro-bridge` inside dev.14's `.deb` | `df7bedd58b8cc10fa09253ed6b1a02d18612c5df4eab01dab880668a59ef6f61` |
| the same inside dev.13's `.deb`, and installed now | `9f88b4125dad3bdd0c8973c68bd061389e6dc1cfa5002d101c1b3c66455e9b6d` |

Against dev.13's binary, each taken from its own `.deb`: **NEEDED the same
13 libraries**, `Depends` identical; **one new undefined symbol,
`g_signal_connect_data`** (the C `load-failed` handler), defined in
`libgobject-2.0.so.0`, which is in NEEDED — read in defined symbols, with a
positive and a negative control.

On the Ubuntu VM: dev.13 installed and **running** — tray **2472**, started at
19:34:20 by this login's autostart, untouched. `config.json` as session 17
§A left it.

On the Fedora VM: unchanged — dev.12 installed. **dev.13 must not go
there.** dev.14's rpm goes after R5 holds here.

## B. What dev.14 carries (D-413)

Two crash fixes and nothing else; D-407 is held for dev.15 (open-items D27).

1. **The drop** (D25): the target is given its formats after construction.
   Its test is red 3/3 with dev.13's constructor put back, reading the
   perturbation byte. That makes D-412's cause a finding for the target's
   formats; **that a drag works is still R5's.**
2. **`load-failed`** (D26): connected in C. Its test, run with WebKit's
   sandbox off by the owner's ruling, is red 3/3 with the binding's handler
   put back.

## C. The order of the readings

1. **The install, the owner's hands** (it needs root; nothing here may
   prompt for it): `sudo apt install ./dist/linux/liro-bridge_0.9.9-dev.14_amd64.deb`,
   then reboot. **One change per boot (D-391)**: the predecessor file is
   dev.13's `.deb`, `9f88b412…` inside it, and the boot's one change is
   this install.
2. **The first reads**, session 17 §E, with dev.14's hashes above:
   `uptime -s`, `last -x -F`, logind in `-b 0`; the install in
   `apt/history.log` and nothing after it in `dpkg.log`;
   `/usr/bin/liro-bridge` against `df7bedd5…`; the tray by
   `ps -C liro-bridge`, tied to the file by start time after its ctime and
   by `bridge.log`'s start line saying `0.9.9-dev.14 27c59d7`.
   `liro-bridge --version` writes a start line; say so.
3. **R5 first**, with session 17 §C's predictions and the monitor of
   session 17 §D (its positive control before the drag). **Let the window
   stay open a while, and drag more than once**: dev.13's crash needed the
   collector to have run.
4. R1 once; R6; R7; then C2 on Xorg; then session 16 §D on Fedora with
   dev.14's rpm, its hash checked there and `rpm -K` read first.

## D. Rules added in session 17

- **The GIR can be wrong.** D-412's rule compares the binding with the GIR;
  for `gtk_drop_target_async_get_formats` both say transfer full and the C
  takes no reference. For a boxed value from a getter, read the C — the
  machine code of the installed library when the source is not here
  (`nm -D` for the address, `objdump -d` from it) — before trusting either.
- **A test of a failure path reads every guard in front of it first.** The
  first `load-failed` test asked for a missing page, and the navigation
  policy refused it before any load (D-413).
- **A parent that believes a child's answer prints it on success too**,
  or the run cannot show that it ran.
- **A list read from tool output is checked for its decoration before it
  is used as names**: `readelf -d` prints `[lib.so]`, and a lookup by those
  names found nothing, quietly (D-413's build reads).
