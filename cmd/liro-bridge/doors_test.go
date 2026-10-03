package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
)

// actionWindow is a window whose page has recorded one action, which is
// all handleAction reads from it.
type actionWindow struct {
	countingWindow
	action string
	handle uintptr
}

func (w *actionWindow) Eval(string) (string, error) {
	inner, _ := json.Marshal(map[string]string{"action": w.action})
	outer, _ := json.Marshal(string(inner))
	return string(outer), nil
}

func (w *actionWindow) Handle() uintptr { return w.handle }

// TestTheMainWindowOpensTheTrayMenusThreeWindows is D31: Settings, the
// certificates and the audit log are reached from the main window, not
// only from a tray a stock GNOME desktop does not have — and Settings is
// opened over this window, through the doors the window was given (the
// agent's own, with the agent's one pairing store).
func TestTheMainWindowOpensTheTrayMenusThreeWindows(t *testing.T) {
	var mu sync.Mutex
	opened := map[string]int{}
	var settingsOwner uintptr
	release := make(chan struct{})
	done := make(chan string, 8)

	door := func(name string) func() {
		return func() {
			mu.Lock()
			opened[name]++
			mu.Unlock()
			<-release
			done <- name
		}
	}

	m := newMainWindow(config.Default(), "en")
	w := &actionWindow{handle: 0x5150}
	m.win = w
	m.doors = windowDoors{
		settings: func(owner uintptr) {
			mu.Lock()
			settingsOwner = owner
			mu.Unlock()
			door("settings")()
		},
		certificates: door("certificates"),
		auditLog:     door("auditLog"),
	}

	for _, a := range []string{"openSettings", "openCertificates", "openAuditLog"} {
		w.action = a
		if closes := m.handleAction(context.Background()); closes {
			t.Fatalf("%s closed the main window; it opens another one beside it", a)
		}
	}
	// A second click on each while its window is still up opens nothing.
	for _, a := range []string{"openSettings", "openCertificates", "openAuditLog"} {
		w.action = a
		m.handleAction(context.Background())
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(opened)
		mu.Unlock()
		if n == 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	mu.Lock()
	for range opened {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("a door's window never returned")
		}
	}
	defer mu.Unlock()
	for _, name := range []string{"settings", "certificates", "auditLog"} {
		if opened[name] != 1 {
			t.Errorf("%s was opened %d times, want once: twice is a second copy of a window that is still up", name, opened[name])
		}
	}
	if settingsOwner != w.handle {
		t.Errorf("Settings was opened over %#x, want the main window %#x", settingsOwner, w.handle)
	}

	if opened["settings"] == 0 {
		return
	}
	select {
	case <-m.settingsClosed:
	case <-time.After(5 * time.Second):
		t.Error("Settings closing did not tell the main window, so what it shows from the configuration would stay stale")
	}
}

// TestTheDoorsAreOnTheMainPageAndNotConditional checks the page: the three
// buttons are in main.html with the tray's own words, and nothing hides
// them the way Izađi is hidden for a window that is not the agent's
// (D-419: not conditional on a tray, on every platform).
func TestTheDoorsAreOnTheMainPageAndNotConditional(t *testing.T) {
	html, err := os.ReadFile("../../internal/ui/assets/pages/main.html")
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile("../../internal/ui/assets/pages/main.js")
	if err != nil {
		t.Fatal(err)
	}
	for id, key := range map[string]string{
		"settings-btn":     "tray.settings",
		"certificates-btn": "tray.certificates",
		"auditlog-btn":     "tray.audit_log",
	} {
		i := strings.Index(string(html), `id="`+id+`"`)
		if i < 0 {
			t.Errorf("main.html has no %s", id)
			continue
		}
		tag := string(html[i:])
		tag = tag[:strings.Index(tag, ">")]
		if !strings.Contains(tag, `data-i18n="`+key+`"`) {
			t.Errorf("%s is not labelled %s, the tray's own word for it", id, key)
		}
		if strings.Contains(tag, "hidden") {
			t.Errorf("%s starts hidden; D-419 makes it unconditional", id)
		}
		if strings.Contains(string(js), `getElementById("`+id+`").hidden`) {
			t.Errorf("main.js hides %s on some condition; D-419 makes it unconditional", id)
		}
	}
	m := newMainWindow(config.Default(), "sr-Latn")
	got := m.staticStrings()
	for _, key := range []string{"tray.settings", "tray.certificates", "tray.audit_log"} {
		if got[key] == "" || got[key] == key {
			t.Errorf("staticStrings does not resolve %s, so the button would be blank", key)
		}
	}
}
