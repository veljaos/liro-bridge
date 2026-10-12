# LDAP, and whether B-LT needs it — the owner's decision (D-466)

**Status: decided by the owner (D-470): no LDAP.** One signature at B-LT with the Pošta card settles the route (`docs/measure-before-release.md` 1.1). Was: Written in session 33 at the owner's request. Nothing here is built. Every claim says where it comes from: **measured** (with the entry), **read** (in the code or in a file in the repository, today), or **not known**.

## 1. The premise, checked against the record

The owner wrote that every Serbian issuer publishes its chain over LDAP, and that this is why B-LT has never been reached. The record says something narrower, and the difference changes the decision:

| issuer | what it publishes over LDAP | what stands between it and B-LT | source |
|---|---|---|---|
| **Pošta** | **one certificate**: its intermediate, *Pošta Srbije CA 1*, which the signer's AIA `caIssuers` names only as `ldap://ldap-ocsp.ca.posta.rs/…` | Revocation is **HTTP**: OCSP `http://ldap-ocsp.ca.posta.rs/ocsp` and CRL `http://repository.ca.posta.rs/crl/PostaSrbijeCA1.crl`. So the intermediate is the only missing piece | measured (D-281); SPEC §11.9 |
| **MUP** | nothing: AIA and CRL are HTTP | **Not LDAP.** The OCSP responder refuses a TCP connection on ports 80 and 443. It was measured twice, seventeen days apart. The CRL is **30,017,248 bytes** (852,923 entries, reissued daily), six times the 5 MB per-artefact cap | measured (D-076 and its re-run) |
| **Halcom** | not known | Its AIA points at a `.crl` file where a certificate belongs (SPEC §11.8). Never signed with here | SPEC §11.8–11.9; no card |
| **PKS** | not known | not known | the record has only its CA certificates (below) |
| **E-Smart Systems** | not known | not known | the same |

So: **LDAP stands in front of exactly one certificate of one issuer.** MUP, the citizens' ID card, is blocked by revocation that LDAP would not change. Three issuers have never been measured at all.

## 2. The route that avoids LDAP already exists

**The Trusted List carries every Serbian issuer's intermediate, and the program bundles a copy of it.** I read `internal/trust/tsl/seed/TSL-RS.xml` (the seed compiled into the binary, issued 2026-05-20) and listed its certificates. Among them:
- Pošta Srbije CA 1 (to 2044), and the new Pošta Srbije CA 2 under "CA Root 2026" (to 2041)
- MUP Gradjani CA 4 (to 2045) and MUPCA Gradjani 3
- Halcom BG CA FL/PL e-signature (to 2028) and e-signature 2 (to 2035)
- PKS CA Class1 (to 2039)
- ESS IQCA1 V3 and IQCA2 V3 (to 2033)
- Pošta's TSA certificates

The roots are **not** in it: the list names services, not roots.

**Production already uses it.** The window and protocol paths pass the Trusted List's CA certificates as `Options.TrustStore` (`interactive.go:495`, `list.CACertificates()`). D-462's evidence walk treats a certificate in that store as an **anchor**: it is used as the issuer, and the walk stops there without asking for its revocation (`dss/gather.go:180–195`). An anchor in the national Trusted List is what a validator such as DSS stops at too.

So **by reading**, a Pošta signature at B-LT needs:
- the intermediate, from the bundled list, with no network;
- the signer's OCSP, over HTTP;
- the timestamp authority's chain, whose certificates are also in the list.

**No LDAP anywhere.** This is D-281's option (a), which was "untried". The bundled seed is the bundling the owner suggested, and it already exists. It is refreshed from the network, so a new intermediate (such as CA 2) arrives without a release.

**Measured against stand-ins since this paper (D-466); never with the card.** `internal/pades/trustlist_blt_test.go` builds Pošta's shape:
- root → intermediate → signer;
- the token carries the signer and the root only;
- the AIA for the intermediate is `ldap://`;
- revocation is HTTP.

