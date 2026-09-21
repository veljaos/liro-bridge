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
}

// rawMessage mirrors the JSON shape the page sends:
// {"type": "approve" | "cancel" | "selectCertificate", "thumbprint": "..."}
type rawMessage struct {
	Type       string `json:"type"`
	Thumbprint string `json:"thumbprint"`
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
		return Message{Type: MessageTypeCancel}, true
	case MessageTypeQuit:
		return Message{Type: MessageTypeQuit}, true
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
