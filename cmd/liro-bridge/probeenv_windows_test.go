//go:build windows

package main

// TEMPORARY measurement harness. Not part of the suite's contract: it
// asserts nothing, it only reports. Delete before the phase's commit.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

func probef(format string, args ...any) {
	fmt.Printf("PROBE "+format+"\n", args...)
}

func TestProbeCertificatePath(t *testing.T) {
	tempConfigHome(t)
	ctx := context.Background()

	start := time.Now()
	svc := platform.NewSmartCardService()
	readers, err := svc.Readers(ctx)
	probef("readers elapsed=%s n=%d err=%v", time.Since(start).Round(time.Millisecond), len(readers), err)
	for _, r := range readers {
		probef("reader name=%q present=%v", r.Name, r.CardPresent)
	}

	start = time.Now()
	certs, err := windowscng.Enumerate(ctx)
	probef("enumerate elapsed=%s n=%d err=%v", time.Since(start).Round(time.Millisecond), len(certs), err)

	cachePath := filepath.Join(filepath.Dir(platform.DefaultConfigFile()), "tsl-cache.xml")
	start = time.Now()
	store, err := tsl.NewFileStore(cachePath, tsl.DefaultURL, tsl.HTTPFetcher)
	probef("tsl-newfilestore elapsed=%s err=%v", time.Since(start).Round(time.Millisecond), err)
	if err == nil {
		start = time.Now()
		refreshErr := store.Refresh(ctx)
		probef("tsl-refresh elapsed=%s err=%v", time.Since(start).Round(time.Millisecond), refreshErr)
		start = time.Now()
		list, prov, curErr := store.Current(ctx)
		probef("tsl-current elapsed=%s list-nil=%v source=%v err=%v", time.Since(start).Round(time.Millisecond), list == nil, prov.Source, curErr)
	}

	start = time.Now()
	report, gatherErr := gatherInteractiveCertificates(ctx)
	probef("gather elapsed=%s rows=%d visible=%d err=%v",
		time.Since(start).Round(time.Millisecond), len(report.Certificates), len(visibleCertificates(report)), gatherErr)
	for _, row := range report.Certificates {
		probef("row subject=%q hardware=%v usable=%v reason=%v hidden=%v", row.Info.Subject.CommonName, row.OnHardware, row.Info.Usable, row.Info.NotUsableReason, row.Hidden())
	}
	probef("softtoken-env p12=%q", os.Getenv("LIRO_SOFTTOKEN_P12"))
	_ = cli.Report{}
}

func TestProbeSignCommand(t *testing.T) {
	tempConfigHome(t)

	dir := t.TempDir()
	in := filepath.Join(dir, "ugovor.pdf")
	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, blank, 0o600); err != nil {
		t.Fatal(err)
	}

	const title = "Liro Bridge"
	before := liroWindowsTitled(t, title)

	out := &probeWriter{}
	start := time.Now()
	done := make(chan int, 1)
	go func() { done <- run([]string{"sign", "--in", in}, out) }()

	var appeared time.Duration
	var hwnd uintptr
	deadline := time.Now().Add(30 * time.Second)
	var code int
	var returned time.Duration = -1
	for time.Now().Before(deadline) {
		if hwnd == 0 {
			for h := range liroWindowsTitled(t, title) {
				if before[h] {
					continue
				}
				if visible, _, _ := procIsWindowVisibleT.Call(h); visible != 0 {
					hwnd = h
					appeared = time.Since(start)
				}
			}
		}
		select {
		case code = <-done:
			returned = time.Since(start)
		default:
		}
		if hwnd != 0 || returned >= 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	probef("sign window-appeared=%v after=%s", hwnd != 0, appeared.Round(time.Millisecond))
	if hwnd != 0 {
		const wmClose = 0x0010
		_, _, _ = procPostMessageT.Call(hwnd, wmClose, 0, 0)
		select {
		case code = <-done:
			returned = time.Since(start)
		case <-time.After(60 * time.Second):
		}
	} else if returned < 0 {
		select {
		case code = <-done:
			returned = time.Since(start)
		case <-time.After(60 * time.Second):
		}
	}
	probef("sign returned=%s code=%d stdout=%q", returned.Round(time.Millisecond), code, out.String())

	// The agent's own log. run() calls config.SetupLogging, which
	// slog.SetDefault's a JSON handler over a file under LOCALAPPDATA —
	// so every slog line the flow writes goes there and NOT to the test
	// output, which is why the CI log says nothing about why no window
	// appeared.
	logDir := platform.DefaultLogDir()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		probef("logdir %q unreadable: %v", logDir, err)
		return
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			probef("log %s", strings.TrimSpace(line))
		}
	}
}

type probeWriter struct{ b []byte }

func (w *probeWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *probeWriter) String() string              { return string(w.b) }

var _ io.Writer = (*probeWriter)(nil)
