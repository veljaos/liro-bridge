//go:build linux

package chooser

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	portalName      = "org.freedesktop.portal.Desktop"
	portalPath      = "/org/freedesktop/portal/desktop"
	fileChooserFace = "org.freedesktop.portal.FileChooser"
	requestFace     = "org.freedesktop.portal.Request"

	// fileChooserFolderVersion is the first FileChooser version with the
	// "directory" option. An older portal ignores an option it does not
	// know, so asking it for a folder would show a file dialog and return a
	// file; below this version a folder is not asked for at all.
	fileChooserFolderVersion = 3
)

// limits are the two waits, as values so that tests can shorten them.
type limits struct {
	call    time.Duration
	ceiling time.Duration
}

// portalFilter and portalRule are the portal's filter type, a(sa(us)):
// a name, then rules that are each 0 and a glob or 1 and a MIME type.
type portalFilter struct {
	Name  string
	Rules []portalRule
}

type portalRule struct {
	Kind  uint32
	Value string
}

// pending is a call the portal has taken: the dialog is up, or about to be.
type pending struct {
	conn    *dbus.Conn
	request dbus.ObjectPath
	signals chan *dbus.Signal
}

// choose asks the portal at the session bus address for what req describes
// and waits for the answer, for the parent to say stop (stop is closed), or
// for lim.ceiling.
//
// The two waits are kept apart on purpose (D-410). The call itself is
// bounded by lim.call however it hangs, including inside the D-Bus
// handshake, because it runs in a goroutine the helper can walk away from:
// the helper exits right after, and that ends it. The person's wait is
// bounded by the dialog's own answer, by the portal leaving the bus, by the
// parent, and by the ceiling.
func choose(address string, req Request, stop <-chan struct{}, lim limits) Result {
	type called struct {
		p   *pending
		res Result
	}
	done := make(chan called, 1)
	go func() {
		p, res := call(address, req)
		done <- called{p, res}
	}()

	var p *pending
	select {
	case c := <-done:
		if c.p == nil {
			return c.res
		}
		p = c.p
	case <-time.After(lim.call):
		return Result{Outcome: OutcomeTimeout, Detail: fmt.Sprintf("the portal did not answer the call within %s", lim.call)}
	case <-stop:
		return Result{Outcome: OutcomeCancelled, Detail: "stopped by the parent before the portal answered"}
	}
	defer func() { _ = p.conn.Close() }()
	return wait(p, stop, lim.ceiling)
}

// call connects, checks that a FileChooser is there and new enough, and
// makes the call. A nil *pending comes with the Result that ends the chooser.
func call(address string, req Request) (*pending, Result) {
	// Connect, not SessionBus: the address is the one the parent gave, and
	// SessionBus's fallbacks end in running dbus-launch, which a process
	// that must not start anything has no business reaching.
	conn, err := dbus.Connect(address)
	if err != nil {
		return nil, Result{Outcome: OutcomeError, Detail: "connecting to the session bus: " + err.Error()}
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()

	portal := conn.Object(portalName, portalPath)

	v, err := portal.GetProperty(fileChooserFace + ".version")
	if err != nil {
		return nil, callFailed("reading the FileChooser version", err)
	}
	version, _ := v.Value().(uint32)
	if req.Kind == KindFolder && version < fileChooserFolderVersion {
		return nil, Result{Outcome: OutcomeNoPortal, Detail: fmt.Sprintf("FileChooser version %d has no folder mode", version)}
	}

	token, err := handleToken()
	if err != nil {
		return nil, Result{Outcome: OutcomeError, Detail: err.Error()}
	}
	names := conn.Names()
	if len(names) == 0 {
		return nil, Result{Outcome: OutcomeError, Detail: "the session bus gave this connection no name"}
	}
	predicted := requestPath(names[0], token)

	// Subscribe before calling. The portal may answer the moment the call
	// returns, and a Response sent before the match exists is gone.
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(requestFace),
		dbus.WithMatchMember("Response"),
		dbus.WithMatchObjectPath(predicted),
	); err != nil {
		return nil, Result{Outcome: OutcomeError, Detail: "subscribing to the portal's response: " + err.Error()}
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, portalName),
	); err != nil {
		return nil, Result{Outcome: OutcomeError, Detail: "watching the portal's name: " + err.Error()}
	}

	var handle dbus.ObjectPath
	err = portal.Call(fileChooserFace+".OpenFile", 0, req.Parent, req.Title, options(req, token)).Store(&handle)
	if err != nil {
		return nil, callFailed("calling OpenFile", err)
	}
	if handle != predicted {
		// A portal older than the handle_token convention. The Response
		// could already have been sent to a path nobody was matching; the
		// wait below then ends at the ceiling rather than never.
		if err := conn.AddMatchSignal(
			dbus.WithMatchInterface(requestFace),
			dbus.WithMatchMember("Response"),
			dbus.WithMatchObjectPath(handle),
		); err != nil {
			return nil, Result{Outcome: OutcomeError, Detail: "subscribing to the portal's response: " + err.Error()}
		}
	}

	ok = true
	return &pending{conn: conn, request: handle, signals: signals}, Result{}
}

