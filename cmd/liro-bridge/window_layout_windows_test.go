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
func assertPageDoesNotScroll(t *testing.T, win ui.Window, window, scrollSelector string, alsoAllowed ...string) {
	t.Helper()
	for _, el := range []string{"document.documentElement", "document.body"} {
		scrollH := evalNumber(t, win, el+".scrollHeight")
		clientH := evalNumber(t, win, el+".clientHeight")
		if scrollH > clientH {
			t.Errorf("%s: %s scrollHeight %v exceeds clientHeight %v — the page itself scrolls", window, el, scrollH, clientH)
		}
		scrollW := evalNumber(t, win, el+".scrollWidth")
		clientW := evalNumber(t, win, el+".clientWidth")
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
func withStepHeader(payload map[string]any) map[string]any {
	m := newMainWindow(config.Default(), "sr-Cyrl")
	payload["step"] = m.headerFor(stepCertificate)
	return payload
}

func TestConsentWindowFitsWithSixCertificates(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	win, _ := sharedConsentWindow(t)

	vm := consent.BuildViewModel(consent.ApplicationLocal,
		[][]byte{{1}, {2}, {3}}, []string{"ugovor.pdf", "aneks.pdf", "izjava.pdf"}, sixCertificates())
	if err := win.PostJSON(withStepHeader(buildConsentInit(c, vm))); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	assertPageDoesNotScroll(t, win, "consent (waiting)", "#cert-list")
	assertButtonsVisible(t, win, "consent (waiting)", "#cert-list")
	assertScrolls(t, win, "consent (waiting)", "#cert-list")

	// With the Details disclosure open the fixed content grows; the
	// certificate list must give way rather than the buttons.
	openConsentDetails(t, win)
	assertPageDoesNotScroll(t, win, "consent (waiting, details open)", "#cert-list", "#file-list")
	assertButtonsVisible(t, win, "consent (waiting, details open)", "#cert-list")
}

// TestConsentWindowFitsWithTenLongFileNames is SPEC §6.6's own reason
// for capping the visible file list at ten names: "so a long name
// cannot push the Approve/Cancel buttons off screen". Ten names, each
// at the 120-character display cap, with the Details section open and
// six certificates behind it.
func TestConsentWindowFitsWithTenLongFileNames(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	win, _ := sharedConsentWindow(t)

	digests := make([][]byte, 0, 14)
	names := make([]string, 0, 14)
	for i := 0; i < 14; i++ {
		digests = append(digests, []byte{byte(i)})
		names = append(names, strings.Repeat("ugovor-o-poslovnoj-saradnji-", 6)+"0.pdf")
	}
	vm := consent.BuildViewModel(consent.ApplicationLocal, digests, names, sixCertificates())
	if err := win.PostJSON(withStepHeader(buildConsentInit(c, vm))); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	openConsentDetails(t, win)

	assertPageDoesNotScroll(t, win, "consent (ten long file names)", "#cert-list", "#file-list")
	assertButtonsVisible(t, win, "consent (ten long file names)", "#cert-list")
}

// TestTheQuestionsAfterApprovalFitToo covers the three screens a batch
// passes through between the approval and the first signature — each
// has its own actions row, and each is now on the page that carries the
// progress and the report rather than on a window of its own.
func TestTheQuestionsAfterApprovalFitToo(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	m, _ := testMainWindow(t, "sr-Cyrl", config.Default(), nil)

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
			assertPageDoesNotScroll(t, m.win, "signing window ("+tc.name+")", "#file-list")
			assertButtonsVisible(t, m.win, "signing window ("+tc.name+")", "#file-list")
		})
	}
}

func TestCertificatesWindowFitsWithSixCertificates(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	win := sharedCertificatesWindow(t)

	if err := win.PostJSON(buildCertificatesInit(c, sixCertificates())); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if got := evalNumber(t, win, "document.querySelectorAll('#cert-list .liro-cert-row').length"); got != 6 {
		t.Fatalf("rendered %v certificate rows, want 6", got)
	}
	assertPageDoesNotScroll(t, win, "certificates", "#cert-list")
	assertButtonsVisible(t, win, "certificates", "#cert-list")
	assertScrolls(t, win, "certificates", "#cert-list")
}

func TestAuditLogWindowFitsWithTwentyEntries(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	win := sharedAuditLogWindow(t)

	if err := win.PostJSON(buildAuditLogInit(c, twentyAuditEntries())); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if got := evalNumber(t, win, "document.querySelectorAll('#entry-list .audit-entry').length"); got != 20 {
		t.Fatalf("rendered %v audit entries, want 20", got)
	}
	assertPageDoesNotScroll(t, win, "audit log", "#entry-list")
	assertButtonsVisible(t, win, "audit log", "#entry-list")
	assertScrolls(t, win, "audit log", "#entry-list")
}

func TestSettingsWindowFitsWithEveryFieldPopulated(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	win, _ := sharedSettingsWindow(t, c, populatedSettings())

	// A status line is part of the fixed content too, and appears
	// exactly when an action has run.
	postWindowStatus(win, fmt.Sprintf(c.T("settings.export_done"), `C:\Users\Veljko\Desktop\Izvoz`), ui.IntentPositive)

	assertPageDoesNotScroll(t, win, "settings", ".settings-form")
	assertButtonsVisible(t, win, "settings", ".settings-form")
	assertScrolls(t, win, "settings", ".settings-form")
}

// TestSettingsLabelsSitAboveTheirInputs is Task 2 measured rather than
// eyeballed: for every labelled field, the label's bottom edge is at or
// above the input's top edge (they are stacked, not side by side), and
// the input spans the field's full width. Serbian is the locale that
// matters here — its labels are the long ones.
func TestSettingsLabelsSitAboveTheirInputs(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	win, _ := sharedSettingsWindow(t, c, populatedSettings())

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
