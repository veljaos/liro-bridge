//go:build linux

package chooser

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// These tests run the helper's portal client against a fake portal on a
// private bus of their own. Nothing here touches the desktop's session bus
// or its portal, and the bus's configuration names no service directory, so
// nothing on it can be started by D-Bus activation — a missing portal stays
// missing, which is what the no-portal test needs.

const privateBusConfig = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:path=SOCKET</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

// privateBus starts a dbus-daemon for one test and returns its address. It
// is ended, by its exact PID, when the test ends.
func privateBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("dbus-daemon is not installed; in CI that is a failure, not a skip")
		}
		t.Skip("dbus-daemon is not installed")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "bus.conf")
	body := strings.Replace(privateBusConfig, "SOCKET", filepath.Join(dir, "bus"), 1)
	if err := os.WriteFile(conf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+conf, "--nofork", "--print-address=1")
	// Ended with the test binary even when t.Cleanup does not run, as when
	// go test's timeout ends it: twice while this file was written, a
	// private bus outlived its test that way (D-410).
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("reading the private bus's address: %v", err)
	}
	return strings.TrimSpace(line)
}

// mode is what the fake portal does with a call.
type mode int

const (
	answerWith        mode = iota // Response with code and uris, sent before the call's reply
	answerAfter                   // Response with code and uris, sent after the call's reply
	refuse                        // OpenFile answered AccessDenied, as D-408 measured
	refuseVersion                 // the version read answered AccessDenied
	noInterface                   // the object has no FileChooser: the version read fails
	silent                        // OpenFile never answered
	hold                          // OpenFile answered with a request, and nothing more
	vanishAfterTaking             // OpenFile answered, then the portal leaves the bus
)

type fakePortal struct {
	conn    *dbus.Conn
	mode    mode
	version uint32
	code    uint32
	uris    []string

	mu      sync.Mutex
	parent  string
	title   string
	options map[string]dbus.Variant
	closed  []dbus.ObjectPath
	release chan struct{}
}

// fakePortalOn owns the portal's name on the private bus at address.
func fakePortalOn(t *testing.T, address string, m mode) *fakePortal {
	t.Helper()
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePortal{conn: conn, mode: m, version: 4, release: make(chan struct{})}
	t.Cleanup(func() {
		close(f.release)
		_ = conn.Close()
	})
	if err := conn.Export(portalProps{f}, portalPath, "org.freedesktop.DBus.Properties"); err != nil {
		t.Fatal(err)
	}
	if m != noInterface {
		if err := conn.Export(portalChooser{f}, portalPath, fileChooserFace); err != nil {
			t.Fatal(err)
		}
	}
	reply, err := conn.RequestName(portalName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("owning %s on the private bus: %v, %v", portalName, reply, err)
	}
	return f
}

type portalProps struct{ f *fakePortal }

func (p portalProps) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	switch {
	case p.f.mode == refuseVersion:
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"Portal operation not allowed: Unable to open /proc/1/root"})
	case p.f.mode == noInterface || iface != fileChooserFace:
		// What GDBus answers for an interface the object does not have.
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"No such interface"})
	case prop == "version":
		return dbus.MakeVariant(p.f.version), nil
	}
	return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"No such property"})
}

type portalChooser struct{ f *fakePortal }

func (p portalChooser) OpenFile(sender dbus.Sender, parent, title string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	f := p.f
	f.mu.Lock()
	f.parent, f.title, f.options = parent, title, options
	f.mu.Unlock()

	token, _ := options["handle_token"].Value().(string)
	request := requestPath(string(sender), token)
	respond := func() {
		results := map[string]dbus.Variant{"uris": dbus.MakeVariant(f.uris)}
		_ = f.conn.Emit(request, requestFace+".Response", f.code, results)
	}

	switch f.mode {
	case refuse:
		return "", dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"Portal operation not allowed: Unable to open /proc/1/root"})
	case silent:
		<-f.release
		return "", dbus.NewError("org.freedesktop.DBus.Error.Failed", []any{"test over"})
	case answerWith:
		// Before the reply: the helper has to have subscribed already.
		respond()
	case answerAfter:
		go func() {
			time.Sleep(50 * time.Millisecond)
			respond()
		}()
	case hold:
		_ = f.conn.Export(requestObject{f, request}, request, requestFace)
	case vanishAfterTaking:
		go func() {
			time.Sleep(50 * time.Millisecond)
			_, _ = f.conn.ReleaseName(portalName)
		}()
	}
	return request, nil
}

