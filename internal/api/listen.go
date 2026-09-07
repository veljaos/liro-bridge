package api

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// loopbackHost is the only address this agent ever binds (SPEC §6.1,
// F7 §4.1). It is a constant rather than a parameter deliberately:
// SPEC §6.1 says binding to a non-loopback interface must be
// impossible, not merely off by default, and the way to make something
// impossible is to leave no way to ask for it. Nothing in this package
// takes a host, an interface or a bind address from anywhere —
// configuration, a flag, an environment variable — so there is no code
// path that could be made to listen anywhere else.
const loopbackHost = "127.0.0.1"

// DefaultPortRangeStart and DefaultPortRangeEnd are SPEC §14's port
// range. The agent takes the first free port in it, so several user
// sessions on one machine (SPEC §14.1: an accounting firm over RDP,
// which is a supported configuration and not an edge case) each get
// their own listener, and each finds it through its own per-user
// bridge.json.
const (
	DefaultPortRangeStart = 17580
	DefaultPortRangeEnd   = 17590
)

// ErrNoFreePort is returned when every port in the range is taken.
// Eleven agents on one machine is a real answer to give rather than a
// reason to widen the range: a twelfth session would be found by
// nobody, since the range is what an SDK and this agent agree on.
var ErrNoFreePort = errors.New("api: no free port in the range")

// Listen binds a loopback-only listener on the first free port in
// [start, end] and returns it together with the port it took.
//
// A zero or out-of-order range falls back to SPEC §14's own
// (DefaultPortRangeStart..DefaultPortRangeEnd) rather than refusing:
// the values come from config.json, where they are validated per field
// and not against each other (D-006), so a nonsensical pair can reach
// here and an agent that will not listen at all is a worse answer than
// one that listens where the specification says.
func Listen(start, end int) (net.Listener, int, error) {
	if start < 1 || end < 1 || start > end {
		start, end = DefaultPortRangeStart, DefaultPortRangeEnd
	}
	var lastErr error
	for port := start; port <= end; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, strconv.Itoa(port)))
		if err != nil {
			lastErr = err
			continue
		}
		return ln, port, nil
	}
	if lastErr == nil {
		lastErr = ErrNoFreePort
	}
	return nil, 0, fmt.Errorf("%w (%d-%d): %v", ErrNoFreePort, start, end, lastErr)
}
