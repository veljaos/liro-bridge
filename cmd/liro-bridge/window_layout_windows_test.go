//go:build windows

package main

// Task 5 (F5 fourth-real-run review): "confirm scrolling, do not assume
// it." Each of the four windows is rendered at its own fixed size with
// realistic content — six certificates, twenty audit entries, every
// settings field populated — and three things are measured in the real
// DOM:
//
//  1. the page itself does not scroll (neither vertically nor
//     horizontally): document.documentElement and document.body both
//     have scrollHeight <= clientHeight;
//  2. only the one region that can genuinely grow — a list, or the
//     settings form — scrolls, and it does scroll when its content
//     overflows, rather than being clipped;
//  3. every action button is fully inside the viewport.
//
// The third is the one that matters most: "a primary action that
// scrolls off screen is the same defect as a button that does nothing."
// A test that only looked at the Go payload would pass while the Save
// button sat below the fold, which is exactly what happened before the
// window was resized last round.
import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// sixCertificates is a realistic list for a machine holding several
// clients' cards (SPEC §14.1's bookkeeper): two usable signing
// certificates, their authentication twins with byte-identical
// subjects (SPEC §11.5), an expired one and one whose card is out of
// the reader. Serbian names, in both scripts, because those are what
// the rows actually have to fit.
func sixCertificates() []classify.Info {
	rows := []struct {
		thumb     string
		name      string
		issuer    string
		purpose   classify.Purpose
		usable    bool
		reason    errs.Code
		qualified bool
	}{
		{"AA11", "ВЕЉКО СТАНОЈЕВИЋ", "MUPCA Sluzbenici 3", classify.PurposeSigning, true, "", true},
		{"AA22", "ВЕЉКО СТАНОЈЕВИЋ", "MUPCA Sluzbenici 3", classify.PurposeAuthentication, false, errs.CodeCertNotUsable, true},
		{"BB11", "Zoran Milovanović", "Halcom CA PO 2", classify.PurposeSigning, true, "", true},
		{"BB22", "Zoran Milovanović", "Halcom CA PO 2", classify.PurposeAuthentication, false, errs.CodeCertNotUsable, true},
		{"CC11", "Redžvel Mešković", "Pošta Srbije CA 1", classify.PurposeSigning, false, errs.CodeCertExpired, true},
		{"DD11", "Milica Đorđević-Petrović", "Pošta Srbije CA 1", classify.PurposeSigning, false, errs.CodeCardNotPresent, true},
	}
	out := make([]classify.Info, 0, len(rows))
	for _, r := range rows {
		qualification := classify.QualificationNotQualified
		if r.qualified {
			qualification = classify.QualificationQualified
		}
		out = append(out, classify.Info{
			Thumbprint:      strings.Repeat(r.thumb, 10),
			Subject:         classify.Subject{DisplayName: r.name},
			IssuerCN:        r.issuer,
			Qualification:   qualification,
			Purpose:         r.purpose,
			Usable:          r.usable,
			NotUsableReason: r.reason,
		})
	}
	return out
}

// twentyAuditEntries is a realistic log: every outcome, both levels
// that get shown, a test-key entry, spread over two days.
func twentyAuditEntries() []audit.Entry {
	outcomes := []audit.Outcome{audit.OutcomeApproved, audit.OutcomeDenied, audit.OutcomeFailed, audit.OutcomePartial}
	levels := []string{"B-LT", "B-T", "B-B", ""}
	entries := make([]audit.Entry, 0, 20)
	start := time.Date(2026, 9, 3, 9, 0, 0, 0, time.Local)
	for i := 0; i < 20; i++ {
		entries = append(entries, audit.Entry{
			Timestamp:     start.Add(time.Duration(i) * 37 * time.Minute),
			Thumbprint:    strings.Repeat("AB12", 10),
			Application:   consent.ApplicationLocal,
			DocumentCount: i%7 + 1,
			Outcome:       outcomes[i%len(outcomes)],
			AchievedLevel: levels[i%len(levels)],
			IsTestKey:     i%9 == 0,
		})
	}
	return entries
}

