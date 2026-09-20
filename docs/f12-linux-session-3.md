# F12 Linux — session 3

**What this is:** the third session on the Linux VM, and the one that did not
advance §3. Sessions 1 and 2 settled §3.1 and §3.2 and built the window host;
`docs/f12-linux-session-1.md` carries both. This session went backwards first
— into why the machine died twice — and then sideways, into the first time
any of the Linux work was compiled somewhere other than this VM.

**Written:** 2026-09-20, Ubuntu 24.04.5 VM, kernel 7.0.0-31-generic.
**Entries this session:** D-334 (the CI failure, and the two unrelated
causes inside it).

**The honest headline: the window did not open.** It was built, it ran, and
it stopped in one place, for one reason, which §3 of this document records.
Everything else here is either diagnosis or the CI work that the push
uncovered.

---

## 0. What this session established, in one list

- Neither crash was memory. **No OOM kill and no kernel panic is recorded in
  either boot**, and swap is 0, so an exhaustion would have had to go through
  the OOM killer and be in the log. §1.
- The two crashes have **different shapes**, and only one of them had any
  workload at all. §1.
- The window host **builds and starts** — GTK4 initialises, the WebView is
  constructed, the window is built, handlers connect — and aborts at exactly
  one call. §3.
- The Linux work had **never been built off this machine**, and the first push
  that carried it failed in 32 seconds. D-334. §4.
- The binary this project ships is **unaffected by all of it** and still links
  no C. §4.

---

## 1. Why the machine died, twice

Both crashes ran on **6 GB and 6 CPUs** (`Memory: 4707368K/6143544K`) — the VM
was 4 GB through session 1 and was resized at the 19:29:58 boot, so §1's
"fits this 3.9 GB VM" in session 1 no longer describes this machine.

**Neither boot names an OOM kill or a panic.** Grepped for `Out of memory`,
`oom-kill`, `Killed process`, `Kernel panic`, `Oops`, `BUG: unable to handle`
— nothing in either. **That absence is partly a property of the medium**: if
the machine dies hard, journald is already gone, so what survives is the
lead-up rather than the death. A VirtualBox Reset from the host leaves no
guest record at all, which is consistent with a log that simply stops.

**Crash A — 19:44:47 to 21:20:01.** The log ends mid-`sysstat-collect` with no
shutdown record. Session 2's last commit was 21:19 and its transcript ends at
21:20:41, so the machine went about ninety seconds after committing. Three
waves of `watchdog: BUG: soft lockup` at 20:46, 20:49 and 21:02, naming
`cc1`, `cgo`, `vet`, `link`, `compile` — and **also `swapper/0`, `swapper/1`,
`swapper/3`, `swapper/4`.** The idle task cannot be stuck on a build. Five or
six CPUs losing time at once, with `nmi_backtrace_stall_check` reporting a
wrapped `last activity: 4294748346 jiffies ago`, is the guest losing the
clock rather than the guest being overloaded.

**Crash B — 21:33:06 to 21:37:43.** **Nothing was running.** No session, no
build. It still died, and here the kernel names something:

```
rcu: INFO: rcu_preempt detected expedited stalls on CPUs/tasks: { 3-...D }
drm_mode_dirtyfb_ioctl+0x1a6/0x1e0
vmw_generic_ioctl+0xc2/0x190 [vmwgfx]
vmw_unlocked_ioctl+0x15/0x30 [vmwgfx]
```

Then `polkitd`, `systemd-logind`, `accounts-daemon`, `snapd` and `umount` all
`blocked for more than 122 seconds`, and processes surviving `SIGKILL`.

**And `vmwgfx` says this on every boot, including the current one:**

```
vmwgfx [drm] *ERROR* vmwgfx seems to be running on an unsupported hypervisor.
vmwgfx [drm] *ERROR* This configuration is likely broken.
```

The host is VirtualBox (`DMI: innotek GmbH VirtualBox`) presenting a VMware
SVGA adapter. This is the same fact session 1 met from the other side as
`VMware: No 3D enabled`.

**What this does not establish:** that `vmwgfx` caused crash A. The kernel
does not say so, and crash A's shape is different. Recorded as two findings
rather than one cause.

**A third loss, earlier and already explained.** The machine was also lost
around 19:06, and that one *is* the workload: D-328's eight busy-loop
processes, 2 h 21 min, no commit, one reset. **That is the only one of the
three that load explains**, which is worth stating because it is the one that
invites the generalisation.