type requestObject struct {
	f    *fakePortal
	path dbus.ObjectPath
}

func (r requestObject) Close() *dbus.Error {
	r.f.mu.Lock()
	r.f.closed = append(r.f.closed, r.path)
	r.f.mu.Unlock()
	return nil
}

func (f *fakePortal) closedRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.closed)
}

var files = Request{
	Kind:  KindFiles,
	Title: "Izaberite PDF dokumente",
	Filters: []Filter{
		{Name: "PDF dokumenti", MIMETypes: []string{"application/pdf"}, Patterns: []string{"*.pdf"}},
		{Name: "Sve datoteke", Patterns: []string{"*"}},
	},
	Parent: "wayland:abc",
}

var realLimits = limits{call: CallTimeout, ceiling: Ceiling}

// TestARefusedPortalIsARefusalInBoundedTimeNotAHang is D-408's failure, the
// portal answering OpenFile with AccessDenied in about a millisecond, and
// the owner's rule that no portal call waits for ever. GTK 4.22 dropped that
// answer and never called back; the helper must come back with it.
//
// It runs with the real CallTimeout and asserts the kind of outcome rather
// than timing the machine (D-201): a refusal is only reported when the
// refusal arrived, so "refused" is the proof that it came back as an answer
// and not as the bound running out, which would be "timeout".
func TestARefusedPortalIsARefusalInBoundedTimeNotAHang(t *testing.T) {
	address := privateBus(t)
	fakePortalOn(t, address, refuse)

	r := choose(address, files, nil, realLimits)
	if r.Outcome != OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want %q", r.Outcome, r.Detail, OutcomeRefused)
	}
	if !strings.Contains(r.Detail, "AccessDenied") {
		t.Errorf("detail %q does not carry the portal's error", r.Detail)
	}
}

// The version read can be refused too, as xdg-desktop-portal authorises
// property reads the way it does method calls. Still a refusal, still an
// answer.
func TestARefusedVersionReadIsARefusal(t *testing.T) {
	address := privateBus(t)
	fakePortalOn(t, address, refuseVersion)
	if r := choose(address, files, nil, realLimits); r.Outcome != OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want %q", r.Outcome, r.Detail, OutcomeRefused)
	}
}

// A portal that takes the call and never answers it is bounded by the call
// timeout, shortened here.
func TestAPortalThatNeverAnswersTheCallTimesOut(t *testing.T) {
	address := privateBus(t)
	fakePortalOn(t, address, silent)
	r := choose(address, files, nil, limits{call: 300 * time.Millisecond, ceiling: time.Minute})
	if r.Outcome != OutcomeTimeout {
		t.Fatalf("outcome = %q (%s), want %q", r.Outcome, r.Detail, OutcomeTimeout)
	}
}

// Nothing on the bus owns the portal's name, and nothing can be activated.
func TestNoPortalOnTheBusIsNoPortal(t *testing.T) {
	address := privateBus(t)
	r := choose(address, files, nil, realLimits)
	if r.Outcome != OutcomeNoPortal {
		t.Fatalf("outcome = %q (%s), want %q", r.Outcome, r.Detail, OutcomeNoPortal)
	}
}

// A portal with no FileChooser interface is no portal, for this purpose.
func TestAPortalWithNoFileChooserIsNoPortal(t *testing.T) {
	address := privateBus(t)
	fakePortalOn(t, address, noInterface)
	if r := choose(address, files, nil, realLimits); r.Outcome != OutcomeNoPortal {
		t.Fatalf("outcome = %q (%s), want %q", r.Outcome, r.Detail, OutcomeNoPortal)
	}
}