// populatedSettings fills every field the settings window shows, with
// values of the length real ones have.
func populatedSettings() config.Config {
	cfg := config.Default()
	cfg.Locale = "sr-Cyrl"
	cfg.TSAURL = "https://test-tsa.ca.posta.rs/timestamp2"
	cfg.TSAUser = "Test.Korisnik"
	cfg.TSAPassword = "123456"
	cfg.TSAClientCertPath = `C:\Users\Veljko\Documents\Sertifikati\posta-tsa-client.p12`
	cfg.TSAClientCertPassword = "1234"
	cfg.OutputSuffix = "-potpisan"
	cfg.SignatureLevel = "b-b"
	return cfg
}

// assertPageDoesNotScroll is Task 5's first and second checks for one
// window: the page as a whole is fixed, and the region named by
// scrollSelector is the only one that scrolls.
//
// alsoAllowed names any further region that is allowed to scroll
// inside it. Exactly one exists: the consent window's file list, which
// lives inside the Details disclosure, is capped at ten names by SPEC
// §6.6 for this very reason, and is closed by default — so it can
// never be a second scrollbar competing with the certificate list for
// the same glance.
//
// The four numbers are read in one Eval rather than four, so that the
// pair being compared always comes from one layout. Four Evals are four
// round trips, and anything that relaid the page out between two of
// them — a resize the page had not caught up with yet, most of all
// (see resizeAndSettle) — produced a comparison of a content height
// measured against one viewport with a client height measured against
// another. That is not a layout defect and never was; it is two
// questions asked at two different moments (D-201).
func assertPageDoesNotScroll(t *testing.T, win ui.Window, window, scrollSelector string, alsoAllowed ...string) {
	t.Helper()
	for _, el := range []string{"document.documentElement", "document.body"} {
		box := evalNumbers(t, win, el+".scrollHeight", el+".clientHeight", el+".scrollWidth", el+".clientWidth")
		scrollH, clientH, scrollW, clientW := box[0], box[1], box[2], box[3]
		if scrollH > clientH {
			t.Errorf("%s: %s scrollHeight %v exceeds clientHeight %v — the page itself scrolls", window, el, scrollH, clientH)
		}
		if scrollW > clientW {
			t.Errorf("%s: %s scrollWidth %v exceeds clientWidth %v — the page scrolls sideways", window, el, scrollW, clientW)
		}
	}

	// Every other element is either inside the scrolling region or must
	// not scroll on its own. A second scrollable ancestor is precisely
	// the double-scrollbar defect this test exists to rule out.
	allowed := append([]string{scrollSelector}, alsoAllowed...)
	allowedSelector := strings.Join(allowed, ", ")
	script := "(function(){var bad=[];" +
		"document.querySelectorAll('*').forEach(function(el){" +
		"if(el.scrollHeight - el.clientHeight <= 1) return;" +
		"if(el.matches(" + jsStringLiteral(allowedSelector) + ")) return;" +
		"if(el.closest(" + jsStringLiteral(scrollSelector) + ")) return;" +
		"var s=getComputedStyle(el);" +
		"if(s.overflowY==='auto'||s.overflowY==='scroll') bad.push(el.tagName+'#'+el.id+'.'+el.className);" +
		"});return bad.join(', ');})()"
	if extra := evalString(t, win, script); extra != "" {
		t.Errorf("%s: a second scrollable region alongside %s: %s", window, scrollSelector, extra)
	}
}

