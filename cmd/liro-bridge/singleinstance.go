package main

// One agent per user session, and what a second launch does instead.
//
// F12 §7.1 asked three questions and this answers all three, because
// they are one question asked from three sides:
//
//   - **Does a second instance refuse, hand over, or run alongside?**
//     It hands over and exits. A second agent would hold a second port,
//     write its own discovery file over the first one's, and leave the
//     first running and unreachable by the only mechanism SPEC §14
//     permits — which is not a hypothetical: it is [[D-323]], observed
//     twice in one afternoon on Windows.
//   - **Who owns `$XDG_RUNTIME_DIR/liro/bridge.json`?** The agent whose
//     port it names ([[D-325]] decided that for removal). What is added
//     here is the other half: an agent that finds a file naming a port
//     nobody answers on may replace it.
//   - **Is a stale file distinguishable from a live one?** On Windows it
//     was not, and the comment in RemoveBridgeFile said so. It is here,
//     and the distinction is a measurement rather than a belief: **a
//     discovery file is a claim, and dialing is the check.**

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// agentDialTimeout bounds the liveness check.
//
// It is short because of where it sits: every `open` and every `tray`
// pays it before doing anything, and the thing at the other end is a
// loopback listener in a process on this same machine. Anything that
// has not answered in this long is not an agent this launch should hand
// its request to.
const agentDialTimeout = 750 * time.Millisecond

// liveAgent reports the agent already running in this session, if there
// is one.
//
// **What it does not do is trust the file.** A discovery file outlives
// the process that wrote it whenever that process did not shut down
// cleanly — a crash, a kill, a machine switched off — and a launch that
// believed one would hand its request to nobody and exit, which is the
// worst of the three possible behaviours: silent. So the file supplies
// a port and the port is asked.
//
// **And "something is listening" is not the answer either.** The port
// range is small and fixed (SPEC §14), and another program may well have
// taken it while this one was not running. What makes it *this* program
// is that it answers GET /v2/health with a protocol version — the one
// endpoint that needs no pairing, exists for exactly this kind of
// question, and cannot be answered by accident.
func liveAgent() (api.BridgeInfo, bool) {
	path := platform.DefaultBridgeFile()
	info, err := api.ReadBridgeFile(path)
	if err != nil {
		// No file, or one this program cannot read. Either way there is
		// nothing to hand over to.
		return api.BridgeInfo{}, false
	}
	if !agentAnswers(info.Port) {
		slog.Info("startup: the discovery file names a port nobody answers on, so it is stale and this agent may replace it",
			"file", path, "port", info.Port)
		return api.BridgeInfo{}, false
	}
	return info, true
}

// agentAnswers asks the port whether this program is behind it.
func agentAnswers(port int) bool {
	if port <= 0 {
		return false
	}
	client := &http.Client{
		Timeout: agentDialTimeout,
		Transport: &http.Transport{
			// Loopback only, and never a proxy: a machine with a proxy
			// configured for everything would otherwise send this
			// question out of the building.
			Proxy:       nil,
			DialContext: (&net.Dialer{Timeout: agentDialTimeout}).DialContext,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentDialTimeout)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/v2/health", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&health); err != nil {
		return false
	}
	// A protocol version is what distinguishes this program's answer
	// from any other program that happens to serve 200 on that port.
	return health.ProtocolVersion > 0
}

// handOver gives a launch's request to the agent already running and
// reports whether it got there.
//
// The request is "show your window", which is all `open` has to say:
// documents are `sign --in`, and the Explorer verb has its own path
// through the same inbox.
func handOver() error {
	box := jobs.NewInbox(shellInboxDir())
	if err := box.RequestOpen(); err != nil {
		return fmt.Errorf("handing the request to the running agent: %w", err)
	}
	return nil
}
