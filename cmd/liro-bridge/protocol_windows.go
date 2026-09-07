//go:build windows

package main

// Starting the protocol: the loopback listener, the discovery file, and
// the wiring that connects internal/api's handlers to this program's own
// windows and card (F7 §4).
//
// It runs inside the tray process and nowhere else. The tray is the
// agent — the thing that is running when a person is not looking at a
// window — and a listener in `liro-bridge sign` or `liro-bridge open`
// would be a second agent on the same machine competing for the same
// port and writing over the same discovery file.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// protocolShutdownGrace is how long the listener is given to finish
// what it is doing when the agent quits.
//
// Short, and deliberately so: what an in-flight request is usually
// doing at that moment is holding an event stream open, which ends the
// moment the job does. A batch actually being signed is not on this
// clock at all — it is on the window's, and the window is what the
// person is looking at.
const protocolShutdownGrace = 2 * time.Second

// protocolAgent is the running protocol: a listener, its server, and
// the discovery file that says where to find it.
type protocolAgent struct {
	server     *http.Server
	port       int
	bridgePath string
	cancel     context.CancelFunc
	flow       *api.PairingFlow
}

// startProtocol binds a loopback port, writes bridge.json and starts
// serving. It returns nil, nil when there is nothing to serve with —
// which is a real state (a secret store that would not open) and is
// reported rather than fatal: an agent that cannot serve the protocol
// can still sign for the person sitting at it.
func startProtocol(fallback config.Config, version string, pairings *api.Pairings) (*protocolAgent, error) {
	if pairings == nil {
		return nil, errors.New("there is no pairing store, so nothing could be authenticated")
	}

	cfg := currentConfig(fallback)
	listener, port, err := api.Listen(cfg.PortRangeStart, cfg.PortRangeEnd)
	if err != nil {
		return nil, fmt.Errorf("binding a loopback port: %w", err)
	}

	flow := api.NewPairingFlow(pairings, newPairingUI(fallback), nil, nil)
	nonces := api.NewNonceCache(nil)
	server := api.NewServer(api.Options{
		Pairings:     pairings,
		Flow:         flow,
		Auth:         api.NewAuthenticator(pairings, nonces, nil),
		Jobs:         newJobRegistry(),
		Signer:       newProtocolSigner(fallback),
		AgentVersion: version,
		// Read at the moment a request arrives, not captured here: a
		// person who switches the whole-document path off in Settings
		// must not have to restart the agent for it to take effect
		// (D-134's rule — the file is the authority).
		DocumentSigningEnabled: func() bool { return currentConfig(fallback).DocumentSigningEnabled },
	})

	bridgePath := platform.DefaultBridgeFile()
	if err := api.WriteBridgeFile(bridgePath, api.BridgeInfo{
		Port:            port,
		AgentVersion:    version,
		ProtocolVersion: api.ProtocolVersion,
	}); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("writing the discovery file: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	agent := &protocolAgent{
		server: &http.Server{
			Handler: server.Handler(),
			// A caller may hold an event stream open for as long as a
			// batch takes, and a hundred documents after one PIN is
			// about 46 seconds — plus however long the person took to
			// approve it. So there is no write timeout: the stream ends
			// when the job does. The read timeouts are what bound a
			// caller that opens a socket and says nothing.
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
		port:       port,
		bridgePath: bridgePath,
		cancel:     cancel,
		flow:       flow,
	}

	go agent.serve(listener)
	go server.SweepJobs(ctx)

	slog.Info("protocol: listening", "port", port, "protocolVersion", api.ProtocolVersion,
		"discoveryFile", bridgePath)
	return agent, nil
}

func (a *protocolAgent) serve(listener net.Listener) {
	if err := a.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("protocol: the listener stopped", "error", err)
	}
}

// stop closes the listener and removes the discovery file, so that a
// client reading it finds nothing rather than a port nobody is
// listening on.
func (a *protocolAgent) stop() {
	if a == nil {
		return
	}
	a.cancel()
	// A pairing window still up belongs to a request nobody will ever
	// confirm now. Ending it as a refusal closes it and tells the
	// caller, rather than leaving a window on a desktop whose agent has
	// gone.
	a.flow.Cancel()

	ctx, cancel := context.WithTimeout(context.Background(), protocolShutdownGrace)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		slog.Debug("protocol: the listener did not shut down cleanly", "error", err)
		_ = a.server.Close()
	}
	if err := api.RemoveBridgeFile(a.bridgePath); err != nil {
		slog.Warn("protocol: could not remove the discovery file", "error", err)
	}
	slog.Info("protocol: stopped", "port", a.port)
}
