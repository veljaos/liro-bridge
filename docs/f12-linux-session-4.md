# F12 Linux — session 4

**What this is:** the fourth session on the Linux VM, and the handover to the
next one, which is a new agent. **Read §A first** — it is the whole state in
one page. Everything after it is the session's own record, kept because the
measurements are in it.

**Written:** 2026-09-21, Ubuntu 24.04.5 VM, kernel 7.0.0-31-generic.
**Entries this session:** [[D-335]] (the CI boundary), [[D-336]] (the briefing
that described another repository), [[D-337]] (F12 §4 measured), [[D-338]]
(§3's last mile), [[D-339]] (what the first real signatures showed).
**Also changed:** SPEC §6.5 and its new §6.5.2, SPEC §19's F13 deferral,
F12 §0's second bullet, F12 §8's note, and `docs/f12-home-list.md` item 2b.

---

## A. The handover, in one page

**F12 §3 is done.** The agent opens its own windows on Linux, on the GTK4 and
WebKitGTK host, and a person has signed a PDF through the consent screen with
the soft token — same pages, same CSS, same catalogue as Windows, nothing
forked. `sign` and `open` are real commands here now ([[D-338]]); the only
window command still refused is `tray`, and that is F12 §6's undecided
question rather than a missing implementation.

**The three defects the first real signatures found, and their shape.** None
was what it looked like, and the shape is the same in all three and in D-338:
*a claim about a platform, written where nothing could check it, harmless for
exactly as long as there was only one platform.*

| what it looked like | what it was | [[D-339]] |
|---|---|---|
| missing glyphs for `č ć đ š ž` | image URLs built as `https://`, WebView2's spelling; WebKitGTK's broken-image mark is a question mark in a box | `ui.HostURL` answers the scheme; round-trip test |
| a Linux font needing more room | every window got **37 fewer points** than asked — the GTK4 header bar | the size request moved to the view; four sizes exact |
| *nobody reported it* | the audit log written to **`./Liro/audit`**, relative, because `ConfigDir` was passed the literal `"windows"` | `runtime.GOOS`, plus two mutation-verified guards |

**What §4 has left, now that SPEC §6.5.2 is written.** The specification is
amended and the measurements are under it ([[D-337]]): a new window per
request works on this compositor, re-showing an existing one does not, and a
clicked notification raises nothing. **None of that is built yet.** What §4
still needs is the code: a new window per request rather than a re-used one,
a desktop notification posted alongside — best-effort, never a refusal,
logged plainly when there is no service — and nothing in the consent path
resting on the window having been seen. `org.freedesktop.Notifications` is on
this session bus. The caller half and the `CONSENT_TIMEOUT` half are
unchanged and already true.

**What comes after §4:** §5's PIN dialog (native, not in the page — SPEC §10;
`pinscreen.ErrNoDialogOnThisPlatform` is what refuses today), then §6's tray
question, then §8's packaging. §7's remaining question is where the audit
chain lives — `$XDG_CONFIG_HOME/liro/audit` today against §7's
`$XDG_STATE_HOME`, a decision nobody has made.

**What this machine needs you to know.** One process at a time, no load
generators, no background jobs (session 3 §6 — three crashes in a day bought
that rule). The AppArmor profile is installed and it names the **path**
`/home/vboxuser/liro-f12probe`: a WebKitGTK window will not start anywhere
else, so build the agent to that path to run it. Point `XDG_CONFIG_HOME`,
`XDG_DATA_HOME` and `XDG_STATE_HOME` at a scratch directory when you do, so
this machine's own profile stays clean — session 1 §0's baseline is still
true and is worth keeping true. `~/go/bin` is not on `PATH`, so invoke
`golangci-lint` by absolute path, and read exit codes rather than last lines
([[D-316]]).