// wait is the person choosing.
func wait(p *pending, stop <-chan struct{}, ceiling time.Duration) Result {
	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	for {
		select {
		case sig, open := <-p.signals:
			if !open {
				return Result{Outcome: OutcomeError, Detail: "lost the session bus while the dialog was open"}
			}
			if r, done := answer(sig, p.request); done {
				return r
			}
		case <-stop:
			p.close()
			return Result{Outcome: OutcomeCancelled, Detail: "stopped by the parent"}
		case <-timer.C:
			p.close()
			return Result{Outcome: OutcomeExpired, Detail: fmt.Sprintf("the dialog was open for longer than %s and was closed", ceiling)}
		}
	}
}

// answer reads one signal. done is false for a signal that is not the end of
// this chooser.
func answer(sig *dbus.Signal, request dbus.ObjectPath) (Result, bool) {
	switch sig.Name {
	case requestFace + ".Response":
		if sig.Path != request {
			return Result{}, false
		}
		var code uint32
		var results map[string]dbus.Variant
		if err := dbus.Store(sig.Body, &code, &results); err != nil {
			return Result{Outcome: OutcomeError, Detail: "reading the portal's response: " + err.Error()}, true
		}
		switch code {
		case 0:
			return chosen(results), true
		case 1:
			return Result{Outcome: OutcomeCancelled}, true
		default:
			return Result{Outcome: OutcomeError, Detail: fmt.Sprintf("the portal ended the dialog with response %d", code)}, true
		}
	case "org.freedesktop.DBus.NameOwnerChanged":
		var name, old, owner string
		if err := dbus.Store(sig.Body, &name, &old, &owner); err != nil || name != portalName || owner != "" {
			return Result{}, false
		}
		return Result{Outcome: OutcomeError, Detail: "the portal left the session bus while the dialog was open"}, true
	}
	return Result{}, false
}

// close asks the portal to take its dialog down, briefly: the helper is
// about to exit either way.
func (p *pending) close() {
	call := p.conn.Object(portalName, p.request).Go(requestFace+".Close", 0, make(chan *dbus.Call, 1))
	select {
	case <-call.Done:
	case <-time.After(StopGrace / 2):
	}
}

// chosen turns the portal's URIs into local paths. Anything that is not a
// local file is left out and counted; the count, not the URI, goes in Detail.
func chosen(results map[string]dbus.Variant) Result {
	var uris []string
	if v, ok := results["uris"]; ok {
		uris, _ = v.Value().([]string)
	}
	paths, skipped := LocalPaths(uris)
	r := Result{Outcome: OutcomeChosen, Paths: paths}
	if skipped > 0 {
		r.Detail = fmt.Sprintf("%d chosen item(s) had no local path and were left out", skipped)
	}
	if len(paths) == 0 {
		r.Outcome = OutcomeCancelled
	}
	return r
}

