//go:build windows

package main

// Task 1 (F5 fourth-real-run review): the consent window offers the
// visible stamp, on by default, with a corner to put it in — and both
// answers travel back to Go and into the configuration. Task 4: the
// output-exists screen. Both are exercised against a real WebView2
// window, because both are page state that no Go-level test can see
// (D-087's lesson: the payload builders were right while the window
// showed nothing).
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

func stampTestCertificate() classify.Info {
	return classify.Info{
		Thumbprint:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Subject:       classify.Subject{DisplayName: "Test Testić"},
		IssuerCN:      "Test CA",
		Qualification: classify.QualificationQualified,
		Purpose:       classify.PurposeSigning,
		Usable:        true,
	}
}

func postConsentWithStamp(t *testing.T, win ui.Window, c *i18n.Catalogue, stamp consent.StampChoice) {
	t.Helper()
	vm := consent.BuildViewModel(consent.ApplicationLocal, [][]byte{{1}}, []string{"ugovor.pdf"},
		[]classify.Info{stampTestCertificate()})
	vm.Stamp = stamp
	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
}

// TestConsentStampCheckboxIsOnByDefaultAndOffersFourCorners is the
// finding this task exists for: every interactive signature was
// invisible because the window never offered to make one visible.
func TestConsentStampCheckboxIsOnByDefaultAndOffersFourCorners(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedConsentWindow(t)
	postConsentWithStamp(t, win, c, consent.DefaultStampChoice())

	if got := evalString(t, win, "String(document.getElementById('stamp-visible').checked)"); got != "true" {
		t.Errorf("the visible-stamp checkbox is %s out of the box, want checked", got)
	}
	if got := evalString(t, win, "document.getElementById('stamp-position').value"); got != consent.StampPositionBottomRight {
		t.Errorf("stamp position = %q, want %q (SPEC §13.1's default corner)", got, consent.StampPositionBottomRight)
	}

	values := evalString(t, win,
		"Array.prototype.map.call(document.querySelectorAll('#stamp-position option'),function(o){return o.value}).join(',')")
	if want := "bottom-right,bottom-left,top-right,top-left"; values != want {
		t.Errorf("corner options = %q, want %q", values, want)
	}
	// The four corners, and only the four corners, each with real text.
	labels := evalString(t, win,
		"Array.prototype.map.call(document.querySelectorAll('#stamp-position option'),function(o){return o.textContent}).join('|')")
	for _, key := range []string{
		"consent.stamp_position_bottom_right", "consent.stamp_position_bottom_left",
		"consent.stamp_position_top_right", "consent.stamp_position_top_left",
	} {
		if want := c.T(key); want == "" || !strings.Contains(labels, want) {
			t.Errorf("corner label %q (%s) missing from the selector: %q", want, key, labels)
		}
	}
	if got := evalString(t, win, "document.querySelector('[data-i18n=\"consent.stamp_visible\"]').textContent"); got != c.T("consent.stamp_visible") {
		t.Errorf("checkbox label = %q, want %q", got, c.T("consent.stamp_visible"))
	}
}

// TestConsentStampPositionHidesWhenTheStampIsOff: a corner selector for
// a stamp nobody is drawing is a dead control.
func TestConsentStampPositionHidesWhenTheStampIsOff(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedConsentWindow(t)

	postConsentWithStamp(t, win, c, consent.StampChoice{Visible: false, Position: consent.StampPositionTopLeft})
	if got := evalString(t, win, "String(document.getElementById('stamp-position-field').hidden)"); got != "true" {
		t.Errorf("the corner selector is not marked hidden with the stamp off (hidden = %s)", got)
	}
	// And it is actually off the screen. The property alone proves
	// nothing: an author rule as ordinary as ".liro-field { display:
	// flex }" outranks the browser's own "[hidden] { display: none }"
	// in the cascade, and this selector was on screen, with its
	// checkbox unticked, while the property below already read true.
	// Found by looking at the window, not by any test.
	if got := evalString(t, win, "getComputedStyle(document.getElementById('stamp-position-field')).display"); got != "none" {
		t.Errorf("the corner selector still renders with the stamp off (display: %s)", got)
	}
	if got := evalNumber(t, win, "document.getElementById('stamp-position-field').getBoundingClientRect().height"); got != 0 {
		t.Errorf("the corner selector occupies %v points with the stamp off", got)
	}

	// Ticking the box brings it back, through the page's own change
	// handler rather than a re-render from Go.
	script := "(function(){var b=document.getElementById('stamp-visible');b.checked=true;" +
		"b.dispatchEvent(new Event('change'));" +
		"return String(document.getElementById('stamp-position-field').hidden);})()"
	if got := evalString(t, win, script); got != "false" {
		t.Errorf("the corner selector stayed hidden after the stamp was switched on (hidden = %s)", got)
	}
	if got := evalNumber(t, win, "document.getElementById('stamp-position-field').getBoundingClientRect().height"); got <= 0 {
		t.Error("the corner selector has no height after the stamp was switched on")
	}
}

