package ui

import (
	"encoding/json"
	"log/slog"
)

// MessageType is one of the exactly three things the page is allowed to
// ask for (F5 §2.4): approve, cancel, and select a certificate by
// thumbprint. Anything else is dropped and logged — the message surface
// is kept this small deliberately.
type MessageType string

const (
	MessageTypeApprove           MessageType = "approve"
	MessageTypeCancel            MessageType = "cancel"
	MessageTypeSelectCertificate MessageType = "selectCertificate"
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
