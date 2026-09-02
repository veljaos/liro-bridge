package dss

import (
	"crypto/sha1" //nolint:gosec // SHA-1 here is the PAdES/ISO 32000-2 VRI dictionary key algorithm, not a document or signature digest (SPEC §18.8 forbids SHA-1 being *produced as a signature digest*; this is an index key into a PDF dictionary, specified by the format itself)
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
func Apply(doc *pdf.Document, cmsBytes []byte, certs []*x509.Certificate, entries []Entry) (*Result, error) {
	if len(certs) == 0 {
		return nil, fmt.Errorf("dss: no certificates to embed")
	}
	if len(entries) != len(certs) {
		return nil, fmt.Errorf("dss: %d certificates but %d revocation entries", len(certs), len(entries))
	}

	u := pdf.NewUpdate(doc)

	var certRefs, ocspRefs, crlRefs pdf.Array
	complete := true
	for i, cert := range certs {
		certRefs = append(certRefs, addStream(u, cert.Raw))

		switch {
		case len(entries[i].OCSPResponse) > 0:
			ocspRefs = append(ocspRefs, addStream(u, entries[i].OCSPResponse))
		case len(entries[i].CRL) > 0:
			crlRefs = append(crlRefs, addStream(u, entries[i].CRL))
		case i+1 < len(certs):
			// Every certificate except the last (whose issuer is the
			// excluded root, see CollectRevocation) was expected to
			// have evidence.
			complete = false
		}
	}

	dssDict := pdf.Dict{Name("Certs"): certRefs}
	if len(ocspRefs) > 0 {
		dssDict[Name("OCSPs")] = ocspRefs
	}
	if len(crlRefs) > 0 {
		dssDict[Name("CRLs")] = crlRefs
	}
	vriEntry := pdf.Dict{Name("Cert"): certRefs}
	if len(ocspRefs) > 0 {
		vriEntry[Name("OCSP")] = ocspRefs
	}
	if len(crlRefs) > 0 {
		vriEntry[Name("CRL")] = crlRefs
	}
	dssDict[Name("VRI")] = pdf.Dict{pdf.Name(VRIKey(cmsBytes)): vriEntry}

	rootRef, ok := doc.Trailer().Get(Name("Root")).(pdf.Reference)
	if !ok {
		return nil, fmt.Errorf("dss: trailer /Root is not an indirect reference")
	}
	catalog, ok := doc.ResolveDict(rootRef)
	if !ok {
		return nil, fmt.Errorf("dss: /Root does not resolve to a dictionary")
	}
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
	return &Result{Bytes: out, Complete: complete}, nil
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
