//go:build windows

package main

// What a hundred documents cost through the protocol, against the same
// hundred signed the way a person signs them (F7's own report ask).
//
// Both halves run through the same jobs.Runner, the same session and
// the same PAdES engine, at the same level. The only difference is the
// path: one reads each document from disk and writes the signature
// beside it, the other holds the documents in memory and hands the
// signatures back. So what this measures is exactly that difference —
// not the card, which is the same for both, and not the network, which
// there is none of.
//
// It is behind an environment variable because it signs three hundred
// documents and takes about a minute, which does not belong in every
// `go test ./...` — the same arrangement D-169 made for the
// window-cycle measurement, and for the same reason: a measurement
// nobody can repeat is a number rather than a check (D-172).
//
//	LIRO_PROTOCOL_TIMING=1 go test ./cmd/liro-bridge/ -run TestAHundredDocuments -v

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/pades"
)

func TestAHundredDocumentsThroughTheProtocolAndLocally(t *testing.T) {
	if os.Getenv("LIRO_PROTOCOL_TIMING") == "" {
		t.Skip("set LIRO_PROTOCOL_TIMING=1 to measure a hundred documents each way")
	}
	const n = 100

	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	session := newStampSession(t)

	// ---- locally: files in, files out ----
	dir, paths := batchFixtures(t, n)
	local := newMainWindow(config.Default(), "sr-Latn")
	local.cfg.VisibleStamp = false
	local.auditStore = tempAuditStore(t)
	local.win = &recordingWindow{}
	if added, _ := local.queue.Add(paths); added != n {
		t.Fatalf("the queue took %d of %d documents", added, n)
	}
	local.readInputs()
	outDir := t.TempDir()
	outputs := make([]interactiveOutput, 0, n)
	for _, it := range local.queue.Items() {
		outputs = append(outputs, interactiveOutput{path: jobs.OutputPathFor(it.Path, outDir, local.cfg.OutputSuffix)})
	}
	localStart := time.Now()
	local.runBatch(context.Background(), consentDecision{
		approved: true, thumbprint: "AABB", session: session,
		level: pades.LevelBB, allowBB: true, outputs: outputs, cfg: local.cfg,
	})
	localElapsed := time.Since(localStart)
	if local.report.Succeeded != n {
		t.Fatalf("locally: %d of %d signed", local.report.Succeeded, n)
	}
	_ = dir

	// ---- the protocol, whole documents: bytes in, bytes out ----
	docReq := api.SignRequest{Application: "Knjigovodstvo doo", Kind: api.SignDocuments}
	for i := 0; i < n; i++ {
		docReq.Documents = append(docReq.Documents, api.Document{Name: "ugovor.pdf", Content: blank})
		sum := sha256.Sum256(blank)
		docReq.Digests = append(docReq.Digests, sum[:])
		docReq.Labels = append(docReq.Labels, "ugovor.pdf")
	}
	remote := protocolWindowFor(t, docReq)
	remote.cfg.VisibleStamp = false
	remote.auditStore = tempAuditStore(t)
	remoteStart := time.Now()
	remote.runBatch(context.Background(), consentDecision{
		approved: true, thumbprint: "AABB", session: session,
		level: pades.LevelBB, allowBB: true, cfg: remote.cfg,
	})
	remoteElapsed := time.Since(remoteStart)
	if got := remote.remote.result().Signed(); got != n {
		t.Fatalf("over the protocol: %d of %d signed", got, n)
	}

	// ---- the protocol, hashes: the agent never sees a document ----
	hashReq := api.SignRequest{Application: "Knjigovodstvo doo", Kind: api.SignDigests, Thumbprint: "AABB"}
	for i := 0; i < n; i++ {
		sum := sha256.Sum256([]byte{byte(i), byte(i >> 8)})
		hashReq.Digests = append(hashReq.Digests, sum[:])
		hashReq.Labels = append(hashReq.Labels, "ugovor.pdf")
	}
	hashes := protocolWindowFor(t, hashReq)
	hashes.auditStore = tempAuditStore(t)
	hashStart := time.Now()
	hashes.runBatch(context.Background(), consentDecision{
		approved: true, thumbprint: "AABB", session: session,
		level: pades.LevelBB, allowBB: true, cfg: hashes.cfg,
	})
	hashElapsed := time.Since(hashStart)
	if got := hashes.remote.result().Signed(); got != n {
		t.Fatalf("over the protocol (hashes): %d of %d signed", got, n)
	}

	t.Logf("%d documents, one session, level B-B, no timestamp authority:", n)
	t.Logf("  locally, read and written as files:  %s  (%s each)", localElapsed.Round(time.Millisecond), (localElapsed / n).Round(time.Microsecond))
	t.Logf("  over the protocol, whole documents:  %s  (%s each)", remoteElapsed.Round(time.Millisecond), (remoteElapsed / n).Round(time.Microsecond))
	t.Logf("  over the protocol, hashes only:      %s  (%s each)", hashElapsed.Round(time.Millisecond), (hashElapsed / n).Round(time.Microsecond))
	t.Logf("  measured first signature: local %s, protocol %s",
		local.report.Timing.FirstSignature.Round(time.Microsecond),
		remote.report.Timing.FirstSignature.Round(time.Microsecond))

}
