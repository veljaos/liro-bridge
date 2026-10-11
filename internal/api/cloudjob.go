package api

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
)

// The cloud channel's jobs (F14, [[D-461]]) are the loopback protocol's
// own requests, fetched from a server rather than posted by a program on
// this machine. So they are read by this package's own rules — the same
// fields, the same limits, the same refusals in the same words — and
// this file is only the envelope around them.

// CloudProtocol is the cloud channel's protocol as its bodies name it.
const CloudProtocol = "liro-cloud/1"

// CloudProtocols is every version of it this agent speaks.
var CloudProtocols = []string{CloudProtocol}

// MaxCloudJobBytes is the largest job body the agent reads: the larger of
// the two loopback limits, since a job may be either kind.
const MaxCloudJobBytes = maxSignPDFBody

type cloudEnvelope struct {
	Protocol  string `json:"protocol"`
	Kind      string `json:"kind"`
	ResultURL string `json:"resultUrl"`
}

// CloudJob is a fetched job, read: the request to put in front of the
// person and where its answer goes.
type CloudJob struct {
	Request   SignRequest
	ResultURL string
}

// ParseCloudJob reads a job fetched over the cloud channel. The
// returned CloudJob carries ResultURL whenever the envelope named one,
// refusals included, so the server can be told why its job was refused. application
// is what the consent window will name — the origin the agent connected
// to, never anything the server wrote (F14 §2). A job in a protocol this
// agent does not speak is PROTOCOL_UNSUPPORTED, with the ones it does;
// documentSigning is the person's setting for the whole-document path,
// which applies here exactly as it does over loopback.
func ParseCloudJob(raw []byte, application string, documentSigning bool) (CloudJob, *errs.Error) {
	var env cloudEnvelope
	if e := decodeJSON(raw, &env); e != nil {
		return CloudJob{}, e
	}
	refused := CloudJob{ResultURL: env.ResultURL}
	if env.Protocol != CloudProtocol {
		return refused, errs.WithDetails(errs.CodeProtocolUnsupported,
			fmt.Errorf("the job is in protocol %q", env.Protocol),
			map[string]any{"protocols": CloudProtocols})
	}
	if env.ResultURL == "" {
		return CloudJob{}, invalidField("resultUrl", "required")
	}
	var (
		req SignRequest
		e   *errs.Error
	)
	switch env.Kind {
	case "documents":
		if !documentSigning {
			return refused, errs.New(errs.CodeDocumentSigningDisabled,
				errors.New("the whole-document signing path is switched off"))
		}
		var body signDocumentsBody
		if e := decodeJSON(raw, &body); e != nil {
			return refused, e
		}
		req, e = validateDocuments(body, application)
	case "digests":
		var body signDigestsBody
		if e := decodeJSON(raw, &body); e != nil {
			return refused, e
		}
		req, e = validateDigests(body, application)
	default:
		return refused, invalidField("kind", `must be "documents" or "digests"`)
	}
	if e != nil {
		return refused, e
	}
	return CloudJob{Request: req, ResultURL: env.ResultURL}, nil
}

// MarshalCloudResult is the body the agent posts back (F14 §3.2): the
// loopback result with the protocol and the batch fingerprint the
// consent window showed, or, for a job nothing was signed in, the code.
func MarshalCloudResult(req SignRequest, result SignResult) ([]byte, error) {
	if result.Code != "" {
		return json.Marshal(map[string]any{"protocol": CloudProtocol, "code": string(result.Code)})
	}
	inner, err := marshalResult(req, result)
	if err != nil {
		return nil, err
	}
	var body map[string]any
	if err := json.Unmarshal(inner, &body); err != nil {
		return nil, err
	}
	body["protocol"] = CloudProtocol
	body["batchFingerprint"] = consent.Fingerprint(req.Digests)
	return json.Marshal(body)
}

// MarshalCloudFailure is the body for a job that could not be put in
// front of the person at all — refused before any window — so the
// server's page can say what happened rather than wait.
func MarshalCloudFailure(e *errs.Error) ([]byte, error) {
	body := map[string]any{"protocol": CloudProtocol, "code": string(e.Code)}
	if len(e.Details) > 0 {
		body["details"] = e.Details
	}
	return json.Marshal(body)
}
