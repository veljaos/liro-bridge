package main

// The cloud channel's agent side (docs/phases/F14.md, [[D-461]]): the
// open-link command a liro-bridge:// link runs, the queue the running
// agent takes links from, the window that asks whether a server may send
// requests, and what the person is told when a link comes to nothing.
//
// internal/cloud does the protocol and decides nothing a window would
// show; this file is the windows.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/cloud"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The origin window's size, measured under Xvfb on WebKitGTK 2.52.6 in
// all three locales by TestTheOriginWindowFitsItsSize (internal/ui),
// which asks that the buttons be on screen, the page not scroll, and the
// whole origin be visible — it is the one thing the person is deciding
// about. The longest origin api accepts (255 characters) needs 354 points
// in English, 372 in Serbian Latin and 390 in Cyrillic; 406 is the last
// plus one --liro-space-4, the slack D-202 chose. An ordinary origin
// needs 244–280, so the window has room to spare for one. Not measured
// in WebView2: Windows' fonts are not these, and that is on the handover.
const (
	originWindowWidth  = pairingWindowWidth
	originWindowHeight = 406
)

// originAskTimeout is how long the question waits: the consent window's
// own, for the same reason — a question nobody answers is a refusal, and
// a link clicked and walked away from must not leave a window up for
// ever.
const originAskTimeout = ConsentTimeout

// cloudLinkQueue is how many links can wait behind the one being
// handled. Two clicks in a row are two requests; twenty are a page
// misbehaving, and the ones past the queue are dropped and said so.
const cloudLinkQueue = 8

// cloudOrigins is the person's list of allowed servers, beside the
// pairings (F14 §2).
func cloudOrigins() *cloud.Origins {
	return cloud.NewOrigins(filepath.Dir(platform.DefaultConfigFile()))
}

// jsCloudOrigins is one row per allowed server, as the settings page
// renders them. A list that cannot be read renders as empty, and the
// channel treats it as allowing nothing, so the two agree.
func jsCloudOrigins(o *cloud.Origins) []map[string]any {
	list, err := o.List()
	if err != nil {
		slog.Warn("settings: the list of allowed servers could not be read", "error", err)
	}
	rows := make([]map[string]any, 0, len(list))
	for _, a := range list {
		rows = append(rows, map[string]any{"origin": a.Origin})
	}
	return rows
}

// runOpenLink is `liro-bridge open-link <link>`, what the desktop runs
// for a liro-bridge:// link. It never fetches anything itself: it reads
// the link, leaves it for the agent, and becomes the agent if none is
// running — the agent is the one process with the consent window's slot,
// the job registry and the card.
func runOpenLink(args []string, out io.Writer, cfg config.Config, version string) int {
	c := i18n.Load(cfg.Locale)
	if len(args) != 1 {
		fprintf(out, "usage: liro-bridge %s <liro-bridge://sign?job=https://…>\n", platform.LinkSubcommand)
		return 2
	}
	if _, err := cloud.ParseLink(args[0]); err != nil {
		// The link itself is not logged: it may carry the server's
		// token for the job.
		slog.Warn("open-link: the link could not be used", "error", err)
		tellCloudOutcome(c, cloud.OutcomeLinkUnusable, "")
		fprintf(out, "%s\n", c.T("cloud.outcome_link_unusable"))
		return 2
	}
	box := jobs.NewInbox(shellInboxDir())
	if err := box.AppendLink(args[0]); err != nil {
		slog.Error("open-link: the link could not be left for the agent", "error", err)
		fprintf(out, "%s\n", c.T("startup.handover_failed"))
		return 1
	}
	if _, live := liveAgent(); live {
		slog.Info("open-link: an agent is running, so the link was left for it")
		return 0
	}
	slog.Info("open-link: no agent is running, so this launch becomes it and takes the link")
	return runTray(cfg, version)
}

// cloudLinks is the running agent's half: links in, one at a time,
// through the channel.
type cloudLinks struct {
	queue   chan string
	handle  func(ctx context.Context, link string) cloud.Outcome
	outcome func(o cloud.Outcome, origin string)
}

// newCloudLinks wires the channel to this agent's own signer and job
// registry — the loopback protocol's, so a cloud request and a loopback
// one share one consent window at a time, one audit log and one card.
func newCloudLinks(fallback config.Config, version string, signer *protocolSigner, registry *jobs.Registry) *cloudLinks {
	ch := &cloud.Channel{
		Client:  cloud.NewClient(version),
		Origins: cloudOrigins(),
		AllowOrigin: func(origin string) bool {
			// The question waits for the slot like a consent window: it
			// never opens over a window somebody is reading.
			allowed := false
			signer.holdSlot(func() { allowed = askOrigin(fallback, origin) })
			return allowed
		},
		Signer:          signer,
		Jobs:            registry,
		DocumentSigning: func() bool { return currentConfig(fallback).DocumentSigningEnabled },
	}
	return &cloudLinks{
		queue:  make(chan string, cloudLinkQueue),
		handle: ch.Handle,
		outcome: func(o cloud.Outcome, origin string) {
			tellCloudOutcome(i18n.Load(currentConfig(fallback).Locale), o, origin)
		},
	}
}

