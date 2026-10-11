package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
)

// The limits of F14 §3.1.
const (
	RequestTimeout = 30 * time.Second
	resultAttempts = 3 // the first and two retries (F14 §3.2)
)

// ErrJobGone is a job the server says no longer exists: expired, or
// answered already.
var ErrJobGone = errors.New("cloud: the request has expired or was already answered")

// Client makes the channel's two requests. The zero value is not usable;
// use NewClient.
type Client struct {
	http      *http.Client
	userAgent string
	retryWait time.Duration
}

// NewClient returns a client that follows no redirect, keeps no cookie,
// sends no credential and checks certificates against the system's roots
// with no way to turn that off. agentVersion goes into the User-Agent.
func NewClient(agentVersion string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = nil // the system's roots; never InsecureSkipVerify
	transport.ResponseHeaderTimeout = RequestTimeout
	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   RequestTimeout,
			// No redirects: a redirect to another origin is what the
			// same-origin rule forbids, and one within the origin is not
			// needed (F14 §3.1).
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		userAgent: "LiroBridge/" + agentVersion + " " + "liro-cloud/1",
		retryWait: time.Second,
	}
}

// withHTTPClient is for tests: an httptest server's client trusts its own
// certificate, which the system's roots do not.
func (c *Client) withHTTPClient(h *http.Client) *Client {
	h.CheckRedirect = c.http.CheckRedirect
	if h.Timeout == 0 {
		h.Timeout = RequestTimeout
	}
	c.http = h
	c.retryWait = 0
	return c
}

// FetchJob GETs a job, reading at most maxBytes of it.
func (c *Client) FetchJob(ctx context.Context, job *url.URL, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloud: fetching the job from %s: %w", OriginOf(job), err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, ErrJobGone
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("cloud: %s answered %d for the job", OriginOf(job), resp.StatusCode)
	}
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return nil, fmt.Errorf("cloud: %s answered %q, not application/json", OriginOf(job), resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cloud: reading the job from %s: %w", OriginOf(job), err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("cloud: the job from %s is larger than %d bytes", OriginOf(job), maxBytes)
	}
	return body, nil
}

// PostResult sends the answer, retrying twice on a failure that might
// pass. A 4xx is the server's refusal and is not retried.
func (c *Client) PostResult(ctx context.Context, result *url.URL, body []byte) error {
	var last error
	for attempt := 0; attempt < resultAttempts; attempt++ {
		if attempt > 0 && c.retryWait > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryWait * time.Duration(attempt)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, result.String(), bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			last = fmt.Errorf("cloud: sending the result to %s: %w", OriginOf(result), err)
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return nil
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			return fmt.Errorf("cloud: %s refused the result with %d", OriginOf(result), resp.StatusCode)
		default:
			last = fmt.Errorf("cloud: %s answered %d to the result", OriginOf(result), resp.StatusCode)
		}
	}
	return last
}