// resizeAndSettle resizes win and returns once the page itself reports
// the new viewport — never before, and never after a fixed wait.
//
// Window.Resize returns when the native window has been moved and the
// WebView2 controller's bounds have been set, both on the window's own
// OS thread. The page learns its new size down a different path, and
// that path is not ordered against ExecuteScript, which is what Eval
// uses. So a measurement taken straight after Resize can be answered
// out of the layout as it stood before it. This is not theoretical and
// was not inferred: an atomic snapshot taken immediately after
// Resize(420, 210) on this machine reported window.innerHeight 330,
// document.body.scrollHeight 330 and clientHeight 330 — the whole
// pre-resize viewport, after Resize had returned.
//
// Waiting for the resize to become observable is the fix; waiting for a
// duration is not (D-112). The deadline exists only to turn a window
// that never resizes at all into a failure rather than a hang, and each
// turn of the loop is a real round trip through the page, so nothing
// here spins.
func resizeAndSettle(t *testing.T, win ui.Window, width, height int) {
	t.Helper()
	if err := win.Resize(width, height); err != nil {
		t.Fatalf("Resize(%dx%d): %v", width, height, err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		got := evalNumbers(t, win, "window.innerWidth", "window.innerHeight")
		if int(got[0]) == width && int(got[1]) == height {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("30s after Resize(%dx%d) the page still reports a %vx%v viewport", width, height, got[0], got[1])
		}
	}
}

// assertButtonsVisible is Task 5's third check: every button that is
// not inside the scrolling region is fully inside the viewport.
//
// A button inside the scrolling region is exempt on purpose — it
// scrolls with the content it belongs to, which is what a scrolling
// region is for. The action buttons are deliberately outside it, and
// they are what this asserts on: "a primary action that scrolls off
// screen is the same defect as a button that does nothing."
func assertButtonsVisible(t *testing.T, win ui.Window, window, scrollSelector string) {
	t.Helper()
	script := "(function(){var bad=[];" +
		"document.querySelectorAll('button').forEach(function(b){" +
		"if(b.offsetParent===null) return;" + // not on the visible screen
		"if(b.closest(" + jsStringLiteral(scrollSelector) + ")) return;" +
		"var r=b.getBoundingClientRect();" +
		"if(r.bottom>window.innerHeight+0.5||r.top<-0.5||r.right>window.innerWidth+0.5||r.left<-0.5)" +
		"bad.push(b.id+' at '+Math.round(r.top)+'..'+Math.round(r.bottom)+' of '+window.innerHeight);" +
		"});return bad.join('; ');})()"
	if bad := evalString(t, win, script); bad != "" {
		t.Errorf("%s: button(s) outside the window: %s", window, bad)
	}
}

// assertScrolls proves the scrolling region really is scrollable when
// its content overflows — the other half of "only a list scrolls when
// it genuinely overflows". A region that clips instead of scrolling
// hides content just as effectively as a button below the fold.
func assertScrolls(t *testing.T, win ui.Window, window, selector string) {
	t.Helper()
	scrollH := evalNumber(t, win, "document.querySelector("+jsStringLiteral(selector)+").scrollHeight")
	clientH := evalNumber(t, win, "document.querySelector("+jsStringLiteral(selector)+").clientHeight")
	overflow := evalString(t, win, "getComputedStyle(document.querySelector("+jsStringLiteral(selector)+")).overflowY")
	if overflow != "auto" && overflow != "scroll" {
		t.Errorf("%s: %s has overflow-y %q — it cannot scroll", window, selector, overflow)
	}
	if scrollH <= clientH {
		t.Logf("%s: %s does not overflow at this size (%v <= %v); nothing is hidden", window, selector, scrollH, clientH)
	} else {
		t.Logf("%s: %s scrolls: %v of %v visible", window, selector, clientH, scrollH)
	}
}

// withStepHeader adds the step header the certificate step really
// carries. A layout measured without it is a layout measured on a
// screen nobody sees — which is exactly how the method step came to
// ship four points too short for its own third option.
func withStepHeaderIn(locale string, payload map[string]any) map[string]any {
	m := newMainWindow(config.Default(), locale)
	payload["step"] = m.headerFor(stepCertificate)
	return payload
}

// everyLocale is the three this product ships (SPEC 9.1), always with
// the script subtag.
//
// Measuring only sr-Cyrl was reasonable -- it is usually the longest
// catalogue -- but "usually" is not a property, and FTEST 9 asks for
// every window at every step in all three with realistic content.
// Serbian Latin carries diacritics Cyrillic does not, and English is
// shorter in most places and longer in a few; a layout that holds in one
// is not evidence about the other two. The windows are shared
// (sharedwindow_windows_test.go), so a second and third locale costs one
// PostJSON each, not a second and third WebView2 environment.
var everyLocale = []string{"sr-Latn", "sr-Cyrl", "en"}

func TestConsentWindowFitsWithSixCertificates(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) { consentSixCertificates(t, locale) })
	}
}

