package api

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// The two signing endpoints' limits (F7 §5, §6). They are constants
// here and sentences in docs/PROTOCOL.md, and the test that reads the
// document against this package is what keeps the two the same.
const (
	// MaxDigests is how many digests one /v2/sign request may carry.
	MaxDigests = 500

	// MaxDocuments, MaxDocumentBytes and MaxRequestDocumentBytes bound
	// /v2/sign/pdf. The agent holds a whole document in memory to sign
	// it, so these are not arbitrary: they are what one machine can be
	// asked to do at once without the person watching a window that has
	// stopped repainting.
	MaxDocuments            = 200
	MaxDocumentBytes        = 100 << 20
	MaxRequestDocumentBytes = 500 << 20

	// MaxLabelLength bounds one display label. It is
	// consent.MaxDisplayLength, which is what the consent window shows
	// a file name at, because a label is exactly that: untrusted text
	// this agent draws on a screen (SPEC §6.6).
	MaxLabelLength = consent.MaxDisplayLength
)

// maxSignBody and maxSignPDFBody are the two endpoints' body limits.
//
// They are derived from the limits above rather than typed out beside
// them, so a document limit and a body limit cannot come to disagree.
// Base64 is four characters per three bytes, and the JSON around it —
// field names, quotes, commas, one name per document — is bounded by
// the document count and the label length.
const (
	// base64Chars(n) is (n+2)/3*4 — four characters per three bytes,
	// padding included. It is written out rather than called because
	// these have to be constants: a limit computed at run time is a
	// limit something could change.
	maxSignBody = int64(MaxDigests)*((sha256DigestLength+2)/3*4+MaxLabelLength+64) + 4<<10

	maxSignPDFBody = (MaxRequestDocumentBytes+2)/3*4 +
		int64(MaxDocuments)*(MaxLabelLength+64) + 4<<10
)

// SignKind says which of the two signing endpoints a job came from.
// It reaches the audit log, where SPEC §4.3's two paths are recorded
// distinctly: the hash path never let the agent see a document, and
// that is a different fact about a signature from the other one.
type SignKind string

const (
	// SignDigests is POST /v2/sign — the caller built the PDF and the
	// CMS itself and sends only hashes. The agent never possesses the
	// document, which is what keeps a compromised web application from
	// extracting documents through the agent (SPEC §4.3).
	SignDigests SignKind = "digests"

	// SignDocuments is POST /v2/sign/pdf — the caller cannot build CMS
	// (an ERP in Delphi, C# or Java) and sends the document itself.
	SignDocuments SignKind = "documents"
)

// Document is one PDF a caller sent to /v2/sign/pdf.
type Document struct {
	// Name is the caller's own name for the document, already
	// sanitised for display (SPEC §6.6). It is never treated as a path,
	// never opened, and never logged.
	Name string

	// Content is the PDF itself.
	Content []byte
}

// SignRequest is one accepted job as the agent's own signing flow
// receives it. Everything in it has already been validated; nothing
// downstream re-checks a length or a digest size.
type SignRequest struct {
	// Application is the display name bound at pairing, never one
	// supplied in this request — otherwise an application pairs as
	// "Test" and presents itself as "Liro" (SPEC §6.6, F7 §2.2).
	Application string

	Kind SignKind

	// Thumbprint is the certificate the caller asked for, or empty when
	// it did not name one.
	//
	// For SignDigests it is required and it is load-bearing: the caller
	// has already built a CMS around a particular signer certificate,
	// so a signature made with any other key produces a document that
	// verifies against nothing. The consent window therefore offers
	// that certificate and no other — the person still chooses it and
	// still presses Approve, which is what SPEC §6.5 and §18.15 are
	// about, but they cannot be led into producing an invalid signature
	// by choosing a different one.
	//
	// For SignDocuments it is optional: the agent builds the CMS, so
	// any usable certificate produces a valid document, and an absent
	// thumbprint means the person chooses exactly as they do locally.
	Thumbprint string

	// Digests are what will actually be signed. For SignDocuments they
	// are SHA-256 over each document as sent — the batch fingerprint
	// the consent window shows is built from them either way, so a
	// caller can compare what it sent against what was approved (SPEC
	// §6.6).
	Digests [][]byte

	// Labels is one display name per document, already sanitised.
	Labels []string

	// Documents is populated for SignDocuments and nil for SignDigests.
	Documents []Document

	// Level is the requested PAdES level for SignDocuments ("b-b",
	// "b-t", "b-lt"), or empty to use the agent's own configured level.
	Level string

	// Stamp is the caller's answer to how the signature should look. A
	// non-nil value is a complete answer and skips the method step, so
	// the person sees the approval and nothing else (F7 §6). Nil means
	// they choose, as they do locally.
	Stamp *consent.StampChoice
}

