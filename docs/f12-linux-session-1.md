# F12 Linux — session 1

**What this is:** what was measured on the first Linux machine this project
has ever had, and what is blocked and on what. `docs/decisions.md` D-324 to
D-327 are the entries; this is the working state between them, and what the
next session needs that is not a decision.

**Written:** 2026-09-20, Ubuntu 24.04.5 VM.
**Entries this session:** D-324 (§3.2, the sandbox), D-325 (§7.1, the
discovery file), D-326 (§1, the linkage of a cgo-direct binary), D-327
(§3.1 closed, and D-326 corrected), and a pointer block on D-322 (the
instrument count reached nineteen).

**§3.1, §3.2 and §7.1 are all answered.** What is left on this machine is
listed in §4.

---

## 0. The machine, and the thing that stops being true

**There was no Liro state on this machine when this session started** — no
`~/.config/liro`, no `~/.local/share/liro`, no `~/.local/state/liro`, no
`$XDG_RUNTIME_DIR/liro`, no `~/.config/autostart`, nothing named `liro`
anywhere under `$HOME` outside the repository. Recorded because the next
session will need to know when that stopped being true, and this session did
not change it: nothing in it ran the agent.

`go test ./internal/... ./scripts/...` is green here, untagged and with
`-tags softtoken`. `go build` succeeds for `linux/amd64`, `windows/amd64`,
`darwin/amd64` and `darwin/arm64`. `go vet -unsafeptr=false` and
`golangci-lint` 2.13.2 report **0 issues** in the `linux`, `windows` and
`darwin` views and in both tagged views. `checkdeps` OK (51 packages),
`checkcss` OK.

**D-308's icon test does not fire here, and that is a third machine agreeing
with D-308's diagnosis.** This VM has exactly go1.26.5, which is what `go.mod`
names, so `compress/flate`'s output matches the committed asset. D-308 said the
test compares the toolchain's compressor rather than the icon; a machine where
the versions agree passing it is the other half of that.

**`golangci-lint` was not installed** and is now, by
`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2`
— the version `ci.yml` pins, so what runs here and what runs on CI are the
same check. No `sudo`, and it lands in `~/go/bin`.

---

## 1. §3.1 is answered: gotk4 v0.3.1, pinned

**The decision is D-327.** `github.com/diamondburned/gotk4/pkg` **v0.3.1**
with `github.com/diamondburned/gotk4-webkitgtk/pkg` `webkit/v6`, pinned,
against a platform floor of Ubuntu 24.04 LTS. It builds, it runs, and this
project's whole page↔Go surface works through it. What follows is the
supporting measurement.

### The candidates, and three of four are already out

| candidate | what its own cgo line demands | verdict |
|---|---|---|
| `webview/webview_go` | `gtk+-3.0 webkit2gtk-4.0` | **excluded** — 24.04 ships 4.1 and 6.0 and no 4.0 at all. `libwebkit2gtk-4.0-dev` is not in the archive; `libwebkit2gtk-4.0-doc` is a transitional dummy. It cannot build here. |
| `gotk3/gotk3` | `gtk+-3.0` and friends | **excluded** — GTK3 only, where F12 §3.1's floor is GTK4. |
| `gotk4/pkg` + `gotk4-webkitgtk/pkg` webkit/v6 | `gtk4`, `webkitgtk-6.0` | **the only candidate that reaches the floor** |
| the same pair's `webkit2` | `webkit2gtk-4.1` | builds on 24.04 and is the GTK3 stack F12 §3.1 rejects |

On disk: gotk4 20 MB, gotk4-webkitgtk 6.0 MB, of which `webkit/v6` is 1.2 MB.

### It covers every API this project's own decisions rest on

Checked against the entries rather than against a feature list, because the
question is not "is it a good binding" but "does it expose what has already
been decided":

| decision | what it needs | |
|---|---|---|
| D-083 | the three-message page→Go surface | `RegisterScriptMessageHandler`, `UserContentManager`, `EvaluateJavascript` — **present** |
| D-259 | a window refuses to become any document but its own page | `ConnectDecidePolicy`, `NavigationPolicyDecision`, `PolicyDecision.Ignore`, `NewWindowAction` — **present** |
| D-082, D-150 | assets from a virtual host, never `file://` | `WebContext.RegisterURIScheme(scheme, callback)`, `SecurityManager` — **present** |
| D-100, D-122 | a capture that does not take the foreground | `SnapshotRegion`, `SnapshotOptions`, `SnapshotFinish` — **present** |
| D-114, D-123 | dropped files reach the frame | `gtk4` `DropTarget`, `NewDropTarget`, `ConnectDrop` — **present** |