func consentSixCertificates(t *testing.T, locale string) {
	t.Helper()
	c := i18n.Load(locale)
	win, _ := sharedConsentWindow(t)

	vm := consent.BuildViewModel(consent.ApplicationLocal,
		[][]byte{{1}, {2}, {3}}, []string{"ugovor.pdf", "aneks.pdf", "izjava.pdf"}, sixCertificates())
	if err := win.PostJSON(withStepHeaderIn(locale, buildConsentInit(c, vm, ""))); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	assertPageDoesNotScroll(t, win, "consent (waiting) "+locale, "#cert-list")
	assertButtonsVisible(t, win, "consent (waiting) "+locale, "#cert-list")
	assertScrolls(t, win, "consent (waiting) "+locale, "#cert-list")

	// With the Details disclosure open the fixed content grows; the
	// certificate list must give way rather than the buttons.
	openConsentDetails(t, win)
	assertPageDoesNotScroll(t, win, "consent (details open) "+locale, "#cert-list", "#file-list")
	assertButtonsVisible(t, win, "consent (details open) "+locale, "#cert-list")
}

// TestConsentWindowFitsWithEveryNoCertificateNotice: the line that says
// why there is nothing to choose (D-236) is a whole sentence where a
// short prompt used to be, and it appears exactly when the certificate
// list is empty — so it is the one case where the fixed region above the
// list grows and the list has nothing to give way with.
//
// Every sentence, in every locale, because the longest of them is not
// the same one in all three.
func TestConsentWindowFitsWithEveryNoCertificateNotice(t *testing.T) {
	notices := []string{
		"error.no_reader",
		"error.card_not_present",
		"error.smart_card_service_down",
		"consent.no_certificate_found",
		"consent.looking_for_certificates",
	}
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win, _ := sharedConsentWindow(t)
			for _, key := range notices {
				vm := consent.BuildViewModel(consent.ApplicationLocal,
					[][]byte{{1}}, []string{"ugovor.pdf"}, nil)
				if err := win.PostJSON(withStepHeaderIn(locale, buildConsentInit(c, vm, c.T(key)))); err != nil {
					t.Fatalf("PostJSON: %v", err)
				}
				where := "consent (" + key + ") " + locale
				assertPageDoesNotScroll(t, win, where, "#cert-list")
				assertButtonsVisible(t, win, where, "#cert-list")

				openConsentDetails(t, win)
				assertPageDoesNotScroll(t, win, where+" details open", "#cert-list", "#file-list")
				assertButtonsVisible(t, win, where+" details open", "#cert-list")
			}
		})
	}
}

// TestConsentWindowFitsWithTenLongFileNames is SPEC §6.6's own reason
// for capping the visible file list at ten names: "so a long name
// cannot push the Approve/Cancel buttons off screen". Ten names, each
// at the 120-character display cap, with the Details section open and
// six certificates behind it.
func TestConsentWindowFitsWithTenLongFileNames(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) { consentTenLongNames(t, locale) })
	}
}

func consentTenLongNames(t *testing.T, locale string) {
	t.Helper()
	c := i18n.Load(locale)
	win, _ := sharedConsentWindow(t)

	digests := make([][]byte, 0, 14)
	names := make([]string, 0, 14)
	for i := 0; i < 14; i++ {
		digests = append(digests, []byte{byte(i)})
		names = append(names, strings.Repeat("ugovor-o-poslovnoj-saradnji-", 6)+"0.pdf")
	}
	vm := consent.BuildViewModel(consent.ApplicationLocal, digests, names, sixCertificates())
	if err := win.PostJSON(withStepHeaderIn(locale, buildConsentInit(c, vm, ""))); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	openConsentDetails(t, win)

	assertPageDoesNotScroll(t, win, "consent (ten long names) "+locale, "#cert-list", "#file-list")
	assertButtonsVisible(t, win, "consent (ten long names) "+locale, "#cert-list")
}

