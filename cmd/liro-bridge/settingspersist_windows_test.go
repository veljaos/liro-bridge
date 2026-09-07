//go:build windows

package main

// The defect the owner reported as "Settings does not persist": change
// the interface language to Serbian Cyrillic, press Save, close the
// window, open it again — and it is back in Latin. The same for every
// other field.
//
// Measured before anything was changed, in this order, because the four
// steps a value passes through fail differently and only one of them was
// actually failing:
//
//  1. the page sends the changed value  — it does, all thirteen of them
//  2. Go builds the right Config        — it does
//  3. Save writes it to disk            — it does, every field correct
//  4. the reopened window reads it back — it does NOT
//
// Step 4 is the whole defect. The window was handed a config.Config by
// whoever opened it — the tray, which read the file once when the
// process started and held that copy for its whole life — and rendered
// that instead of the file it had just written. The value was never
// lost; it was written correctly and then never read again. Worse, the
// next Save folded the form onto that same stale copy and wrote it back
// over the file, so the first save survived only until the second.
//
// The test that existed passed throughout, because it called
// handleSettingsAction with the very Config it then asserted against:
// it could only prove that a save folds the form onto whatever it is
// handed, which is what the code did and what was wrong. These tests go
// through the file instead — save, then reopen, then read.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// settingsProbe is one field of the settings form: the value the test
// types into it, the Config field it must reach, and the script that
// reads it back off the reopened page. "Every field in Settings must
// survive Save, close and reopen" is checked field by field, not for the
// language alone.
type settingsProbe struct {
	name    string
	set     string // changes the control, as a person would
	read    string // reads it back off the page, as a string
	want    string
	inSaved func(config.Config) string
}

func settingsProbes() []settingsProbe {
	return []settingsProbe{
		{
			"language", `document.getElementById('locale').value='sr-Cyrl'`,
			`document.getElementById('locale').value`, "sr-Cyrl",
			func(c config.Config) string { return c.Locale },
		},
		{
			"start with Windows", `document.getElementById('start-with-windows').checked=false`,
			`String(document.getElementById('start-with-windows').checked)`, "false",
			func(c config.Config) string { return boolText(c.StartWithWindows) },
		},
		{
			"TSA URL", `document.getElementById('tsa-url').value='https://tsa.example/tsr'`,
			`document.getElementById('tsa-url').value`, "https://tsa.example/tsr",
			func(c config.Config) string { return c.TSAURL },
		},
		{
			"TSA user", `document.getElementById('tsa-user').value='Test.Korisnik'`,
			`document.getElementById('tsa-user').value`, "Test.Korisnik",
			func(c config.Config) string { return c.TSAUser },
		},
		{
			"TSA password", `document.getElementById('tsa-password').value='123456'`,
			`document.getElementById('tsa-password').value`, "123456",
			func(c config.Config) string { return c.TSAPassword },
		},
		{
			"TSA client certificate", `document.getElementById('tsa-client-cert-path').value='C:\\Sertifikati\\posta.p12'`,
			`document.getElementById('tsa-client-cert-path').value`, `C:\Sertifikati\posta.p12`,
			func(c config.Config) string { return c.TSAClientCertPath },
		},
		{
			"TSA client certificate password", `document.getElementById('tsa-client-cert-password').value='1234'`,
			`document.getElementById('tsa-client-cert-password').value`, "1234",
			func(c config.Config) string { return c.TSAClientCertPassword },
		},
		{
			"output suffix", `document.getElementById('output-suffix').value='-potpisan'`,
			`document.getElementById('output-suffix').value`, "-potpisan",
			func(c config.Config) string { return c.OutputSuffix },
		},
		{
			"output folder", `document.getElementById('output-folder').value='C:\\Potpisano'`,
			`document.getElementById('output-folder').value`, `C:\Potpisano`,
			func(c config.Config) string { return c.OutputFolder },
		},
		{
			"Explorer menu", `document.getElementById('explorer-menu').checked=false`,
			`String(document.getElementById('explorer-menu').checked)`, "false",
			func(c config.Config) string { return boolText(c.ExplorerMenuEnabled) },
		},
		{
			"signature level", `document.getElementById('level-bt').checked=true`,
			`document.querySelector('input[name=level]:checked').value`, "b-t",
			func(c config.Config) string { return c.SignatureLevel },
		},
		{
			"daily update check", `document.getElementById('check-updates-daily').checked=false`,
			`String(document.getElementById('check-updates-daily').checked)`, "false",
			func(c config.Config) string { return boolText(c.UpdateCheckEnabled) },
		},
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// tempConfigHome points LOCALAPPDATA — and with it config.DefaultPath
// and everything else internal/platform resolves — at a directory of
// this test's own.
//
// Not t.TempDir(): a WebView2 window created while LOCALAPPDATA points
// here puts its user-data folder underneath it and keeps files in it
// open after the window has closed, which t.TempDir()'s own cleanup
// then reports as a failure of a test that passed. The window is the
// thing under test here, so the directory has to tolerate it.
//
// The cleanup below retries, and sweepStaleConfigHomes picks up what it
// still could not take. Both are needed and neither is enough alone —
// see FTEST Group 3, C-6: the browser process group holding these files
// was measured still running two hours after its test binary exited,
// and 196 suite runs had left 1.26 GB in %TEMP%. The failure was
// already being discarded here; discarding it is what made it
// invisible.
func tempConfigHome(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", configHomePrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", dir)
	t.Cleanup(func() { removeWithRetry(dir) })
}

// configHomePrefix names the temporary directories tempConfigHome makes,
// so the sweep below can recognise its own leftovers and nothing else.
const configHomePrefix = "liro-config-"

// removeWithRetry deletes dir, giving WebView2's browser process group a
// few seconds to let go of the user-data folder underneath it first. It
// gives up quietly: a directory it cannot take is one sweepStaleConfigHomes
// will find on a later run, and failing a test that otherwise passed
// because a browser is slow to exit would be worse than either.
func removeWithRetry(dir string) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// sweepStaleConfigHomes removes config homes an earlier run could not,
// which is how this stops accumulating rather than merely accumulating
// more slowly. Only directories older than an hour are touched, so a
// suite running in parallel with another one cannot delete the other's.
func sweepStaleConfigHomes() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), configHomePrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
	}
}

