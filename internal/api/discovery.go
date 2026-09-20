package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// ProtocolVersion is the version of this protocol, the "2" in every
// path. It changes only when a caller written against the old one would
// break; adding an endpoint or a field does not change it.
const ProtocolVersion = 2

// MinimumClientVersion is the oldest SDK version this agent will serve
// (SPEC §15.2, F7 §4.3). An SDK reads it from /v2/health and tells the
// person to update rather than failing obscurely later.
//
// It is "0.0.0" while no SDK exists (F8 builds the first one): a
// minimum that refused something would be refusing a client this
// project has not written yet, and the point of carrying the number at
// all is that it can be raised later without inventing the mechanism
// then.
const MinimumClientVersion = "0.0.0"

// BridgeInfo is the content of bridge.json — the discovery file SDKs
// read to find the agent (SPEC §14, F7 §4.1).
//
// Three fields and no more. The minimum client version deliberately is
// not here even though an SDK wants it: it is what /v2/health returns,
// and a fact stated in two places is a fact that can disagree with
// itself — this project has recorded that failure for a rule (D-108),
// for a question (D-124) and for a margin (D-138). The file's job is
// to say where the agent is; everything else is one request away once
// you know.
type BridgeInfo struct {
	Port            int    `json:"port"`
	AgentVersion    string `json:"agentVersion"`
	ProtocolVersion int    `json:"protocolVersion"`
}

// WriteBridgeFile writes info to path, creating the directory if it is
// not there.
//
// The write is atomic (platform.WriteFileAtomic, J-8): an SDK polling
// for the agent must never read a half-written file and conclude the
// port is 1758.
func WriteBridgeFile(path string, info BridgeInfo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("api: creating the directory for %s: %w", filepath.Base(path), err)
	}
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := platform.WriteFileAtomic(path, b, 0o600); err != nil {
		return fmt.Errorf("api: writing the discovery file: %w", err)
	}
	return nil
}

// ReadBridgeFile reads a discovery file. It exists so that this
// project's own tests, and a future Go SDK, read the file the same way
// rather than each having their own idea of its shape.
func ReadBridgeFile(path string) (BridgeInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return BridgeInfo{}, err
	}
	var info BridgeInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return BridgeInfo{}, fmt.Errorf("api: the discovery file is not valid JSON: %w", err)
	}
	return info, nil
}

// RemoveBridgeFile deletes the discovery file, but only if the file
// names port. The agent calls it on the way out with its own port, so
// that a client reading the file finds nothing rather than a port
// nobody is listening on. It reports whether it removed anything.
//
// The port is the whole of the ownership test, and no field was added
// to carry one: an agent knows which port it bound, and the file
// already says which port it describes. D-186 declined a pid field
// because it is one more thing an unauthenticated reader learns about
// the machine, and that reasoning is untouched here — what makes the
// test possible is a value the file has carried since F7.
//
// Why conditional, measured rather than argued. Both ends of this
// file's life used to be unconditional, and D-323 measured what that
// produces on a real machine: a second agent starts, overwrites the
// file with its own port, exits, removes it — and the first agent is
// left running, holding a port, and undiscoverable by the only
// mechanism SPEC §14 permits, since an SDK must never scan ports. The
// benign direction was already handled and is unchanged; this is the
// harmful one.
//
// The write stays unconditional, and that is deliberate rather than
// half a fix. Refusing to overwrite a live agent's file without a
// single-instance mechanism to hand over to would produce the same
// defect with the roles swapped: a second agent that starts, cannot
// claim discovery, and runs invisibly. The write is right to be
// unconditional until one agent per session is enforced (SPEC §14.1);
// the removal was wrong either way.
//
// A file that is already gone is not an error: two agents shutting down
// together, or a user who deleted it, are both states this should end
// in silently.
//
// A file that cannot be read is left alone and reported. An agent that
// removed what it could not identify would be back to unconditional
// removal with extra steps, which is the behaviour being taken out.
func RemoveBridgeFile(path string, port int) (removed bool, err error) {
	info, err := ReadBridgeFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	case info.Port != port:
		return false, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return true, nil
}