// The application name on the consent screen comes from the pairing
// (SPEC §6.6, D-178), so it has the same length bound the pairing
// window's own name does — consent.MaxDisplayLength, 120 characters —
// and the same shape of risk: a value with no length this window
// controls, in a window with a fixed height. It is checked here for the
// same reason the pairing window's was.
//
// The certificate list is the region that gives way (D-106), and it has
// a 120-point floor, so this is not a foregone conclusion: a name long
// enough to grow the fixed footer past what is left over would push the
// buttons off the bottom.
func TestALongApplicationNameFitsOnTheConsentScreen(t *testing.T) {
	name := consent.TruncateMiddle(
		consent.SanitizeDisplayText(strings.Repeat("Padding ", 12)+"Knjigovodstvo d.o.o. ERP"),
		consent.MaxDisplayLength)
	if got := len([]rune(name)); got != consent.MaxDisplayLength {
		t.Fatalf("the name under test is %d characters, want the full %d", got, consent.MaxDisplayLength)
	}

	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win, _ := sharedConsentWindow(t)
			vm := consent.BuildViewModel(name,
				[][]byte{{1}, {2}, {3}}, []string{"ugovor.pdf", "aneks.pdf", "izjava.pdf"}, sixCertificates())
			if err := win.PostJSON(withStepHeaderIn(locale, buildConsentInit(c, vm, ""))); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}
			if got := evalString(t, win, "document.getElementById('application-name').textContent"); got != name {
				t.Fatalf("the application name renders as %q, want the bound name", got)
			}
			assertPageDoesNotScroll(t, win, "consent (long application name) "+locale, "#cert-list")
			assertButtonsVisible(t, win, "consent (long application name) "+locale, "#cert-list")
			assertNoHorizontalOverflow(t, win, "consent (long application name) "+locale)

			openConsentDetails(t, win)
			assertPageDoesNotScroll(t, win, "consent (long application name, details) "+locale, "#cert-list", "#file-list")
			assertButtonsVisible(t, win, "consent (long application name, details) "+locale, "#cert-list")
		})
	}
}

// assertNoHorizontalOverflow is the other half of "a value with no
// length bound wraps rather than widening its container" (D-096): the
// page-level check above catches a body that scrolls sideways, and this
// catches an element inside it that does.
func assertNoHorizontalOverflow(t *testing.T, win ui.Window, window string) {
	t.Helper()
	script := "(function(){var bad=[];" +
		"document.querySelectorAll('*').forEach(function(el){" +
		"if(el.scrollWidth - el.clientWidth <= 1) return;" +
		"bad.push(el.tagName+'#'+el.id+' '+el.scrollWidth+' of '+el.clientWidth);" +
		"});return bad.join('; ');})()"
	if bad := evalString(t, win, script); bad != "" {
		t.Errorf("%s: element(s) wider than the space they have: %s", window, bad)
	}
}

// TestTheQuestionsAfterApprovalFitToo covers the three screens a batch
// passes through between the approval and the first signature — each
// has its own actions row, and each is now on the page that carries the
// progress and the report rather than on a window of its own.
func TestTheQuestionsAfterApprovalFitToo(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) { questionsAfterApproval(t, locale) })
	}
}

func questionsAfterApproval(t *testing.T, locale string) {
	t.Helper()
	c := i18n.Load(locale)
	m, _ := testMainWindow(t, locale, config.Default(), nil)

	for _, tc := range []struct {
		name    string
		payload map[string]any
	}{
		{"tsaChoice", askTSAChoicePayload(consent.TSAReasonNotConfigured, c)},
		{"outputExists", askOutputExistsPayload(
			`C:\Users\Veljko\Documents\Ugovori\ugovor-potpisan.pdf`,
			`C:\Users\Veljko\Documents\Ugovori\ugovor-potpisan-2.pdf`, c)},
		{"failed", askFailedPayload(c.T("error.output_exists"), fmt.Errorf("output file already exists"), c)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.win.PostJSON(tc.payload); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}
			assertPageDoesNotScroll(t, m.win, "signing window ("+tc.name+") "+locale, "#file-list")
			assertButtonsVisible(t, m.win, "signing window ("+tc.name+") "+locale, "#file-list")
		})
	}
}

