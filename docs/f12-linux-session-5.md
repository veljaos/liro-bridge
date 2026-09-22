# F12 Linux — session 5

**What this is:** the fifth session on the Linux VM, and the handover to the
next one. **Read §A first** — it is the whole state in one page. Everything
after it is the session's own record, kept because the measurements are in it.

**Written:** 2026-09-22, Ubuntu 24.04.5 VM, kernel 7.0.0-31-generic.
**Entries this session:** [[D-346]] (the check that noticed), [[D-347]] (the
Secret Service branch and why the transfer is plain), [[D-348]] (the XDG
directories, and every path being relative when the environment is silent),
[[D-349]] (the Linux PKCS#11 layer, run rather than compiled), [[D-350]] (the
GTK PIN dialog), [[D-351]] (its conclusion withdrawn), [[D-352]] (the
withdrawal withdrawn), [[D-353]] (what the runners caught that this machine
could not).
**Also changed:** SPEC §6.4's Linux row, and the CI boundary guard.

---

## A. The handover, in one page

**§7 is done.** SPEC §6.4's Secret Service branch is built — pure Go over
godbus, no libsecret — with the encrypted file as the fallback where no
keyring answers. **The transfer algorithm is "plain", decided with a
measurement**: plain does put the secret on the bus, DH does not, and then a
different binary with a different parent, which never saw the bus traffic,
asked the keyring and was handed the secret. gnome-keyring applies no
per-application check to an unlocked collection, so DH would encrypt the wire
between two parties who both hand the plaintext to the same caller.

**A locked keyring never blocks the agent and never raises a dialog.**
Measured against a deliberately locked collection — the probe made its own,
with its own master password, so nobody's login keyring was touched. Nothing
in the Secret Service interface shows anything until `Prompt.Prompt()` is
called; this program does not contain that call, and an AST guard with a
control and a mutation is what keeps it that way.

**Which store a person got is visible in three places**: `Describe()` on the
interface, so a new store cannot avoid answering; the log line §6.4 requires,
carrying the reason; and a row in the Settings window that renders the
fallback as the warning it is.

**§7's paths are done too, and the guard written for them found something
bigger.** `$XDG_STATE_HOME` and `$XDG_CACHE_HOME` held nothing, and the config
directory held the log, the update state, the trust list and **the rendered
page images of the document about to be signed** — under the one directory
people point backup and sync tools at. All moved. Then the guard found that
with `LOCALAPPDATA` or `HOME` unset, **every path this program writes to was
relative to the current directory, on every platform**: [[D-339]] fixed the
instance and the class was still live.

**§5's PKCS#11 half is done and runs.** `internal/keysource/pkcs11` could not
reach a token on this platform at all — `openModule` refused. It is cgo
against the platform's own `pkcs11.h` now, and against SoftHSM it produces **a
256-byte signature that verifies against the certificate's own public key**,
one wrong PIN costing exactly one attempt, and a third module with clause 1's
protected-path flag clear. The neutral logic is *shared*, not copied:
`ckULong` is a type alias, `uint32` on Windows and `uint64` here.

**§5's dialog is built and nobody has typed a PIN into it.** It is a native
GTK window (SPEC §10), the PIN never becomes a Go string, and the AST guard
names the binding's `Text()` accessor because that is the obvious thing to
reach for and it would compile.

**And §5's conclusion about clause 2 is withdrawn.** Read [[D-350]], [[D-351]]
and [[D-352]] in that order — it is three entries on one question in one
evening and the arc is the point. **The state to carry forward is: the
keystroke path has never been measured, and the finding is that nobody has
looked at it, not that it is clean.**

**CI went red on the three pushed commits and is fixed but not yet
re-run.** Four causes, in [[D-353]], and not one of them a defect in what the
code does: `libp11-kit-dev` is a development package this machine had and the
runner never did; "needs GTK" and "needs a C toolchain" stopped being the same
set of packages; a build constraint went missing in a substitution that
silently did not match, where only the **darwin** cross-build could see it;
and D-349's clamp swallowed an `ulMaxPinLen` of a million, which is evidence
that a struct layout was misread rather than that a token is unusual. **The
last one was caught by a Windows test written long before, and the test was
right.**

**§8 has not been started.** That is tomorrow.

---

## B. What the next session should do first

