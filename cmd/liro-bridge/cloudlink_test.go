package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/cloud"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// The scheme the agent registers and the one the parser accepts are the
// same string, kept in two packages (see platform.LinkScheme).
func TestTheLinkSchemeIsOneString(t *testing.T) {
	if cloud.Scheme != platform.LinkScheme {
		t.Fatalf("cloud.Scheme %q and platform.LinkScheme %q differ: the desktop would run a link the parser refuses", cloud.Scheme, platform.LinkScheme)
	}
}

// Every way a link can end is either said to the person, in every
// catalogue, naming the server where there is one — or deliberately
// silent because the person already knows (they answered the window,
// or refused the server themselves).
func TestEveryCloudOutcomeIsSaidOrDeliberatelySilent(t *testing.T) {
	silent := map[cloud.Outcome]bool{cloud.OutcomeOriginRefused: true, cloud.OutcomeAnswered: true}
	for o := cloud.OutcomeLinkUnusable; o <= cloud.OutcomeUndelivered; o++ {
		for _, loc := range []string{"en", "sr-Latn", "sr-Cyrl"} {
			c := i18n.Load(loc)
			text := cloudOutcomeText(c, o, "https://erp.example.com")
			if silent[o] {
				if text != "" {
					t.Errorf("outcome %d in %s says %q; it is one the person already knows", o, loc, text)
				}
				continue
			}
			if text == "" || strings.HasPrefix(text, "cloud.") {
				t.Errorf("outcome %d in %s has no sentence (%q)", o, loc, text)
				continue
			}
			if strings.Contains(text, "%!") {
				t.Errorf("outcome %d in %s formats wrongly: %q", o, loc, text)
			}
			if o != cloud.OutcomeLinkUnusable && !strings.Contains(text, "https://erp.example.com") {
				t.Errorf("outcome %d in %s does not name the server: %q", o, loc, text)
			}
		}
	}
}

// A link the parser refuses is said, and is never left for the agent:
// there is nothing an agent could do with it.
func TestOpenLinkRefusesABadLinkAndLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "HOME"} {
		t.Setenv(v, dir)
	}
	n := useFakeNotifier(t)
	var out bytes.Buffer
	code := runOpenLink([]string{"liro-bridge://sign?job=http://erp.example.com/j/1"}, &out, config.Default(), "test")
	if code != 2 {
		t.Fatalf("runOpenLink on an http job URL returned %d, want 2", code)
	}
	if want := i18n.Load(config.Default().Locale).T("cloud.outcome_link_unusable"); n.posted != 1 || n.body != want {
		t.Errorf("the person was told %d time(s), %q; want one notification saying the link cannot be used", n.posted, n.body)
	}
	if links, err := newInboxForTest().TakeLinks(); err != nil || len(links) != 0 {
		t.Fatalf("a refused link was left for the agent: %q (err %v)", links, err)
	}
}

// A good link is left for the agent, verbatim.
func TestOpenLinkLeavesAGoodLinkForTheAgent(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_RUNTIME_DIR", "APPDATA", "LOCALAPPDATA", "HOME"} {
		t.Setenv(v, dir)
	}
	box := newInboxForTest()
	const link = "liro-bridge://sign?job=https%3A%2F%2Ferp.example.com%2Fj%2F1%3Ft%3Dsecret"
	if err := box.AppendLink(link); err != nil {
		t.Fatal(err)
	}
	links, err := box.TakeLinks()
	if err != nil || len(links) != 1 || links[0] != link {
		t.Fatalf("TakeLinks = %q (err %v), want the one link", links, err)
	}
}

