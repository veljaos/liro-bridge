package dss

import (
	"crypto/sha1" //nolint:gosec // SHA-1 here is the PAdES/ISO 32000-2 VRI dictionary key algorithm, not a document or signature digest (SPEC §18.8 forbids SHA-1 being *produced as a signature digest*; this is an index key into a PDF dictionary, specified by the format itself)
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// Result reports what Apply actually embedded, so the caller can report
// the achieved level honestly (F3 §7.3: never claim a level that was
// not reached).
type Result struct {
	Bytes []byte

	// Complete is true only if every certificate that needed revocation
	// evidence (every certificate but the last, whose issuer — the
	// excluded root — is unknown, see CollectRevocation) got either an
	// OCSP response or a CRL. False means the caller reports B-T, not
	// B-LT.
	Complete bool

	// TooLarge is true when Complete is false specifically because at
	// least one certificate's OCSP response or CRL was obtained but
	// exceeded the configured size cap (Task 1b), rather than because no
	// evidence could be obtained at all. A caller reporting the
	// degradation honestly (Task 1c) needs to say which; LargestSkippedBytes
	// is the biggest single skipped artefact's size, for that message.
	TooLarge            bool
	LargestSkippedBytes int64
}

// VRIKey computes the /VRI dictionary key for one signature: the
// uppercase hex SHA-1 hash of the CMS bytes exactly as embedded in
// /Contents (ISO 32000-2 Annex A / PAdES). This is a dictionary index
// required by the PDF format itself, not a cryptographic digest this
// project produces as part of a signature — SPEC §18.8's "no SHA-1
// produced anywhere" is about signature and document digests, which
// F3 §12.4 pins to SHA-256 throughout; it is not a blanket ban on a
// format-mandated lookup key.
func VRIKey(cms []byte) string {
	h := sha1.Sum(cms) //nolint:gosec // see package-level justification above
	return strings.ToUpper(hex.EncodeToString(h[:]))
}

// Apply appends one incremental revision (F3 §7.1) adding a /DSS
// dictionary to doc's catalog: every certificate in certs, an OCSP
// response or CRL for each entry that has one, and a /VRI entry keyed
// by VRIKey(cmsBytes) pointing at exactly this signature's own
// evidence. certs and entries must be the same length and in the same
// order (entries[i] is certs[i]'s evidence, or a zero Entry if none was
// collected).
//
// If entries carries no OCSP response or CRL at all — every certificate's
// evidence missing, too large, or simply never fetched — no revision is
// written: Result.Bytes is doc's own unmodified bytes (D-079). A /DSS
// whose /Certs is its only content asserts nothing a CMS signature does
// not already carry, so it is not worth a revision. Result.Complete is
// false in that case, because B-LT is B-T plus a /DSS carrying
// revocation evidence and a document with no /DSS has not reached it.
func Apply(doc *pdf.Document, cmsBytes []byte, certs []*x509.Certificate, entries []Entry) (*Result, error) {
	if len(certs) == 0 {
		return nil, fmt.Errorf("dss: no certificates to embed")
	}
	if len(entries) != len(certs) {
		return nil, fmt.Errorf("dss: %d certificates but %d revocation entries", len(certs), len(entries))
	}

	var ocsps, crls [][]byte
	complete := true
	var tooLarge bool
	var largestSkipped int64
	for i := range certs {
		switch {
		case len(entries[i].OCSPResponse) > 0:
			ocsps = append(ocsps, entries[i].OCSPResponse)
		case len(entries[i].CRL) > 0:
			crls = append(crls, entries[i].CRL)
		case entries[i].TooLarge:
			complete = false
			tooLarge = true
			if entries[i].SkippedBytes > largestSkipped {
				largestSkipped = entries[i].SkippedBytes
			}
		case i+1 < len(certs):
			// Every certificate except the last (whose issuer is the
			// excluded root, see CollectRevocation) was expected to
			// have evidence.
			complete = false
		}
	}
	return ApplyMaterial(doc, cmsBytes, Material{
		Certs: certs, OCSPs: ocsps, CRLs: crls,
		Complete: complete, TooLarge: tooLarge, LargestSkippedBytes: largestSkipped,
	})
}

// ApplyMaterial appends one incremental revision (F3 §7.1) carrying m in
// the document's /DSS, and a /VRI entry for this signature.
//
// **A /DSS already in the document is kept** ([[D-462]]). Until session
// 33 this wrote a new /DSS over the catalog's, so a second signature at
// B-LT took away the first one's validation data — DSS 6.5 merges, and so
// does this: every array the old dictionary had is carried over, its
// references untouched, and an artefact already there (by its bytes) is
// not written again; the old /VRI entries stay beside the new one.
//
// If m carries no OCSP response or CRL, no revision is written (D-079):
// Result.Bytes is doc's own bytes and Complete is false.
func ApplyMaterial(doc *pdf.Document, cmsBytes []byte, m Material) (*Result, error) {
	if len(m.OCSPs) == 0 && len(m.CRLs) == 0 {
		// D-079: certificates alone never justify a /DSS revision — they
		// are already in the CMS, so a DSS carrying only /Certs asserts
		// long-term validation evidence the document does not actually
		// have. Skip the revision entirely and hand back doc's own bytes,
		// untouched (Document.Data's own contract: "the same slice, never
		// a copy"), so the caller's result stays exactly the B-T bytes it
		// already had.
		//
		// Complete is false here whatever the caller concluded, and that
		// is the whole of the level claim: B-LT *is* B-T plus a /DSS
		// carrying revocation evidence (SPEC §12.6), so a document with
		// no /DSS in it has not reached B-LT and must never be reported
		// as having done so (SPEC §18.11, D-047). Measured once through
		// the shipped binary, before this rule: `sign --level b-lt`
		// reported "Nivo: B-LT" for output containing no /DSS at all.
		return &Result{Bytes: doc.Data(), Complete: false, TooLarge: m.TooLarge, LargestSkippedBytes: m.LargestSkippedBytes}, nil
	}

	rootRef, ok := doc.Trailer().Get(Name("Root")).(pdf.Reference)
	if !ok {
		return nil, fmt.Errorf("dss: trailer /Root is not an indirect reference")
	}
	catalog, ok := doc.ResolveDict(rootRef)
	if !ok {
		return nil, fmt.Errorf("dss: /Root does not resolve to a dictionary")
	}

	u := pdf.NewUpdate(doc)
	dssDict := pdf.Dict{}
	existingVRI := pdf.Dict{}
	if old, ok := doc.ResolveDict(catalog.Get(Name("DSS"))); ok {
		for k, v := range old {
			dssDict[k] = v
		}
		if vri, ok := doc.ResolveDict(old.Get(Name("VRI"))); ok {
			existingVRI = vri
		}
	}

	certRefs := appendArtefacts(doc, u, dssDict, "Certs", rawCerts(m.Certs))
	ocspRefs := appendArtefacts(doc, u, dssDict, "OCSPs", m.OCSPs)
	crlRefs := appendArtefacts(doc, u, dssDict, "CRLs", m.CRLs)

	vriEntry := pdf.Dict{Name("Cert"): certRefs}
	if len(ocspRefs) > 0 {
		vriEntry[Name("OCSP")] = ocspRefs
	}
	if len(crlRefs) > 0 {
		vriEntry[Name("CRL")] = crlRefs
	}
	vri := pdf.Dict{}
	for k, v := range existingVRI {
		vri[k] = v
	}
	vri[pdf.Name(VRIKey(cmsBytes))] = vriEntry
	dssDict[Name("VRI")] = vri

	catalogCopy := make(pdf.Dict, len(catalog)+1)
	for k, v := range catalog {
		catalogCopy[k] = v
	}
	dssNum := u.NewObjectNumber()
	u.Set(dssNum, dssDict)
	catalogCopy[Name("DSS")] = pdf.Reference{Num: dssNum}
	u.Set(rootRef.Num, catalogCopy)

	out, err := u.Apply()
	if err != nil {
		return nil, err
	}
	return &Result{Bytes: out, Complete: m.Complete, TooLarge: m.TooLarge, LargestSkippedBytes: m.LargestSkippedBytes}, nil
}

func rawCerts(certs []*x509.Certificate) [][]byte {
	out := make([][]byte, 0, len(certs))
	for _, c := range certs {
		out = append(out, c.Raw)
	}
	return out
}

// appendArtefacts sets dssDict[key] to the array the old /DSS had there,
// followed by a new stream for each of items not already in it, and
// returns the references to items — old or new — for this signature's
// /VRI entry.
func appendArtefacts(doc *pdf.Document, u *pdf.Update, dssDict pdf.Dict, key string, items [][]byte) pdf.Array {
	var arr pdf.Array
	have := map[[32]byte]pdf.Reference{}
	if old, ok := doc.Resolve(dssDict.Get(Name(key))).(pdf.Array); ok {
		for _, el := range old {
			arr = append(arr, el)
			ref, isRef := el.(pdf.Reference)
			st, isStream := doc.Resolve(el).(*pdf.Stream)
			if !isRef || !isStream {
				continue
			}
			if data, err := doc.DecodeStream(st); err == nil {
				have[sha256.Sum256(data)] = ref
			}
		}
	}
	var mine pdf.Array
	for _, raw := range items {
		h := sha256.Sum256(raw)
		if ref, ok := have[h]; ok {
			mine = append(mine, ref)
			continue
		}
		ref := addStream(u, raw)
		have[h] = ref
		arr = append(arr, ref)
		mine = append(mine, ref)
	}
	if len(arr) > 0 {
		dssDict[Name(key)] = arr
	}
	return mine
}

// addStream registers raw as a new plain-stream object (no filter, no
// extra dictionary keys beyond what pdf.Stream always writes) and
// returns a reference to it.
func addStream(u *pdf.Update, raw []byte) pdf.Reference {
	num := u.NewObjectNumber()
	u.Set(num, &pdf.Stream{Dict: pdf.Dict{}, Raw: raw})
	return pdf.Reference{Num: num}
}

// Name is a local alias so this file reads naturally without a
// pdf.Name(...) conversion at every dictionary key.
type Name = pdf.Name
