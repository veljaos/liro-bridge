package ui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestTheTriggerSaysWhatFiredTheButton is D34's rule: what the page saw
// of a click becomes a pointer, a key, neither, or the page's own script
// — never "a person", which none of them shows (D-425).
func TestTheTriggerSaysWhatFiredTheButton(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Message
	}{
		{"quit reporting nothing", `{"type":"quit"}`, Message{Type: MessageTypeQuit}},
		{"a mouse click", `{"type":"quit","trigger":{"detail":1,"trusted":true,"byKey":false}}`, Message{Type: MessageTypeQuit, Trigger: TriggerPointer}},
		{"a double click with a key down is still a pointer", `{"type":"quit","trigger":{"detail":2,"trusted":true,"byKey":true}}`, Message{Type: MessageTypeQuit, Trigger: TriggerPointer}},
		{"Enter or Space on the button", `{"type":"quit","trigger":{"detail":0,"trusted":true,"byKey":true}}`, Message{Type: MessageTypeQuit, Trigger: TriggerKey}},
		{"no count and no key: an accessibility press or something else", `{"type":"quit","trigger":{"detail":0,"trusted":true,"byKey":false}}`, Message{Type: MessageTypeQuit, Trigger: TriggerNeither}},
		{"an untrusted click is the page's script whatever it claims", `{"type":"quit","trigger":{"detail":1,"trusted":false,"byKey":true}}`, Message{Type: MessageTypeQuit, Trigger: TriggerScript}},
		{"Zatvori's cancel carries it too", `{"type":"cancel","trigger":{"detail":0,"trusted":true,"byKey":true}}`, Message{Type: MessageTypeCancel, Trigger: TriggerKey}},
		// The page cannot say a window was closed from outside: only Go
		// sets that, and a trigger in a shape not read is named so, with
		// the cancel itself still taken.
		{"the page cannot claim closed-from-outside", `{"type":"cancel","trigger":"closed-from-outside"}`, Message{Type: MessageTypeCancel, Trigger: TriggerUnreadable}},
		{"approve carries no trigger", `{"type":"approve","trigger":{"detail":1,"trusted":true}}`, Message{Type: MessageTypeApprove}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseMessage([]byte(tc.raw))
			if !ok {
				t.Fatalf("ParseMessage(%s) was rejected", tc.raw)
			}
			if got != tc.want {
				t.Errorf("ParseMessage(%s) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestAnUnreportedTriggerIsNamedInTheLog: a line for a message that
// reported nothing says "not-reported", not a blank a reader fills in.
func TestAnUnreportedTriggerIsNamedInTheLog(t *testing.T) {
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "trigger", Message{}.Trigger)
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line["trigger"] != "not-reported" {
		t.Errorf("an empty trigger logs as %q, want not-reported", line["trigger"])
	}
}

// TestIzadjiAndZatvoriSendWhatFiredThem checks the pages: the two buttons
// whose log lines D-425 could not trust send the click's facts, and the
// bridge watches the two keys a button turns into a click.
func TestIzadjiAndZatvoriSendWhatFiredThem(t *testing.T) {
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	bridge := read("assets/bridge.js")
	i := strings.Index(bridge, "window.liroClickFacts = function")
	if i < 0 {
		t.Fatal("bridge.js defines no liroClickFacts")
	}
	body := bridge[i:]
	body = body[:strings.Index(body, "\n  };")]
	for _, want := range []string{
		`ev.type === "keypress" && ev.key === "Enter"`,
		`ev.type === "keyup" && ev.key === " "`,
		"detail: ev.detail", "trusted: ev.isTrusted", "byKey: byKey",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liroClickFacts lacks %s", want)
		}
	}

	for page, send := range map[string]string{
		"assets/pages/main.js":     `window.liroSend("quit", { trigger: facts(ev) })`,
		"assets/pages/settings.js": `window.liroSend("cancel", { trigger: closeFacts(ev) })`,
	} {
		if !strings.Contains(read(page), send) {
			t.Errorf("%s does not send what fired its button: no %s", page, send)
		}
	}
}
