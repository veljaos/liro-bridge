//go:build linux

package chooser

import (
	"bufio"
	"encoding/json"
	"io"
	"os"

	"github.com/veljaos/liro-bridge/internal/platform/corelimit"
)

// maxRequest bounds the request line. A title, a few filters and a folder
// are well under a kilobyte.
const maxRequest = 64 << 10

// Run is the helper, from main. It reads one Request from stdin, asks the
// portal, writes one Result to stdout, and returns the exit status.
//
// It sets the core limit to zero and leaves the dumpable flag alone: that is
// the whole reason it is a process of its own (protocol.go). If the limit
// cannot be set it does nothing else, because a helper that could leave a
// core would leave one holding the paths chosen.
func Run(stdin io.Reader, stdout io.Writer) int {
	if err := corelimit.Set(); err != nil {
		return reply(stdout, Result{Outcome: OutcomeError, Detail: err.Error()})
	}

	in := bufio.NewReaderSize(io.LimitReader(stdin, maxRequest+1), 4096)
	line, err := in.ReadBytes('\n')
	if err != nil {
		return reply(stdout, Result{Outcome: OutcomeError, Detail: "reading the request: " + err.Error()})
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return reply(stdout, Result{Outcome: OutcomeError, Detail: "decoding the request: " + err.Error()})
	}
	if req.Kind != KindFiles && req.Kind != KindFolder {
		return reply(stdout, Result{Outcome: OutcomeError, Detail: "the request names no kind of chooser"})
	}

	// After the request, stdin stays open for as long as the parent wants
	// an answer. Its end, for any reason, is the parent saying stop.
	stop := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		close(stop)
	}()

	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		return reply(stdout, Result{Outcome: OutcomeError, Detail: "no session bus address was given"})
	}
	return reply(stdout, choose(address, req, stop, limits{call: CallTimeout, ceiling: Ceiling}))
}

func reply(stdout io.Writer, r Result) int {
	b, err := json.Marshal(r)
	if err != nil {
		return 1
	}
	if _, err := stdout.Write(append(b, '\n')); err != nil {
		return 1
	}
	if r.Outcome == OutcomeError {
		return 1
	}
	return 0
}