// keepThisMachinesAutostartAndMenu snapshots the two registry
// registrations a save writes outside the config file, and puts them
// back afterwards.
//
// A save calls platform.NewAutostart().SetEnabled and applyExplorerMenu
// with exePath(), and os.Executable() inside a test binary is the test
// binary — so without this, running the suite points the developer's own
// autostart entry and Explorer context menu at a deleted file in a
// temporary directory. Measured, not assumed: after one run of the
// measurement harness this pass began with, HKCU's LiroBridge value read
// "…\Temp\go-build…\liro-bridge.test.exe".
//
// The same discipline internal/platform's own TestWindowsAutostartRoundTrip
// uses: touch the real key, restore exactly what was found.
func keepThisMachinesAutostartAndMenu(t *testing.T) {
	t.Helper()

	const (
		runKey     = `Software\Microsoft\Windows\CurrentVersion\Run`
		runValue   = "LiroBridge"
		menuKey    = `Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`
		menuCmdKey = menuKey + `\command`
	)

	type value struct {
		key, name string
		text      string
		present   bool
	}
	read := func(key, name string) value {
		v := value{key: key, name: name}
		k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
		if err != nil {
			return v
		}
		defer func() { _ = k.Close() }()
		s, _, err := k.GetStringValue(name)
		if err != nil {
			return v
		}
		v.text, v.present = s, true
		return v
	}

	before := []value{
		read(runKey, runValue),
		read(menuKey, ""),
		read(menuKey, "Icon"),
		read(menuKey, "MultiSelectModel"),
		read(menuCmdKey, ""),
	}

	t.Cleanup(func() {
		for _, v := range before {
			if !v.present {
				continue
			}
			k, _, err := registry.CreateKey(registry.CURRENT_USER, v.key, registry.SET_VALUE)
			if err != nil {
				t.Errorf("restoring HKCU\\%s: %v", v.key, err)
				continue
			}
			if err := k.SetStringValue(v.name, v.text); err != nil {
				t.Errorf("restoring HKCU\\%s\\%s: %v", v.key, v.name, err)
			}
			_ = k.Close()
		}
	})
}