// SignOutcome is what one document in a job produced.
type SignOutcome struct {
	// Signature is the raw signature bytes, for SignDigests.
	Signature []byte

	// Document is the signed PDF, for SignDocuments.
	Document []byte

	// AchievedLevel is the level this document actually reached, for
	// SignDocuments. Never the requested one (SPEC §18.11).
	AchievedLevel string

	// Code is set, and everything else empty, when this document failed.
	Code errs.Code
}

// SignResult is a finished job.
type SignResult struct {
	// Outcomes has one entry per document, in the order they were sent.
	Outcomes []SignOutcome

	// Code is set when the whole job failed before anything could be
	// signed — the person refused, the window timed out, the card was
	// not there. Outcomes is then empty.
	Code errs.Code
}

// Signed reports how many of the outcomes actually produced something.
//
// Carrying no code is not enough: an outcome with neither a signature
// nor a document is a document nothing happened to, and counting it as
// a success would hand a caller an empty entry where a signature should
// be. A Signer is expected to give every document it did not sign a
// code; this is what makes the count right even if one ever does not.
func (r SignResult) Signed() int {
	n := 0
	for _, o := range r.Outcomes {
		if o.Code == "" && (len(o.Signature) > 0 || len(o.Document) > 0) {
			n++
		}
	}
	return n
}

// Signer runs one accepted job to completion: it shows the agent's own
// consent window, waits for a person, opens the card, signs, and
// publishes what is happening onto job as it goes.
//
// It is an interface because internal/api must not import internal/ui
// (SPEC §4.2 rule 4): cmd/liro-bridge is the one place that knows a
// consent window is drawn by WebView2, and a test drives the whole
// protocol with a fake signer and no window at all.
//
// It must never return without either a SignResult carrying outcomes
// or an error — and it must never sign anything without a person
// having pressed Approve (SPEC §6.5, F7 §11). Nothing in this package
// can enforce the second; what this package does is give it no way to
// be told to skip the window, because there is no field for that.
type Signer interface {
	Sign(ctx context.Context, req SignRequest, job *jobs.Job) (SignResult, error)
}

// ---- request bodies -------------------------------------------------

// signDigestsBody is POST /v2/sign (F7 §5).
type signDigestsBody struct {
	CertificateThumbprint string   `json:"certificateThumbprint"`
	DigestAlgorithm       string   `json:"digestAlgorithm"`
	Digests               []string `json:"digests"`
	Labels                []string `json:"labels"`
}

// signDocumentsBody is POST /v2/sign/pdf (F7 §6).
type signDocumentsBody struct {
	CertificateThumbprint string           `json:"certificateThumbprint"`
	Documents             []documentBody   `json:"documents"`
	Level                 string           `json:"level"`
	Stamp                 *stampChoiceBody `json:"stamp"`
}

type documentBody struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// stampChoiceBody is the caller's answer to how the signature should
// look. Visible is a pointer so that "absent" and "false" are different
// facts: an absent stamp block means the person is asked, and
// `{"visible": false}` means the caller has answered, and answered no.
type stampChoiceBody struct {
	Visible  *bool  `json:"visible"`
	Position string `json:"position"`
}

// acceptedSHA256 are the spellings of SHA-256 this endpoint accepts.
// Leniency in what is read costs nothing and refusing "SHA-256"
// because the documentation writes "SHA256" would be a rejection
// nobody could diagnose.
var acceptedSHA256 = map[string]bool{"SHA256": true, "SHA-256": true}

// sha256DigestLength is what a SHA-256 digest must measure. F7 §5
// requires the length to be validated "before anything else happens",
// which is what stops a caller getting a signature over something that
// is not the hash it thinks it is.
const sha256DigestLength = 32