**Two of these were first reported absent and were my grep rather than the
binding** — `URISchemeHandler` is a callback type and not a handler type, and
the snapshot method is `SnapshotFinish` rather than `GetSnapshot`. D-296's
second question, asked of an instrument that was reporting absences.

### Two things that are genuinely missing, and one of them is a wall

**The binding is generated against WebKitGTK 2.42 and this machine runs
2.52.6.** The highest versioned file in `webkit/v6` is `_2_42`, and the module
is a pseudo-version dated **2024-01-08** — twenty months stale as of today.
Being generated rather than hand-written makes staleness cheaper than it would
otherwise be, and it does not make it nothing: anything WebKitGTK added after
2.42 is not reachable, and nobody has regenerated it against a library this
project has to support.

**GTK4 has no tray, and the obvious remedy cannot be used.** `GtkStatusIcon`
was removed in GTK4 and `gotk4`'s `gtk/v4` has no replacement — measured, not
assumed. The usual answer is `libayatana-appindicator`, and on this machine:

```
libayatana-appindicator3-1  Depends: libgtk-3-0t64 (>= 3.0.0)
```

**It is a GTK3 library.** Loading GTK3 and GTK4 into one process is not
supported and does not work, so the C library is not available to a GTK4
agent at all. What is left is speaking **StatusNotifierItem over D-Bus
directly** — which SPEC §1.1's fifth clause already prefers on its own terms:
*"a pure-Go D-Bus client compiled at `CGO_ENABLED=0` was measured to stay
fully static"*, and *"a dependency that can be satisfied in Go is not a reason
to declare one"*. So the tray, if there is one, is Go and D-Bus and links
nothing.

### The linkage, and the correction that matters most to §8

D-326 measured a **cgo-direct** program — GTK4 and WebKitGTK reached straight
from `#cgo pkg-config`, no binding — and concluded the linkage is a property
of the platform. **That conclusion is wrong and D-327 corrects it**; both
entries were written the same day and neither had been pushed, so the
correction is written as one rather than the entry quietly edited.

| | the agent today, `CGO_ENABLED=0` | GTK4 + WebKitGTK 6.0 |
|---|---|---|
| `PT_INTERP` | none | `/lib64/ld-linux-x86-64.so.2` |
| `DT_NEEDED` | 0 | **5** |
| imported symbols | 0 | 58 |
| size | 10 231 252 B | 2 395 320 B |
| cold cgo build | — | 12.5 s, 286 MB peak |

**Built through the binding instead, the same program names thirteen** —
because every generated Go package carries its own `#cgo pkg-config` line, so
pango, cairo, graphene, gdk-pixbuf, libsoup and JavaScriptCore move from
transitive to declared. None of them is a new dependency at run time; all
eight extras were already in the 131-object closure.

| | cgo-direct | through gotk4 v0.3.1 |
|---|---|---|
| `DT_NEEDED` | 5 | **13** |
| imported symbols | 58 | **10 094** |
| size | 2 395 320 B | 21 049 752 B |
| `dpkg-shlibdeps` | 4 packages | **11 packages** |

**So §8's `Depends` is the eleven-package line, generated from the binary
that ships** — and the version floors move with it, `libgtk-4-1 (>= 4.0.0)`
becoming `(>= 4.14.1)`. 24.04 ships 4.14.5 and satisfies it; nothing in the
build would have said so until a `.deb` refused to install somewhere older.

### The cost, and the wall that is a version rather than a package

| | |
|---|---|
| cold build, empty cache, 4 CPUs | **14 min 52 s** |
| peak resident | **1.93 GiB** — fits this 3.9 GB VM, no OOM |
| binary | 21 049 752 B |
| warm rebuild of one package | 4.1 s |

Fifteen minutes cold is a **CI** number, not a developer one, and it is what
§8's pipeline has to budget on a runner that starts empty every time.

