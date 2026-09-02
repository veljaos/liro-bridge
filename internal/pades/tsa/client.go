package tsa

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

// F3 §6.3's explicit retry numbers.
const (
	maxAttempts     = 3
	attemptTimeout  = 15 * time.Second
	backoffAttempt1 = 1 * time.Second
	backoffAttempt2 = 3 * time.Second
)

// maxSkew is F3 §6.5's explicit window: genTime must be within ±10
// minutes of now, allowing for clock skew on both sides.
const maxSkew = 10 * time.Minute

const (
	contentTypeQuery = "application/timestamp-query"
	contentTypeReply = "application/timestamp-reply"
)

// Auth configures the two authentication modes F3 §6.2 requires be
// implemented, since production TSAs use client certificates and
// exercising only HTTP Basic would leave that path untested.
type Auth struct {
	// BasicUsername/BasicPassword, if BasicUsername is non-empty, send
	// HTTP Basic authentication with every request.
	BasicUsername string
	BasicPassword string

	// ClientCertificate, if non-nil, is presented for TLS client
	// certificate authentication.
	ClientCertificate *tls.Certificate
}

// Client is an RFC 3161 time-stamping client for one TSA endpoint.
type Client struct {
	Endpoint   string
	Auth       Auth
	HTTPClient *http.Client // nil means build one from Auth
}

// NewClient returns a Client for endpoint, building an *http.Client
// configured for auth's TLS client certificate if one is set.
func NewClient(endpoint string, auth Auth) *Client {
	c := &Client{Endpoint: endpoint, Auth: auth}
	transport := &http.Transport{}
	if auth.ClientCertificate != nil {
		transport.TLSClientConfig = &tls.Config{
			Certificates: []tls.Certificate{*auth.ClientCertificate},
			MinVersion:   tls.VersionTLS12,
		}
	}
	c.HTTPClient = &http.Client{Transport: transport}
	return c
}

// retryableError marks an error as one F3 §6.3 says to retry: a network
// error, a timeout, or an HTTP 5xx. Everything else (a 4xx, or an
// RFC 3161-level rejection) is deterministic and is returned to the
// caller immediately instead.
type retryableError struct{ err error }

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// HTTPStatusError is returned for an HTTP 4xx response (Task 6): the TSA
// actively refused the request — bad credentials, a malformed query —
// which is a different problem from a network failure or a repeated 5xx
// and needs a different user action (check credentials, not wait and
// retry). Distinguishable via errors.As, alongside *RejectionError for an
// RFC 3161-level rejection, so a caller can map both to TSA_REJECTED
// instead of the generic TSA_UNAVAILABLE.
type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("tsa: HTTP %d", e.StatusCode)
}

// Timestamp requests a timestamp over digest (SHA-256) and validates
// the response before returning it: the echoed nonce, the message
// imprint, and genTime's ±10 minute skew window (F3 §6.5). TSA
// certificate-chain validation against the Trusted List is the caller's
// job (this package has no dependency on internal/trust — see
// internal/pades/cms.CompleteChain for why that split exists).
//
// Failure after three attempts returns a plain error; F3 §12.8 requires
// the caller, not this package, to decide what happens next (abort, or
// save at B-B) — this package never silently downgrades anything on its
// own.
func (c *Client) Timestamp(ctx context.Context, digest []byte) (*Response, error) {
	reqDER, nonce, err := buildRequest(digest)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := backoffAttempt1
			if attempt == 2 {
				backoff = backoffAttempt2
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := c.doOnce(ctx, reqDER)
		if err == nil {
			if verr := validateResponse(resp, digest, nonce); verr != nil {
				return nil, verr // deterministic: retrying will not fix a wrong nonce or imprint
			}
			return resp, nil
		}
		lastErr = err
		var re *retryableError
		if !errors.As(err, &re) {
			return nil, err // 4xx or RFC 3161 rejection: never retried
		}
	}
	return nil, fmt.Errorf("tsa: %s: no response after %d attempts: %w", c.Endpoint, maxAttempts, lastErr)
}

// doOnce performs exactly one HTTP round trip with a 15-second timeout.
func (c *Client) doOnce(ctx context.Context, reqDER []byte) (*Response, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.Endpoint, bytes.NewReader(reqDER))
	if err != nil {
		return nil, fmt.Errorf("tsa: building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentTypeQuery)
	if c.Auth.BasicUsername != "" {
		httpReq.SetBasicAuth(c.Auth.BasicUsername, c.Auth.BasicPassword)
	}

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, &retryableError{fmt.Errorf("tsa: %s: %w", c.Endpoint, err)}
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return nil, &retryableError{fmt.Errorf("tsa: %s: reading response: %w", c.Endpoint, err)}
	}

	if httpResp.StatusCode >= 500 {
		return nil, &retryableError{fmt.Errorf("tsa: %s: HTTP %d", c.Endpoint, httpResp.StatusCode)}
	}
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("tsa: %s: %w", c.Endpoint, &HTTPStatusError{StatusCode: httpResp.StatusCode})
	}

	resp, err := parseResponse(body)
	if err != nil {
		return nil, err // malformed body or RFC 3161 rejection: not retried
	}
	return resp, nil
}

// validateResponse checks the three things F3 §6.1/§6.5 require before
// a response is trusted: the nonce we sent comes back unchanged, the
// message imprint matches what we asked to be timestamped, and genTime
// is within the ±10 minute skew window.
func validateResponse(resp *Response, digest []byte, nonce *big.Int) error {
	if resp.Nonce == nil || resp.Nonce.Cmp(nonce) != 0 {
		return fmt.Errorf("tsa: nonce mismatch: sent %s, got %v", nonce, resp.Nonce)
	}
	if !resp.MessageImprintAlg.Equal(oidSHA256) {
		return fmt.Errorf("tsa: response messageImprint uses algorithm %v, want SHA-256", resp.MessageImprintAlg)
	}
	if !bytes.Equal(resp.MessageImprintHash, digest) {
		return fmt.Errorf("tsa: response messageImprint does not match the requested digest")
	}
	skew := time.Since(resp.GenTime)
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSkew {
		return fmt.Errorf("tsa: genTime %s is %s from now, outside the %s skew window", resp.GenTime, skew, maxSkew)
	}
	return nil
}
