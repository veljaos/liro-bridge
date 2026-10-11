// Package cloud is the agent's side of the cloud channel (docs/phases/F14.md,
// [[D-461]]): a liro-bridge:// link names a job on a server, the agent
// fetches it over HTTPS, the person approves it in the consent window like
// any other request, and the answer goes back to the server it came from.
//
// This package is the part with no window in it: the link, the two HTTPS
// requests and the person's list of servers. It signs nothing and decides
// nothing — a link only says where to look, and what is fetched is put in
// front of the person before anything happens.
package cloud

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Scheme is the URL scheme the agent registers.
const Scheme = "liro-bridge"

// MaxLinkLength bounds a link before anything parses it.
const MaxLinkLength = 2048

// Link is a parsed liro-bridge://sign?job=… link.
type Link struct {
	// JobURL is the job's https URL, as the server wrote it.
	JobURL *url.URL
	// Origin is scheme://host[:port] of JobURL, lower-cased, with the
	// default port left out — the identity the person allows and the
	// consent window shows.
	Origin string
}

// ErrLink is wrapped by every refusal of a link, so a caller can tell a
// link that cannot be used from a server that cannot be reached.
var ErrLink = errors.New("cloud: this link cannot be used")

func refuse(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrLink, fmt.Sprintf(format, args...))
}

// ParseLink reads a link strictly (F14 §1): the scheme, the action
// "sign", one query parameter "job", nothing else; the job URL https,
// with a host, no user information and no fragment.
func ParseLink(raw string) (Link, error) {
	if len(raw) > MaxLinkLength {
		return Link{}, refuse("it is %d bytes, more than %d", len(raw), MaxLinkLength)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Link{}, refuse("it is not a URL")
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return Link{}, refuse("its scheme is %q, not %s", u.Scheme, Scheme)
	}
	// liro-bridge://sign?… puts "sign" in the host; liro-bridge:sign?…
	// puts it in Opaque. Both are the same link to a person.
	action := u.Host
	if action == "" {
		action = u.Opaque
	}
	if action != "sign" || (u.Path != "" && u.Path != "/") || u.User != nil || u.Fragment != "" {
		return Link{}, refuse("it is not a sign link")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Link{}, refuse("its query cannot be read")
	}
	if len(q) != 1 || len(q["job"]) != 1 {
		return Link{}, refuse("it must carry exactly one parameter, job")
	}
	job, err := ParseJobURL(q.Get("job"))
	if err != nil {
		return Link{}, err
	}
	return Link{JobURL: job, Origin: OriginOf(job)}, nil
}

// ParseJobURL checks one URL the way every URL this channel touches is
// checked: https, a host, no user information, no fragment.
func ParseJobURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, refuse("the job URL is not a URL")
	}
	if u.Scheme != "https" {
		return nil, refuse("the job URL is %q, and only https is used", u.Scheme)
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, refuse("the job URL has no host")
	}
	if u.User != nil {
		return nil, refuse("the job URL carries user information")
	}
	if u.Fragment != "" {
		return nil, refuse("the job URL carries a fragment")
	}
	if port := u.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return nil, refuse("the job URL's port is not a port")
		}
	}
	return u, nil
}

// OriginOf is scheme://host[:port], lower-cased, without the default
// port.
func OriginOf(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && port != "443" {
		host += ":" + port
	}
	return "https://" + host
}

// SameOrigin reports whether raw is an https URL on origin — the rule a
// job's resultUrl must keep (F14 §3.1).
func SameOrigin(raw, origin string) (*url.URL, bool) {
	u, err := ParseJobURL(raw)
	if err != nil {
		return nil, false
	}
	return u, OriginOf(u) == origin
}

// Redacted is u without its query string or fragment: what may be logged.
// A capability token in a URL is a credential (F14 §4).
func Redacted(u *url.URL) string {
	c := *u
	c.RawQuery, c.Fragment, c.User = "", "", nil
	return c.String()
}