// TestReadStampChoiceReadsWhatThePageHolds proves the answer travels
// back to Go — the half that decides whether a stamp is actually drawn.
func TestReadStampChoiceReadsWhatThePageHolds(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedConsentWindow(t)
	postConsentWithStamp(t, win, c, consent.DefaultStampChoice())

	if got := readStampChoice(win, consent.StampChoice{}); got != consent.DefaultStampChoice() {
		t.Errorf("readStampChoice = %+v, want %+v", got, consent.DefaultStampChoice())
	}

	script := "(function(){document.getElementById('stamp-position').value='top-left';" +
		"document.getElementById('stamp-visible').checked=false;return 'ok';})()"
	if got := evalString(t, win, script); got != "ok" {
		t.Fatalf("setting the form failed: %q", got)
	}
	got := readStampChoice(win, consent.DefaultStampChoice())
	want := consent.StampChoice{Visible: false, Position: consent.StampPositionTopLeft}
	if got != want {
		t.Errorf("readStampChoice = %+v, want %+v", got, want)
	}
}

// TestInteractiveStampOptionsMapsTheChoice covers the Go half: off
// means nil (SPEC §13.4's invisible path, untouched), and each corner
// maps to its own appearance.Corner.
func TestInteractiveStampOptionsMapsTheChoice(t *testing.T) {
	c := i18n.Load("sr-Latn")
	if got := interactiveStampOptions(c, consent.StampChoice{Visible: false, Position: "bottom-right"}); got != nil {
		t.Errorf("a stamp that was not asked for produced %+v, want nil", got)
	}
	for _, tc := range []struct {
		position string
		want     appearance.Corner
	}{
		{consent.StampPositionBottomRight, appearance.BottomRight},
		{consent.StampPositionBottomLeft, appearance.BottomLeft},
		{consent.StampPositionTopRight, appearance.TopRight},
		{consent.StampPositionTopLeft, appearance.TopLeft},
	} {
		got := interactiveStampOptions(c, consent.StampChoice{Visible: true, Position: tc.position})
		if got == nil {
			t.Fatalf("position %q produced no stamp options", tc.position)
		}
		if got.Corner != tc.want {
			t.Errorf("position %q mapped to corner %v, want %v", tc.position, got.Corner, tc.want)
		}
		if got.Label != c.T("sign.stamp_label") {
			t.Errorf("stamp label = %q, want the localised %q", got.Label, c.T("sign.stamp_label"))
		}
		if got.ShowDocumentID {
			t.Error("the identity-document line is on by default (SPEC §13.5 forbids that)")
		}
	}
}

// TestInteractiveLevelMapsConfiguredLevels is Task 3's Go half: b-b is
// a level the user can choose, and it reaches pades as B-B.
func TestInteractiveLevelMapsConfiguredLevels(t *testing.T) {
	for _, tc := range []struct {
		configured string
		want       pades.Level
	}{
		{"b-b", pades.LevelBB},
		{"b-t", pades.LevelBT},
		{"b-lt", pades.LevelBLT},
		{"", pades.LevelBLT},
	} {
		cfg := populatedSettings()
		cfg.SignatureLevel = tc.configured
		if got := interactiveLevel(cfg); got != tc.want {
			t.Errorf("interactiveLevel(%q) = %v, want %v", tc.configured, got, tc.want)
		}
	}
}