// A folder is not asked of a FileChooser too old to have a folder mode,
// because an old portal ignores the option and returns a file.
func TestAFolderIsNotAskedOfAnOldPortal(t *testing.T) {
	address := privateBus(t)
	f := fakePortalOn(t, address, answerWith)
	f.version = 2
	r := choose(address, Request{Kind: KindFolder, Title: "x"}, nil, realLimits)
	if r.Outcome != OutcomeNoPortal {
		t.Fatalf("outcome = %q (%s), want %q", r.Outcome, r.Detail, OutcomeNoPortal)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.options != nil {
		t.Error("OpenFile was called on a portal with no folder mode")
	}
}

// The person chooses; only local files come back, in order. The Response
// is sent before the call's reply, which is legal and which a helper that
// subscribed after calling would miss for ever.
func TestAChoiceComesBackAsLocalPathsEvenWhenTheAnswerOutrunsTheReply(t *testing.T) {
	for _, m := range []mode{answerWith, answerAfter} {
		address := privateBus(t)
		f := fakePortalOn(t, address, m)
		f.code = 0
		f.uris = []string{
			"file:///home/someone/ugovor.pdf",
			"https://example.invalid/faktura.pdf",
			"file:///tmp/%C4%8Cita%C4%8D/ra%C4%8Dun.pdf",
		}
		r := choose(address, files, nil, realLimits)
		want := []string{"/home/someone/ugovor.pdf", "/tmp/Čitač/račun.pdf"}
		if r.Outcome != OutcomeChosen || !reflect.DeepEqual(r.Paths, want) {
			t.Fatalf("mode %d: result = %+v, want chosen %q", m, r, want)
		}
		if strings.Contains(r.Detail, "example.invalid") || strings.Contains(r.Detail, "/home") {
			t.Errorf("mode %d: detail %q carries what was chosen; it goes to the log", m, r.Detail)
		}
	}
}

// What the portal is asked: the parent and title as given, several files,
// the PDF filter first and current, and nothing about folders.
//
// The filters' types are read off the wire by a monitor on the private bus,
// not from the fake: godbus's Export re-wraps every variant it hands a method
// in MakeVariant of the decoded value, so the fake sees "aav" for filters
// sent as a(sa(us)). That was this test's first reading, and it was the
// instrument's (D-410).
func TestTheCallCarriesWhatTheWindowAskedFor(t *testing.T) {
	address := privateBus(t)
	f := fakePortalOn(t, address, answerWith)
	f.code = 1
	wire := monitorOpenFile(t, address)
	_ = choose(address, files, nil, realLimits)

	var sent map[string]dbus.Variant
	select {
	case msg := <-wire:
		if len(msg.Body) != 3 {
			t.Fatalf("OpenFile carried %d arguments, want 3", len(msg.Body))
		}
		sent, _ = msg.Body[2].(map[string]dbus.Variant)
	case <-time.After(5 * time.Second):
		t.Fatal("the monitor saw no OpenFile call; it cannot say what was sent")
	}
	if got := sent["filters"].Signature().String(); got != "a(sa(us))" {
		t.Errorf("filters went on the wire as %s, want a(sa(us))", got)
	}
	if got := sent["current_filter"].Signature().String(); got != "(sa(us))" {
		t.Errorf("current_filter went on the wire as %s, want (sa(us))", got)
	}
	if got := sent["current_folder"]; got.Signature().String() != "" && got.Value() != nil {
		t.Errorf("current_folder was sent for a files request with no initial folder: %v", got)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.parent != "wayland:abc" || f.title != files.Title {
		t.Errorf("parent, title = %q, %q", f.parent, f.title)
	}
	if v, _ := f.options["multiple"].Value().(bool); !v {
		t.Error("multiple is not true for files")
	}
	if _, ok := f.options["directory"]; ok {
		t.Error("directory is set for files")
	}
}

// monitorOpenFile makes a connection to the private bus a monitor of
// OpenFile calls and returns what it sees, decoded straight off the wire.
func monitorOpenFile(t *testing.T, address string) <-chan *dbus.Message {
	t.Helper()
	mon, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mon.Close() })
	// Monitor first, then eavesdrop: once Eavesdrop is set, godbus hands
	// every incoming message to that channel, the reply to BecomeMonitor
	// included, and the call waits for ever. That hung this test's first
	// version until go test's own timeout (D-410).
	rule := "type='method_call',interface='" + fileChooserFace + "',member='OpenFile'"
	if err := mon.BusObject().Call("org.freedesktop.DBus.Monitoring.BecomeMonitor", 0, []string{rule}, uint32(0)).Err; err != nil {
		t.Fatalf("becoming a monitor: %v", err)
	}
	all := make(chan *dbus.Message, 64)
	mon.Eavesdrop(all)
	calls := make(chan *dbus.Message, 1)
	go func() {
		for msg := range all {
			if msg.Type == dbus.TypeMethodCall {
				calls <- msg
				return
			}
		}
	}()
	return calls
}

