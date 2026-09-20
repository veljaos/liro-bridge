package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/platform"
)

func TestBridgeFileRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "bridge.json")
	want := BridgeInfo{Port: 17583, AgentVersion: "1.2.3", ProtocolVersion: ProtocolVersion}
	if err := WriteBridgeFile(path, want); err != nil {
		t.Fatalf("WriteBridgeFile: %v", err)
	}
	got, err := ReadBridgeFile(path)
	if err != nil {
		t.Fatalf("ReadBridgeFile: %v", err)
	}
	if got != want {
		t.Fatalf("read %+v, wrote %+v", got, want)
	}
}

// TestBridgeFileCarriesExactlyWhatF7Asks pins the file's shape rather
// than its round trip: an SDK reads this file, so a field appearing or
// disappearing is a change to the protocol, not to this package.
func TestBridgeFileCarriesExactlyWhatF7Asks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.json")
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17580, AgentVersion: "9.9.9", ProtocolVersion: 2}); err != nil {
		t.Fatalf("WriteBridgeFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("the discovery file is not JSON: %v\n%s", err, raw)
	}
	want := map[string]bool{"port": true, "agentVersion": true, "protocolVersion": true}
	for k := range fields {
		if !want[k] {
			t.Errorf("the discovery file carries an unexpected field %q", k)
		}
	}
	for k := range want {
		if _, ok := fields[k]; !ok {
			t.Errorf("the discovery file is missing %q", k)
		}
	}
	// It says where the agent is and nothing about the person at it.
	for _, forbidden := range []string{"user", "name", "path", "pid", "certificate"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Errorf("the discovery file mentions %q:\n%s", forbidden, raw)
		}
	}
}

func TestRemoveBridgeFileIsSilentWhenThereIsNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.json")
	removed, err := RemoveBridgeFile(path, 17581)
	if err != nil {
		t.Fatalf("removing a file that is not there: %v", err)
	}
	if removed {
		t.Fatal("it reported removing a file that was never there")
	}
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17581}); err != nil {
		t.Fatalf("WriteBridgeFile: %v", err)
	}
	removed, err = RemoveBridgeFile(path, 17581)
	if err != nil {
		t.Fatalf("RemoveBridgeFile: %v", err)
	}
	if !removed {
		t.Fatal("it did not report removing its own file")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the discovery file is still there: %v", err)
	}
}

// TestTwoSessionsHaveTwoDiscoveryFiles is SPEC §14.1 at the level this
// package can reach: the file's location comes from the environment
// that differs between two signed-in users, so two sessions cannot
// find each other's agent by reading it.
//
// The other half — that they cannot find each other by scanning either
// — is what the per-user pairing store and the per-user secret store
// enforce, and is checked where those live.
func TestTwoSessionsHaveTwoDiscoveryFiles(t *testing.T) {
	ana := func(key string) string {
		if key == "LOCALAPPDATA" {
			return `C:\Users\ana\AppData\Local`
		}
		return ""
	}
	bojan := func(key string) string {
		if key == "LOCALAPPDATA" {
			return `C:\Users\bojan\AppData\Local`
		}
		return ""
	}
	anaPath := platform.BridgeFile("windows", ana)
	bojanPath := platform.BridgeFile("windows", bojan)
	if anaPath == bojanPath {
		t.Fatalf("two users share one discovery file: %s", anaPath)
	}
	if !strings.HasSuffix(anaPath, filepath.Join("Liro", "bridge.json")) {
		t.Fatalf("the discovery file is at %s, not beside config.json", anaPath)
	}
}

// TestAnAgentNeverRemovesAnotherAgentsDiscoveryFile is D-323's measured
// sequence, committed. Two agents in one session: the second starts,
// takes the file, and exits — and the first must still be findable.
//
// Written as the sequence rather than as a call to RemoveBridgeFile with
// a mismatched port, because the sequence is what was observed on a real
// machine and a mismatched port is only the mechanism. It runs against
// the exported functions the agent itself calls.
func TestAnAgentNeverRemovesAnotherAgentsDiscoveryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "liro", "bridge.json")

	// Agent A starts and claims discovery.
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17580, AgentVersion: "A", ProtocolVersion: ProtocolVersion}); err != nil {
		t.Fatalf("agent A writing the discovery file: %v", err)
	}
	// Agent B starts in the same session and overwrites it. The write is
	// deliberately unconditional — see RemoveBridgeFile's own comment for
	// why making it conditional without single-instance would be the same
	// defect with the roles swapped.
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17581, AgentVersion: "B", ProtocolVersion: ProtocolVersion}); err != nil {
		t.Fatalf("agent B writing the discovery file: %v", err)
	}
	// Agent B stops.
	removed, err := RemoveBridgeFile(path, 17581)
	if err != nil {
		t.Fatalf("agent B removing the discovery file: %v", err)
	}
	if !removed {
		t.Fatal("agent B did not remove the file it had just written")
	}
	// Agent A is still running. It stops, and must not have destroyed
	// anything — but more to the point, while it was running there was
	// nothing of its own left for it to be found by, which is the state
	// this rule cannot fix and single-instance has to.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("agent B left its own file behind: %v", err)
	}

	// Now the direction the rule does fix: agent A stops last.
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17580, AgentVersion: "A", ProtocolVersion: ProtocolVersion}); err != nil {
		t.Fatalf("agent A writing the discovery file: %v", err)
	}
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17581, AgentVersion: "B", ProtocolVersion: ProtocolVersion}); err != nil {
		t.Fatalf("agent B writing the discovery file: %v", err)
	}
	// Agent A stops first. The file describes B, so A must leave it.
	removed, err = RemoveBridgeFile(path, 17580)
	if err != nil {
		t.Fatalf("agent A removing the discovery file: %v", err)
	}
	if removed {
		t.Fatal("agent A removed a discovery file naming agent B's port; " +
			"agent B is still running and is now undiscoverable (D-323)")
	}
	info, err := ReadBridgeFile(path)
	if err != nil {
		t.Fatalf("the discovery file is gone after agent A stopped: %v", err)
	}
	if info.Port != 17581 {
		t.Fatalf("the discovery file names port %d, want agent B's 17581", info.Port)
	}
}

// TestADiscoveryFileThatCannotBeReadIsLeftAlone pins the third case, which
// is a decision rather than a consequence: an agent that removed what it
// could not identify would be back to unconditional removal.
func TestADiscoveryFileThatCannotBeReadIsLeftAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveBridgeFile(path, 17580)
	if err == nil {
		t.Fatal("a discovery file that cannot be read was reported as fine")
	}
	if removed {
		t.Fatal("a discovery file that cannot be read was removed anyway")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the unreadable discovery file was removed: %v", err)
	}
}