// offer queues a link, or drops it when the queue is full.
func (l *cloudLinks) offer(link string) {
	select {
	case l.queue <- link:
	default:
		slog.Warn("cloud: a link was dropped: too many are already waiting", "waiting", cap(l.queue))
	}
}

// run handles links until quit, one at a time.
func (l *cloudLinks) run(quit <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-quit
		cancel()
	}()
	for {
		select {
		case <-quit:
			return
		case link := <-l.queue:
			origin := ""
			if parsed, err := cloud.ParseLink(link); err == nil {
				origin = parsed.Origin
			}
			l.outcome(l.handle(ctx, link), origin)
		}
	}
}

// cloudOutcomeKeys is what the person is told for each way a link can
// end. An outcome with no key is one the person already knows: they
// answered the window, or refused the server themselves.
var cloudOutcomeKeys = map[cloud.Outcome]string{
	cloud.OutcomeLinkUnusable: "cloud.outcome_link_unusable",
	cloud.OutcomeUnreachable:  "cloud.outcome_unreachable",
	cloud.OutcomeJobGone:      "cloud.outcome_job_gone",
	cloud.OutcomeJobRefused:   "cloud.outcome_job_refused",
	cloud.OutcomeBusy:         "cloud.outcome_busy",
	cloud.OutcomeUndelivered:  "cloud.outcome_undelivered",
}

// cloudOutcomeText is the sentence for o, or "" for none.
func cloudOutcomeText(c *i18n.Catalogue, o cloud.Outcome, origin string) string {
	key, ok := cloudOutcomeKeys[o]
	if !ok {
		return ""
	}
	if o == cloud.OutcomeLinkUnusable {
		return c.T(key)
	}
	return fmt.Sprintf(c.T(key), origin)
}

// tellCloudOutcome posts the sentence as a desktop notification. A link
// that came to nothing is otherwise silence — the person clicked, and no
// window came — so this is the one place in the program a notification
// says something other than "look at the window" (SPEC §6.5.2). Best
// effort, like that one: no notification service means a line in the log.
func tellCloudOutcome(c *i18n.Catalogue, o cloud.Outcome, origin string) {
	text := cloudOutcomeText(c, o, origin)
	if text == "" {
		return
	}
	if _, err := newNotifier().Notify(c.T("cloud.notice_summary"), text); err != nil {
		slog.Info("cloud: no desktop notification was posted for a link's outcome", "outcome", int(o), "error", err)
	}
}

// askOrigin is the window of F14 §2: may origin send requests? It names
// the origin and nothing else, and blocks until the person answers;
// closing it, or letting it wait out originAskTimeout, is a refusal.
func askOrigin(fallback config.Config, origin string) bool {
	cfg := currentConfig(fallback)
	c := i18n.Load(cfg.Locale)
	answer := make(chan bool, 1)
	var once sync.Once
	decide := func(v bool) { once.Do(func() { answer <- v }) }

	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("origin.window_title"),
		Width:       originWindowWidth,
		Height:      originWindowHeight,
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/origin.html",
		OnMessage: func(m ui.Message) {
			switch m.Type {
			case ui.MessageTypeApprove:
				decide(true)
			case ui.MessageTypeCancel:
				decide(false)
			}
		},
		OnClosed: func() { decide(false) },
	})
	if err != nil {
		slog.Warn("cloud: the window asking about a server could not open", "error", err)
		return false
	}
	defer func() { _ = win.Close() }()
	if err := win.PostJSON(buildOriginInit(c, origin)); err != nil {
		if !errors.Is(err, ui.ErrWindowClosed) {
			slog.Warn("cloud: the window asking about a server could not be filled", "error", err)
		}
		return false
	}
	select {
	case v := <-answer:
		return v
	case <-time.After(originAskTimeout):
		slog.Info("cloud: nobody answered whether a server may send requests", "origin", origin)
		return false
	}
}

// buildOriginInit is the payload origin.html renders.
func buildOriginInit(c *i18n.Catalogue, origin string) map[string]any {
	keys := []string{"origin.server_label", "origin.explain", "origin.every_request", "origin.allow", "origin.deny"}
	strs := make(map[string]string, len(keys))
	for _, k := range keys {
		strs[k] = c.T(k)
	}
	return map[string]any{
		"type":    "init",
		"strings": strs,
		"model":   map[string]any{"origin": origin},
	}
}