// Links are handled one at a time, in the order they came, and each
// outcome is reported with the server its link named.
func TestTheAgentHandlesLinksOneAtATimeInOrder(t *testing.T) {
	var (
		mu       sync.Mutex
		inFlight int
		maxIn    int
		order    []string
		told     []string
	)
	release := make(chan struct{})
	done := make(chan struct{}, 3)
	l := &cloudLinks{
		queue: make(chan string, cloudLinkQueue),
		handle: func(_ context.Context, link string) cloud.Outcome {
			mu.Lock()
			inFlight++
			if inFlight > maxIn {
				maxIn = inFlight
			}
			order = append(order, link)
			mu.Unlock()
			<-release
			mu.Lock()
			inFlight--
			mu.Unlock()
			return cloud.OutcomeUnreachable
		},
		outcome: func(_ cloud.Outcome, origin string) {
			mu.Lock()
			told = append(told, origin)
			mu.Unlock()
			done <- struct{}{}
		},
	}
	quit := make(chan struct{})
	defer close(quit)
	go l.run(quit)

	links := []string{
		"liro-bridge://sign?job=https://a.example/j/1",
		"liro-bridge://sign?job=https://b.example/j/2",
		"liro-bridge://sign?job=https://c.example:8443/j/3",
	}
	for _, link := range links {
		l.offer(link)
	}
	for range links {
		// Long enough for a handler that should not have started to
		// start, so the count below sees it.
		time.Sleep(100 * time.Millisecond)
		release <- struct{}{}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a link was never reported")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if maxIn != 1 {
		t.Errorf("%d links were handled at once, want 1", maxIn)
	}
	if strings.Join(order, " ") != strings.Join(links, " ") {
		t.Errorf("handled in the order %q, want %q", order, links)
	}
	want := []string{"https://a.example", "https://b.example", "https://c.example:8443"}
	if strings.Join(told, " ") != strings.Join(want, " ") {
		t.Errorf("outcomes named %q, want %q", told, want)
	}
}

// Past the queue, a link is dropped rather than blocking the watcher
// that takes links and open-requests alike.
func TestAFullQueueDropsALinkRatherThanBlocking(t *testing.T) {
	l := &cloudLinks{queue: make(chan string, 1)}
	l.offer("one")
	finished := make(chan struct{})
	go func() {
		l.offer("two")
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("offer blocked on a full queue")
	}
	if got := <-l.queue; got != "one" || len(l.queue) != 0 {
		t.Fatalf("the queue holds %q and %d more, want only the first link", got, len(l.queue))
	}
}

// The origin window's payload has every string it shows, in every
// catalogue, and the origin as given.
func TestTheOriginWindowHasEveryStringItShows(t *testing.T) {
	for _, loc := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		p := buildOriginInit(i18n.Load(loc), "https://erp.example.com")
		strs := p["strings"].(map[string]string)
		for k, v := range strs {
			if v == "" || v == k {
				t.Errorf("%s: %s has no text", loc, k)
			}
		}
		if got := p["model"].(map[string]any)["origin"]; got != "https://erp.example.com" {
			t.Errorf("%s: the origin is %v", loc, got)
		}
	}
}

func newInboxForTest() *jobs.Inbox { return jobs.NewInbox(shellInboxDir()) }

// The origin window's size is measured in internal/ui, which cannot
// import this package, so the numbers are written twice; this keeps the
// measured ones and the ones the window opens at the same.
func TestTheOriginWindowSizeIsOneNumber(t *testing.T) {
	raw, err := os.ReadFile("../../internal/ui/originlayout_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for name, v := range map[string]int{"originWindowWidth": originWindowWidth, "originWindowHeight": originWindowHeight} {
		if want := fmt.Sprintf("%s = %d", name, v); !strings.Contains(strings.Join(strings.Fields(src), " "), want) {
			t.Errorf("internal/ui's layout test does not measure %q", want)
		}
	}
}

// Where no notification can be posted (Windows posts none), the sentence
// is shown in the native box rather than only logged.
func TestAnOutcomeWithNoNotifierIsShownInABox(t *testing.T) {
	n := useFakeNotifier(t)
	n.err = errors.New("no notifier here")
	var shown string
	original := showCloudNotice
	t.Cleanup(func() { showCloudNotice = original })
	showCloudNotice = func(_, body string) { shown = body }

	c := i18n.Load("en")
	tellCloudOutcome(c, cloud.OutcomeUnreachable, "https://erp.example.com")
	if want := cloudOutcomeText(c, cloud.OutcomeUnreachable, "https://erp.example.com"); shown != want {
		t.Fatalf("the box said %q, want %q", shown, want)
	}
}
