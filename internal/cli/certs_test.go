package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

func TestRunCertsTextOutput(t *testing.T) {
	der := loadDER(t, "mup_signing.der")
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}
	deps := fakeDeps(t, nil, true, store)
	_ = der

	var out bytes.Buffer
	code := RunCerts(context.Background(), nil, &out, "en", deps)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "Trusted List:") {
		t.Fatalf("expected Trusted List heading, got: %s", out.String())
	}
}

func TestRunCertsJSONFlag(t *testing.T) {
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}
	deps := fakeDeps(t, nil, false, store)

	var out bytes.Buffer
	code := RunCerts(context.Background(), []string{"--json"}, &out, "en", deps)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("--json output does not look like JSON: %s", out.String())
	}
}

func TestRunCertsUnknownFlagFails(t *testing.T) {
	store := &fakeStore{list: bundledTSLList(t)}
	deps := fakeDeps(t, nil, false, store)

	var out bytes.Buffer
	code := RunCerts(context.Background(), []string{"--not-a-real-flag"}, &out, "en", deps)
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero for an unrecognised flag")
	}
}
