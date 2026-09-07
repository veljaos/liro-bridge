package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPairingsRoundTripThroughDisk(t *testing.T) {
	clock := newFakeClock()
	secrets := newMemSecretStore()
	path := filepath.Join(t.TempDir(), "pairings.json")

	first, err := OpenPairings(path, secrets, clock.Now)
	if err != nil {
		t.Fatalf("OpenPairings: %v", err)
	}
	app, secret, err := first.Add("My ERP", "https://erp.example.com")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	second, err := OpenPairings(path, secrets, clock.Now)
	if err != nil {
		t.Fatalf("re-opening: %v", err)
	}
	got, ok := second.Get(app.AppID)
	if !ok {
		t.Fatal("the pairing did not survive re-opening")
	}
	if got.Name != "My ERP" || got.Origin != "https://erp.example.com" {
		t.Fatalf("the pairing came back as %+v", got)
	}
	back, ok := second.Secret(app.AppID)
	if !ok || !bytes.Equal(back, secret) {
		t.Fatal("the device secret did not survive re-opening")
	}
}

// The metadata file is what Settings reads and what a person may look
// at. It must not carry a secret.
func TestThePairingFileHoldsNoSecret(t *testing.T) {
	clock := newFakeClock()
	secrets := newMemSecretStore()
	path := filepath.Join(t.TempDir(), "pairings.json")
	p, err := OpenPairings(path, secrets, clock.Now)
	if err != nil {
		t.Fatalf("OpenPairings: %v", err)
	}
	_, secret, err := p.Add("My ERP", "https://erp.example.com")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the pairing file: %v", err)
	}
	if bytes.Contains(raw, secret) {
		t.Fatal("the pairing file contains the device secret verbatim")
	}
	if bytes.Contains(raw, []byte(EncodeDeviceSecret(secret))) {
		t.Fatal("the pairing file contains the device secret, base64-encoded")
	}

	// And nothing shaped like a secret is in the parsed structure
	// either: the type has nowhere to put one.
	var f pairingsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("the pairing file is not valid JSON: %v", err)
	}
	if len(f.Pairings) != 1 {
		t.Fatalf("the file holds %d pairings, want 1", len(f.Pairings))
	}
}

func TestRevokeRemovesBothTheMetadataAndTheSecret(t *testing.T) {
	h := newHarness(t)
	app, _ := h.pair("My ERP", "https://erp.example.com")

	if h.secrets.count() != 1 {
		t.Fatalf("%d secrets stored after pairing, want 1", h.secrets.count())
	}
	if err := h.pairings.Revoke(app.AppID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := h.pairings.Get(app.AppID); ok {
		t.Fatal("the pairing is still listed after being revoked")
	}
	if _, ok := h.pairings.Secret(app.AppID); ok {
		t.Fatal("the device secret is still readable after the pairing was revoked")
	}
	if h.secrets.count() != 0 {
		t.Fatalf("%d secrets remain in the store after revocation", h.secrets.count())
	}
	if err := h.pairings.Revoke(app.AppID); !errors.Is(err, ErrNoSuchPairing) {
		t.Fatalf("revoking twice returned %v, want ErrNoSuchPairing", err)
	}
}

// Of the two half-finished states Add could leave behind, the one that
// matters is a pairing a person can see and nothing can authenticate
// against. It must not happen.
func TestAddLeavesNothingBehindWhenTheSecretCannotBeStored(t *testing.T) {
	clock := newFakeClock()
	secrets := newMemSecretStore()
	secrets.setErr = errSetFailed
	p, err := OpenPairings(filepath.Join(t.TempDir(), "pairings.json"), secrets, clock.Now)
	if err != nil {
		t.Fatalf("OpenPairings: %v", err)
	}

	if _, _, err := p.Add("My ERP", "https://erp.example.com"); err == nil {
		t.Fatal("Add succeeded with an unwritable secret store")
	}
	if got := len(p.List()); got != 0 {
		t.Fatalf("%d pairings exist after a failed Add, want 0", got)
	}
}

// Metadata without a secret is not a pairing anything can be
// authenticated against, and must not read as one.
func TestAPairingWithNoStoredSecretIsNotUsable(t *testing.T) {
	h := newHarness(t)
	app, _ := h.pair("My ERP", "https://erp.example.com")

	if err := h.secrets.Delete(secretName(app.AppID)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := h.pairings.Secret(app.AppID); ok {
		t.Fatal("a pairing whose secret is gone still reports one")
	}
}

func TestListIsNewestFirst(t *testing.T) {
	h := newHarness(t)

	h.pair("First", "https://first.example.com")
	h.clock.Advance(time.Minute)
	h.pair("Second", "https://second.example.com")

	list := h.pairings.List()
	if len(list) != 2 {
		t.Fatalf("%d pairings, want 2", len(list))
	}
	if list[0].Name != "Second" {
		t.Fatalf("the list is %q first; it should be newest first", list[0].Name)
	}
}

// Every authenticated request updates "last used", and a
// hundred-document batch is a hundred requests. Writing the file each
// time would be a hundred rewrites of something only a settings window
// reads.
func TestLastUsedIsPersistedNoMoreThanOncePerMinute(t *testing.T) {
	h := newHarness(t)
	app, _ := h.pair("My ERP", "https://erp.example.com")
	path := h.pairings.path

	h.clock.Advance(lastUsedGranularity)
	h.pairings.Touch(app.AppID, h.clock.Now())
	afterFirst := mustReadFile(t, path)

	// Nine more requests inside the same minute.
	for range 9 {
		h.clock.Advance(time.Second)
		h.pairings.Touch(app.AppID, h.clock.Now())
	}
	if !bytes.Equal(afterFirst, mustReadFile(t, path)) {
		t.Fatal("the pairing file was rewritten for a request inside the same minute")
	}

	// In memory, though, it is always current: a settings window opened
	// now shows the truth.
	got, _ := h.pairings.Get(app.AppID)
	if !got.LastUsedAt.Equal(h.clock.Now()) {
		t.Fatalf("last used in memory is %v, want %v", got.LastUsedAt, h.clock.Now())
	}

	h.clock.Advance(lastUsedGranularity)
	h.pairings.Touch(app.AppID, h.clock.Now())
	if bytes.Equal(afterFirst, mustReadFile(t, path)) {
		t.Fatal("the pairing file was not rewritten after a minute had passed")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

func TestDeviceSecretEncodingRoundTrips(t *testing.T) {
	h := newHarness(t)
	_, secret := h.pair("My ERP", "https://erp.example.com")

	back, err := DecodeDeviceSecret(EncodeDeviceSecret(secret))
	if err != nil {
		t.Fatalf("DecodeDeviceSecret: %v", err)
	}
	if !bytes.Equal(back, secret) {
		t.Fatal("the device secret did not survive its own encoding")
	}
}

func TestEveryPairingGetsItsOwnIdentifierAndSecret(t *testing.T) {
	h := newHarness(t)

	seenID := map[string]bool{}
	seenSecret := map[string]bool{}
	for range 20 {
		app, secret := h.pair("My ERP", "https://erp.example.com")
		if seenID[app.AppID] {
			t.Fatalf("application identifier %q was issued twice", app.AppID)
		}
		if seenSecret[string(secret)] {
			t.Fatal("the same device secret was issued twice")
		}
		seenID[app.AppID] = true
		seenSecret[string(secret)] = true
		// Past the per-origin rate window, so this test measures what
		// it is about rather than F7 §10's limit.
		h.clock.Advance(pairingRateWindow)
	}
}