Results:
- With the intermediate in `TrustStore`: **B-LT**, nothing asked of AIA, no revocation asked for the intermediate, and two OCSP responses in `/DSS` (the signer's and the timestamp authority's).
- The control, without it: **B-T**, with "OCSP/CRL unavailable for at least one certificate".

Both were run in this container under `-race`. **What the test cannot show:**
- that the real list's certificate is byte-for-byte the one Pošta's signers chain to;
- that Pošta's OCSP answers in a form this code accepts;
- what a validator makes of the result.

**One signing at B-LT with the owner's Pošta card** settles all three. Only the owner can do that.

## 3. What an LDAP client would cost, and what it would open

- **A SPEC change first.** SPEC §6.8 enumerates four kinds of outbound request and calls any other a bug. LDAP would be a fifth, needing its own amendment. (Fetching certificates over HTTP through AIA, which the program already does, is not named in §6.8 either. Whether it counts under item 2, "OCSP / CRL", is a reading the owner may want to make while here.)
- **A dependency, or a parser of our own.** Go's standard library has no LDAP. The choices are:
  - `github.com/go-ldap/ldap/v3`, which brings its own BER decoder;
  - an anonymous LDAPv3 base-object search written here: a bind, a search for `cACertificate;binary` or `certificateRevocationList;binary`, and the response. A few hundred lines of BER decoding over `encoding/asn1`.

  Either way, it is **a new parser fed by the network**. It needs fuzzing, as the PDF parser has, and a CI job with an LDAP server to test against.
- **It does not work where the program's users are.** LDAP is TCP port 389, in cleartext, and it does not pass through an HTTP proxy. An office whose only way out is a proxy, which is common where ERPs run, reaches HTTP and not LDAP. The program would gain a feature that fails, slowly, at a timeout, in exactly the networks it is sold into.
- **Little to protect, but more to say.** Integrity is not at stake: what comes back is signed by its issuer and verified as such. But it is one more host the person's machine contacts, for every signature, and the person cannot see it.
- **What it would buy:** an intermediate the Trusted List does not carry. Under SPEC §11.1 a chain that does not end at a service the list grants is **not qualified**, so for the signatures this program exists to make, LDAP adds nothing the list does not already give. It would also reach revocation published only over LDAP, which no issuer in the record does.

## 4. The options

- **A (recommended): no LDAP; the Trusted List route, proven.**
  - The end-to-end test with a TrustStore is written and green against stand-ins (§2).
  - The owner signs once at B-LT with the Pošta card.
  - SPEC §6.8 stays as it is, and the decision is recorded: LDAP is out, because the list carries every qualified intermediate.
  - If the Pošta run fails, it fails with a reason the notes will give, and that reason decides what comes next.
- **B: bundle a chain file of our own** (the five issuers' intermediates and roots). This is redundant with A: the list already has the intermediates, and anchors make the roots unnecessary. Worth it only if A's run shows a validator wanting the root inside `/DSS`, and then the root comes from the card (Pošta's carries it, D-274) or from this file.
- **C: an LDAP client.** The costs are in §3. I would not, unless an issuer turns up whose *revocation* is LDAP-only, and none in the record is.
- **Separate from LDAP, and to be decided anyway: MUP.** B-LT on a MUP card means embedding a 30 MB CRL in every signed PDF, or an OCSP responder that answers. The first is a cap decision (D-076's 5 MB); the second is MUP's. Until one of them changes, **B-T is MUP's ceiling**, and decision-timestamp-failure.md §3.3 is the question of how that is said.

## 5. What B-LT reaches today, plainly

**No Serbian card has reached B-LT.** D-462 built B-LT and measured it against stand-in servers only (see `docs/s33-what-ran.md`). Through the Trusted List it reaches B-LT on a stand-in built in Pošta's shape (§2). On the Pošta card itself it has never been run. On a MUP card it cannot, for reasons outside the program. Halcom, PKS and E-Smart Systems are unmeasured.