// TestSettingsOffersThreeLevelsWithBBFirst is Task 3 in the window: the
// user who does not want a timestamp has something to choose, and it
// is the first option.
func TestSettingsOffersThreeLevelsWithBBFirst(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := populatedSettings() // SignatureLevel "b-b"
	win, _ := sharedSettingsWindow(t, c, cfg)

	values := evalString(t, win,
		"Array.prototype.map.call(document.querySelectorAll('input[name=level]'),function(r){return r.value}).join(',')")
	if want := "b-b,b-t,b-lt"; values != want {
		t.Errorf("level radios = %q, want %q (B-B first)", values, want)
	}
	if got := evalString(t, win, "document.querySelector('input[name=level]:checked').value"); got != "b-b" {
		t.Errorf("with b-b configured, the checked radio is %q", got)
	}
	if got := evalString(t, win, "document.querySelector('[data-i18n=\"settings.level_bb\"]').textContent"); got != c.T("settings.level_bb") {
		t.Errorf("B-B label = %q, want %q", got, c.T("settings.level_bb"))
	}

	// And the form reports it back — the value that is actually saved.
	state := collectSettingsState(t, win)
	if state.SignatureLevel != "b-b" {
		t.Errorf("__liroCollectState reported level %q, want b-b", state.SignatureLevel)
	}
}

// TestSettingsSavePreservesTheStampChoice guards the field the settings
// form does not show: saving any setting must not switch the visible
// stamp back off, which is what rebuilding config.Config from the form
// alone did.
func TestSettingsSavePreservesTheStampChoice(t *testing.T) {
	cfg := populatedSettings()
	cfg.VisibleStamp = true
	cfg.StampPosition = consent.StampPositionTopLeft
	cfg.LogLevel = "debug"

	state := settingsFormState{
		Action:         "save",
		Locale:         "en",
		OutputSuffix:   "-signed",
		SignatureLevel: "b-t",
	}

	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)

	c := i18n.Load("sr-Latn")
	win, _ := sharedSettingsWindow(t, c, cfg)
	if !handleSettingsAction(win, c, cfg, state) {
		t.Fatal("saving did not close the window")
	}

	saved := readSavedConfig(t)
	if !saved.VisibleStamp {
		t.Error("saving Settings switched the visible stamp off")
	}
	if saved.StampPosition != consent.StampPositionTopLeft {
		t.Errorf("saved stamp position = %q, want top-left", saved.StampPosition)
	}
	if saved.LogLevel != "debug" {
		t.Errorf("saved logLevel = %q, want the configured debug — the form does not show it", saved.LogLevel)
	}
	if saved.SignatureLevel != "b-t" {
		t.Errorf("saved signatureLevel = %q, want the form's b-t", saved.SignatureLevel)
	}
}

