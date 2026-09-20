//go:build linux

package ui

// #cgo pkg-config: webkitgtk-6.0
// #include <stdlib.h>
// #include "webkitjs_linux.h"
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
)

// evalInWebView is evaluateJavascript expressed in the binding's own
// types, so that nothing above this file has to hold a view pointer.
//
// Object.Native() already returns a uintptr, and it stays one all the
// way to C — so there is no unsafe.Pointer conversion here for
// `unsafeptr` to have an opinion about, and none of the lifetime
// question that a *C.WebKitWebView held in a Go struct would raise.
func evalInWebView(view *webkit.WebView, script string) (string, error) {
	// coreglib.BaseObject, not view.Native(): a WebView is a GtkWidget
	// too, and GtkWidget has its own Native() returning a
	// *gtk.NativeSurface — a different thing that compiles in the same
	// place. The GObject address is the one WebKit's C API wants.
	return evaluateJavascript(coreglib.BaseObject(view).Native(), script)
}

// evalResult is one completed evaluation: the engine's own JSON
// encoding of the value, or the error WebKit reported.
type evalResult struct {
	json string
	err  error
}

// pendingEval is one evaluation in flight. The view is kept because
// webkit_web_view_evaluate_javascript_finish needs the same view the
// call started on, and the callback is handed only a GAsyncResult.
type pendingEval struct {
	view uintptr
	done chan evalResult
}

// evalRegistry maps an integer token to the evaluation it belongs to.
//
// The token exists because cgo forbids a Go pointer being stored in C
// memory, and user_data is C memory for as long as the operation is in
// flight. A counter and a map are the standard answer; this package
// already does the same thing on the other platform, where a callback
// from foreign code has to find its way back to a Go object.
var evalRegistry = struct {
	sync.Mutex
	next uint64
	m    map[uint64]*pendingEval
}{m: make(map[uint64]*pendingEval)}

// ErrEvalFailed is returned when the page's own engine reported a
// failure — a syntax error in the script, or an exception it threw.
// It is not returned for a value that simply cannot be encoded: that
// is "null", which is what WebView2 returns for the same case and what
// Window.Eval's contract already says.
var ErrEvalFailed = errors.New("ui: evaluating script in the page failed")

// evaluateJavascript runs script in view's page and returns the
// engine's JSON encoding of its result.
//
// **It must not be called from the UI thread.** The evaluation
// completes through the GLib main loop, so a caller that occupied that
// loop waiting for the answer would be waiting for the thing that
// delivers it. The start is marshalled onto the UI thread and the wait
// happens off it, which is the whole shape of this function and the
// reason it is not simply a do() around a C call.
func evaluateJavascript(view uintptr, script string) (string, error) {
	p := &pendingEval{view: view, done: make(chan evalResult, 1)}

	evalRegistry.Lock()
	evalRegistry.next++
	token := evalRegistry.next
	evalRegistry.m[token] = p
	evalRegistry.Unlock()

	cScript := C.CString(script)

	// Starting the call is a GTK call and belongs on the UI thread;
	// waiting for it must not be there. do returns as soon as the
	// evaluation has been handed to WebKit.
	err := theUIThread.do(func() {
		defer C.free(unsafe.Pointer(cScript))
		C.liro_eval_start(C.ulonglong(view), cScript, C.ulonglong(token))
	})
	if err != nil {
		evalRegistry.Lock()
		delete(evalRegistry.m, token)
		evalRegistry.Unlock()
		return "", err
	}

	r := <-p.done
	return r.json, r.err
}

//export liroEvalDone
func liroEvalDone(res C.ulonglong, token C.ulonglong) {
	evalRegistry.Lock()
	p := evalRegistry.m[uint64(token)]
	delete(evalRegistry.m, uint64(token))
	evalRegistry.Unlock()

	if p == nil {
		// The window went away while the evaluation was in flight.
		// Nothing is waiting, and finishing would only produce a value
		// with nowhere to go.
		return
	}

	var cErr *C.char
	cJSON := C.liro_eval_finish(C.ulonglong(p.view), res, &cErr)

	if cJSON == nil {
		msg := "unknown error"
		if cErr != nil {
			msg = C.GoString(cErr)
			C.g_free(C.gpointer(unsafe.Pointer(cErr)))
		}
		p.done <- evalResult{err: fmt.Errorf("%w: %s", ErrEvalFailed, msg)}
		return
	}

	// Past this point cJSON is non-NULL by construction: the C side
	// turns "unencodable" into the JSON "null" itself, because that is
	// the only place the difference between "no value" and "no error"
	// is still visible. Window.Eval's contract is written in those
	// terms and WebView2 answers the same way, so the two platforms
	// agree rather than each being truthful in its own dialect.
	json := C.GoString(cJSON)
	C.g_free(C.gpointer(unsafe.Pointer(cJSON)))
	p.done <- evalResult{json: json}
}