1. **Read [[D-350]], [[D-351]] and [[D-352]] in order.** Not for the
   conclusion — for the shape. Two of the three overturnings came from a
   control rather than from anybody's reading, and the third came from the
   owner's hands.
2. **Check that CI is green** before anything else. The fixes are in
   `d619a82` and were verified here by running CI's own steps — the computed
   `CGO_ENABLED=0` package list, and all three cross-builds — but the
   `libp11-kit-dev` half cannot be verified anywhere but on a runner.
3. **§8, packaging**, which is the whole of what is left on the briefing and
   has not been touched. Its list is in the briefing and in F12 §8.
4. **The keystroke path**, when somebody has half an hour: the probe's
   `baseline` mode is the control it needed, its interactive mode now demands
   sixteen unguessable characters, and the question is whether GdkEvent or an
   input method leaves anything. **Do not start from the assumption that it
   does** — that was D-351's error — and do not start from the assumption that
   it does not, which was D-350's.
5. **Type a PIN into the dialog against SoftHSM.** The seam is wired, the
   module signs, and the one thing that has not happened is a person putting
   characters into this window and a card answering.
6. **Watch the live handover and the notification's withdrawal**, both still
   built and unwatched since session 4, and both still on that session's list.

---

## C. The instruments that failed this session, which is five of them

Recorded together because the pattern is the session's main lesson and it is
easier to see in a list than spread across seven entries. **Not one was caught
by reading; every one was caught by a control.**

| the instrument | what it reported | what was wrong |
|---|---|---|
| the bus monitor ([[D-347]]) | 0 copies for plain, DH **and** a string never sent | walked only the top level of each message body; the secret is inside a `(oayays)` struct |
| the undo check ([[D-350]]) | "undo restored nothing" on a password entry | its control said the same on a **plain visible entry**: `gtk_editable_insert_text` does not feed GTK's undo history at all |
| the memory scan, first version ([[D-350]]) | 2 copies **before anything was set** | it found its own global needle and its own scan buffer |
| the memory scan, interactive mode ([[D-351]], [[D-352]]) | 8 copies after a person typed | the needle was a string a person chose; a window with **no input at all** contains 8 of them |
| the relative-path guard ([[D-348]]) | every path absolute | only because the account database answered; the control removes it and requires the guard to fail |
| three new path tests ([[D-353]]) | green here | they compared a platform-*parameterised* function against one platform's separators, so they asserted the runner's separator rather than the argument's |

**The one that was not an instrument failure** is worth naming beside them:
`pin_test.go` refused a refactor that would have passed the PIN to a second
function ([[D-349]]), and there the guard was right and the code was wrong.
Three guards fired on this session's own work; one of them was correct.

---

## D. This machine, and what changed on it

**The rules are unchanged**: one process at a time, no background jobs, no
parallel builds, no load generators. Session 3 §6 bought that with three
crashes and [[D-346]] measured what else it buys — `node --test` in parallel
took **1 318 s** on this VM and **13 s** one at a time, the same twelve files.
Anybody reintroducing parallelism here should measure the serial time first.

**One flake seen and not fixed.** `TestAWaitForAHeldLockExpiresRatherThanBlockingForEver`
failed once under a saturated parallel `go test ./...`, exceeding a 30-second
backstop on a 50-millisecond timeout. It passes alone three times out of three
and the suite passes clean on a re-run. **The test was not changed to pass
under load this session created** — it is recorded here instead.

**Packages installed** (apt, this session): `nodejs`, `npm`, `softhsm2`,
`softhsm2-common`, `libsofthsm2`, `opensc`, `opensc-pkcs11`, `gnutls-bin`,
`libp11-kit-dev`, and as dependencies `pcscd`, `libccid`, `libeac3`,
`libevent-2.1-7t64`, `libunbound8`, `libgnutls-dane0t64`, plus npm's own
`node-*` closure.

**`pcscd.socket` is enabled and active.** It arrived as an `opensc`
dependency rather than by anybody asking for it. F12 §9 wants it, and F12 §9
also warns that it "is socket-activated and has historically been left
disabled after install" — so this machine is now the *good* case and is not
evidence about a fresh one.

**The AppArmor profile is unchanged** and still names
`/home/vboxuser/liro-f12probe`. A WebKitGTK window will not start anywhere
else, so build the agent to that path to run it, and point `XDG_CONFIG_HOME`,
`XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME` and `XDG_RUNTIME_DIR` at a
scratch directory when you do.