// TestConsentOutputExistsScreenShowsBothPaths is Task 4: the choice
// names the file that exists and the name it would save under instead,
// so the decision is made with the answer in view.
func TestConsentOutputExistsScreenShowsBothPaths(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedConsentWindow(t)

	// The window always receives an init before any progress payload —
	// that is what resolves the page's static labels — so the test
	// starts the same way the real flow does.
	postConsentWithStamp(t, win, c, consent.DefaultStampChoice())

	existing := `C:\Users\Veljko\Documents\ugovor-potpisan.pdf`
	rename := `C:\Users\Veljko\Documents\ugovor-potpisan-2.pdf`
	if err := win.PostJSON(consentOutputExistsPayload(existing, rename, c)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	if got := evalString(t, win, "String(document.getElementById('state-outputexists').hidden)"); got != "false" {
		t.Fatal("the output-exists screen is not showing")
	}
	if got := evalString(t, win, "document.getElementById('output-exists-path').textContent"); got != existing {
		t.Errorf("existing path rendered %q, want %q", got, existing)
	}
	renameLabel := evalString(t, win, "document.getElementById('output-rename-btn').textContent")
	if !strings.Contains(renameLabel, "ugovor-potpisan-2.pdf") {
		t.Errorf("the rename button does not name the file it would write: %q", renameLabel)
	}
	if got := evalString(t, win, "document.getElementById('output-overwrite-btn').textContent"); got != c.T("consent.output_exists_overwrite") {
		t.Errorf("overwrite button = %q, want %q", got, c.T("consent.output_exists_overwrite"))
	}
	// Nothing that writes a file is focused first.
	if got := evalString(t, win, "document.activeElement.id"); got != "output-cancel-btn" {
		t.Errorf("initial focus is on %q, want output-cancel-btn", got)
	}

	// Each button records its own choice for Go to read back.
	for _, tc := range []struct{ button, want string }{
		{"output-overwrite-btn", "overwrite"},
		{"output-rename-btn", "rename"},
	} {
		if _, err := win.Eval("document.getElementById('" + tc.button + "').click()"); err != nil {
			t.Fatalf("Eval(click %s): %v", tc.button, err)
		}
		if got := readOutputChoice(win); got != tc.want {
			t.Errorf("%s recorded choice %q, want %q", tc.button, got, tc.want)
		}
	}
}

// TestNextFreeOutputPathCountsFromTwo covers Task 4's "append a numeric
// suffix": the first free name, counting from 2, in the same folder and
// with the same extension.
func TestNextFreeOutputPathCountsFromTwo(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "ugovor-potpisan.pdf")
	if err := os.WriteFile(base, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	want := filepath.Join(dir, "ugovor-potpisan-2.pdf")
	if got := nextFreeOutputPath(base); got != want {
		t.Fatalf("nextFreeOutputPath = %q, want %q", got, want)
	}

	if err := os.WriteFile(want, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want3 := filepath.Join(dir, "ugovor-potpisan-3.pdf")
	if got := nextFreeOutputPath(base); got != want3 {
		t.Errorf("nextFreeOutputPath = %q, want %q", got, want3)
	}
}

// TestOutputExistsIsItsOwnErrorCode is Task 4's other half: the
// condition the owner met as "an unexpected error occurred" now has a
// code and a message of its own, in every locale.
func TestOutputExistsIsItsOwnErrorCode(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "already-there.pdf")
	if err := os.WriteFile(out, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// A real input, because signInteractiveOne now reads the document
	// itself (one at a time, rather than the whole batch up front) and
	// would otherwise report the missing input rather than the existing
	// output — which is a different, also-correct answer to a different
	// question than this test asks.
	in := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(in, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := signInteractiveOne(t.Context(), in, nil, interactiveSignOptions{outPath: out})
	if err == nil {
		t.Fatal("signing over an existing file succeeded; it must refuse")
	}
	if got := codeOfInteractive(err); got != errs.CodeOutputExists {
		t.Fatalf("error code = %q, want %q (never INTERNAL)", got, errs.CodeOutputExists)
	}
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		msg := c.T(i18n.CodeKey(errs.CodeOutputExists))
		if msg == "" || msg == i18n.CodeKey(errs.CodeOutputExists) {
			t.Errorf("locale %s has no message for OUTPUT_EXISTS", locale)
		}
		if msg == c.T(i18n.CodeKey(errs.CodeInternal)) {
			t.Errorf("locale %s still reports an existing file as the unexpected-error message", locale)
		}
	}
}

// collectSettingsState reads the settings form back exactly as
// runSettingsWindow does — through __liroCollectState and Eval's own
// return value — so the test sees the value that would really be
// saved, not one it assembled itself.
func collectSettingsState(t *testing.T, win ui.Window) settingsFormState {
	t.Helper()
	raw, err := win.Eval("window.__liroCollectState()")
	if err != nil {
		t.Fatalf("Eval(__liroCollectState): %v", err)
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		t.Fatalf("decoding the form-state envelope %q: %v", raw, err)
	}
	var state settingsFormState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		t.Fatalf("decoding the form state %q: %v", jsonStr, err)
	}
	return state
}

// readSavedConfig loads the configuration file from wherever
// config.DefaultPath points right now — the test redirects that with
// LOCALAPPDATA, so this reads what the code under test actually wrote.
func readSavedConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		t.Fatalf("loading the saved configuration: %v", err)
	}
	return cfg
}

