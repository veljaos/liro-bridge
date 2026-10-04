package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// logLines runs f with the default logger writing JSON to a buffer, and
// returns each line it wrote.
func logLines(t *testing.T, f func()) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)
	f()
	var lines []map[string]any
	for _, l := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var line map[string]any
		if err := json.Unmarshal(l, &line); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		lines = append(lines, line)
	}
	return lines
}

// TestTheQuitLineSaysWhatFiredItAndClaimsNoPerson is D34: at 21:36:47 on
// the Ubuntu VM the log said "the person quit the agent from its own
// window" and no person had (D-425). The line names what fired Izađi, as
// the page saw it, and does not say a person did.
func TestTheQuitLineSaysWhatFiredItAndClaimsNoPerson(t *testing.T) {
	for _, tc := range []struct {
		trigger ui.Trigger
		want    string
	}{
		{ui.TriggerKey, "key"},
		{ui.TriggerNeither, "neither-pointer-nor-key"},
		{"", "not-reported"},
	} {
		m := newMainWindow(config.Default(), "en")
		quit := false
		m.quitAgent = func() { quit = true }
		m.messages <- ui.Message{Type: ui.MessageTypeQuit, Trigger: tc.trigger}

		lines := logLines(t, func() {
			done := make(chan struct{})
			go func() { m.loop(context.Background()); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the window did not stop on a quit")
			}
		})
		if !quit {
			t.Errorf("%s: the agent was not asked to quit", tc.want)
		}
		if len(lines) != 1 {
			t.Fatalf("%s: %d log lines, want 1: %v", tc.want, len(lines), lines)
		}
		msg, _ := lines[0]["msg"].(string)
		if strings.Contains(msg, "person") {
			t.Errorf("the quit line says a person did it, which the page cannot see: %q", msg)
		}
		if lines[0]["trigger"] != tc.want {
			t.Errorf("the quit line's trigger is %v, want %s", lines[0]["trigger"], tc.want)
		}
	}
}

// TestSettingsSaysHowItWasClosed is D34's other half: Settings' log line
// for a close was "the page sent cancel" both for Zatvori and for the
// window closed from outside (D-425). Now they differ, and Zatvori's says
// what fired it.
func TestSettingsSaysHowItWasClosed(t *testing.T) {
	lines := logLines(t, func() {
		logSettingsMessage(settingsClosedFromOutside)
		logSettingsMessage(ui.Message{Type: ui.MessageTypeCancel, Trigger: ui.TriggerPointer})
	})
	if len(lines) != 2 {
		t.Fatalf("%d log lines, want 2: %v", len(lines), lines)
	}
	outside, _ := lines[0]["msg"].(string)
	if !strings.Contains(outside, "closed from outside") {
		t.Errorf("a window closed from outside logs %q", outside)
	}
	zatvori, _ := lines[1]["msg"].(string)
	if !strings.Contains(zatvori, "Zatvori") || lines[1]["trigger"] != "pointer" {
		t.Errorf("Zatvori logs %q with trigger %v, want Zatvori and pointer", zatvori, lines[1]["trigger"])
	}
	if outside == zatvori {
		t.Errorf("both closes log the same line, %q", outside)
	}
}