// validateDigests turns a /v2/sign body into a SignRequest, or refuses
// it.
func validateDigests(body signDigestsBody, application string) (SignRequest, *errs.Error) {
	alg := strings.ToUpper(strings.TrimSpace(body.DigestAlgorithm))
	if alg == "SHA1" || alg == "SHA-1" {
		// Named rather than folded into "unsupported": SHA-1 is not an
		// algorithm this project declines to support, it is one SPEC
		// §18.8 forbids producing anywhere, and an integrator reaching
		// for it deserves to be told which field is wrong.
		return SignRequest{}, invalidField("digestAlgorithm", "SHA-1 is never signed")
	}
	if !acceptedSHA256[alg] {
		return SignRequest{}, invalidField("digestAlgorithm", "unsupported digest algorithm")
	}

	if len(body.Digests) == 0 {
		return SignRequest{}, invalidField("digests", "no digests")
	}
	if len(body.Digests) > MaxDigests {
		return SignRequest{}, errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("too many digests"),
			map[string]any{"field": "digests", "max": MaxDigests})
	}

	digests := make([][]byte, 0, len(body.Digests))
	for _, encoded := range body.Digests {
		d, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return SignRequest{}, invalidField("digests", "a digest is not base64")
		}
		if len(d) != sha256DigestLength {
			return SignRequest{}, errs.WithDetails(errs.CodeRequestInvalid,
				errors.New("a digest is the wrong length for the algorithm"),
				map[string]any{"field": "digests", "expectedBytes": sha256DigestLength})
		}
		digests = append(digests, d)
	}

	thumbprint, e := validateThumbprint(body.CertificateThumbprint, true)
	if e != nil {
		return SignRequest{}, e
	}

	labels, e := validateLabels(body.Labels, len(digests))
	if e != nil {
		return SignRequest{}, e
	}

	return SignRequest{
		Application: application,
		Kind:        SignDigests,
		Thumbprint:  thumbprint,
		Digests:     digests,
		Labels:      labels,
	}, nil
}

// validateDocuments turns a /v2/sign/pdf body into a SignRequest, or
// refuses it.
func validateDocuments(body signDocumentsBody, application string) (SignRequest, *errs.Error) {
	if len(body.Documents) == 0 {
		return SignRequest{}, invalidField("documents", "no documents")
	}
	if len(body.Documents) > MaxDocuments {
		return SignRequest{}, errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("too many documents"),
			map[string]any{"field": "documents", "max": MaxDocuments})
	}

	var total int64
	docs := make([]Document, 0, len(body.Documents))
	labels := make([]string, 0, len(body.Documents))
	for _, d := range body.Documents {
		content, err := base64.StdEncoding.DecodeString(d.Content)
		if err != nil {
			return SignRequest{}, invalidField("documents", "a document's content is not base64")
		}
		if len(content) == 0 {
			return SignRequest{}, invalidField("documents", "a document is empty")
		}
		if int64(len(content)) > MaxDocumentBytes {
			return SignRequest{}, errs.WithDetails(errs.CodeRequestInvalid,
				errors.New("a document is too large"),
				map[string]any{"field": "documents", "maxBytes": int64(MaxDocumentBytes)})
		}
		total += int64(len(content))
		if total > MaxRequestDocumentBytes {
			return SignRequest{}, errs.WithDetails(errs.CodeRequestInvalid,
				errors.New("the documents are too large together"),
				map[string]any{"field": "documents", "maxBytes": int64(MaxRequestDocumentBytes)})
		}
		name := sanitiseLabel(d.Name)
		docs = append(docs, Document{Name: name, Content: content})
		labels = append(labels, name)
	}

	thumbprint, e := validateThumbprint(body.CertificateThumbprint, false)
	if e != nil {
		return SignRequest{}, e
	}

	level, e := validateLevel(body.Level)
	if e != nil {
		return SignRequest{}, e
	}

	stamp, e := validateStamp(body.Stamp)
	if e != nil {
		return SignRequest{}, e
	}

	return SignRequest{
		Application: application,
		Kind:        SignDocuments,
		Thumbprint:  thumbprint,
		Digests:     documentDigests(docs),
		Labels:      labels,
		Documents:   docs,
		Level:       level,
		Stamp:       stamp,
	}, nil
}