**What is open, in the order it is likely to matter:** §4's code; drag and
drop, which needs `gdk_file_list_get_files` that the binding does not
generate ([[D-330]]'s shape, third instance) and whose absence the page still
invites; the Settings window's two Windows-shaped rows on Linux; PKCS#11 here,
compiled and never exercised; and `lowerLevel`, which is not a Linux question
and is on the home list as item 2b.

---

## 0. The briefing and the repository disagree, and the repository is what was measured

The session opened with: *"§3.1 and §3.2 are closed … The first WebKitGTK
window opened, then the consent screen, then a whole signing flow ending in a
signed PDF … three `os.Exit` calls in code that must not exit … The log ends
around D-339 … the push that split the jobs … the job that failed is `ci`, not
`linux-gui`."*

**None of that second half exists here.** Checked four independent ways before
anything was touched, because a session that starts by absorbing a discrepancy
is the shape D-304 exists to refuse:

| checked | found |
|---|---|
| `git log`, `git reflog`, `git stash`, branches | tip is `1d90ba8`, session 3's own commit. Nothing after it, nothing stashed, one branch |
| `git fetch origin` | `origin/master` **equals** the local tip, 0 ahead and 0 behind. Not an unpushed-work case |
| `docs/decisions.md` | **334 entries.** The log ends at D-334, not D-339 |
| `.github/workflows/ci.yml` | **four jobs** — `ci`, `windows`, `packaging`, `sdk-typescript`. There is no `linux-gui` job, and no push has ever split them |

And the machine agrees with the repository:

- **No Liro state anywhere under `$HOME`** — no `~/.config/liro`, no
  `~/.local/share/liro`, no `~/.local/state/liro`, no
  `$XDG_RUNTIME_DIR/liro`, no `~/.config/autostart`. That is session 1 §0's
  baseline, which was recorded so that the next session would know *when it
  stopped being true.* It has not stopped being true. **The agent has never
  run on this machine.**
- **No `liro-f12-window` AppArmor profile is loaded.** `/etc/apparmor.d/` holds
  the session-1 probe profile naming `/usr/bin/python3.12` and nothing else —
  so the one `sudo` command session 3 §7 staged has not been run, and
  `LoadURI` still aborts exactly where session 3 left it.
- **No boot between 2026-09-20 22:36 and 2026-09-21 15:57.** `journalctl
  --list-boots` runs continuously from session 1 through session 3 and then
  jumps to today's. No session ran in between, and no snapshot rollback erased
  one either — a rollback would have taken the older boot records with it.
- **`os.Exit` appears nowhere in `internal/`** outside tests. Grepped.

**What is true of the briefing:** the CI failure is real and is quoted
exactly. Run `35542306925`, job `ci`, `go vet`, `gobject-introspection-1.0 was
not found` — and it is the **current** tip's run, not a later one. §3.1 and
§3.2 are indeed closed (D-327, D-324). The window host exists (D-331) and
stops at one call (session 3 §3). The tray, `SetForegroundWindow` and
`GetDpiForWindow` are indeed what did not port, and D-331 records each being
refused or replaced on purpose.

**What this session did with the difference:** worked from the repository,
which is the only thing it can measure, and did not renumber anything to make
room for entries that are not here. **The next entry number is D-336.** If
there is a session that produced D-335 through D-339 and a `linux-gui` job,
it is somewhere this machine and this GitHub repository cannot see, and the
D-335 written today will collide with it.

---

## 1. CI: two layers, and both are now closed

[[D-335]] is the entry. What a reader needs without opening it:

**The shallow layer — the instrument could not have failed.** D-334 chose the
runner's two packages by asking what this VM has, and verified with `go vet
./...`, exit 0 in 3.1 s. Two things were wrong with that:

- `gobject-introspection-1.0.pc` is not on this machine at all and **no
  installed package owns one** — `dpkg -S` finds nothing. What satisfied it
  was session 1's hand-unpacked `~/.local/gir-prefix`, the one item §3a calls
  out as able to change a measurement silently.
- And more generally: **a warm build cache does not re-resolve `pkg-config`.**
  Measured today with the file unreachable — `go vet ./...` **exit 0, 7.1 s**;
  the same work with one package forced to rebuild, **exit 1 in 0.265 s**,
  byte-identical to CI's error. So the green sweep said nothing about the
  dependency. Every build timing taken here is warm (session 3 §5) — this is
  the same property with a second consequence.

**The right package is `gobject-introspection`**, not `libgirepository1.0-dev`
and not any `-dev` package: the `.pc` lives in the tools package and the
`-dev` one merely depends on it. Closure 618 → **677**.

**The deeper layer — there was no split.** The `ci` job was *supposed* to have
no GTK, in the sense that the property is worth having; it had never had it.
D-334 deliberately installed GTK there. So the claim had not lapsed, it had
never been made, and adding a third package would have made it unmakeable.

**And a third failure stood behind those two.** The step that proves a release
binary carries no soft token builds `./cmd/liro-bridge` with `-tags softtoken`
at `CGO_ENABLED=0` — and measured on a worktree at the pre-fix tip, that build
fails too, through `internal/pinscreen` into gotk4, which has no buildable Go
files with cgo off. **No apt package fixes that one**, so installing the third
one would have moved the red three steps down the job. It is exit 0 now, with
all six of that step's symbol assertions passing here.

**What is now true:**

| | `ci` | `linux-gui` (new) |
|---|---|---|
| GTK | none, deliberately | the three packages |
| sweeps | the 50 packages that need none | `internal/ui`, and lint's whole-module linux view |
| proves | nothing shipped needs a C library on linux | the window host builds off this VM |

with a guard step that **computes the boundary and then asserts it**, in both
the untagged and the softtoken view, and fails naming the package that broke
it. The guard was run against a control (pointed at `internal/pinscreen`
instead) to prove it can report more than one package and would have failed on
yesterday's tree.

**And the boundary in the code was one symbol.** `pinscreen_other.go` returned
`ui.ErrUnsupportedPlatform` from a function whose whole body is a refusal —
and a Go package is atomic, so that one reference put gotk4 behind
`internal/pinscreen`, and behind `cmd/liro-bridge` **under `-tags softtoken`
and not otherwise**. It now returns `pinscreen.ErrNoDialogOnThisPlatform`,
declared in its own neutral file. Measured after: the packages needing
`internal/ui` on linux number exactly one, and it is `internal/ui`.

### Measured here, all of it warm

| | |
|---|---|
| `go vet` over the 50-package set | exit 0, 0.86 s |
| `go test <50 pkgs> -race -count=1` | exit 0, **2 m 04 s** |
| the same `-tags softtoken` | exit 0, **1 m 43 s** |
| `CGO_ENABLED=0 GOOS=linux go build` over all 50 | exit 0, 5.3 s |
| the shipped binary, re-measured | `DT_NEEDED` 0, no `PT_INTERP`, **10 231 220 B** |
| `golangci-lint` 2.13.2, linux / windows / softtoken views | exit 0, 0 issues each |
| `checkdeps` | OK, 51 packages |
| `checkcss` | OK |
| `go test ./internal/ui/... -count=1` | ok, 0.428 s |

**None of it is evidence about the runner**, for the reason the first half of
this section gives. The next push is.

---

### The push, and the first evidence that could have failed

Run `35628430772` is green on all five jobs — the owner pushed it. `ci` 7 m
06 s, **`linux-gui` 27 m 26 s**, `windows` 5 m 38 s, `sdk-typescript` 2 m
09 s, `packaging` 1 m 02 s.

**Where the 27 minutes went**, per step rather than guessed: `go vet
(internal/ui)` **768 s**, which is gotk4 compiling cold; the `-race` probe
**806 s**, which is gotk4 compiling *again* because an instrumented build is a
separate cache namespace; everything else together ~64 s. The cache saved
**183 MB** under its own key, so the arrangement D-335 reasoned about did
happen. **Prediction for the next push: 2–4 minutes.** If it is not, the
instrumented objects are not surviving in the cache and the probe stops
running on every push.

**The probe passed** — `ok internal/ui 1.088s` — and that is a smaller claim
than it looks: too fast to have opened a window, so the GTK tests skipped on
the headless runner as D-334 supposed. It is not evidence that gotk4 survives
`checkptr`, only that nothing which ran reached it. The step had no `-v`, so
even the skipping was inferred; it has one now. [[D-336]] is where that shape
— a silence that reads as an answer — is written down.

---

## 2. SPEC §19 gained two paragraphs about a machine nobody has to own

In the **deferral entry**, not the F13 table row, because it is about what
deferring costs rather than about what the phase contains.

GitHub Actions offers macOS runners and they are free on public repositories,
which this one is (checked: `veljaos/liro-bridge` is `PUBLIC`). So most of F13
— compilation, the Keychain logic, the PKCS#11 layer against a software token,
packaging, even opening a window and photographing it — can be built with a
machine in hand, which is the *only* way this project has ever built anything.
What still needs hardware on a desk is three things the exit condition already
names: a person clicking Approve (D-094 forbids simulating it), a card and a
PIN, and how Gatekeeper meets an unsigned program.

And the second paragraph carries Linux's lesson into it: **everything Linux
found had built and passed its tests first** — a binding that generates every
asynchronous operation's finishing half and none of its starting halves
(D-330), a doc comment that described the opposite of what its own function
sent (D-332), pages drifted from their own documented design (D-333), three
places where the platform cannot do what the Windows contract says (D-331).
The Mac days will be days of finding, not confirming.

The specifics of that paragraph are the four entries above rather than the
ones the briefing named, for §0's reason: `os.Exit` is nowhere in `internal/`,
so it is not cited in a document that governs every phase.

---

## 3. What was still exactly where session 3 left it — **superseded, and kept**

> **Read §6 and §8 instead for the current state.** This section was true when
> it was written, in the middle of the session: the profile was not installed,
> the window had never opened, and `sudo` was the blocker. The owner installed
> the profile, the window opened, and the agent now signs. It is kept rather
> than corrected because the list of what a measurement here can silently
> depend on is still accurate, and because a document that quietly rewrites
> its own earlier state is a document nobody can date.

- **The window has never opened here.** Session 3 §7's first instruction is
  still the first instruction, still one `sudo` command, still staged at
  `/home/vboxuser/liro-f12probe` (the binary is still there, 21 957 224 B,
  dated 22:02 on 20 September):

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

- **`sudo` still wants a password** (`sudo -n true` → *a password is
  required*), so this session installed no apt package. It needed none: every
  measurement above was taken with the packages already present, and the two
  the runner needs were established with `apt-get download` and `dpkg-deb -c`,
  which need no root.
- **The probe AppArmor profile is still loaded** and still names
  `/usr/bin/python3.12`. Still the one thing here that can silently change a
  measurement — and this session is the second time it has.
- **`~/go/bin` is still not on `PATH`.** `golangci-lint` was invoked by
  absolute path.

---

## 4. Two things this machine says today that it did not say yesterday

Both were free, both were taken with session 1 §3's own control — `busctl`
can see `org.gnome.Shell`, twelve names today — and both bear on sections
that are still open.

**`org.kde.StatusNotifierWatcher` is on this session bus.** Owned by
`gnome-shell`, PID 2332, with `ubuntu-appindicators@ubuntu.com` reporting
`State: ACTIVE`. **Session 1 §3 measured it absent on 20 September**, tried
three spellings, and concluded that on this installation *"a tray icon
published right now would have nobody watching"*, adding that it was worth one
check on a second machine before §6 is decided on it. The second check has
happened on the same machine, one boot later, and **disagrees**.

*Why* they disagree is not established and cannot be: that session is gone, so
whether the extension had not yet activated, or the measurement reached a
different bus, is not recoverable. It is recorded as a disagreement rather
than resolved into a story. What follows from it is narrower and is enough:
**session 1's sentence must not be carried into §6 as a property of Ubuntu.**
The tray's first question — what happens when nothing is watching — is still
the right question, and this machine is no longer an example of the answer.

**This session is Wayland.** `XDG_SESSION_TYPE=wayland`,
`XDG_CURRENT_DESKTOP=ubuntu:GNOME`. Worth stating plainly because §4's entire
design question is about what Wayland does not permit, and because the
briefing described the current state as being on X11 (§0).

**And `org.freedesktop.Notifications` is on the bus too**, owned by `gjs`, PID
2452 — the shell's own extension process. That is the listener F12 §4's
"a desktop notification alongside" needs, and it exists here, which is one
fewer assumption in the shape that goes to the owner.

---

## 5. F12 §4's assumptions, measured — two of four, and the two that needed nobody

**The instrument:** a plain GTK4 probe, no WebView, built outside the
repository against the same pinned gotk4 (a GTK-dependent package inside the
repository would fail §1's own boundary guard, which is the guard working).
Focus is a property of the toplevel and the compositor rather than of what is
painted inside it. **What that does not cover is mapping time** — a WebKitGTK
window takes longer to appear and focus-stealing prevention is a rule about
timestamps — so these are worth re-running against the real host once the
profile is in. No synthetic input anywhere (D-094): every focus change is one
this program asked the compositor for, and every reading is taken off the
window itself.

GTK 4.14.5, Wayland, GNOME 46, software rendering (`libEGL`/`MESA` errors on
every start — §0.1).

| what | result |
|---|---|
| a **new** window, opened 0.1 s after the person launched the process, while the terminal was active | **took focus**, 1.10 s after `present()` |
| a **second** new window, opened 6 s later while our own first window was active | **took focus**, 0.14 s |
| `present()` on an **existing** window, while another window **of our own process** was active | **raised it**, 0.12 s |
| a new window from a process that had been **idle for 45 s** — the agent's own shape, since it starts at login and opens a window hours later because a request arrived | **took focus**, 0.52 s |

**The fourth row is the one that matters** and it is why the mode exists: the
first three could all be explained by GNOME granting focus to an application
the person had just launched or was already attending to, which the agent
never is.

**And the fifth reading is the one that decides the section.** Taken by the
owner, with the profile installed and a foreign window focused:

```
[ 21.01s] present() — the window has been inactive for 3s, and this
          process has no focused window of its own. This is the agent's case.
[ 24.02s] RESULT  present() on an unfocused window, no token: active=false
```

**`gtk_window_present` does not raise an existing unfocused window on this
compositor.** Set against the fourth row above — a *new* window takes focus
even from a process idle for 45 seconds — F12 §4's shape is now measured
rather than predicted:

> **A new window per request works. Re-showing an existing one does not.**

Which is what F12 §4 already required for its own reasons — *"a new window
for every request, not a hidden one shown again"* — arrived at from the other
end, by measurement, on the platform it was written about.

### What is still open, and neither can be taken without a person

- **`present()` on an unfocused window while a *foreign* window is focused.**
  Every raise measured so far was asked for by a process the compositor was
  already attending to. The agent's case is a browser in front and no window
  of its own in sight. The probe's `raise` mode waits until its window has
  been inactive for three seconds and refuses to report anything if that
  never happens.
- **Whether a notification's activation token raises a window that
  `present()` alone would not. The first attempt measured nothing and the
  run is void**, which is worth recording rather than quietly re-running:
  the probe sent the notification 0.33 s after opening its window, so the
  window was still active when the person clicked, and "active afterwards"
  was a reading with nothing behind it. **A probe that reports a result
  when its own precondition never held is an instrument that cannot fail** —
  the same family as [[D-336]] — so it now waits for the window to lose
  focus the way `raise` does, and prints `INVALID` instead of a result if
  the window is active when the click arrives.

  **The rerun is clean and the answer is no.** Precondition held — the window
  was unfocused for 3 s before the notification went out — and three seconds
  after the click it was still unfocused:

  ```
  [  4.78s] SENT    notification id=3 — the window is unfocused
  [  6.65s] SIGNAL  ActionInvoked, window active BEFORE the click=false
  [  6.65s]         NO token arrived
  [  6.68s] SIGNAL  NotificationClosed id=3 reason=2 (dismissed by the person)
  [  9.68s] RESULT  3s after the click, WITHOUT a token: window active=false
  ```

  **So on this machine the notification is a prompt to go and look, not a way
  to reach the window** — and the token half is bounded: the probe could
  carry no `desktop-entry` hint, because nothing is installed and there is no
  `.desktop` file to name, so the shell had no way to map the notification to
  the window. The finding is **no token without a desktop-entry hint**, and
  it is deliberately not *"GNOME does not send tokens"*. F12 §8 now carries
  the measurement that is waiting for a package. [[D-337]].

  Two things from the void run do stand: **no `ActivationToken` arrived**,
  which is not yet interpretable either way, and the probe's own
  `NotificationClosed` handler called `Variant.String()` — that is
  `g_variant_get_string` — on a `(uu)` tuple, which the binding's own
  documentation calls an error. It never fired, so the defect was found by
  reading rather than by running. Both halves are fixed. `org.freedesktop.Notifications` is on this
  bus, `GetCapabilities` includes `actions`, and the interface carries
  **`ActivationToken(u, s)`** alongside `ActionInvoked` — introspected, not
  assumed. The probe subscribes to both, calls `gtk_window_set_startup_id`
  with the token and then `present()`.

**None of this is built into anything.** It is measurement before design, and
the design goes to the owner first.

---

## 6. The window host opened, and the readings were re-taken on the real artefact

**The profile is installed and the window host works end to end. Nothing on
this machine had ever done this.**

```
[  0.00s] mode=host — internal/ui.NewWindow on /pages/consent.html
[  1.13s] OPEN    the window host is up and /pages/consent.html finished loading
[  2.15s] PAGE    document.hasFocus()=true  document.title=""
[ 13.27s] RESULT  the host opened one of this program's own pages and answered JavaScript
```

Three things closed at once, each of which had been open since session 3:

- **D-324's remedy, through the real host.** Session 3 died at `LoadURI` with
  `bwrap: setting up uid map: Permission denied`. With the profile naming the
  binary, WebKitGTK's sandbox starts and the page loads in 1.13 s. The
  AppArmor profile is the answer, measured now from the program rather than
  from a probe.
- **D-330's hand-written `Eval` bridge ran against a real page**, and
  answered. The whole of Go→page works on this platform: the binding
  generates no way to start `evaluate_javascript`, `internal/ui` supplies one
  in cgo, and it returns values.
- **`document.title` is empty and that is correct** — no page in
  `internal/ui/assets/pages` carries a `<title>`, because the window's title
  is `Options.Title` and belongs to the toplevel rather than to the document.

`Overriding existing handler for signal 10` still appears on every start
(session 1's open item), and `VMware: No 3D enabled` still means software
rendering (§0.1).

### The same instrument, the other artefact

Every reading in [[D-337]] came from a bare GTK4 toplevel. Re-taken against a
GTK4 window whose only child is a WebKitGTK view loading this program's own
`/pages/consent.html` over a custom scheme — the same shape `internal/ui`
builds:

| | bare GTK4 | WebKitGTK |
|---|---|---|
| new window, just launched, foreign window active | took focus, 1.10 s | **took focus, 0.42 s** |
| second new window, our own window active | took focus, 0.14 s | **took focus, 0.20 s** |
| `present()` on existing window, our own other window active | raised it, 0.12 s | **raised it, 0.03 s** |
| new window from a process idle 45 s | took focus, 0.52 s | **took focus, 0.47 s** |

**A prediction written down before the run failed**: I expected the WebKitGTK
window to be answered *later*, since it has more to paint and
focus-stealing prevention is a rule about timestamps. It is answered sooner.
The toplevel maps when GTK shows it and does not wait for the web process's
first frame, so the thing the substitution was suspected of changing is not
in the path at all.

**One row is still outstanding** — `present()` on an unfocused WebKitGTK
window with a foreign window focused — because it needs a person to click
away, and it is the row the specification's wording rests on.

---

## 7. The two things every WebKit run prints, answered

### `Overriding existing handler for signal 10` — noted and harmless, and here is the reason

Session 1 listed this as open. JavaScriptCore takes **SIGUSR1** for its
garbage collector's thread suspension, in a process that is also a Go
runtime, which is precisely the class F12 §2 names as a reason the PKCS#11
worker exists. Three facts settle it, all read off this machine rather than
recalled:

- **Go does not use signal 10 for itself.** `go1.26.5`'s own signal table,
  `/usr/local/go/src/runtime/sigtab_linux_generic.go`:

  ```
  /* 10 */ {_SigNotify, "SIGUSR1: user-defined signal 1"},
  ```

  `_SigNotify` alone — no `_SigThrow`, no `_SigPanic`, no `_SigUnblock`. It
  means the runtime wants the signal only so that `os/signal.Notify` can
  deliver it, and does nothing with it otherwise.
- **The runtime's own signal is a different one.**
  `runtime/signal_unix.go:74`: `const sigPreempt = _SIGURG` — signal 23.
  Asynchronous preemption, which is the thing whose loss would actually
  break a Go program, is not what WebKit took.
- **Nothing in this program asks for it.** No file under `internal/` or
  `cmd/` imports `os/signal`, names `SIGUSR1`, or calls `signal.Notify` at
  all.

So what WebKit takes away is a capability no part of this program uses. **The
cost is real but it is contingent and silent**: if anything here ever calls
`signal.Notify(syscall.SIGUSR1)`, it will simply never fire, with no error at
the call and no warning beyond the line WebKit already prints at startup. The
remedy is named in that line — `JSC_SIGNAL_FOR_GC` — and the place for it is
`PrepareWebKitEnvironment`, which already exists to set variables before GTK
initialises (D-329). It is **not** set now, deliberately: nothing needs it,
and a third variable set on speculation is the opposite of what D-329
decided.

### The `GLib-GIO-WARNING` after exit — teardown order, in a process that is not ours

```
(process:2): GLib-GIO-WARNING **: Error releasing name
org.webkit.app-….Sandboxed.WebProcess-…: The connection is closed
```

**`(process:2)` is the evidence.** That is GLib's log prefix carrying the
process's own pid, and pid 2 is not something this agent can be: it is pid 2
*inside bubblewrap's pid namespace*. The name it failed to release says the
same thing twice more — `Sandboxed` and `WebProcess`. So the warning comes
from WebKitGTK's sandboxed web process while it is shutting itself down, not
from the program.

And the sequence says what it is: the message arrives **after** `Close()`
returned and after the run's own `RESULT` line. The UI process closed the
view, which closed the connection, and the child then tried to unregister a
bus name over a connection that had already gone. **Nothing depends on that
release** — a D-Bus name dies with its connection — so it is an ordering
artefact of teardown and not a leak. Harmless, and worth having written down
once so it is not diagnosed again.

---

## 8. §3's last mile: the agent opens its own window here now

[[D-338]] is the entry. What a reader needs without opening it:

**The port was a filename problem.** A throwaway copy of `cmd/liro-bridge`
with every `_windows` suffix neutralised compiled for `GOOS=linux` — the
whole windowed flow, ~3 800 lines, with exactly one genuine failure
(`console_windows.go`). So the work was moving code to where its callers are,
not writing code.

**Two files were holding things that were not theirs**: `tray_windows.go` had
the Settings window, the audit export and every window's status line;
`uninstall_windows.go` had the stale-preview sweep. Both split, the first as a
pure move verified line by line.

**The first run was refused by the program's own honesty.** `sign` got as far
as `ui: dropped files are not implemented on Linux yet` — D-331 chose to
refuse a window that asks for drops rather than ignore the request, and the
signing window has always asked. Drops need `gdk_file_list_get_files`, which
the binding does not generate (D-330's shape, third instance), so the caller
asks for them only where they work and the gap is written down.

**Then it ran.** With the AppArmor profile in place and the binary at the
profiled path:

```
$ liro-bridge sign --in doc.pdf
(runs; opens its window; waits)
```

**And the binary is what SPEC §1.1 describes**, measured on the artefact
rather than on a probe: `PT_INTERP` present, **`DT_NEEDED` exactly the
thirteen libraries D-327 predicted**, 31 661 504 B.

### The boundary inverted one session after it was drawn

§1's `ci` job proves nothing shipped needs a C library on linux — and the
agent now does, which is what SPEC §1.1 always said. So the guard expects two
packages instead of one and still fails on a third; the steps that build the
agent moved to `linux-gui`; and the linux proof stopped being "`DT_NEEDED` is
0" and became §1.1's own test. **The new assertion caught a flaw in itself
before CI ever ran it**: `sort` collates punctuation by locale, so the same
thirteen libraries compared unequal to the same thirteen libraries until
`LC_ALL=C` pinned it.

### Two things found on the way that are not about Linux

- **`lowerLevel` has no caller.** It computes the weakest PAdES level a batch
  reached — SPEC §18.11's rule — and nothing in the program invokes it; the
  only reference is a Windows-only test. Either the report computes the level
  another way or a mixed batch reports a level it did not reach. D-247's
  shape, and it needs the Windows reporting path in front of somebody.
- **A 31 MB binary walked into a commit.** `go build ./cmd/...` drops the
  executable in the working directory and `.gitignore` covered `*.exe` but
  not an extensionless linux binary — harmless until today, when the command
  started building here. Caught in `git show --stat` before any push; the
  commit was amended and `/liro-bridge` is ignored now.

---

## 9. What the first real signatures showed

[[D-339]] is the entry. The signature itself worked, on the agent's own window,
with no page, stylesheet, script or catalogue changed. Three defects came out
of it and **not one was what it looked like**:

| reported | looked like | actually |
|---|---|---|
| question marks in the placement picker | missing glyphs for `č ć đ š ž` or Cyrillic | image addresses built as `https://`, which is WebView2's spelling; both images failed and WebKitGTK drew its broken-image mark, which is a question mark in a box |
| the method screen too short, needing a scroll | the Linux UI font being wider than Segoe UI | every window on this platform was handed **37 fewer points** than its caller asked for — the GTK4 header bar, measured constant at four sizes |
| *(nobody reported this one)* | — | the audit log was written to **`./Liro/audit`**, relative to wherever the program was started, because `platform.ConfigDir` was passed the literal `"windows"` |

**The signed PDF is correct** and was checked first, because it is the only one
of the three that could have reached a document somebody relies on: the stamp
decodes through the PDF's own ToUnicode map to four clean lines with no
replacement character in them.

**The stamp renderer was never the problem.** Rendered here with a Cyrillic
label and `Čačak Đorđe Šimšić žžž`, every glyph draws; the committed substitute
font carries all of them. Ten minutes of measurement against what would have
been a day of font plumbing.

Each fix carries a guard that fails against the old behaviour — verified by
mutation, not by assertion: a round-trip test that an address this program
hands a page is one its own resolver answers, and two guards on the audit
directory, one of which reproduces `"Liro/audit"` exactly.

**The third one is the shape of this whole session.** A claim about a platform,
written where nothing could check it, harmless for exactly as long as there was
only one platform — the same as D-338's suffixes and D-335's `ci` job.

---

## 10. What the next session should do first

1. **Read §A, then [[D-338]] and [[D-339]].** The first says what the program
   is on this platform now; the second says what a person found in one run
   that no test saw, and how each of the three was diagnosed by measuring the
   thing rather than the guess.
2. **§4's code, on the shape SPEC §6.5.2 now fixes.** It is the phase's next
   box and the specification is written, so this is building rather than
   deciding. Post the notification through `org.freedesktop.Notifications`
   over D-Bus in pure Go — SPEC §1.1 prefers that to a declared dependency,
   and the interface is on this bus.
3. **Watch the first `linux-gui` run after a cold cache.** D-335 predicted 2–4
   minutes warm against 27 cold; if it is still 27, the instrumented objects
   are not surviving in the cache and the `-race` probe stops running on every
   push.
4. **Read `fitToContent`'s numbers.** It logs `declared` against `measured`
   for every step; the method step is the one that overflowed, and the header
   bar fix gave it back 37 points. Whether it still needs more is a number in
   the next run's log rather than an opinion.
5. **Not §6.** The tray's first question is still unanswered — and note that
   `org.kde.StatusNotifierWatcher` *is* on this bus today where session 1
   found nothing watching (§4 above), so the premise that question rests on
   needs re-taking before it is decided.

**§0.1 applies to everything in this document**: a virtual machine with no
working GPU driver, and a graphics driver that calls its own configuration
broken on every boot. And §0 applies to whatever the next session is told
before it starts.
