# The default signature level — the owner's decision (D-474)

**Status:** for the owner's decision. Its own question, not C26's (the owner, 2026-10-12). Nothing here is built.

## 1. What is true today

- **SPEC §12.6 and `config.go` make B-LT the default** (`signatureLevel: "b-lt"`).
- **MUP's card cannot reach B-LT with this program, by any route it has.** Its OCSP responder refuses a TCP connection; this was measured twice, seventeen days apart (D-076). Its CRL is 30,017,248 bytes against the 5 MB cap. LDAP would not change either fact (D-466).
- **Pošta's card reaches B-LT through the Trusted List on a stand-in of its shape.** It has never done so with the card (worklist 1.1).
- **Halcom, PKS and E-Smart Systems have never been measured.**
- **A default install reaches neither B-T nor B-LT on its own.** No timestamp authority is built in (D-433, D-434), so until the person enters one, the window asks before every batch: sign without a timestamp, configure one, or cancel. "B-LT by default" therefore describes a level that needs, first, an authority the person configures (and, for a qualified stamp, pays Pošta for), and second, an issuer whose evidence can be had.
- **Since C26 (D-469),** a B-T reached where B-LT was set is shown in the warning colour, with a sentence. On a MUP card that is **every** successful signature, permanently: nothing the person can do will make it go away.

So the default names a level that the citizens' ID card cannot reach, and a default install reaches neither it nor B-T until an authority is configured.

## 2. The options, with what each costs

### A. Make B-T the default, and say why

- **What changes:** `defaultSignatureLevel` becomes `b-t`, and SPEC §12.6 says why: B-LT is reachable only for some issuers, and a default should be a level the program can deliver. B-LT stays a choice in Settings, and stays something a calling program can name.
- **What it costs:**
  - Pošta holders, and any other issuer that can reach B-LT, get B-T unless they change Settings. For a signature that must still be checkable after the certificate expires or is revoked, that is weaker: B-LT carries the revocation evidence inside the document, and B-T relies on it being obtainable later.
  - Existing installs that saved `b-lt` keep it. Only new defaults change, and the default cannot be told from a chosen B-LT in a saved config (D-469).
  - The guide and SPEC change; small code.
- **What it buys:** on a default install the report stops warning about a level nobody chose. A warning that is always on, and that the person cannot act on, teaches them to ignore the warning colour — the same lesson as a test that cannot fail.

### B. Keep B-LT, and let it fall short with a warning (what is built now)

- **What changes:** nothing.
- **What it costs:**
  - Every MUP signature carries a warning the person cannot act on, for as long as MUP's responder refuses connections.
  - The warning colour stops meaning "look at this" for the largest group of users.
  - A Settings level B-LT that a person *chose* cannot be told from the default (D-469), so it can never be refused as a request.
- **What it buys:** issuers that can reach B-LT get it with no one changing anything. The shortfall is never hidden.

### C. "Best available" as the default, and a chosen level as a request (recommended)

- **What changes:**
  - The default is a new value, `signatureLevel: ""`, read as *the best level the evidence allows*. It means B-LT where the issuer's evidence can be had and B-T where it cannot, **with no warning**, because nobody asked for more. The report states the level reached, as it does now.
  - A level the person **chooses** in Settings (B-B, B-T or B-LT) is then a request, held to C26's rule like a calling program's: reached, or refused with `LEVEL_NOT_REACHED`.
  - Settings gains a fourth choice, "the best available (B-LT where the issuer allows it)", selected by default.
- **What it costs:**
  - A config value and a Settings radio; SPEC §12.6 rewritten; the guide; tests.
  - The `""` value has to survive load and save without being replaced by a level. That is the very thing `config.go` does today, so it is a real change to config handling, not a rename.
- **What it buys:**
  - The default delivers the most the card allows.
  - It resolves D-469's ambiguity, because config can then tell "nobody chose" from "chose B-LT".
  - A person who does choose B-LT gets exactly the owner's rule: refuse rather than quietly weaker.
- **The tension, said plainly:** under C, a MUP signature on a default install is B-T with no warning. That is not a downgrade from anything anyone asked for, and the level is stated. But a person who assumes "the program does B-LT" would not be told that it did not. Whether *the program's own default* counts as a promise is exactly what this option decides that it does not.

### D. A default per issuer

- **What changes:** the agent knows that MUP cannot reach B-LT, and defaults to B-T for MUP certificates and to B-LT otherwise.
- **What it costs:** it hard-codes issuers' current failures into the program. When MUP's responder comes back, the program goes on asking for less until a release changes it. It is against D-433's "hard-codes no service" in spirit, and it is knowledge the program has no way to keep true.
- **Not recommended.**

## 3. What it does not decide

- Whether a default install should have a timestamp authority at all. That is D-433/D-434's decision: none is built in. Every option above sits on top of the person configuring one.
- MUP's cap. Embedding a 30 MB CRL in every MUP-signed document would make B-LT reachable for MUP at a cost no one has weighed (D-076).
- The Pošta measurement (worklist 1.1), which may show B-LT unreachable for Pošta too. That would strengthen A and C equally.

## 4. Recommendation

**C**, because it is the one option under which the program's default never warns about something nobody asked for, and a level somebody did ask for is never quietly weaker. **If C's cost is too much for now, A** is the cheaper honest choice. **B** is what exists, and its cost grows with every MUP signature.
