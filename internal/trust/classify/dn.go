package classify

import (
	"crypto/x509"
	"encoding/asn1"
	"strings"
)

var (
	oidSerialNumber     = asn1.ObjectIdentifier{2, 5, 4, 5}
	oidSurname          = asn1.ObjectIdentifier{2, 5, 4, 4}
	oidCountry          = asn1.ObjectIdentifier{2, 5, 4, 6}
	oidLocality         = asn1.ObjectIdentifier{2, 5, 4, 7}
	oidOrganization     = asn1.ObjectIdentifier{2, 5, 4, 10}
	oidGivenName        = asn1.ObjectIdentifier{2, 5, 4, 42}
	oidOrganizationID   = asn1.ObjectIdentifier{2, 5, 4, 97}
	oidEmailAddressInDN = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}
)

// personalIdentifierPrefixes are every semantic-identifier prefix SPEC
// §11.6 Trap 2 documents inside the serialNumber attribute: the national
// ID (all three issuers), plus Halcom's ID-card and passport variants
// for non-residents. Every value with one of these prefixes is personal
// data and is discarded, never stored in Subject.
var personalIdentifierPrefixes = []string{"PNORS", "IDCRS", "PAS"}

func hasPersonalIdentifierPrefix(v string) bool {
	for _, p := range personalIdentifierPrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// parseSubject builds a scrubbed Subject from cert's DN.
//
// It iterates every entry of cert.Subject.Names — never "the" value of
// a given attribute type — because SPEC §11.6 Trap 1 measured that
// Serbian certificates put two values of the same attribute type
// (serialNumber, organizationIdentifier) inside a single multi-valued
// RDN. A parser that looks up "the" serialNumber attribute gets
// whichever of the two happens to be returned, non-deterministically,
// which is the easiest way to leak a national ID number into a field
// meant for an internal CA reference.
func parseSubject(cert *x509.Certificate) Subject {
	s := Subject{CommonName: cert.Subject.CommonName}

	for _, atv := range cert.Subject.Names {
		v, ok := atv.Value.(string)
		if !ok {
			continue
		}
		switch {
		case atv.Type.Equal(oidSerialNumber):
			switch {
			case strings.HasPrefix(v, "CA:RS-"):
				s.IssuerAssignedID = strings.TrimPrefix(v, "CA:RS-")
			case hasPersonalIdentifierPrefix(v):
				// National ID, ID-card or passport number — personal
				// data, deliberately discarded (SPEC §11.6 Trap 2).
			}
		case atv.Type.Equal(oidOrganizationID):
			switch {
			case strings.HasPrefix(v, "MB:RS-"):
				s.CompanyID = strings.TrimPrefix(v, "MB:RS-")
			case strings.HasPrefix(v, "VATRS-"):
				s.TaxID = strings.TrimPrefix(v, "VATRS-")
			}
		case atv.Type.Equal(oidGivenName):
			s.GivenName = v
		case atv.Type.Equal(oidSurname):
			s.Surname = v
		case atv.Type.Equal(oidCountry):
			s.Country = v
		case atv.Type.Equal(oidLocality):
			s.Locality = v
		case atv.Type.Equal(oidOrganization):
			s.Organisation = v
		case atv.Type.Equal(oidEmailAddressInDN):
			// Halcom puts the email address inside the Subject DN
			// itself, not SubjectAlternativeName (SPEC §11.6 Trap 3).
			// Discarded, exactly like the SAN case in classify.go.
		}
	}

	// The display name is built from givenName + surname, never parsed
	// out of CN (SPEC §11.7): CN carries a CA-specific number and,
	// depending on issuer, literal suffix text like " Sign", and is not
	// reliably split by any regular expression. If either name part is
	// missing, fall back to the raw CN rather than trying to clean it.
	switch {
	case s.GivenName != "" && s.Surname != "":
		s.DisplayName = s.GivenName + " " + s.Surname
	case s.GivenName != "":
		s.DisplayName = s.GivenName
	case s.Surname != "":
		s.DisplayName = s.Surname
	default:
		s.DisplayName = s.CommonName
	}

	return s
}