**The SoftHSM token is in the scratch directory, not in `/var/lib`.** To use
it: `SOFTHSM2_CONF=<scratch>/softhsm/softhsm2.conf` and
`LIRO_SOFTHSM_PIN=648219`. It holds one RSA key and one self-signed
certificate, `CN=Liro F12 Test Signer`. The real-module tests skip with a
reason when that variable is unset, which is every CI runner.

**Nothing was left in the person's keyring.** The Secret Service tests delete
what they write and the delete is checked; measured after every run, 0 items
with `application=liro-bridge`.

---

## E. The three probes, and what each is for

All in the scratch directory, all built, none in the repository.

- **`pinmem`** — where GTK keeps what is typed. Three modes: no argument
  (random 32-byte needle, zero baseline, the automated measurement);
  `type` (waits for a person, demands sixteen unguessable characters, never
  prints what they typed); `baseline <string>` (**the control** — the same
  window, no input at all, the number of coincidences that must be subtracted).
- **`secretprobe` / `stealprobe`** — the two halves of [[D-347]]'s transfer
  measurement: what appears on the bus, and what an unrelated process gets by
  asking.
- **`lockprobe`** — what a locked collection answers, on a collection of its
  own so that nobody's login keyring is touched.

---

## F. What is open, in the order it is likely to matter

- **§8, all of it.** Nothing has been done.
- **The keystroke path into the PIN dialog**, unmeasured — §C's fourth row.
- **A person typing a PIN into the dialog**, and a signature coming out.
- **The live handover and the notification's withdrawal**, still unwatched
  since session 4.
- **Five example clients still Windows-only** ([[D-345]]), and the SDK README
  teaches the Windows-only path for three of them. The README-versus-file
  guard cannot see this: it checks that a fragment matches its file, which
  says nothing about whether the file is right ([[D-346]]).
- **Drag and drop**, which needs `gdk_file_list_get_files` that the binding
  does not generate ([[D-330]]'s shape) and whose absence the page still
  invites.
- **The Settings window's two Windows-shaped rows on Linux.**
- **`lowerLevel`**, which has no caller and is not a Linux question.
- **The npm-cli resolver's third layout.** `package.test.mjs` finds
  `npm-cli.js` through `npm_execpath` or two guesses, and Debian's location is
  neither. It passes under `npm test` everywhere that matters, so a fourth
  guess was deliberately not added ([[D-346]]).

---

## F.1 Run these before pushing, because this session did not

One command each, all cheap, and each of them would have caught something
tonight:

```
for g in "linux amd64" "windows amd64" "darwin arm64"; do set -- $g
  CGO_ENABLED=0 GOOS=$1 GOARCH=$2 go build -o /tmp/x/ ./...
done                                  # §3 of D-353: only darwin saw it
GOOS=windows go vet ./...             # the platform whose tests this VM cannot run
```

and the two package lists CI computes, which are the only local way to know
that the `CGO_ENABLED=0` sweep still builds:

```
go list ./... | grep -vxF "$(printf '%s\n%s\n%s' \
  github.com/veljaos/liro-bridge/cmd/liro-bridge \
  github.com/veljaos/liro-bridge/internal/pinscreen \
  github.com/veljaos/liro-bridge/internal/ui)" > /tmp/gtkfree.txt
p11=github.com/veljaos/liro-bridge/internal/keysource/pkcs11
go list -f '{{.ImportPath}}{{range .Deps}}{{if eq . "'"$p11"'"}} needs{{end}}{{end}}' ./... |
  awk -v p="$p11" '$1==p || $2=="needs" {print $1}' | sort > /tmp/needsp11.txt
grep -vxF -f /tmp/needsp11.txt /tmp/gtkfree.txt > /tmp/cgofree.txt
CGO_ENABLED=0 GOOS=linux go build -o /tmp/x/ $(cat /tmp/cgofree.txt)
```

**What none of this can check is an apt package the runner does not have.**
That is §1 of [[D-353]] and it has now happened twice ([[D-335]]), both times
for the same reason: the machine the code is written on has the package
because writing the code required installing it.

---

## G. §0.1 still applies to everything above

A virtual machine with no working GPU driver, and a graphics driver that calls
its own configuration broken on every boot. Every reading here is a VM
reading. What a VM cannot show is in F12 §0.1 and has not changed: a real
reader, a real card, and the GPU path.