func TestAFolderRequestAsksForAFolderStartingWhereTheWindowSaid(t *testing.T) {
	address := privateBus(t)
	f := fakePortalOn(t, address, answerWith)
	f.code = 0
	f.uris = []string{"file:///home/someone/Potpisani"}
	r := choose(address, Request{Kind: KindFolder, Title: "x", InitialFolder: "/home/someone/Dokumenti"}, nil, realLimits)
	if r.Outcome != OutcomeChosen || len(r.Paths) != 1 || r.Paths[0] != "/home/someone/Potpisani" {
		t.Fatalf("result = %+v", r)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, _ := f.options["directory"].Value().(bool); !v {
		t.Error("directory is not true for a folder")
	}
	if b, _ := f.options["current_folder"].Value().([]byte); string(b) != "/home/someone/Dokumenti\x00" {
		t.Errorf("current_folder = %q, want the path and its NUL", b)
	}
}

func TestACancelledDialogIsACancel(t *testing.T) {
	address := privateBus(t)
	f := fakePortalOn(t, address, answerWith)
	f.code = 1
	if r := choose(address, files, nil, realLimits); r.Outcome != OutcomeCancelled || len(r.Paths) != 0 {
		t.Fatalf("result = %+v, want cancelled", r)
	}
}

// The portal leaving the bus while its dialog is open ends the wait.
func TestThePortalLeavingEndsTheWait(t *testing.T) {
	address := privateBus(t)
	fakePortalOn(t, address, vanishAfterTaking)
	r := choose(address, files, nil, limits{call: CallTimeout, ceiling: time.Minute})
	if r.Outcome != OutcomeError || !strings.Contains(r.Detail, "left the session bus") {
		t.Fatalf("result = %+v, want the portal's departure", r)
	}
}

// The parent saying stop — the window closed — takes the dialog down.
func TestStopClosesTheDialog(t *testing.T) {
	address := privateBus(t)
	f := fakePortalOn(t, address, hold)
	stop := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		close(stop)
	}()
	r := choose(address, files, stop, limits{call: CallTimeout, ceiling: time.Minute})
	if r.Outcome != OutcomeCancelled {
		t.Fatalf("result = %+v, want cancelled", r)
	}
	if f.closedRequests() != 1 {
		t.Errorf("Request.Close was called %d times, want 1", f.closedRequests())
	}
}

// The ceiling, shortened: the dialog is closed and the outcome says why.
func TestTheCeilingClosesTheDialogAndSaysSo(t *testing.T) {
	address := privateBus(t)
	f := fakePortalOn(t, address, hold)
	r := choose(address, files, nil, limits{call: CallTimeout, ceiling: 300 * time.Millisecond})
	if r.Outcome != OutcomeExpired {
		t.Fatalf("result = %+v, want expired", r)
	}
	if f.closedRequests() != 1 {
		t.Errorf("Request.Close was called %d times, want 1", f.closedRequests())
	}
}

// The request's object path follows the portal's documented rule.
func TestRequestPath(t *testing.T) {
	if got := requestPath(":1.234", "liro_ab"); got != "/org/freedesktop/portal/desktop/request/1_234/liro_ab" {
		t.Errorf("requestPath = %s", got)
	}
}

// Both shapes godbus gives an error in are sorted.
func TestCallFailedReadsBothErrorShapes(t *testing.T) {
	denied := dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied", Body: []any{"no"}}
	if r := callFailed("x", denied); r.Outcome != OutcomeRefused {
		t.Errorf("value: %q", r.Outcome)
	}
	if r := callFailed("x", &denied); r.Outcome != OutcomeRefused {
		t.Errorf("pointer: %q", r.Outcome)
	}
	if r := callFailed("x", errors.New("something")); r.Outcome != OutcomeError {
		t.Errorf("plain: %q", r.Outcome)
	}
}
