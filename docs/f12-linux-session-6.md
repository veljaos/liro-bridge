# F12 Linux — session 6

**What this is:** the sixth session on the Linux VM, and the handover to the
next one. **Read §A first** — it is the whole state in one page. **Then read
`docs/open-items.md`**, which is every open thing in the project in one
list, written tonight because the owner had lost track of them and so had
the documents.

**Written:** 2026-09-23, Ubuntu 24.04.5 VM, kernel 7.0.0-31-generic.
**Entries this session:** [[D-354]] (§8 built), [[D-355]] (§8 installed and
run, and the four defects that only running it could find).

---

## A. The handover, in one page

**The first signature on Linux through an installed package exists.** The
owner installed the `.deb`, ran `sdk/examples/sign.py` against the installed
agent, approved in the consent window, **typed the PIN into the native GTK
dialog**, and got `ugovor-signed.pdf` at B-B from a SoftHSM token. This
project's verifier and OpenSSL both accept it, each with controls that
fail. CI was green on all nine jobs, including installs on clean
`fedora:44`, `debian:trixie` and `ubuntu:24.04`.

**It took four defects to get there, and no test could have seen any of
them**, because each lives in a path that only a person, an installed
package or a real module reaches:

1. **The PIN dialog had never worked on Linux.** `entry.Widget.Native()` is
   gotk4's binding for `gtk_widget_get_native()` — the toplevel, as a Go
   object — not the entry's C pointer, and the cgo check panicked on the
   first C call. **Every exit from the dialog killed the agent**: OK, Enter,
   Cancel, the close box. The owner was the first person ever to press one of
   its buttons. §C below says what that means.
2. **"Open with" opened two windows** whenever an agent was running — a race
   D-344 introduced on both platforms.
3. **Every window leaked its WebKit web process**, ~31 MB each, for the life
   of the agent.
4. **One empty SoftHSM slot hid the token beside it**, and the text
   `certs` report hid the failure that said so.

All four are fixed, each with a test that fails when the fix is removed, and
the owner confirmed the first two on the installed build.

**What is not fixed, and is next:** the notification outlives the approval
([[D-355]] §7 — the code is wrong against D-341, SPEC is silent), and SPEC
§6.5.2's table now states something measured false (a clicked notification
*does* raise the window; why is unexplained). **The SPEC text is the owner's
to amend.**

---

## B. What the next session should do first

1. **Read `docs/open-items.md`.** Everything below is also there, with a
   pointer and what closes it.
2. **Build package signing** (§E). The owner ruled it is done now, not
   carried. It waits only for the key the owner generates; the commands are in
   §E.
3. **Fix the notification's lifetime**: withdraw it when the request is
   answered — at approval, refusal or timeout — not when the window closes.
   One call moved, one test.
4. **Then §9 (pcsclite), §10 (module discovery), §11 (testing)** in that
   order, per the owner. §11 needs a Fedora machine; §G says what it needs.
5. **Rebuild the package from the committed tree before any measurement.**
   The installed `0.9.9-dev.3` says `commit d7e4276` and carries the
   uncommitted fixes of that moment (`build.sh` does not mark a dirty tree —
   an open item).

---

## C. The PIN dialog, and what 42 of 42 green was worth

`internal/ui` was **42 of 42 green** when the owner pressed OK and the agent
died. Every test was right about what it tested. The dialog's guards parsed
its source — no `Text()`, no `C.GoString`, the overwrite textually before the
destroy — and were mutation-verified against the real file. **Not one of
them executed the dialog's handlers**, because only a person can press its
buttons (D-094), and nothing else in the suite made the dialog's C calls.

**So the lesson is not "write more tests". It is that a guard over source is
a statement about source**, and on the path that holds a secret the only
check that counts is one that runs the code. The test that now exists —
`TestTheDialogsCCallsWorkOnAnEntryInsideAWindow` — makes the three C calls on
a real entry in a real window, presses nothing, asserts the state the defect
needed (inside a window `Widget.Native()` is a live wrapper), and with the old
line restored fails with the owner's exact panic.

**Two more things this changed:**

- **D-350's conclusion is true of GTK and was never true of the dialog.**
  D-350 measured GTK's password buffer with a separate probe that called GTK
  correctly; this program's own calls could not run. Read [[D-350]] to
  [[D-352]] with [[D-355]] §1 beside them.
- **The window tests had never run on this machine either.** They skip where
  bwrap cannot start, which is every `go test` binary here (no AppArmor
  profile names it) and every CI runner (no display). To run them: 
  `go test -c -o /home/vboxuser/liro-f12probe ./internal/ui/ && (cd internal/ui && /home/vboxuser/liro-f12probe -test.v)`
  — from the package directory, because the AST guards read source relative
  to it. **The ordinary suite's "ok" for `internal/ui` says nothing about
  windows**, and that is on the open list as a CI question.

**What happened to the PIN**, for the record: nothing reached this program's
memory — the panic was at the length query, before the copy — and there was
no core dump. [[D-355]] §2 has what Linux does with a crash in general: no
core under Go's defaults, but **apport keeps a full memory image when one is
produced**, measured with a marker that survived into it. GNOME's soft core
limit of 0 is what protects a normal launch today; the program does nothing
to ensure it. Hardening is on the open list, as a decision.

