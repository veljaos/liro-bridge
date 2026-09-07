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
	if err := RemoveBridgeFile(path); err != nil {
		t.Fatalf("removing a file that is not there: %v", err)
	}
	if err := WriteBridgeFile(path, BridgeInfo{Port: 17581}); err != nil {
		t.Fatalf("WriteBridgeFile: %v", err)
	}
	if err := RemoveBridgeFile(path); err != nil {
		t.Fatalf("RemoveBridgeFile: %v", err)
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