**gotk4 v0.4.0 and v0.4.1 do not compile on this platform at all.** They name
five GLib functions that do not exist in 24.04's GLib 2.80, and their cgo line
is a bare `pkg-config: glib-2.0` with no minimum version — so configuration
succeeds and the mismatch surfaces fifteen minutes later as unresolved C
references in generated code. v0.3.1 (2024-07-31) is the last release that
builds here; v0.4.0 arrived two years after it. **The pin is the finding**,
and `go get -u` is what breaks it.

---

## 2. §3.2 is answered, and the answer is in D-324

The short form, because the entry is long:

- **WebKitGTK 6.0 does not start on this machine with nothing set.** `bwrap`
  cannot write its child's uid map, and the process aborts with a core dump.
  Not a blank window.
- **The mechanism** is that creating a user namespace *succeeds* and transitions
  the process into the `unprivileged_userns` AppArmor profile, whose first line
  denies every capability — and bwrap's parent needs `CAP_SETUID` to map its
  child.
- **The remedy is a four-line AppArmor profile naming the binary**, which is
  what Ubuntu ships 93 of in `/etc/apparmor.d/`. That is §8's packaging work
  and it is per-distribution: Fedora's SELinux has no equivalent restriction.
- **With the sandbox off it renders correctly**, so §3.2's blank-window failure
  does not occur here.
- **This VM is not the DMABUF case the handover supposed.** VirtualBox's 3D
  setting is on and Mesa cannot get a driver — `VMware: No 3D enabled`. It is
  a software renderer, so it has said nothing about the path §3.2 warns of.

**That step is now closed, and the answer is sharper than "a profile works".**
With a profile naming only the interpreter the probe runs under, WebKitGTK
starts and renders with **no environment variables set at all**, and the
transition is what the profile stops — observed from inside the process:
`liro-f12-probe (unconfined)` before and after `unshare(CLONE_NEWUSER)`,
against `unconfined` → `unprivileged_userns (enforce)` without it.

**The profile must name the binary that starts the tree, not bwrap**, because
bwrap inherits it. Measured both ways: `bwrap` run from a shell still fails,
`bwrap` run from the profiled binary succeeds and reports the inherited label.
So what §8 ships is a profile naming the agent's own installed binary, which
is the shape every one of Ubuntu's 93 profiles already has.

Confirmed a second time from Go rather than from Python: the cgo GTK4/WebKit
binary has no profile and fails identically, surfacing as `SIGTRAP: trace
trap` — a Go runtime crash, which is how a person would meet it.

**The probe profile is still loaded on this machine.** It names
`/usr/bin/python3.12` and nothing else. Remove with
`sudo apparmor_parser -R /etc/apparmor.d/liro-f12-probe && sudo rm /etc/apparmor.d/liro-f12-probe`.

---

## 3. §6's premise measured early, because it was free

Not decided — §6 is a product decision the phase document reserves — but
measured, because it was one command and it bears on §3.1's tray:

**There is no `StatusNotifierWatcher` on this session bus.** Three spellings
tried, against a control that first confirmed `busctl` can see
`org.gnome.Shell`. Ubuntu *does* ship
`gnome-shell-extension-appindicator` and it is installed; it is not on the bus
here.

So F12 §6's sentence — *"Ubuntu ships an AppIndicator extension; Fedora does
not"* — is true and is not the whole of it: **shipping it and it being active
are different things**, and on this installation a tray icon published right
now would have nobody watching. Whether that is this image, this session, or
the default is not established and is worth one check on a second machine
before §6 is decided on it.

---

## 3a. What is on this machine that is not on a clean one

Recorded so the next session knows what it is standing on, and so that a
measurement that silently depends on one of these can be spotted.

**Installed by `apt` (packages — what is on the machine):**

```
pkg-config  libgtk-4-dev  libwebkitgtk-6.0-dev  libwebkit2gtk-4.1-dev
libpcsclite-dev
```

`libwebkit2gtk-4.1-dev` is the GTK3 alternative §3.1 asks to compare and was
not in the end needed, since GTK3 is ruled out on §3.1's own floor.
`libpcsclite-dev` is §9's and is not yet used by anything.

**Unpacked into a user prefix, because `sudo` here wants a password:**

```
~/.local/gir-prefix/     libgirepository1.0-dev, libgirepository-1.0-dev,
                         gir1.2-girepository-2.0-dev, gobject-introspection
                         (1.80.1-1, fetched with `apt-get download`,
                          unpacked with `dpkg-deb -x`)
```

