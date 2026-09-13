# Handover: what F10 left, for whoever picks up F11

Written at v0.9.1. Read `docs/phases/F10.md` for what the phase was for
and `docs/decisions.md` D-236…D-263 for how it went. This file is only
the part that is still owed, and where it lives, so nobody derives it
again.

---

## 1. The exit condition is not met, and it is not a code task

**"A stranger installs it and signs a document."** Neither the agent nor
the owner can be that stranger, which F10 says at the top of itself.
Everything up to that point is built and measured; the last step is
somebody's afternoon.

What stands in for it so far, and what each one is *not*:

| Measured | Where | What it is not |
|---|---|---|
| per-user MSI installs, registers, signs with a real card, 100 documents on one PIN, refuses an upgrade mid-batch, uninstalls leaving the audit log | [[D-243]] | not the exit condition — D-243 says so in its own words |
| the per-user package installs and uninstalls under a SAFER Basic User token, stricter than a standard user | [[D-249]] | not a standard account on a machine that never held a Go toolchain, which is what F10's rules ask for |
| the package *declares* it needs no elevation, read from its own summary stream | [[D-249]] | not the same as a token with no rights completing the install — which D-249 then also measured |

**The owner keeps four things and they are not F11's:** his three
Windows machines as a first sieve, a machine without the WebView2
runtime, [[D-249]]'s standard account, and the stranger.

---

## 2. The one artefact check that has never been exercised

**[[D-246]]: the installer's WebView2 `LaunchCondition`.** The MSI
carries a `RegistrySearch` over three `EdgeUpdate` client keys and
refuses the install with an address to get the runtime from. It is
written, it is read, and **it has never run on a machine that genuinely
lacks the runtime.**

What *is* measured is the different thing next to it: what the agent
does at run time with the runtime absent, which is now a message box
naming the runtime and Microsoft's download link, where before it was a
0.35-second silent exit. The two checks read different things — a loader
path and three registry keys — and D-246 keeps them apart deliberately.

Deleting the registry keys to simulate it is explicitly rejected there:
it produces a green check and no knowledge.

---

## 3. The release pipeline has never run

**[[D-239]]: the `release` job has never executed.** v0.9.0 and v0.9.1
were built by it, but nothing has yet exercised a *failure* in it, and
two of its steps had never run at all when [[D-251]] gave them
annotations. `verifyrelease`'s newest check ([[D-263]]) will first be
seen on the first tag after it.

Not a defect. A thing to know before reading a red release run as a
mystery.

---

## 4. The drag is closed, and the instrument is the deliverable

**[[D-260]]** closes it as not reproducible and not explained, after
seven successful drags and two days of elimination. Do not re-open it by
constructing another run.

What matters for anybody who meets it again: the agent now says, once a
second, when something is in front of its own window ([[D-258]]), and
`scripts/dragreport.cmd` collects the rest in one command. **The next
occurrence's log contains the answer or its absence, and either is
conclusive.** That is the whole reason the investigation was allowed to
close.

---

## 5. Reported and deliberately not fixed

Each of these is a decision that says why, not an oversight.

| | Entry |
|---|---|
| A per-user install writes its ARP entry to **HKLM**, so it appears in Programs and Features for every user of the machine — a row other people cannot act on, on SPEC §14.1's shared machine. Not a defect in the package; the Windows Installer service writes it as LocalSystem. Recorded as a question. | [[D-249]] |
| `assertPageDoesNotScroll`'s one-pixel epsilon costs coverage and buys nothing, because the numbers it compares are integers. A `body` overflowing by exactly one pixel draws a scrollbar and passes. Reversing half of [[D-240]] is its own entry. | [[D-252]] |
| The pairing-window layout flake is **still unidentified**. [[D-252]] measured the margins (0, 16 and 36.8 points) and established it is neither the 202-race family nor, on this machine, the tolerance family. [[D-240]]'s three-decimal reporting is what will classify the next occurrence in one reading: a fraction means a tolerance, tens of points mean a stale layout, an absent value means a race. | [[D-240]], [[D-252]] |
| `--out` and `--resign` have no replacement in a release binary. That is the choice, not a gap: both are flags whose value is running with nobody at the machine, and `sign` now opens a window and waits for a person. | [[D-233]] |
| The guide's figures were captured by a path that shifts the primary button's blue; a figure recaptured now renders the true token colour and differs slightly from the ten beside it. Not corrected by editing a screenshot. | [[D-262]] |
| The update prompt has never been shown a real newer release. The channel is tested end to end against a fake server; nobody has yet watched an installed agent offer a genuine update and install it. | [[D-245]] |

---

## 6. What F11 needs to know from F10, specifically

**F11 is the first phase that gives up `CGO_ENABLED=0`.** That trap has
been load-bearing since F0 §10 and three decisions turn on it:

- [[D-012]] scopes `CGO_ENABLED` per CI step rather than per job, so
  `-race` can run while the cross-compilation steps stay pure. Read it
  before changing the workflow: the two rules look contradictory and are
  not.
- [[D-136]] rejected PDFium through cgo for the placement window's
  rasteriser *specifically* to keep that property, and wrote a renderer
  by hand instead. When cgo arrives, that reasoning is worth re-reading
  rather than reversing by default — the renderer's argument was never
  only about cgo, it was that a bug in it cannot reach a signature.
- **`-race` has never run on this machine.** No C compiler; [[D-012]],
  [[D-112]] and [[D-201]] all record it. The Windows runner is the only
  place that check happens, which is why three timing defects were only
  ever seen there. If F11 brings a C toolchain, `-race` becomes runnable
  locally for the first time, and it is worth running the whole suite
  under it once before trusting anything.

**The window layer is where this project's defects live.** Six recorded
lifetime defects ([[D-099]], [[D-101]], [[D-114]], [[D-129]],
[[D-169]], [[D-170]]), and [[D-259]] is a seventh of a different kind.
`internal/ui`'s package doc comment is the summary; read it before
touching anything in there.

**Every window refuses to become any document but one of its own
pages** ([[D-259]]). If F12/F13 bring WKWebView and WebKitGTK, that rule
is the one to carry across — it is stated as what is allowed, not as a
list of what is refused, and that shape is the point.

---

## 7. The habit F10 kept proving

Seven entries in this log now say the same thing from different angles
([[D-087]], [[D-122]], [[D-128]], [[D-161]], [[D-172]], [[D-219]],
[[D-247]]): **a green suite is not evidence about a running program**,
and [[D-247]] adds the sharpest form of it — *a feature can be fully
implemented, fully tested, and never invoked.* F10 found three of those
in one phase, and [[D-263]] found a fourth in a field that had been
written into every release and read by nothing.

The check that finds this class is not a better unit test. It is asking,
of a feature, **what invokes this, and when** — and then doing that
thing to a built binary and watching.