// TestEverySettingsFieldSurvivesSaveCloseAndReopen is the report itself,
// as a test: change every field, save, close, open again, and look at
// what the window shows.
//
// The reopen goes through settingsOnOpening — the production function
// runSettingsWindow calls to decide what a freshly opened settings
// window displays — handed the stale Config the tray would still be
// holding. That staleness is the point: before this pass it is what got
// rendered, and every field comes back as it was before the save.
func TestEverySettingsFieldSurvivesSaveCloseAndReopen(t *testing.T) {
	keepThisMachinesAutostartAndMenu(t)
	tempConfigHome(t)

	// The configuration a long-lived caller read once and kept. Nothing
	// after this point may render it.
	stale := config.Default()
	if err := config.Save(config.DefaultPath(), stale); err != nil {
		t.Fatal(err)
	}

	probes := settingsProbes()

	c, opened, _ := settingsOnOpening(stale)
	win, messages := newProductionSettingsWindow(t, c, opened)
	for _, p := range probes {
		if _, err := win.Eval(p.set); err != nil {
			t.Fatalf("setting %s: %v", p.name, err)
		}
	}

	// Saved through the page's own button (D-094's Eval carve-out —
	// nothing here touches the real cursor).
	if _, err := win.Eval(`document.getElementById('save-btn').click()`); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-messages:
		if msg.Type != ui.MessageTypeApprove {
			t.Fatalf("Save produced %v, want approve", msg.Type)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Save never reached Go")
	}
	if !handleSettingsAction(win, c, stale, collectSettingsState(t, win)) {
		t.Fatal("saving did not close the window")
	}
	_ = win.Close()

	// On disk, straight afterwards.
	saved := readSavedConfig(t)
	for _, p := range probes {
		if got := p.inSaved(saved); got != p.want {
			t.Errorf("after Save, config.json holds %s = %q, want %q", p.name, got, p.want)
		}
	}

	// Closed, and opened again by a caller still holding the Config it
	// read before any of this happened.
	reopenC, reopened, _ := settingsOnOpening(stale)
	if reopened.Locale != "sr-Cyrl" {
		t.Errorf("the reopened window reads locale %q, want the saved sr-Cyrl", reopened.Locale)
	}
	if got, want := reopenC.T("settings.window_title"), i18n.Load("sr-Cyrl").T("settings.window_title"); got != want {
		t.Errorf("the reopened window's own language gives the title %q, want %q", got, want)
	}
	win2, messages2 := newProductionSettingsWindow(t, reopenC, reopened)
	defer func() { _ = win2.Close() }()
	for _, p := range probes {
		if got := evalString(t, win2, p.read); got != p.want {
			t.Errorf("the reopened window shows %s = %q, want the saved %q", p.name, got, p.want)
		}
	}

	// And a second Save from that reopened window must not undo the
	// first: the caller's stale copy is never the base of what is
	// written.
	if _, err := win2.Eval(`document.getElementById('save-btn').click()`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-messages2:
	case <-time.After(20 * time.Second):
		t.Fatal("the second Save never reached Go")
	}
	if !handleSettingsAction(win2, reopenC, stale, collectSettingsState(t, win2)) {
		t.Fatal("the second save did not close the window")
	}
	again := readSavedConfig(t)
	for _, p := range probes {
		if got := p.inSaved(again); got != p.want {
			t.Errorf("a second Save wrote %s back to %q, want the saved %q", p.name, got, p.want)
		}
	}
}

