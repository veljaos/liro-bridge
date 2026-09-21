package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/veljaos/liro-bridge/internal/jobs"
)

// portOf is the port a test server is listening on.
func portOf(t *testing.T, s *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("parsing %s: %v", s.URL, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("port of %s: %v", s.URL, err)
	}
	return port
}

// TestAnAgentIsRecognisedByWhatItAnswers is F12 §7.1's rule stated as a
// test: a discovery file is a claim and dialing is the measurement.
func TestAnAgentIsRecognisedByWhatItAnswers(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agentVersion":"dev","protocolVersion":2,"minimumClientVersion":"0.0.0"}`))
	}))
	defer agent.Close()

	if !agentAnswers(portOf(t, agent)) {
		t.Error("an agent answering /v2/health was not recognised")
	}
}

// TestSomethingElseOnThePortIsNotAnAgent is the half that a bare TCP
// connect would get wrong.
//
// The port range is small and fixed (SPEC §14), so another program may
// well be holding the port this program used last time. "Something
// accepted a connection" is not "the agent is running", and handing a
// person's request to whatever answered would be worse than starting a
// second agent.
func TestSomethingElseOnThePortIsNotAnAgent(t *testing.T) {
	for _, c := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"a server that 404s everything", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}},
		{"a server that says OK and nothing else", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("OK"))
		}},
		{"a server with no protocol version", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agentVersion":"dev"}`))
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := httptest.NewServer(c.handler)
			defer s.Close()
			if agentAnswers(portOf(t, s)) {
				t.Error("this was taken for a running agent")
			}
		})
	}
}

// TestAPortNobodyIsOnIsStale is the case the Windows defect was: a
// discovery file left behind by a process that did not shut down
// cleanly. D-323 is what that cost.
func TestAPortNobodyIsOnIsStale(t *testing.T) {
	// A port that was open and is not any more: the reliable way to
	// name one nothing is listening on.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	if agentAnswers(port) {
		t.Errorf("port %d has nothing on it and was taken for a running agent", port)
	}
	if agentAnswers(0) {
		t.Error("port 0 was taken for a running agent")
	}
}

// TestAHandoverRequestSurvivesToTheAgent covers the other half of §7.1:
// a second launch leaves something the running agent will find.
func TestAHandoverRequestSurvivesToTheAgent(t *testing.T) {
	box := jobs.NewInbox(t.TempDir())

	if taken, err := box.TakeOpenRequest(); err != nil || taken {
		t.Fatalf("an empty directory reported a request: taken=%v err=%v", taken, err)
	}
	if err := box.RequestOpen(); err != nil {
		t.Fatalf("RequestOpen: %v", err)
	}
	// Twice, because two launches while the agent has not yet looked
	// are one window rather than two.
	if err := box.RequestOpen(); err != nil {
		t.Fatalf("RequestOpen twice: %v", err)
	}
	taken, err := box.TakeOpenRequest()
	if err != nil {
		t.Fatalf("TakeOpenRequest: %v", err)
	}
	if !taken {
		t.Fatal("the agent did not find the request a launch left for it")
	}
	if again, err := box.TakeOpenRequest(); err != nil || again {
		t.Errorf("the request was still there after being taken: taken=%v err=%v", again, err)
	}
}
