//go:build linux

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// GDK 4.22.5's arithmetic, worked by hand from gdksettings-wayland.c. 1.1 is
// the case that tells it from GDK 4.14's GSettings path, which gives 108134.
func TestTheDPIIsComputedAsGDKDoes(t *testing.T) {
	for _, c := range []struct {
		factor float64
		want   int32
	}{
		{1.0, 98304}, // the portal's value on Fedora's desktop (D-424, K4)
		{1.25, 122880},
		{1.5, 147456},
		{0.5, 49152},
		{3.0, 294912},
		{1.1, 108133}, // (int)(1.1 × 65536) = 72089; 96 × 72089 / 65536 × 1024 = 108133.5
	} {
		if got := xftDPIFromTextScaling(c.factor); got != c.want {
			t.Errorf("text-scaling-factor %v: gtk-xft-dpi %d, want %d", c.factor, got, c.want)
		}
	}
}

// Only GTK's "no value" is replaced; any value GTK got stays, including a
// nonsensical one, which is not this workaround's to judge.
func TestOnlyAMissingDPIIsSupplied(t *testing.T) {
	for v, want := range map[int]bool{-1: true, 0: false, 98304: false, 122880: false} {
		if got := gtkLacksXftDPI(v); got != want {
			t.Errorf("gtkLacksXftDPI(%d) = %v, want %v", v, got, want)
		}
	}
}

const xftDPIChildEnv = "LIRO_TEST_XFTDPI_CHILD"

// runXftDPIChild runs test name in a child of this test binary with env
// added, and returns what the child printed as "xftdpi <label> <value>"
// lines. GTK is process-wide, so each case gets a process of its own.
func runXftDPIChild(t *testing.T, name, mode string, env ...string) map[string]int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$")
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GSETTINGS_SCHEMA_DIR=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, append(env, xftDPIChildEnv+"="+mode)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 3 {
		t.Skip("no display for GTK")
	}
	if err != nil {
		t.Fatalf("child: %v\nstdout:\n%s\nstderr:\n%s", err, out, stderr.String())
	}
	got := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "xftdpi" {
			v, _ := strconv.Atoi(f[2])
			got[f[1]] = v
		}
	}
	t.Logf("child stderr:\n%s", stderr.String())
	return got
}

// xftDPIChildStart brings GTK up in a child and forces gtk-xft-dpi to -1, the
// state D-424 read on Fedora (K6's shape), whatever this machine's GTK found.
// The thread's own supplyMissingXftDPI has already run by then, against
// whatever GTK had, so the child calls it again on -1.
func xftDPIChildStart() *gtk.Settings {
	if err := theUIThread.start(); err != nil {
		os.Exit(3)
	}
	var s *gtk.Settings
	_ = theUIThread.do(func() {
		s = gtk.SettingsGetDefault()
		s.SetObjectProperty(gtkXftDPIProperty, int32(xftDPIUnset))
		fmt.Printf("xftdpi forced %d\n", s.ObjectProperty(gtkXftDPIProperty).(int))
		desktopInterface = nil
		supplyMissingXftDPI()
		fmt.Printf("xftdpi supplied %d\n", s.ObjectProperty(gtkXftDPIProperty).(int))
	})
	return s
}

// With the schema present: -1 becomes the desktop's value, and a change of the
// desktop's text scaling reaches GTK. GSettings' memory backend stands in for
// dconf, so the person's real setting is neither read nor touched; its
// text-scaling-factor starts at the schema's default, 1.0.
func TestAMissingDPIIsTakenFromTheDesktopAndFollowsIt(t *testing.T) {
	if os.Getenv(xftDPIChildEnv) == "follow" {
		s := xftDPIChildStart()
		changed := make(chan struct{}, 1)
		_ = theUIThread.do(func() {
			s.NotifyProperty(gtkXftDPIProperty, func() {
				select {
				case changed <- struct{}{}:
				default:
				}
			})
			gio.NewSettings(desktopInterfaceSchema).SetDouble(textScalingKey, 1.25)
		})
		select {
		case <-changed:
		case <-time.After(5 * time.Second):
		}
		_ = theUIThread.do(func() {
			fmt.Printf("xftdpi followed %d\n", s.ObjectProperty(gtkXftDPIProperty).(int))
		})
		os.Exit(0)
	}
	got := runXftDPIChild(t, "TestAMissingDPIIsTakenFromTheDesktopAndFollowsIt", "follow",
		"GSETTINGS_BACKEND=memory")
	if got["forced"] != xftDPIUnset {
		t.Fatalf("the child could not force gtk-xft-dpi to -1: %v", got)
	}
	if got["supplied"] != 98304 {
		t.Errorf("gtk-xft-dpi after supplyMissingXftDPI = %d, want 98304 (text-scaling-factor 1.0)", got["supplied"])
	}
	if got["followed"] != 122880 {
		t.Errorf("gtk-xft-dpi after the desktop's factor became 1.25 = %d, want 122880", got["followed"])
	}
}

// With no schemas installed at all: -1 stays, and the process is not aborted
// by g_settings_new on a schema that does not exist.
func TestWithoutTheSchemaTheDPIStaysMissing(t *testing.T) {
	if os.Getenv(xftDPIChildEnv) == "noschema" {
		xftDPIChildStart()
		os.Exit(0)
	}
	empty := t.TempDir()
	got := runXftDPIChild(t, "TestWithoutTheSchemaTheDPIStaysMissing", "noschema",
		"GSETTINGS_BACKEND=memory", "XDG_DATA_DIRS="+empty, "XDG_DATA_HOME="+empty)
	if got["forced"] != xftDPIUnset {
		t.Fatalf("the child could not force gtk-xft-dpi to -1: %v", got)
	}
	if got["supplied"] != xftDPIUnset {
		t.Errorf("gtk-xft-dpi with no schema = %d, want it left at -1", got["supplied"])
	}
}