// TestSettingsWindowOpensInTheLanguageOnDisk asserts the same property
// against the real window runSettingsWindow itself opens, rather than
// against the payload it posts. The title is the one thing a window
// shows that can be read from outside its own page, and it comes from
// the catalogue the window loaded.
//
// Run against the build that was reported it fails: the window opens
// titled "Podešavanja", from the caller's stale Config, while the file
// says sr-Cyrl.
func TestSettingsWindowOpensInTheLanguageOnDisk(t *testing.T) {
	tempConfigHome(t)

	stale := config.Default() // sr-Latn
	onDisk := config.Default()
	onDisk.Locale = "sr-Cyrl"
	if err := config.Save(config.DefaultPath(), onDisk); err != nil {
		t.Fatal(err)
	}

	cyrl := i18n.Load("sr-Cyrl").T("settings.window_title")
	if latn := i18n.Load("sr-Latn").T("settings.window_title"); cyrl == latn {
		t.Fatal("the two titles are identical, so this test cannot tell them apart")
	}

	returned := make(chan error, 1)
	go func() { returned <- runSettingsWindow(stale, 0) }()

	hwnd := waitForWindowTitled(t, cyrl, 40*time.Second)

	const wmClose = 0x0010
	_, _, _ = procPostMessageT.Call(hwnd, wmClose, 0, 0)
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("runSettingsWindow: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the settings window would not close")
	}
}

// TestSettingsSaveKeepsWhatTheStampWindowWrote is the other half of the
// same fault, one step removed and inside a single window's lifetime:
// the stamp window is reachable from Settings and writes its answer to
// disk while Settings is still open, so a Save that folds the form onto
// the Config Settings was opened with writes the pre-stamp values
// straight back over it.
func TestSettingsSaveKeepsWhatTheStampWindowWrote(t *testing.T) {
	keepThisMachinesAutostartAndMenu(t)
	tempConfigHome(t)

	// What Settings was opened with.
	opened := config.Default()
	opened.VisibleStamp = false
	opened.StampPosition = consent.StampPositionBottomRight

	// What the stamp window wrote while Settings stayed open.
	afterStamp := opened
	afterStamp.VisibleStamp = true
	afterStamp.StampPosition = consent.StampPositionTopRight
	afterStamp.StampPage = config.StampPageLast
	afterStamp.StampReference = "Ugovor 2026/114"
	afterStamp.StampShowDocumentID = true
	if err := config.Save(config.DefaultPath(), afterStamp); err != nil {
		t.Fatal(err)
	}

	c := i18n.Load("sr-Latn")
	win, _ := sharedSettingsWindow(t, c, opened)
	if !handleSettingsAction(win, c, opened, settingsFormState{
		Action: "save", Locale: "sr-Latn", OutputSuffix: "-signed", SignatureLevel: "b-lt",
	}) {
		t.Fatal("saving did not close the window")
	}

	saved := readSavedConfig(t)
	if !saved.VisibleStamp {
		t.Error("Settings' Save switched the stamp off again after the stamp window turned it on")
	}
	if saved.StampPosition != consent.StampPositionTopRight {
		t.Errorf("stamp position = %q, want the top-right the stamp window wrote", saved.StampPosition)
	}
	if saved.StampPage != config.StampPageLast {
		t.Errorf("stamp page = %q, want the last the stamp window wrote", saved.StampPage)
	}
	if saved.StampReference != "Ugovor 2026/114" {
		t.Errorf("stamp reference = %q, want the one the stamp window wrote", saved.StampReference)
	}
	if !saved.StampShowDocumentID {
		t.Error("the identity-document line was switched back off")
	}
}

// TestOneStampButtonPressSendsOneMessage: syncPresetSelection ran with a
// copy of the stamp button's click handler pasted inside it, so it added
// another listener every time it ran — once on init and once per
// keystroke in the TSA URL field. Measured at seven messages for one
// press after five keystrokes, which is seven stamp windows opened one
// after another.
func TestOneStampButtonPressSendsOneMessage(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, messages := sharedSettingsWindow(t, c, config.Default())

	for range 5 {
		if _, err := win.Eval(`document.getElementById('tsa-url').dispatchEvent(new Event('input'))`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := win.Eval(`document.getElementById('stamp-settings-btn').click()`); err != nil {
		t.Fatal(err)
	}

	n := 0
	deadline := time.After(4 * time.Second)
collect:
	for {
		select {
		case <-messages:
			n++
		case <-deadline:
			break collect
		}
	}
	if n != 1 {
		t.Fatalf("one press of the stamp button sent %d messages, want 1 — Settings would open that many stamp windows", n)
	}
	if state := collectSettingsState(t, win); state.Action != "stampSettings" {
		t.Fatalf("the page recorded action %q, want stampSettings", state.Action)
	}
}