---

## 2. What sessions 1 and 2 left, without restating it

Read the entries rather than this paragraph: **D-323** to **D-333**, and
`docs/f12-linux-session-1.md` for the working state between them. §3.1 closed
on gotk4 v0.3.1 (**D-327**), §3.2 on the AppArmor profile (**D-324**,
sharpened by **D-329**), the window host itself in **D-331**, with **D-330**
and **D-332** the two corrections that cost the most.

**Eight commits sat unpushed** from 19:50 to 21:19 for the whole of this
session's first half. They are pushed now, and pushing them is what produced
§4.

---

## 3. The window: built, started, and stopped in one place

The smallest thing that would prove the stack end to end — a GTK4 window, a
WebKitGTK view, one of this program's own pages over `liro://`, on screen.

**Built in 10.2 seconds** against the warm 2.0 GB build cache, which is
D-327's warm figure rather than its cold one. Binary 21,957,224 B, and
`readelf -d` reports **exactly 13 `DT_NEEDED`** — D-327's number, confirmed
from a second binary:

```
libwebkitgtk-6.0 libgtk-4 libpango-1.0 libgdk_pixbuf-2.0 libcairo-gobject
libcairo libgraphene-1.0 libsoup-3.0 libglib-2.0 libgio-2.0
libjavascriptcoregtk-6.0 libgobject-2.0 libc
```

**Three things the run established before it stopped:**

```
[  0.00s] webkit env set: [WEBKIT_DISABLE_DMABUF_RENDERER __NV_DISABLE_EXPLICIT_SYNC]
Overriding existing handler for signal 10. Set JSC_SIGNAL_FOR_GC ...
bwrap: setting up uid map: Permission denied
SIGTRAP: trace trap
```

1. **D-329's `PrepareWebKitEnvironment` works** — both variables set, as the
   first statement, before GTK touched anything.
2. **The SIGUSR1 contention is real and visible**, not inferred. Session 1
   listed it as open; this is it happening.
3. **It died at one call**, and the stack names it:

```
ui.NewWindow.func1()              window_linux.go:172
webkit.(*WebView).LoadURI
_Cfunc_webkit_web_view_load_uri
```

**This is the first time D-324's failure has been reproduced through the real
window host.** D-324 met it from Python and from a bare cgo probe; this is
`ui.NewWindow` itself, which is the shape a person would actually meet.
Everything up to `LoadURI` worked.

### What it is blocked on, and what is staged

A Go binary has no AppArmor profile here — the probe profile from session 1
names `/usr/bin/python3.12` and nothing else — so `bwrap` cannot map its
child's uid. D-324 already decided the remedy: **a profile naming the binary
that starts the tree.** It needs `sudo`, which wants a password on this VM.

Staged and ready: the binary at **`/home/vboxuser/liro-f12probe`**, and

```
sudo tee /etc/apparmor.d/liro-f12-window >/dev/null <<'EOF'
abi <abi/4.0>,
include <tunables/global>
profile liro-f12-window /home/vboxuser/liro-f12probe flags=(unconfined) {
  userns,
}
EOF
sudo apparmor_parser -r /etc/apparmor.d/liro-f12-window
```

**`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` is the other way past it and is
the worse one**, for the reason D-327 already paid: a run with the sandbox off
says nothing about §3.2. It was also refused by this session's tool policy,
which reads the variable name as a bypass — a correct refusal from a
mechanism that cannot tell a deliberate measurement from a careless one.

---

## 4. CI: the first build anywhere but here

**The entry is D-334** and is not restated. The short form:

- The push failed in **32 seconds** at `go vet ./...` with `Package glib-2.0
  was not found in the pkg-config search path`. Everything else green.
- **There are two failures, not one.** The second is `build linux/amd64`,
  which runs `./...` under `CGO_ENABLED: 0` — and with cgo off gotk4's
  packages have *no buildable Go files at all*. **No `apt-get install` fixes
  that.** The two look alike in a log and are unrelated.
- **Two packages, not the five this VM has**: `libgtk-4-dev` and
  `libwebkitgtk-6.0-dev`. Closure measured with `apt-cache depends
  --recurse`: **618 packages against 646**. `libwebkit2gtk-4.1-dev` is the
  GTK3 stack §3.1 rejected; `libpcsclite-dev` is §9's and nothing imports it.
