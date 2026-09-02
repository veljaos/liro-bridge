package tsl

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// The structs below mirror the ETSI TS 119 612 shape enough to extract
// what F1 needs. encoding/xml matches by local name when a tag has no
// namespace prefix, so these work regardless of which prefix the
// document happens to bind to a given namespace (F1 §4.2 — the real list
// binds the XML-DSig namespace to "ns2" at the root but "ds" locally on
// the Signature element itself; struct-based parsing here never has to
// care).
type xmlDocument struct {
	XMLName xml.Name `xml:"TrustServiceStatusList"`
	Scheme  struct {
		SequenceNumber int    `xml:"TSLSequenceNumber"`
		IssueDateTime  string `xml:"ListIssueDateTime"`
	} `xml:"SchemeInformation"`
	Providers []xmlProvider `xml:"TrustServiceProviderList>TrustServiceProvider"`
}

type xmlProvider struct {
	Names    []xmlLangString `xml:"TSPInformation>TSPName>Name"`
	Services []xmlService    `xml:"TSPServices>TSPService"`
}

type xmlLangString struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type xmlService struct {
	TypeIdentifier string          `xml:"ServiceInformation>ServiceTypeIdentifier"`
	Names          []xmlLangString `xml:"ServiceInformation>ServiceName>Name"`
	Certificates   []string        `xml:"ServiceInformation>ServiceDigitalIdentity>DigitalId>X509Certificate"`
	Status         string          `xml:"ServiceInformation>ServiceStatus"`
	StatusStart    string          `xml:"ServiceInformation>StatusStartingTime"`
	Qualifiers     []xmlQualifier  `xml:"ServiceInformation>ServiceInformationExtensions>Extension>Qualifications>QualificationElement>Qualifiers>Qualifier"`
	History        []xmlHistory    `xml:"ServiceHistory>ServiceHistoryInstance"`
}

type xmlQualifier struct {
	URI string `xml:"uri,attr"`
}

type xmlHistory struct {
	Status      string `xml:"ServiceStatus"`
	StatusStart string `xml:"StatusStartingTime"`
}

// englishOrFirst picks the "en" entry from a list of language-tagged
// strings, falling back to whatever is first. TSP and service names are
// published in several languages; the agent's own display logic (later
// phases) can re-localise, but a name is needed here regardless.
func englishOrFirst(names []xmlLangString) string {
	for _, n := range names {
		if n.Lang == "en" {
			return n.Value
		}
	}
	if len(names) > 0 {
		return names[0].Value
	}
	return ""
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func decodeCert(b64 string) ([]byte, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return nil, nil
	}
	// The document wraps base64 across lines; Go's base64 decoder
	// rejects embedded whitespace, so strip it first.
	b64 = strings.Join(strings.Fields(b64), "")
	return base64.StdEncoding.DecodeString(b64)
}

// Parse decodes raw Trusted List XML into a List. It does not verify the
// document's signature — see Verify — and does not decide whether any
// individual certificate is trustworthy.
func Parse(data []byte) (*List, error) {
	var doc xmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing trusted list XML: %w", err)
	}

	issuedAt, err := parseTime(doc.Scheme.IssueDateTime)
	if err != nil {
		return nil, fmt.Errorf("parsing ListIssueDateTime: %w", err)
	}

	list := &List{
		Sequence: doc.Scheme.SequenceNumber,
		IssuedAt: issuedAt,
	}

	for _, xp := range doc.Providers {
		p := Provider{Name: englishOrFirst(xp.Names)}
		for _, xs := range xp.Services {
			svc, err := convertService(xs)
			if err != nil {
				return nil, fmt.Errorf("provider %q: %w", p.Name, err)
			}
			p.Services = append(p.Services, svc)
		}
		list.Providers = append(list.Providers, p)
	}
	return list, nil
}

func convertService(xs xmlService) (Service, error) {
	statusStart, err := parseTime(xs.StatusStart)
	if err != nil {
		return Service{}, fmt.Errorf("service %q: parsing StatusStartingTime: %w", xs.TypeIdentifier, err)
	}

	var cert []byte
	for _, c := range xs.Certificates {
		der, err := decodeCert(c)
		if err != nil {
			return Service{}, fmt.Errorf("service %q: decoding X509Certificate: %w", xs.TypeIdentifier, err)
		}
		if der != nil {
			cert = der
			break
		}
	}

	svc := Service{
		Type:        xs.TypeIdentifier,
		Name:        englishOrFirst(xs.Names),
		Certificate: cert,
		Status:      xs.Status,
		StatusStart: statusStart,
	}
	for _, q := range xs.Qualifiers {
		svc.Qualifiers = append(svc.Qualifiers, q.URI)
	}
	for _, h := range xs.History {
		hStart, err := parseTime(h.StatusStart)
		if err != nil {
			return Service{}, fmt.Errorf("service %q: parsing history StatusStartingTime: %w", xs.TypeIdentifier, err)
		}
		svc.History = append(svc.History, HistoryEntry{Status: h.Status, StatusStart: hStart})
	}
	return svc, nil
}