The `.pc` files in it have their `prefix=` rewritten to point at the prefix,
and `libgirepository-1.0.so` — which ships as a relative symlink and dangles
outside `/usr` — is repointed by absolute path at the system library. Reach it
with:

```
export PKG_CONFIG_PATH=$HOME/.local/gir-prefix/usr/lib/x86_64-linux-gnu/pkgconfig:$PKG_CONFIG_PATH
```

**On a machine with working `sudo` this is one line instead**, and every §3.1
number is worth re-taking there if it ever decides anything:

```
sudo apt-get install -y libgirepository1.0-dev
```

**Installed by `go install` (no root):**

```
golangci-lint v2.13.2        -> ~/go/bin, the version ci.yml pins
```

**Changed about how the system behaves — one thing, and it is still in
place:**

```
/etc/apparmor.d/liro-f12-probe    a profile naming /usr/bin/python3.12,
                                  whose entire content is `userns,`
```

**That one matters more than the packages and is called out separately for
the reason the owner gave: it is a thing a measurement here could silently
depend on.** Any WebKitGTK measurement taken on this machine through
`python3` is now taken with the sandbox working, where a clean 24.04 would
abort. Anything measured through a *Go* binary is unaffected, because no
profile names one — which is why D-327's probe had to be run with
`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS`, and why that run says nothing
about §3.2.

**`sudo` on this VM requires a password.** So nothing above was installed by
this session except the `go install` and the unpacked prefix; the `apt`
packages and the AppArmor profile were run by hand by the owner.

## 4. What the next session should not have to rediscover

- **`golangci-lint` is installed now**, pinned to CI's 2.13.2, in `~/go/bin`.
  D-295's three views, and read the **exit code** rather than the last line —
  D-316 measured that pipeline calling a tree clean that did not compile.
- **A real Linux `-race` run is available here for the first time.** This
  machine has gcc and `CGO_ENABLED` defaults to 1, so `go test -race` works —
  which no machine in this project has ever had. D-287 records `cmd/liro-bridge`
  as never having been race-checked anywhere; that is now possible for whatever
  the Linux view of that package comes to contain.
- **The snapshot list.** D-323 recorded that it shrank and what that cost. On
  this machine the list is short because there is nothing to snapshot yet —
  and the moment the agent is first run here, it becomes
  `~/.config/liro/`, `~/.local/share/liro/`, `~/.local/state/liro/`,
  `$XDG_RUNTIME_DIR/liro/bridge.json` and `~/.config/autostart/`. Section 0
  above is the baseline to compare against.
- **`$XDG_RUNTIME_DIR` is a tmpfs unmounted at last logout**, so anything left
  there does not survive — measured, with `Linger=no` load-bearing. D-325 has
  the detail and the two caveats.
- **The gotk4 pin is load-bearing and silent.** `go get -u` takes v0.4.x,
  which does not compile on 24.04, and the error names GLib rather than the
  binding. D-327. If the module ever moves into `go.mod` proper, it wants a
  comment saying why the version is old.
- **Fifteen minutes cold.** Any CI job that builds the window layer from an
  empty cache should expect it, and the warm figure — four seconds for the one
  package that changed — is the one that describes working on it.

### What is actually left on this machine

- **§6 — the tray.** D-326 established GTK4 has none, the C remedy
  (`libayatana-appindicator`) is GTK3-only and therefore unusable in a GTK4
  process, and there is **no `StatusNotifierWatcher` on this session bus**
  although Ubuntu's AppIndicator extension is installed. So the remaining work
  is StatusNotifierItem over D-Bus in Go, and the question §6 has to answer
  first is what happens when nothing is watching.
- **§7.1's hand-over half.** D-325 decided the ownership rule and built it;
  what a second instance *does* — refuse, hand over, or run alongside — is
  decided and deliberately not built, because building it would settle §6 by
  implication.
- **The signal-10 contention.** WebKitGTK overrides the Go runtime's handler
  for `SIGUSR1` and says so on every start. Nothing has failed because of it
  in runs of six and twenty-five seconds. `JSC_SIGNAL_FOR_GC` is the named
  remedy and it has never been exercised.
- **Nothing NVIDIA, and nothing DMABUF.** This VM has no working GPU driver
  (`VMware: No 3D enabled`), so it is a software renderer and says nothing
  about either of §3.2's two original failures. §0.1 applies to every finding
  in this document.