// TestResolveOutputConflictAppliesTheChoice drives Task 4's whole loop
// against a real window: the file exists, the screen appears, the user
// answers, and the answer decides where the signature goes — and then
// applies to the rest of the batch rather than asking again per
// document. The clicks are Eval-driven, inside the page's own DOM
// (D-094's carve-out); nothing simulates the real cursor.
func TestResolveOutputConflictAppliesTheChoice(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, messages := sharedConsentWindow(t)
	postConsentWithStamp(t, win, c, consent.DefaultStampChoice())

	dir := t.TempDir()
	existing := filepath.Join(dir, "ugovor-potpisan.pdf")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	free := filepath.Join(dir, "aneks-potpisan.pdf")

	// A path with nothing at it never asks anything.
	answer := outputConflictUnanswered
	if path, overwrite, proceed := resolveOutputConflict(win, messages, c, free, false, &answer); path != free || overwrite || !proceed {
		t.Errorf("a free path produced (%q, %v, %v), want (%q, false, true)", path, overwrite, proceed, free)
	}
	if answer != outputConflictUnanswered {
		t.Error("a free path recorded an answer nobody was asked for")
	}

	// --force is the command line having already answered.
	if path, overwrite, proceed := resolveOutputConflict(win, messages, c, existing, true, &answer); path != existing || !overwrite || !proceed {
		t.Errorf("--force produced (%q, %v, %v), want (%q, true, true)", path, overwrite, proceed, existing)
	}

	for _, tc := range []struct {
		name      string
		button    string
		wantPath  string
		overwrite bool
		answer    outputConflictAnswer
	}{
		{"overwrite", "output-overwrite-btn", existing, true, outputConflictOverwrite},
		{"rename", "output-rename-btn", filepath.Join(dir, "ugovor-potpisan-2.pdf"), false, outputConflictRename},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer := outputConflictUnanswered
			type result struct {
				path      string
				overwrite bool
				proceed   bool
			}
			done := make(chan result, 1)
			go func() {
				p, o, ok := resolveOutputConflict(win, messages, c, existing, false, &answer)
				done <- result{p, o, ok}
			}()

			waitForOutputExistsScreen(t, win)
			if _, err := win.Eval("document.getElementById('" + tc.button + "').click()"); err != nil {
				t.Fatalf("Eval(click %s): %v", tc.button, err)
			}

			select {
			case got := <-done:
				if got.path != tc.wantPath || got.overwrite != tc.overwrite || !got.proceed {
					t.Errorf("choice %s produced (%q, %v, %v), want (%q, %v, true)",
						tc.name, got.path, got.overwrite, got.proceed, tc.wantPath, tc.overwrite)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("resolveOutputConflict never returned after the choice was clicked")
			}

			if answer != tc.answer {
				t.Errorf("the answer was not remembered for the batch: %v", answer)
			}
			// The remembered answer settles the next conflicting
			// document without asking again — nothing is posted, and no
			// message is consumed.
			path, overwrite, proceed := resolveOutputConflict(win, messages, c, existing, false, &answer)
			if !proceed || overwrite != tc.overwrite || path != tc.wantPath {
				t.Errorf("second document produced (%q, %v, %v), want the remembered (%q, %v, true)",
					path, overwrite, proceed, tc.wantPath, tc.overwrite)
			}
		})
	}

	// Cancel stops the batch and writes nothing.
	answer = outputConflictUnanswered
	type result struct {
		path      string
		overwrite bool
		proceed   bool
	}
	done := make(chan result, 1)
	go func() {
		p, o, ok := resolveOutputConflict(win, messages, c, existing, false, &answer)
		done <- result{p, o, ok}
	}()
	waitForOutputExistsScreen(t, win)
	if _, err := win.Eval("document.getElementById('output-cancel-btn').click()"); err != nil {
		t.Fatalf("Eval(click cancel): %v", err)
	}
	select {
	case got := <-done:
		if got.proceed || got.overwrite {
			t.Errorf("Cancel produced (%q, %v, %v), want proceed false and overwrite false", got.path, got.overwrite, got.proceed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("resolveOutputConflict never returned after Cancel")
	}
}

// waitForOutputExistsScreen waits for the page to actually show the
// choice — PostJSON and the page's own rendering are asynchronous, so
// clicking before it is up would click a hidden button.
func waitForOutputExistsScreen(t *testing.T, win ui.Window) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if evalString(t, win, "String(document.getElementById('state-outputexists').hidden)") == "false" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the output-exists screen never appeared")
}

