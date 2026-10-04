package ui

import (
	"encoding/json"
	"log/slog"
)

// MessageType is one of the four things the page is allowed to ask for
// (F5 §2.4): approve, cancel, select a certificate by thumbprint, and
// quit. Anything else is dropped and logged — the message surface is
// kept this small deliberately.
//
// **It was three until F12 §6, and the fourth was added with its
// reasoning rather than by widening a list.** A stock GNOME desktop
// draws no tray (D-342), so on that desktop the tray's Quit item does
// not exist and the agent's own window is the only thing a person can
// reach it through. The alternative was leaving somebody with a running
// agent and no way to stop it that does not involve a terminal.
//
// What it costs, stated rather than waved past: a page that could be
// made to send "quit" could stop the agent. That is a denial of
// service and not a signature — and the same page can already send
// "approve", which *is* the consent gate (SPEC §6.5). A surface that
// already carries the decision worth attacking is not made meaningfully
// worse by carrying the one that stops the program; the reason to keep
// the list short is that every entry has to be justified, and this one
// is.
type MessageType string

const (
	MessageTypeApprove           MessageType = "approve"
	MessageTypeCancel            MessageType = "cancel"
	MessageTypeSelectCertificate MessageType = "selectCertificate"
	MessageTypeQuit              MessageType = "quit"
)

// Message is one validated message from the page.
type Message struct {
	Type MessageType

	// Thumbprint is set only for MessageTypeSelectCertificate.
	Thumbprint string

	// Trigger says what fired the button that sent a quit or a cancel,
	// as far as the page could see it; empty when nothing was reported.
	Trigger Trigger
}

// Trigger is what made a button send its message (D34). A log line that
// says "the person quit" was written at 21:36:47 on the Ubuntu VM when no
// person had (D-425): the page's click listener runs alike for a mouse,
// a key and an accessibility action, so the line asserted what the code
// could not know. The page now reports what it saw and this says which.
type Trigger string

const (
	// TriggerPointer is a click with a click count: a mouse, a touchpad
	// or a touch. WebKit gives every click it simulates a detail of 0
	// (SimulatedClick.cpp, 2.52.6), so a count of one or more is a
	// pointer's.
	TriggerPointer Trigger = "pointer"
	// TriggerKey is a simulated click right after Enter or Space reached
	// the button — the two keys a button turns into a click
	// (HTMLButtonElement.cpp).
	TriggerKey Trigger = "key"
	// TriggerNeither is a click with no count and no key before it: an
	// accessibility action ("press", AccessibilityObject.cpp), an access
	// key, or something not yet known. It is not a person's press, and
	// it is not ruled out that one was behind it.
	TriggerNeither Trigger = "neither-pointer-nor-key"
	// TriggerScript is a click the page's own script made (an untrusted
	// event). Nothing in our pages does that.
	TriggerScript Trigger = "script"
	// TriggerClosedFromOutside is set by Go, not the page: the window was
	// closed by GTK's or Windows' close request — its title bar, a key the
	// desktop handles, or the desktop itself — and the page's own button
	// was not involved.
	TriggerClosedFromOutside Trigger = "closed-from-outside"
	// TriggerUnreadable is a trigger the page sent in a shape this does
	// not read. The message itself is still taken: a cancel dropped for
	// a bad report would leave a window that cannot be closed.
	TriggerUnreadable Trigger = "unreadable"
)

// LogValue names an empty Trigger in the log, so that a line for a
// message that reported nothing says so rather than showing a blank.
func (t Trigger) LogValue() slog.Value {
	if t == "" {
		return slog.StringValue("not-reported")
	}
	return slog.StringValue(string(t))
}

// rawMessage mirrors the JSON shape the page sends:
// {"type": "approve" | "cancel" | "selectCertificate" | "quit",
// "thumbprint": "...", "trigger": {"detail": n, "trusted": b, "byKey": b}}
type rawMessage struct {
	Type       string          `json:"type"`
	Thumbprint string          `json:"thumbprint"`
	Trigger    json.RawMessage `json:"trigger"`
}

// rawTrigger is what the page saw of the click (bridge.js,
// liroClickFacts): the event's detail, whether the browser made it, and
// whether Enter or Space reached the button in the same task.
type rawTrigger struct {
	Detail  int  `json:"detail"`
	Trusted bool `json:"trusted"`
	ByKey   bool `json:"byKey"`
}

// parseTrigger turns what the page saw into a Trigger. Its order
// matters: an untrusted event is the page's script whatever its fields
// say, and a click count is a pointer's even with a key down.
func parseTrigger(raw json.RawMessage) Trigger {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var r rawTrigger
	if err := json.Unmarshal(raw, &r); err != nil {
		return TriggerUnreadable
	}
	switch {
	case !r.Trusted:
		return TriggerScript
	case r.Detail >= 1:
		return TriggerPointer
	case r.ByKey:
		return TriggerKey
	default:
		return TriggerNeither
	}
}

// ParseMessage validates one raw JSON message received from the page
// (F5 §2.4: "Messages are JSON with a type field and are validated in
// Go before anything is acted on"). A message that fails to parse, or
// whose type is not one of the three recognised values, is rejected:
// ok is false, and the caller is expected to log and drop it rather
// than act on the zero Message — this function itself only reports the
// rejection reason via the returned error's text, since a real caller
// (window_windows.go) also has slog available and can attach context
// (which window, raw bytes truncated) that this pure function does not
// have.
func ParseMessage(raw []byte) (Message, bool) {
	var m rawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return Message{}, false
	}
	switch MessageType(m.Type) {
	case MessageTypeApprove:
		return Message{Type: MessageTypeApprove}, true
	case MessageTypeCancel:
		return Message{Type: MessageTypeCancel, Trigger: parseTrigger(m.Trigger)}, true
	case MessageTypeQuit:
		return Message{Type: MessageTypeQuit, Trigger: parseTrigger(m.Trigger)}, true
	case MessageTypeSelectCertificate:
		if m.Thumbprint == "" {
			return Message{}, false
		}
		return Message{Type: MessageTypeSelectCertificate, Thumbprint: m.Thumbprint}, true
	default:
		return Message{}, false
	}
}

// dispatchMessage is the single entry point window_windows.go's
// WebMessageReceived handler calls with the raw bytes it got from
// ICoreWebView2WebMessageReceivedEventArgs::WebMessageAsJson. It exists
// so that the untestable Windows glue is a one-line call into logic
// this file's tests exercise directly via ParseMessage — dispatchMessage
// itself is trivial enough not to need its own test beyond what
// ParseMessage and the OnMessage plumbing already cover.
func dispatchMessage(onMessage func(Message), raw []byte) {
	msg, ok := ParseMessage(raw)
	if !ok {
		slog.Warn("ui: dropped a message from the page: not one of approve/cancel/selectCertificate", "raw", string(raw))
		return
	}
	if onMessage != nil {
		onMessage(msg)
	}
}