// LocalPaths is the local path of each file:// URI, in order, and how many
// were something else. A URI with a host other than localhost is not a file
// on this disk.
//
// Shared with the window's drop (internal/ui), which reads text/uri-list:
// the same rule for the same kind of list, from two places.
func LocalPaths(uris []string) (paths []string, skipped int) {
	for _, s := range uris {
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") || u.Path == "" {
			skipped++
			continue
		}
		paths = append(paths, u.Path)
	}
	return paths, skipped
}

// options is OpenFile's a{sv}.
func options(req Request, token string) map[string]dbus.Variant {
	o := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"modal":        dbus.MakeVariant(true),
	}
	switch req.Kind {
	case KindFolder:
		o["directory"] = dbus.MakeVariant(true)
		o["multiple"] = dbus.MakeVariant(false)
	default:
		o["multiple"] = dbus.MakeVariant(true)
		if len(req.Filters) > 0 {
			filters := make([]portalFilter, 0, len(req.Filters))
			for _, f := range req.Filters {
				pf := portalFilter{Name: f.Name}
				for _, m := range f.MIMETypes {
					pf.Rules = append(pf.Rules, portalRule{Kind: 1, Value: m})
				}
				for _, g := range f.Patterns {
					pf.Rules = append(pf.Rules, portalRule{Kind: 0, Value: g})
				}
				filters = append(filters, pf)
			}
			o["filters"] = dbus.MakeVariant(filters)
			o["current_filter"] = dbus.MakeVariant(filters[0])
		}
	}
	if req.InitialFolder != "" {
		// A byte string with its terminating NUL, which is how the portal
		// takes a path it does not want to guess the encoding of.
		o["current_folder"] = dbus.MakeVariant(append([]byte(req.InitialFolder), 0))
	}
	return o
}

// callFailed sorts an error from the call into the outcome that tells the
// window what to say and the log what happened.
func callFailed(what string, err error) Result {
	// godbus delivers a remote error as a dbus.Error value and builds a
	// *dbus.Error in NewError; either can arrive here.
	var de dbus.Error
	var dp *dbus.Error
	found := errors.As(err, &de)
	if !found && errors.As(err, &dp) && dp != nil {
		de, found = *dp, true
	}
	if found {
		// The error's name as well as its message: godbus's Error() is
		// the message alone, and "AccessDenied" is the part of D-408's
		// refusal that says what it was.
		detail := what + ": " + de.Name + ": " + de.Error()
		switch de.Name {
		case "org.freedesktop.DBus.Error.AccessDenied", "org.freedesktop.portal.Error.NotAllowed":
			return Result{Outcome: OutcomeRefused, Detail: detail}
		case "org.freedesktop.DBus.Error.ServiceUnknown",
			"org.freedesktop.DBus.Error.NameHasNoOwner",
			"org.freedesktop.DBus.Error.UnknownInterface",
			"org.freedesktop.DBus.Error.UnknownMethod",
			"org.freedesktop.DBus.Error.UnknownObject",
			"org.freedesktop.DBus.Error.UnknownProperty",
			// What GDBus answers a Properties.Get for an interface the
			// object does not have.
			"org.freedesktop.DBus.Error.InvalidArgs":
			return Result{Outcome: OutcomeNoPortal, Detail: detail}
		}
		return Result{Outcome: OutcomeError, Detail: detail}
	}
	return Result{Outcome: OutcomeError, Detail: what + ": " + err.Error()}
}

// handleToken is a fresh token for the request's object path.
func handleToken() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("making a request token: %w", err)
	}
	return "liro_" + hex.EncodeToString(b), nil
}

// requestPath is where the portal will put the request for this connection
// and token: the unique name without its colon, dots made underscores.
func requestPath(unique, token string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(unique, ":"), ".", "_")
	return dbus.ObjectPath(portalPath + "/request/" + sender + "/" + token)
}