// TestResolveInteractiveOutputsSettlesTheWholeBatchBeforeSigning: every
// output path is decided before the card session is opened, and one
// answer covers the batch — so a user who cancels has not entered a PIN
// for a batch that was never going to be saved (the same reasoning that
// moved the timestamp question ahead of the card, D-095).
func TestResolveInteractiveOutputsSettlesTheWholeBatchBeforeSigning(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, messages := sharedConsentWindow(t)
	postConsentWithStamp(t, win, c, consent.DefaultStampChoice())

	dir := t.TempDir()
	inputs := make([]interactiveInput, 0, 3)
	for _, name := range []string{"prvi.pdf", "drugi.pdf", "treci.pdf"} {
		in := filepath.Join(dir, name)
		if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		// Two of the three already have a signed output waiting.
		if name != "treci.pdf" {
			signed := filepath.Join(dir, strings.TrimSuffix(name, ".pdf")+"-potpisan.pdf")
			if err := os.WriteFile(signed, []byte("x"), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}
		inputs = append(inputs, interactiveInput{path: in})
	}

	type outcome struct {
		outputs []interactiveOutput
		ok      bool
	}
	done := make(chan outcome, 1)
	go func() {
		o, ok := resolveInteractiveOutputs(win, messages, c, inputs, "-potpisan", false)
		done <- outcome{o, ok}
	}()

	// Exactly one question, answered once, for two conflicting files.
	waitForOutputExistsScreen(t, win)
	if _, err := win.Eval("document.getElementById('output-rename-btn').click()"); err != nil {
		t.Fatalf("Eval(click rename): %v", err)
	}

	select {
	case got := <-done:
		if !got.ok {
			t.Fatal("resolveInteractiveOutputs reported cancellation after Save-as was chosen")
		}
		if len(got.outputs) != 3 {
			t.Fatalf("settled %d outputs, want 3", len(got.outputs))
		}
		want := []string{
			filepath.Join(dir, "prvi-potpisan-2.pdf"),
			filepath.Join(dir, "drugi-potpisan-2.pdf"),
			filepath.Join(dir, "treci-potpisan.pdf"), // no conflict, untouched
		}
		for i, w := range want {
			if got.outputs[i].path != w {
				t.Errorf("output %d = %q, want %q", i, got.outputs[i].path, w)
			}
			if got.outputs[i].overwrite {
				t.Errorf("output %d may overwrite, though Save-as was chosen", i)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("resolveInteractiveOutputs never returned")
	}
}

// TestPersistStampChoiceWritesTheConfiguration is the other half of
// "persist both choices so they carry to the next run" (Task 1): the
// answer reaches config.json, and an unchanged answer writes nothing.
func TestPersistStampChoiceWritesTheConfiguration(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	cfg := config.Default()
	changed := persistStampChoice(cfg, consent.StampChoice{Visible: false, Position: consent.StampPositionTopRight})
	if changed.VisibleStamp || changed.StampPosition != consent.StampPositionTopRight {
		t.Fatalf("returned configuration = %+v, want the new choice", changed)
	}
	saved := readSavedConfig(t)
	if saved.VisibleStamp {
		t.Error("config.json still says the stamp is visible")
	}
	if saved.StampPosition != consent.StampPositionTopRight {
		t.Errorf("config.json holds position %q, want top-right", saved.StampPosition)
	}

	// An unchanged choice does not rewrite the file: nothing to save,
	// and nothing to lose if the file is momentarily unwritable.
	if err := os.Remove(config.DefaultPath()); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	persistStampChoice(changed, consent.StampChoice{Visible: false, Position: consent.StampPositionTopRight})
	if _, err := os.Stat(config.DefaultPath()); err == nil {
		t.Error("an unchanged stamp choice rewrote config.json")
	}
}
