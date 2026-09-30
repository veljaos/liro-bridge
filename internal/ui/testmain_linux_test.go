//go:build linux

package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

// TestMain answers chooser.Subcommand with a fake helper, so that
// spawnChooser can be tested against a real child process that reports
// exactly what it was given.
//
// The fake is this test binary, not the real helper, and it never spawns
// anything: whatever it is asked, it answers and exits, so no generation
// of it can become another (D-293). The real helper is tested as the real
// binary in cmd/liro-bridge.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == chooser.Subcommand {
		os.Exit(fakeHelper())
	}
	os.Exit(m.Run())
}

// fakeHelper behaves as its request's title says:
//
//   - "report": answers chosen, with Detail a JSON record of its command
//     line, environment and working directory;
//   - "hang:<file>": writes its PID to file and never answers or reads
//     stdin again;
//   - "wait-stdin": answers cancelled once stdin ends;
//   - "garbage": writes a line that is not a Result.
func fakeHelper() int {
	line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
	if err != nil {
		return 3
	}
	var req chooser.Request
	if err := json.Unmarshal(line, &req); err != nil {
		return 3
	}
	switch {
	case req.Title == "report":
		cwd, _ := os.Getwd()
		rec, _ := json.Marshal(fakeReport{Args: os.Args[1:], Env: os.Environ(), Cwd: cwd, Request: req})
		return answer(chooser.Result{Outcome: chooser.OutcomeChosen, Paths: []string{"/chosen.pdf"}, Detail: string(rec)})
	case strings.HasPrefix(req.Title, "hang:"):
		_ = os.WriteFile(strings.TrimPrefix(req.Title, "hang:"), []byte(fmt.Sprint(os.Getpid())), 0o600)
		select {}
	case req.Title == "wait-stdin":
		_, _ = io.Copy(io.Discard, os.Stdin)
		return answer(chooser.Result{Outcome: chooser.OutcomeCancelled})
	case req.Title == "garbage":
		fmt.Println("this is not a result")
		return 0
	}
	return 3
}

type fakeReport struct {
	Args    []string
	Env     []string
	Cwd     string
	Request chooser.Request
}

func answer(r chooser.Result) int {
	b, _ := json.Marshal(r)
	fmt.Println(string(b))
	return 0
}
