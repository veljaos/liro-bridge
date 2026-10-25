package ui

import "sync"

// RunMain runs f — the whole program, from main — and returns its exit
// code. On macOS (F13) AppKit must own the process's main thread, so f
// runs on a goroutine while the main thread runs the application's loop;
// everywhere else it is f(). main calls it once, and a test binary that
// opens windows on macOS calls it from TestMain.
func RunMain(f func() int) int { return runMain(f) }

// urls is the links the operating system handed this process directly —
// macOS's kAEGetURL (F13); Windows and Linux put a link in argv instead,
// and never call deliverURL. One that arrives before SetURLHandler is
// kept for it: the event that launched the application comes first.
var urls struct {
	sync.Mutex
	handler func(string)
	pending []string
}

// SetURLHandler sets what is done with a link the system delivers, and
// hands it any that arrived before.
func SetURLHandler(f func(string)) {
	urls.Lock()
	urls.handler = f
	pending := urls.pending
	urls.pending = nil
	urls.Unlock()
	for _, u := range pending {
		f(u)
	}
}

func deliverURL(u string) {
	urls.Lock()
	f := urls.handler
	if f == nil {
		urls.pending = append(urls.pending, u)
	}
	urls.Unlock()
	if f != nil {
		go f(u) // never on the main thread: the handler may open a window
	}
}