func TestCertificatesWindowFitsWithSixCertificates(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win := sharedCertificatesWindow(t)

			if err := win.PostJSON(buildCertificatesInit(c, sixCertificates())); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}
			if got := evalNumber(t, win, "document.querySelectorAll('#cert-list .liro-cert-row').length"); got != 6 {
				t.Fatalf("rendered %v certificate rows, want 6", got)
			}
			assertPageDoesNotScroll(t, win, "certificates "+locale, "#cert-list")
			assertButtonsVisible(t, win, "certificates "+locale, "#cert-list")
			assertScrolls(t, win, "certificates "+locale, "#cert-list")
		})
	}
}

func TestAuditLogWindowFitsWithTwentyEntries(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win := sharedAuditLogWindow(t)

			if err := win.PostJSON(buildAuditLogInit(c, twentyAuditEntries())); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}
			if got := evalNumber(t, win, "document.querySelectorAll('#entry-list .audit-entry').length"); got != 20 {
				t.Fatalf("rendered %v audit entries, want 20", got)
			}
			assertPageDoesNotScroll(t, win, "audit log "+locale, "#entry-list")
			assertButtonsVisible(t, win, "audit log "+locale, "#entry-list")
			assertScrolls(t, win, "audit log "+locale, "#entry-list")
		})
	}
}

func TestSettingsWindowFitsWithEveryFieldPopulated(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) { settingsFitsPopulated(t, locale) })
	}
}

func settingsFitsPopulated(t *testing.T, locale string) {
	t.Helper()
	c := i18n.Load(locale)
	cfg := populatedSettings()
	cfg.Locale = locale
	win, _ := sharedSettingsWindow(t, c, cfg)

	// A status line is part of the fixed content too, and appears
	// exactly when an action has run.
	postWindowStatus(win, fmt.Sprintf(c.T("settings.export_done"), `C:\Users\Veljko\Desktop\Izvoz`), ui.IntentPositive)

	assertPageDoesNotScroll(t, win, "settings "+locale, ".settings-form")
	assertButtonsVisible(t, win, "settings "+locale, ".settings-form")
	assertScrolls(t, win, "settings "+locale, ".settings-form")
}

// TestSettingsLabelsSitAboveTheirInputs is Task 2 measured rather than
// eyeballed: for every labelled field, the label's bottom edge is at or
// above the input's top edge (they are stacked, not side by side), and
// the input spans the field's full width. Serbian is the locale that
// matters here — its labels are the long ones.
func TestSettingsLabelsSitAboveTheirInputs(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) { settingsLabelsStack(t, locale) })
	}
}

func settingsLabelsStack(t *testing.T, locale string) {
	t.Helper()
	c := i18n.Load(locale)
	cfg := populatedSettings()
	cfg.Locale = locale
	win, _ := sharedSettingsWindow(t, c, cfg)

	script := "(function(){var bad=[];" +
		"document.querySelectorAll('.liro-field').forEach(function(f){" +
		"var label=f.querySelector('.liro-field-label');" +
		"var input=f.querySelector('input,select');" +
		"if(!label||!input) return;" +
		"var lr=label.getBoundingClientRect(), ir=input.getBoundingClientRect(), fr=f.getBoundingClientRect();" +
		"if(lr.bottom>ir.top+0.5) bad.push(input.id+': label bottom '+Math.round(lr.bottom)+' below input top '+Math.round(ir.top));" +
		"if(ir.width<fr.width-2) bad.push(input.id+': input width '+Math.round(ir.width)+' < field width '+Math.round(fr.width));" +
		"});return bad.join('; ');})()"
	if bad := evalString(t, win, script); bad != "" {
		t.Errorf("settings fields are not label-above-input: %s", bad)
	}

	// And no label wraps: with the full width to itself, each one fits
	// on a single line. Two lines was the finding.
	script = "(function(){var bad=[];" +
		"document.querySelectorAll('.liro-field-label').forEach(function(l){" +
		"var lines=Math.round(l.getBoundingClientRect().height/parseFloat(getComputedStyle(l).lineHeight));" +
		"if(lines>1) bad.push(l.textContent+' ('+lines+' lines)');" +
		"});return bad.join('; ');})()"
	if bad := evalString(t, win, script); bad != "" {
		t.Errorf("settings labels still wrap onto a second line: %s", bad)
	}
}
