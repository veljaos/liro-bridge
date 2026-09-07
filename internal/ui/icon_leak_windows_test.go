//go:build windows

package ui

import (
	"runtime"
	"testing"
)

// A window's two title-bar icons are loaded with LoadImageW and
// LR_LOADFROMFILE, which creates a new icon every call: no LR_SHARED,
// so the caller owns it, and WM_SETICON does not take ownership.
// Nothing destroyed them, so every window this agent opened cost GDI
// objects that never came back.
//
// Measured on the machine this was found on: one icon is 3 GDI objects
// and 1 USER object, two icons per window, so 6 GDI per window — and a
// process's default GDI quota is 10,000. Found by opening each of the
// seven windows a hundred times and watching the counters (FTEST Group
// 3); the growth was exactly linear at 6.00 per cycle.
//
// This test is the cheap half: it proves the primitive gives the
// objects back. cmd/liro-bridge's TestOpeningAndClosingWindowsDoesNotLeakGDIObjects
// is the half that proves a real window actually calls it.

var procGetGuiResourcesForTest = user32DLL.NewProc("GetGuiResources")

const (
	guiGDIObjects  = 0
	guiUserObjects = 1
)

func gdiObjects() int {
	r, _, _ := procGetGuiResourcesForTest.Call(currentProcessPseudoHandle(), guiGDIObjects)
	return int(r)
}

func userObjects() int {
	r, _, _ := procGetGuiResourcesForTest.Call(currentProcessPseudoHandle(), guiUserObjects)
	return int(r)
}

// currentProcessPseudoHandle is GetCurrentProcess()'s documented
// constant, (HANDLE)-1, which every per-process query below accepts
// without a real handle to open or close.
func currentProcessPseudoHandle() uintptr {
	return ^uintptr(0)
}

func TestAnIconLoadedForAWindowIsGivenBack(t *testing.T) {
	// Extracting the .ico on the first call is a one-time cost that
	// would otherwise be counted against the first icon.
	if h := loadIconAt(16, 16); h != 0 {
		destroyWindowIcons(h, 0)
	}
	runtime.GC()

	const n = 50
	gdiBefore, userBefore := gdiObjects(), userObjects()

	icons := make([][2]uintptr, 0, n)
	for i := 0; i < n; i++ {
		small := loadIconAt(16, 16)
		big := loadIconAt(32, 32)
		if small == 0 || big == 0 {
			t.Skipf("the embedded icon could not be loaded on this machine (small=%v big=%v); "+
				"nothing to measure", small, big)
		}
		icons = append(icons, [2]uintptr{small, big})
	}

	gdiLoaded, userLoaded := gdiObjects(), userObjects()
	if gdiLoaded <= gdiBefore {
		t.Fatalf("loading %d icons cost no GDI objects at all (%d then %d): "+
			"the counter is not measuring what this test thinks it is",
			n*2, gdiBefore, gdiLoaded)
	}
	t.Logf("%d icons cost %d GDI and %d USER objects (%.2f and %.2f each)",
		n*2, gdiLoaded-gdiBefore, userLoaded-userBefore,
		float64(gdiLoaded-gdiBefore)/float64(n*2), float64(userLoaded-userBefore)/float64(n*2))

	for _, pair := range icons {
		destroyWindowIcons(pair[0], pair[1])
	}

	gdiAfter, userAfter := gdiObjects(), userObjects()
	// A handful of objects may legitimately be held by something else
	// on this thread; what must not survive is the bulk of what was
	// just allocated.
	const slack = 8
	if gdiAfter > gdiBefore+slack {
		t.Errorf("destroying %d icons gave back %d of %d GDI objects: %d are still held",
			n*2, gdiLoaded-gdiAfter, gdiLoaded-gdiBefore, gdiAfter-gdiBefore)
	}
	if userAfter > userBefore+slack {
		t.Errorf("destroying %d icons gave back %d of %d USER objects: %d are still held",
			n*2, userLoaded-userAfter, userLoaded-userBefore, userAfter-userBefore)
	}
}

// TestDestroyingNoIconIsHarmless: setWindowIcons returns 0 for a size it
// could not load, and the teardown path must not care.
func TestDestroyingNoIconIsHarmless(t *testing.T) {
	destroyWindowIcons(0, 0)
}
