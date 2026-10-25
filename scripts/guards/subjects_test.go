// Package guards holds checks over this repository's own source that no
// single package can make for itself. It has no code of its own: the
// tests are the package.
package guards

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// printsSomething matches the calls that put text where a person, a
// terminal, a transcript or a CI log will see it. say( is the probes'
// own printer.
var printsSomething = regexp.MustCompile(`\b(fmt\.(Print|Printf|Println|Fprint|Fprintf|Fprintln|Sprintf)|say|slog\.(Info|Warn|Error|Debug))\(`)

// readsASubject matches a certificate's subject being read off a parsed
// certificate: parsed.Subject, c.Subject.CommonName, and so on. A
// struct literal's `Subject:` field has no dot before it and does not
// match.
var readsASubject = regexp.MustCompile(`\.Subject\b`)

// theMarker lets one line say that the subject it prints belongs to a
// machine. It must say which machine and why, so the exemption is read
// rather than copied.
const theMarker = "subject-ok:"

// TestNoToolInScriptsPrintsACertificateSubject is D59's second half.
//
// The first half was one tool (verifypdf, D-508). This is the rule
// behind it: **a certificate's subject is a person until something says
// otherwise.** On a Serbian identity card it carries the holder's name
// and their JMBG, these tools print to standard output, and they are
// run with a real card in the reader — which is the only condition
// under which any of it leaks. D-480 took certificate names out of
// every message the agent writes; the tools in scripts/ were not
// changed with it, and D-487 is what that cost once a third party's
// number had reached a public repository.
//
// A line that genuinely prints a machine's subject — a timestamp unit,
// a CA — marks itself with theMarker and says which. Nothing else may.
func TestNoToolInScriptsPrintsACertificateSubject(t *testing.T) {
	root := ".."
	var bad []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			if !readsASubject.MatchString(line) || !printsSomething.MatchString(line) {
				continue
			}
			// The marker may sit on the line itself, or anywhere in the
			// comment block directly above it — an explanation worth
			// reading rarely fits on one line, and where in its own
			// comment somebody puts the marker is not a rule worth
			// having.
			marked := strings.Contains(line, theMarker)
			for j := i - 1; j >= 0 && !marked; j-- {
				above := strings.TrimSpace(lines[j])
				if !strings.HasPrefix(above, "//") {
					break
				}
				marked = strings.Contains(above, theMarker)
			}
			if !marked {
				bad = append(bad, filepath.ToSlash(path)+":"+itoa(i+1)+"  "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if len(bad) > 0 {
		t.Errorf("a certificate subject reaches the output in %d place(s). On an identity card\n"+
			"that is a person's name and their JMBG. Withhold it, or mark the line %q and say\n"+
			"which machine's subject it is:\n  %s",
			len(bad), theMarker, strings.Join(bad, "\n  "))
	}
}

// TestTheMarkerIsActuallyUsedSomewhere keeps the rule above honest: a
// guard whose exemption nothing uses has never been exercised, and
// would pass just as happily if the marker were misspelt.
func TestTheMarkerIsActuallyUsedSomewhere(t *testing.T) {
	found := false
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(src), theMarker) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking: %v", err)
	}
	if !found {
		t.Errorf("nothing uses %q, so the exemption in the guard above has never been exercised", theMarker)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