// validateThumbprint checks the certificate the caller named.
//
// It is hexadecimal because a SHA-1 thumbprint is, and the comparison
// downstream is against a hex string this project produced. Case is not
// significant — Windows shows thumbprints in upper case and this
// project prints them in upper case, and refusing the other spelling
// would be a rejection nobody could diagnose from a response that says
// only which field was wrong.
func validateThumbprint(value string, required bool) (string, *errs.Error) {
	v := strings.TrimSpace(value)
	if v == "" {
		if required {
			return "", invalidField("certificateThumbprint", "no certificate named")
		}
		return "", nil
	}
	if _, err := hex.DecodeString(v); err != nil {
		return "", invalidField("certificateThumbprint", "not hexadecimal")
	}
	return strings.ToUpper(v), nil
}

// validateLabels checks the display names a caller supplied. They are
// optional; when present there must be exactly one per document, since
// a shorter list would leave the window showing some documents with a
// name and some without and no way for a person to tell which is which.
func validateLabels(labels []string, want int) ([]string, *errs.Error) {
	if len(labels) == 0 {
		return nil, nil
	}
	if len(labels) != want {
		return nil, errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("labels and digests are different lengths"),
			map[string]any{"field": "labels", "expected": want})
	}
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, sanitiseLabel(l))
	}
	return out, nil
}

// sanitiseLabel puts caller-supplied display text through the same
// pipeline every file name in this project goes through (SPEC §6.6,
// F5 §5.3): control characters and Unicode direction overrides
// stripped, then truncated with the middle elided.
//
// The sanitised form is what is stored and shown, exactly as a pairing
// binds the sanitised name rather than the raw one (D-178): a value
// that is sanitised at each screen is a value two screens can come to
// disagree about.
func sanitiseLabel(name string) string {
	return consent.SanitizeFileName(name)
}

// validateLevel checks a requested PAdES level.
func validateLevel(level string) (string, *errs.Error) {
	v := strings.ToLower(strings.TrimSpace(level))
	switch v {
	case "":
		// Absent means "whatever this agent is configured to produce",
		// which is the person's own standing answer to the same
		// question. A caller that has no opinion should not be made to
		// have one, and defaulting to a level here would be this
		// package overriding a setting it cannot see.
		return "", nil
	case "b-b", "b-t", "b-lt":
		return v, nil
	default:
		return "", invalidField("level", "not a signature level")
	}
}

// validateStamp checks the caller's answer to how the signature should
// look.
func validateStamp(body *stampChoiceBody) (*consent.StampChoice, *errs.Error) {
	if body == nil {
		return nil, nil
	}
	if body.Visible == nil {
		// A stamp block that does not say whether there is to be a
		// stamp is not an answer, and F7 §6 skips the method step only
		// when the stamp is "supplied in full". Half an answer is
		// refused rather than completed on the caller's behalf.
		return nil, invalidField("stamp.visible", "missing")
	}
	choice := consent.StampChoice{Visible: *body.Visible, Position: strings.TrimSpace(body.Position)}
	if !choice.Visible {
		// No stamp, so the corner is not a question. A position sent
		// alongside visible:false is ignored rather than refused: it is
		// consistent, not contradictory — it says where the stamp would
		// go if there were one.
		choice.Position = consent.StampPositionBottomRight
		return &choice, nil
	}
	if choice.Position == "" {
		return nil, invalidField("stamp.position", "missing")
	}
	if !consent.ValidStampPosition(choice.Position) {
		return nil, invalidField("stamp.position", "not one of the four corners")
	}
	return &choice, nil
}

// documentDigests is SHA-256 over each document as sent. The batch
// fingerprint is built from these, so the value a caller can compute
// from its own bytes is the value the consent window shows.
func documentDigests(docs []Document) [][]byte {
	out := make([][]byte, 0, len(docs))
	for _, d := range docs {
		out = append(out, consent.DigestOf(d.Content))
	}
	return out
}

// invalidField is REQUEST_INVALID naming the field that was wrong. One
// code for every malformed field, because every one of them needs the
// same thing from the caller — fix the request — and a code per field
// is a second vocabulary growing without limit beside the first.
func invalidField(field, why string) *errs.Error {
	return errs.WithDetails(errs.CodeRequestInvalid, errors.New(why),
		map[string]any{"field": field})
}