- **Cost:** first run 15–22 minutes (D-327's cold gotk4), steady state ~7–8,
  because `actions/setup-go@v5` already caches the build cache keyed on
  `go.sum`. Caching is not an optimisation here; it is what makes it
  affordable.
- **`build linux/amd64` narrowed to `./cmd/...`**, because what that step
  proves is that the shipped binary links no C — and it still does.

**What ships is untouched by all of it.** Measured, not assumed:
`cmd/liro-bridge` imports neither `internal/ui` nor `internal/pinscreen` on
linux, and `CGO_ENABLED=0 GOOS=linux go build ./cmd/liro-bridge` still gives
`DT_NEEDED 0`, no `PT_INTERP`, ~10.2 MB. **SPEC §1.1 is intact.**

**One package is the whole cause.** `internal/pinscreen` imports
`internal/ui` unconditionally, and nothing on linux imports `internal/pinscreen`
— so a package reachable by no shipped code on this platform is what made
every module-wide sweep require a GTK stack. Worth a look before §5 builds on
it.

---

## 5. What this machine has that a clean one does not

Session 1 §3a is still the list and is still accurate. What this session adds:

- **Five apt packages were installed; two were needed.** The other three
  (`libwebkit2gtk-4.1-dev`, `libpcsclite-dev`, and `pkg-config`, which the CI
  runner already has) were not load-bearing for §3.1. D-334.
- **`~/go/bin` is not on `PATH`.** `.bashrc` adds `/usr/local/go/bin` and
  `~/.local/bin` only, so `golangci-lint` — which session 1 records as
  installed, and which is installed, v2.13.2 — does not resolve in a fresh
  shell. Invoke it by absolute path, or add the line.
- **`~/.cache/go-build` is 2.0 GB and `~/go/pkg/mod` is 527 MB.** Both survive
  a reset. **Every build timing taken here from now on is a warm one** unless
  the cache is deliberately cleared — this session's 10.2 s against D-327's
  14 m 52 s is the whole spread.
- **The kernel is not the ISO's**: `linux-generic-hwe-24.04` → 7.0.0-31, and
  VirtualBox Guest Additions 7.2.18 via DKMS, which taints it `OE`.
- **The probe AppArmor profile is still loaded** and still names
  `/usr/bin/python3.12`. It remains the one thing on this machine that can
  silently change a measurement.

---

## 6. The standing rules this VM runs under

Three crashes and one two-hour loss in a day are the reason. They are rules,
not preferences.

- **No load generators, ever.** From the owner, after D-328. A measurement
  that needs a loaded machine is recorded as **unmeasurable on this VM**
  rather than attempted.
- **One build at a time. Nothing in the background.** The two-hour loss was
  two background samplers being polled rather than one job being watched.
- **No fuzzing here.** Same entry, same reason.
- **Say how long before anything long.** Two hours is acceptable when it is
  known to be two hours; it is not acceptable when it is indistinguishable
  from a wedged machine. This session gave an estimate before its one build —
  1–4 minutes warm, 10–15 cold, wedged past 20 — and it took 10.2 seconds.

---

## 7. What the next session should do first

**Install the profile in §3 and open the window.** It is one `sudo` command
and it is the only thing between the current state and a window on screen.
Everything in §3 up to `LoadURI` already works; nothing after it has been
observed at all.

Then, in order:

1. **Push the CI fix and watch one run.** It is committed but not pushed.
   Expect 15–22 minutes and expect it to be the first run that has ever
   compiled gotk4 off this machine. **D-334 names the one thing that could
   still fail**: `-race` implies `checkptr`, D-330 measured gotk4 failing it,
   and the UI tests' `t.Skip("no windowing system")` is a runtime skip rather
   than a compile-time exclusion.
2. **§4, consent — and it is a design question before it is code.** No Wayland
   protocol lets a client raise itself; D-331 already accepts and ignores
   `AlwaysOnTop` for that reason; `CollectPIN` is defined in exactly one file,
   `internal/ui/pindialog_windows.go`, and has **no Linux half to adapt**. So
   SPEC §6.5's mechanism has to be designed rather than translated, and §4.1
   has already refused the X11 fallback that would restore it. **The shape
   should reach the owner before anything is built.**
3. **Not §6.** The tray's first question — what happens when nothing is
   watching — is unanswered, and there is still no `StatusNotifierWatcher` on
   this session bus.

**§0.1 applies to everything above.** This is a virtual machine with no
working GPU driver and a graphics driver that declares its own configuration
broken on every boot.