---

## D. Where §8 stands

| item | state |
|---|---|
| `.deb` and `.rpm` from one nFPM file | **done**, installed and run here; CI installs both on three clean images |
| Flatpak and Snap refused with the reason | **done** (D-354) |
| Dependencies per distribution | **done** — `.deb` generated from the binary and checked by CI; `.rpm` in sonames, resolved on `fedora:44` by CI |
| Built in the oldest supported glibc | **done** — `ubuntu:24.04` container in CI and release; `build.sh` refuses a binary needing more than 2.39 |
| AppArmor profile in the package | **done** — loaded by `postinst`, the installed agent runs under it, sandbox up with no environment |
| Autostart as an XDG entry | **done**; a left-behind entry measured silent on the generator path and refused by GIO; **GNOME's session itself not watched** (needs a logout) |
| Desktop entry, icon, `MimeType=application/pdf` | **done**, seen by the owner |
| Desktop launcher / GNOME trusted question | **decided: none on Linux**, owner's ruling (D-355 §9) |
| Update route | **decided and built**: says a version exists, installs nothing |
| Uninstall keeps | **package half done on CI**; home half is by construction (the package cannot reach it) and **was not run here** |
| **Package signing** | **not built** — §E; owner ruled it is done now |
| The `desktop-entry` notification measurement §8 carried | **done**, and it reversed D-337 (D-355 §6) |

**Left in §8: package signing, and watching GNOME skip a left-behind
autostart entry across a logout.** Everything else in the phase document's
§8 box is done.

---

## E. Package signing — what the owner generates, and what a person does to check

**The key is OpenPGP, because that is what `gpg` and `rpm` verify.** It is not
the Ed25519 release key and cannot be derived from it (D-238's key signs the
update manifest in a format only this agent reads). Same handling as D-238:
the owner generates it, the private half goes only into the `release`
environment, the public half is committed, and signing runs only for a `v*`
tag.

**On the owner's own machine, not this VM:**

```
gpg --quick-generate-key "Liro Bridge Linux packages" ed25519 sign 3y
FPR=$(gpg --list-keys --with-colons "Liro Bridge Linux packages" | awk -F: '/^fpr/{print $10; exit}')
echo "$FPR"                                                   # goes in the README and the release notes
gpg --armor --export "$FPR" > liro-bridge-packages.asc         # public: commit as build/linux/liro-bridge-packages.asc
gpg --armor --export-secret-keys "$FPR" | base64 -w0 > secret.b64
```

`gpg` asks for a passphrase; set one. Then in GitHub → Settings →
Environments → `release`, add two secrets — `LIRO_PACKAGE_SIGNING_KEY` (the
contents of `secret.b64`) and `LIRO_PACKAGE_SIGNING_PASSPHRASE` — and delete
`secret.b64`. Keep an offline backup of the key; the session never sees the
private half.

**What gets built** (next session): a `sign-linux` job, `needs:
build-linux`, `environment: release`, tag pushes only, in an `ubuntu:24.04`
container. It signs the `.rpm` in place (`rpmsign --addsign`, an embedded
signature `rpm` and `dnf` understand), writes `SHA256SUMS` over the `.deb`
and the `.rpm`, signs it detached (`SHA256SUMS.asc`), and then **verifies
both against the committed public key in an empty keyring** before handing
anything on — the Linux form of `verifyrelease`. The `.deb` carries no
embedded signature, deliberately: `apt install ./file.deb` does not check
one, and `debsig-verify` needs a policy nobody has installed, so an embedded
`.deb` signature would be ceremony. The signed checksum file is the check.

**What a person does**, which will go in the README with the fingerprint:

*Debian and Ubuntu*
```
gpg --import liro-bridge-packages.asc          # compare the fingerprint it prints with the README's
gpg --verify SHA256SUMS.asc SHA256SUMS         # "Good signature from Liro Bridge Linux packages"
sha256sum --check --ignore-missing SHA256SUMS  # "liro-bridge_<v>_amd64.deb: OK"
sudo apt install ./liro-bridge_<v>_amd64.deb
```

*Fedora*
```
sudo rpm --import liro-bridge-packages.asc
rpm -K liro-bridge-<v>.x86_64.rpm              # "digests signatures OK"
sudo dnf install ./liro-bridge-<v>.x86_64.rpm
```

`dnf` does not check a local package's signature by default
(`localpkg_gpgcheck` is off), so `rpm -K` is the check and the README has to
say so. **The honest limit**: the fingerprint and the key are fetched from
the same host as the packages. It protects against a tampered mirror or a
swapped file, not against a compromised repository; publishing the
fingerprint somewhere else as well is what would close that, and it is the
owner's choice.

---

