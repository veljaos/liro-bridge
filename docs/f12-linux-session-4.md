# F12 Linux — session 4

**What this is:** the fourth session on the Linux VM. It did one thing and
found one thing. The thing it did is the CI boundary — `ci` now installs no
GTK, and that absence is what it proves. The thing it found is that **the
state this session was briefed on is not the state this repository is in**,
and that is §0 because nothing else in this document matters if it is not read
first.

**Written:** 2026-09-21, Ubuntu 24.04.5 VM, kernel 7.0.0-31-generic.
**Entries this session:** [[D-335]] (the CI boundary, in code and in the
workflow).
**Also changed:** SPEC §19's F13 deferral entry, on macOS runners.

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

## 3. What is still exactly where session 3 left it

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

## 5. What the next session should do first

1. **Push and watch one run.** It is the first that could ever have proved
   anything about the boundary, and D-335 names what it may still find: the
   `-race` probe over `internal/ui`, and whether `linux-gui`'s own cache key
   behaves as reasoned.
2. **Install the profile and open the window.** Unchanged from session 3 §7,
   and it is one command of the owner's hands.
3. **§4, consent — the shape first.** The owner has reserved it: it goes to
   them before anything is built. A draft shape is in the session's closing
   report rather than here, because it is a proposal and not a finding.
4. **Not §6.** Unchanged: no `StatusNotifierWatcher` on this session bus.

**§0.1 applies to everything above**, and §0 of this document applies to
everything the next session is told before it starts.
