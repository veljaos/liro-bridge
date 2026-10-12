//go:build linux

package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

// The certificate step with a cloud request's application — its origin
// (D-461) — at the longest api accepts, 255 characters, where a paired
// name is at most 120. At the size the signing flow opens it
// (stepCertificateWidth/Height in cmd/liro-bridge, 520x760): the whole
// origin on screen, the buttons on screen, the page not scrolling either way.
// Open item B32.
func TestTheConsentStepShowsALongOriginWhole(t *testing.T) {
	long := "https://" + strings.Repeat("a", 255-len("https://")-len(".rs")) + ".rs"
	w := openRealPage(t, "/pages/consent.html", 520, 760)
	if err := w.PostJSON(map[string]any{
		"type": "init",
		"strings": catalogueStrings(t, "sr-Latn", []string{
			"consent.cancel", "consent.approve", "consent.application_label", "consent.details_toggle",
			"consent.fingerprint_label", "consent.files_label", "consent.select_certificate_prompt", "consent.copy_fingerprint",
		}),
		"model": map[string]any{
			"documentCount":     3,
			"documentCountText": "3 dokumenta",
			"applicationName":   long,
			"fingerprint":       "AB12 CD34 EF56 7890 AB12 CD34 EF56 7890",
			"fingerprintShort":  "AB12 CD34",
			"files":             []string{"ugovor.pdf", "aneks-1.pdf", "aneks-2.pdf"},
			"certificates":      []any{},
			"certNoticeText":    "Lista sertifikata se još učitava.",
		},
	}); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	raw, err := w.Eval(`(function () {
  var out = [];
  var vh = document.documentElement.clientHeight, vw = document.documentElement.clientWidth;
  var name = document.getElementById("application-name");
  if (name.textContent.length !== 255) out.push("the name holds " + name.textContent.length + " characters");
  var r = name.getBoundingClientRect();
  if (r.bottom > vh + 1 || r.top < -1) out.push("the name runs " + Math.round(r.top) + ".." + Math.round(r.bottom) + " in " + vh);
  if (r.right > vw + 1) out.push("the name reaches past the window: " + Math.round(r.right) + " of " + vw);
  if (name.scrollHeight > name.clientHeight + 1) out.push("the name is clipped");
  ["cancel-btn", "approve-btn"].forEach(function (id) {
    var b = document.getElementById(id).getBoundingClientRect();
    if (b.bottom > vh + 1) out.push(id + " ends at " + Math.round(b.bottom) + " of " + vh);
  });
  if (document.documentElement.scrollWidth > vw + 1) out.push("the page scrolls sideways");
  if (document.documentElement.scrollHeight > vh + 1) out.push("the page scrolls: " + document.documentElement.scrollHeight + " in " + vh);
  return JSON.stringify({problems: out, nameHeight: Math.round(r.height), vh: vh, approveBottom: Math.round(document.getElementById("approve-btn").getBoundingClientRect().bottom), scrollH: document.documentElement.scrollHeight});
})()`)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	var inner string
	var res struct {
		Problems      []string `json:"problems"`
		NameHeight    int      `json:"nameHeight"`
		VH            int      `json:"vh"`
		ApproveBottom int      `json:"approveBottom"`
		ScrollH       int      `json:"scrollH"`
	}
	if err := json.Unmarshal([]byte(raw), &inner); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	if err := json.Unmarshal([]byte(inner), &res); err != nil {
		t.Fatalf("%s: %v", inner, err)
	}
	t.Logf("the 255-character origin takes %d points of height; the page is %d high (scrolls to %d), Approve ends at %d",
		res.NameHeight, res.VH, res.ScrollH, res.ApproveBottom)
	if len(res.Problems) > 0 {
		t.Errorf("the certificate step with a 255-character origin:\n  %s", strings.Join(res.Problems, "\n  "))
	}
}