## F. The instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| a build piped into `tail` | success | the exit status was `tail`'s. **Second time this week** after [[D-322]]; replaced by a form — output to a file, `exit $?` on its own |
| the first cold build | a compile error | I edited sources while it compiled them |
| `/sys/kernel/security/apparmor/profiles` | no liro profile loaded | unreadable without root; the process's own `attr/current` is the instrument |
| my `$!` after `cd … && cmd &` | agent label `unconfined` | `$!` was the subshell; the agent itself was confined |
| the notification probe, twice | "no click" / "window stayed active" | it posted before its own window existed |
| the autostart generator run by hand | P5 held | GNOME does not use it; its units are all dead here |
| the first `/var/crash` check | no core | apport wrote it to `/var/lib/apport/coredump/` |
| `pgrep -f WebKit` after the agent stopped | WebKit processes remain | it matched my own shell |
| `internal/ui` in the suite | ok | every window test skipped |
| the PIN dialog's guards | green | they read source; nothing ran the handlers |

---

## G. §11 needs a Fedora machine, and this is what it needs

F12 §11 names **Fedora Workstation 44** beside Ubuntu 24.04. CI now proves the
`.rpm` installs and links on a clean `fedora:44` image. What only a Fedora
desktop can show:

- **Whether WebKitGTK's sandbox starts under SELinux** with no profile — the
  Fedora half of D-324, never measured.
- **Stock GNOME with no tray and no desktop icons** — F12 §6's reference case,
  only ever simulated here with an empty bus.
- The notification, focus and autostart readings, on GNOME 50 rather than 46.
- `pcsc-lite` socket activation after install (§9).

**The VM:** Fedora Workstation 44 live ISO (~2.3 GB), 4 CPUs, 4–6 GB RAM,
25 GB disk, 3D off to match this one. It cannot run beside this VM on this
host at 6 GB each, so one at a time. About an hour to install and set up; the
package installs with one `dnf` command. **The exit condition's card half —
a real reader passed through, and SafeSign for Linux installed — is needed on
both machines and is not a VM question.** The owner decides whether to build
it or defer that half of §11.

---

## H. This machine, and what changed on it

**Rules unchanged**: one process at a time, no background jobs beyond the
agent under test, no parallel builds or tests (`go test -p 1`), no load
generators.

**Installed this session** (apt): `gobject-introspection` (the build
dependency this VM had only as a hand-unpacked prefix — builds no longer need
`PKG_CONFIG_PATH`), and **`liro-bridge 0.9.9~dev.3`**, which also loaded
`/etc/apparmor.d/liro-bridge`. `nfpm` v2.47.0 is in `~/go/bin` (a Go tool,
not a system package).

**Left deliberately**: the package installed; `~/.config/liro/config.json`;
the audit log at `~/.local/share/liro/audit/` and the logs at
`~/.local/state/liro/logs/`, which hold the record of tonight's first
signature and are cited by D-355.

**Removed, and checked**: the autostart entry (the agent would otherwise
start at the next login); eight test pairings from the login keyring (search
for `application=liro-bridge` finds 0, where it found 8 before); their
`pairings.json`; `~/liro-s8-test`; the probe's core file.

**`/home/vboxuser/liro-f12probe` is now the `internal/ui` test binary**, not
last session's probe. It is only a path an AppArmor profile names.

**The SoftHSM token is in this session's scratch directory** and will be gone
after a reboot, as last session's was. Recreating it is five commands:

```
H=<scratch>/softhsm; mkdir -p $H/tokens
printf 'directories.tokendir = %s/tokens\nobjectstore.backend = file\n' $H > $H/softhsm2.conf
export SOFTHSM2_CONF=$H/softhsm2.conf
softhsm2-util --init-token --free --label "Liro F12 Test" --pin 648219 --so-pin 11223344
openssl req -x509 -newkey rsa:2048 -nodes -keyout $H/k.pem -out $H/c.pem -days 30 \
  -subj "/CN=Liro F12 Test Signer/givenName=Test/surname=Signer" \
  -addext "keyUsage=critical,digitalSignature,nonRepudiation"
openssl pkcs8 -topk8 -nocrypt -in $H/k.pem -out $H/k8.pem && openssl x509 -in $H/c.pem -outform DER -out $H/c.der
softhsm2-util --import $H/k8.pem --token "Liro F12 Test" --label signer --id 01 --pin 648219
pkcs11-tool --module /usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so --token-label "Liro F12 Test" \
  --login --pin 648219 --write-object $H/c.der --type cert --id 01 --label signer
shred -u $H/k.pem $H/k8.pem
```

The certificate needs `nonRepudiation` (SPEC §11.4). The installed agent sees
it only when started with `SOFTHSM2_CONF` in its environment.

---

## I. What this session did not read

**`docs/decisions.md` is 1.9 MB and was not read in full.** Read in full:
D-324, D-353, D-354, D-355 (written here), and D-342 and D-344. Read in part:
D-244, D-283, D-326, D-327, D-337, D-338, D-350. **Everything else — D-001 to
D-323 apart from D-244 and D-283, and D-325, D-328 to D-336, D-339 to D-341,
D-343, D-345 to D-349, D-351 and D-352 — was not read by me this session**,
beyond titles and searches for a word. `docs/open-items.md` was compiled by
sweeping D-280 onward for open sections and the whole file for a set of
words; an item stated only in an entry's prose outside those sections could
be missing from it.

SPEC and F12's phase document were read in full.
