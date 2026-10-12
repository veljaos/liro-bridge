//go:build darwin && cgo

package ui

// The Go functions AppKit and WebKit call (darwin_objc.go). A file that
// exports may hold only declarations in its preamble, which is why these
// are apart from the Objective-C they answer.

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"strings"
	"unsafe"
)

//export goRunOnMain
func goRunOnMain(h C.uintptr_t) { runOnMainHandle(uintptr(h)) }

//export goWindowMessage
func goWindowMessage(wid C.uintptr_t, json *C.char) {
	if w := darwinWindowByID(uintptr(wid)); w != nil {
		raw := []byte(C.GoString(json))
		w.emit(func() { dispatchMessage(w.onMessage, raw) })
	}
}

//export goWindowLoaded
func goWindowLoaded(wid C.uintptr_t, errText *C.char) {
	if w := darwinWindowByID(uintptr(wid)); w != nil {
		msg := ""
		if errText != nil {
			msg = C.GoString(errText)
		}
		w.loaded(msg)
	}
}

//export goWindowClosed
func goWindowClosed(wid C.uintptr_t) {
	if w := darwinWindowByID(uintptr(wid)); w != nil {
		w.emit(w.onClosed)
		w.markClosed()
	}
}

//export goWindowDropped
func goWindowDropped(wid C.uintptr_t, paths *C.char) {
	if w := darwinWindowByID(uintptr(wid)); w != nil && w.onFiles != nil {
		list := strings.Split(C.GoString(paths), "\n")
		w.emit(func() { w.onFiles(list) })
	}
}

//export goAllowNavigation
func goAllowNavigation(wid C.uintptr_t, url *C.char) C.int {
	w := darwinWindowByID(uintptr(wid))
	if w == nil {
		return 0
	}
	if w.allowNavigation(C.GoString(url)) {
		return 1
	}
	return 0
}

//export goResolveAsset
func goResolveAsset(wid C.uintptr_t, url *C.char, n *C.size_t, mime **C.char) unsafe.Pointer {
	w := darwinWindowByID(uintptr(wid))
	if w == nil {
		return nil
	}
	data, contentType, ok := w.asset(C.GoString(url))
	if !ok {
		return nil
	}
	*n = C.size_t(len(data))
	*mime = C.CString(contentType)
	if len(data) == 0 {
		return C.malloc(1) // a non-NULL empty body; NSData takes length 0
	}
	return C.CBytes(data)
}

//export goEvalDone
func goEvalDone(token C.uintptr_t, json *C.char) { evalDone(uintptr(token), C.GoString(json)) }

//export goTrayAction
func goTrayAction(item C.int) { trayAction(int(item)) }

//export goTrayLabels
func goTrayLabels() *C.char { return C.CString(trayLabels()) }

//export goOpenURL
func goOpenURL(url *C.char) { deliverURL(C.GoString(url)) }
