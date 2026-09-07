//go:build windows

package main

// Process resource counters, for the window-lifetime measurement.
//
// They live in a _test.go file on purpose: verification scaffolding does
// not belong in the product (D-100), and nothing the agent ships needs
// to know its own handle count. runtime.MemStats cannot see a leaked
// HWND, HDC or WebView2 controller, which is exactly what the window
// layer leaks when it leaks anything, so the counters have to come from
// Windows itself.

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	counterKernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessHandles = counterKernel32.NewProc("GetProcessHandleCount")

	counterUser32           = windows.NewLazySystemDLL("user32.dll")
	procGetGuiResourceCount = counterUser32.NewProc("GetGuiResources")
)

const (
	guiResourcesGDIObjects  = 0
	guiResourcesUserObjects = 1
)

func processHandleCount() uint32 {
	var n uint32
	_, _, _ = procGetProcessHandles.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	runtime.KeepAlive(&n)
	return n
}

func processGDIObjects() uint32 {
	r, _, _ := procGetGuiResourceCount.Call(uintptr(windows.CurrentProcess()), guiResourcesGDIObjects)
	return uint32(r)
}

func processUserObjects() uint32 {
	r, _, _ := procGetGuiResourceCount.Call(uintptr(windows.CurrentProcess()), guiResourcesUserObjects)
	return uint32(r)
}

// processThreadCount walks a thread snapshot, because Windows offers no
// call that answers "how many threads does this process have" without
// one.
func processThreadCount() int {
	pid := windows.GetCurrentProcessId()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return -1
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ThreadEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := windows.Thread32First(snap, &e); err != nil {
		return -1
	}
	n := 0
	for {
		if e.OwnerProcessID == pid {
			n++
		}
		if err := windows.Thread32Next(snap, &e); err != nil {
			if err == syscall.ERROR_NO_MORE_FILES {
				break
			}
			break
		}
	}
	return n
}
