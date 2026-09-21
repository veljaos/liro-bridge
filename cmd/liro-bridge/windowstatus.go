package main

// One line of feedback under a window's actions, and the shape an
// action that wrote files reports. Three small functions that every
// window in this program uses and that have nothing to do with the
// tray, where they lived.

import (
	"github.com/veljaos/liro-bridge/internal/ui"
)

// postWindowStatus shows one line of feedback under a window's action
// buttons. intent picks its colour family, never a colour (D-093).
//
// Named for windows rather than for Settings because the audit log
// window renders the same payload in the same place: both have an
// Export button, and both have to say where the files went.
func postWindowStatus(win ui.Window, text string, intent ui.Intent) {
	postWindowStatusFiles(win, text, nil, intent)
}

// exportedFile is one file an action wrote: its name, and in one line,
// in the person's own language, what that file is.
type exportedFile struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// postWindowStatusFiles is postWindowStatus for an action that wrote
// files. The heading says where they went; each file then names itself
// and says what it is, because two files appearing in a folder with
// nothing on screen about either of them is how the verification report
// came to be opened and asked about.
func postWindowStatusFiles(win ui.Window, text string, files []exportedFile, intent ui.Intent) {
	_ = win.PostJSON(map[string]any{
		"type": "status",
		"status": map[string]any{
			"text":   text,
			"files":  files,
			"intent": string(intent),
		},
	})
}
