//go:build linux

package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// originWindowWidth/Height are cmd/liro-bridge's (cloudlink.go); this
// package cannot import that one, so they are repeated here, and
// TestTheOriginWindowSizeIsOneNumber there reads this file to keep the
// two the same.
const (
	originWindowWidth  = 420
	originWindowHeight = 406
)

// originFits is evaluated in the page: whether both buttons are wholly
// inside the window, the page itself does not scroll, and the explanation
// ends above the buttons.
const originFits = `(function () {
  var out = [];
  var vh = document.documentElement.clientHeight;
  ["deny-btn", "allow-btn"].forEach(function (id) {
    var r = document.getElementById(id).getBoundingClientRect();
    if (r.bottom > vh + 1) out.push(id + " ends at " + Math.round(r.bottom) + " of " + vh);
  });
  if (document.documentElement.scrollHeight > vh + 1) out.push("the page scrolls: " + document.documentElement.scrollHeight + " in " + vh);
  var e = document.querySelector(".explain").getBoundingClientRect();
  var a = document.querySelector(".actions").getBoundingClientRect();
  if (e.bottom > a.top + 1) out.push("the explanation runs into the buttons: " + Math.round(e.bottom) + " past " + Math.round(a.top));
  // The origin is the thing the person is being asked about: all of it
  // is on screen, not scrolled out of its own region.
  var id = document.querySelector(".identity");
  if (id.scrollHeight > id.clientHeight + 1) out.push("the origin is cut to " + id.clientHeight + " of its " + id.scrollHeight + " points");
  return JSON.stringify(out);
})()`

func catalogueStrings(t *testing.T, locale string, keys []string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../i18n/locales/" + locale + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]string
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, k := range keys {
		out[k] = all[k]
	}
	return out
}

// The window that asks whether a server may send requests (F14 §2), at
// the size cmd/liro-bridge opens it, in all three locales, with an
// ordinary origin and with the longest one api accepts: both buttons on
// screen, the page not scrolling, nothing running into them, and the
// whole origin visible. It also
// logs the smallest height at which each case fits, which is how the
// number was chosen.
func TestTheOriginWindowFitsItsSize(t *testing.T) {
	keys := []string{"origin.server_label", "origin.explain", "origin.every_request", "origin.allow", "origin.deny"}
	long := "https://" + strings.Repeat("a", 255-len("https://")-len(".rs")) + ".rs"
	for _, loc := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		for _, origin := range []string{"https://erp.example.com", long} {
			name := fmt.Sprintf("%s/%d", loc, len(origin))
			w := openRealPage(t, "/pages/origin.html", originWindowWidth, originWindowHeight)
			if err := w.PostJSON(map[string]any{
				"type":    "init",
				"strings": catalogueStrings(t, loc, keys),
				"model":   map[string]any{"origin": origin},
			}); err != nil {
				t.Fatalf("%s: PostJSON: %v", name, err)
			}
			check := func() []string {
				raw, err := w.Eval(originFits)
				if err != nil {
					t.Fatalf("%s: Eval: %v", name, err)
				}
				var inner string
				var problems []string
				if err := json.Unmarshal([]byte(raw), &inner); err != nil {
					t.Fatalf("%s: %s: %v", name, raw, err)
				}
				if err := json.Unmarshal([]byte(inner), &problems); err != nil {
					t.Fatalf("%s: %s: %v", name, inner, err)
				}
				return problems
			}
			if problems := check(); len(problems) > 0 {
				t.Errorf("%s at %dx%d:\n  %s", name, originWindowWidth, originWindowHeight, strings.Join(problems, "\n  "))
			}
			smallest := 0
			for h := 160; h <= 600; h += 2 {
				if err := w.Resize(originWindowWidth, h); err != nil {
					t.Fatalf("%s: Resize: %v", name, err)
				}
				// A resize reaches the page later than Resize returns;
				// measuring before it lands measures the old size.
				landed := false
				for i := 0; i < 100 && !landed; i++ {
					got, err := w.Eval("document.documentElement.clientHeight")
					landed = err == nil && got == strconv.Itoa(h)
					if !landed {
						time.Sleep(20 * time.Millisecond)
					}
				}
				if !landed {
					t.Fatalf("%s: the page never became %d points high", name, h)
				}
				if len(check()) == 0 {
					smallest = h
					break
				}
			}
			t.Logf("%s fits from %d points high", name, smallest)
			_ = w.Close()
		}
	}
}
