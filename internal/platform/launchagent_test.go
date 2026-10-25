package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The plist is well-formed XML with the job this program means, an
// executable path with XML's special characters in it is escaped, and a
// relative path is refused.
func TestTheLaunchAgentRunsTheTrayAtLoginAndNeverRestarts(t *testing.T) {
	exe := `/Applications/Liro Bridge & Co <test>.app/Contents/MacOS/liro-bridge`
	b, err := launchAgentPlist(exe)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"<string>/Applications/Liro Bridge &amp; Co &lt;test&gt;.app/Contents/MacOS/liro-bridge</string>",
		"<string>tray</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<false/>",
		"<string>" + LaunchAgentLabel + "</string>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the plist lacks %q:\n%s", want, s)
		}
	}
	if _, err := launchAgentPlist("liro-bridge"); err == nil {
		t.Error("a relative path was accepted")
	}
	// plutil is macOS's own parser; where it exists (CI's macOS job),
	// the file must pass it.
	if plutil, err := exec.LookPath("plutil"); err == nil {
		path := filepath.Join(t.TempDir(), "agent.plist")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(plutil, "-lint", path).CombinedOutput(); err != nil {
			t.Fatalf("plutil -lint: %v\n%s", err, out)
		}
	}
}

func TestTheLaunchAgentSwitchesOnAndOff(t *testing.T) {
	a := launchAgent{path: filepath.Join(t.TempDir(), "LaunchAgents", LaunchAgentLabel+".plist")}
	if on, err := a.IsEnabled(); err != nil || on {
		t.Fatalf("IsEnabled before = %v, %v", on, err)
	}
	if err := a.ensure(true, "/Applications/Liro Bridge.app/Contents/MacOS/liro-bridge"); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.IsEnabled(); !on {
		t.Fatal("not enabled after ensure(true)")
	}
	if err := a.SetEnabled(false, ""); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.IsEnabled(); on {
		t.Fatal("still enabled after SetEnabled(false)")
	}
	if err := a.SetEnabled(false, ""); err != nil {
		t.Fatalf("switching off what is off: %v", err)
	}
}
