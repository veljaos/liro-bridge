# Corrections to decisions.md, recorded and not yet made

`docs/decisions.md` is a dated record: an entry says what was true when it was
written and is not rewritten afterwards. That rule is what makes the file worth
reading, and it is also why an error inside one **propagates** — a later entry
cites the earlier one, and a briefing cites the later one.

This file is where such an error is written down between being found and being
fixed, so that the gap is a known state rather than something only the finder
knows. An item leaves here when the edit is made, in the same commit.

It is **not** for readings that were overturned. Those are entries of their own —
D-355 §6 against D-337, D-351 against D-350, D-394 against D-392's explanation.
This file is for a citation or a fact about the repository that is simply wrong,
where nothing was re-measured and nothing needs to be.

---

## 1. "D-290 measured the Windows edit control" — it did not; D-277 did

**Found:** 2026-09-27, while checking the citations in the B22 briefing before
measuring anything. **Status:** recorded, not fixed, by the owner's decision —
the measurement was the evening's work and this is a different day's.
**Referenced by:** D-395, whose commit message says it is recorded. This is that
record.

### The claim, and the five places it lives

Found in two entries; the sweep below then found it in **four**, in four
consecutive entries, and the origin is **D-350** rather than D-351.

1. `decisions.md`, **D-350** — the origin:
   > *"So the answer had to be measured the way [[D-290]] measured the Windows
   > edit control, not read out of GTK's documentation."*
2. `decisions.md`, **D-351**:
   > *"[[D-290]] measured the edit control and [[D-279]] §6 wrote the exception
   > from it, and **neither measured what was in front of it either.**"*
3. `decisions.md`, **D-352**:
   > *"[[D-279]] §6 wrote the exception from [[D-290]]'s measurement of the
   > control rather than of what fed it."*
4. `decisions.md`, **D-384**, under "Whether Windows does the same":
   > *"The Windows PIN dialog is a Win32 edit control with `ES_PASSWORD`;
   > [[D-290]] measured its memory, not what the accessibility layers were told
   > about it."*
5. The B22 briefing of 2026-09-27, which took it from D-384.

Every one of the five is load-bearing in the same way: each cites D-290 as the
authority for what is known about the Windows edit control, which is the one
thing D-290 says nothing about.

### Why it is wrong

**D-290** is *"The worker's guards are written before the code they constrain,
and its contract is read off the dependency closure rather than the import
block; the control I wrote for it held only under a build tag, which is a
control about the wrong program."* Its body is 133 lines and contains **no
occurrence of "edit control", "WM_GETTEXT", or even "dialog"**. It is about AST
guards, a dependency closure, and a control that held only under
`-tags softtoken`.

### What the right citations are

- **D-277** — *"The PIN dialog is a native window, and it is the only one in the
  product that is not HTML: clause 2 is a statement about this program's memory,
  and a WebView2 page is not this program's memory."* This is where the edit
  control was measured; the line is *"The edit control's own copy is Windows'
  memory. It cannot be wiped, but…"*
- **D-279 §6** — where the exception was written from that measurement. D-351
  already cites this one correctly.

### Why it is worth recording rather than quietly editing

It is not a typo in one place. **It propagated through four entries, and its
fifth stop was a set of instructions to go and measure something.** D-350 wrote
it, D-351 and D-352 carried it within days, D-384 cited it a week later, and the
B22 briefing cited D-384's. That is [[D-336]]'s shape exactly — a briefing
describing a state the repository was not in — caught by pointing D-304's fifth
question at the instructions rather than at an instrument.

**And the count itself is the second finding.** This was recorded as two
occurrences on the strength of the two that had been read. The sweep that the
last section of this item recommends was then run before the record was
committed, and found four, in a different origin entry. A record of an error is
as liable to be under-measured as anything else, and this one was — by half,
for the length of one evening.

It also costs something specific. D-290's real lesson is *"a control about the
wrong program"*, and a reader who follows the citation to check what was
measured about the edit control finds a build-tag argument instead, and has to
work out which of the two entries is misfiled.

### What closing it looks like

One commit, no measurement: `[[D-290]]` → `[[D-277]]` in the four sentences
quoted above — D-350, D-351, D-352, D-384 — and delete this item from this file
in the same commit, saying in the commit message that the citation had reached
four entries and a briefing, because the propagation is the part worth knowing.

**The sweep, already run, and what it leaves.** `grep -n "D-290"
docs/decisions.md` returns **fifteen** lines, which account for themselves as:

- **one** — D-290's own heading;
- **four** — the error above, in D-350, D-351, D-352 and D-384;
- **ten** — read, and correct. They concern the allow-list's control, reading the
  import block versus the whole dependency closure, and the `-tags softtoken`
  control, which is what D-290 is actually about.

Outside `decisions.md`: `f12-handover.md`'s table row and `open-items.md`'s D11
(the PIN guard walker copied across seven packages) are both correct uses.
`open-items.md`'s D21 and this file are about the error rather than instances of
it, so **a future grep returns more lines than this paragraph counts, and the
four to change are still only the four named above.**

That is one reading by one pair of eyes, on an evening when the first count of
this same error was wrong by half. **Re-run the grep when the fix is made** and
check the twelve rather than trusting this line.
