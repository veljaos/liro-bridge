//go:build linux

package ui

import (
	"os"
	"slices"
	"testing"
)

// fakeEnv is a lookup that distinguishes "set to empty" from "not set",
// which is the distinction the whole of webkitEnvToSet turns on and the
// one os.Getenv cannot express.
func fakeEnv(set map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := set[name]
		return v, ok
	}
}

func namesOf(vars []webkitEnvVar) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.name)
	}
	return out
}

func TestWebKitEnvToSetOnAnUntouchedEnvironment(t *testing.T) {
	got := namesOf(webkitEnvToSet(fakeEnv(nil)))
	want := []string{"WEBKIT_DISABLE_DMABUF_RENDERER", "__NV_DISABLE_EXPLICIT_SYNC"}
	if !slices.Equal(got, want) {
		t.Fatalf("webkitEnvToSet(empty) = %v, want %v", got, want)
	}
}

// The case F12 §3.2 names explicitly: a user who has set the variable
// to "0" has turned the workaround off, and must keep it off.
func TestWebKitEnvToSetRespectsAUserValue(t *testing.T) {
	for _, value := range []string{"0", "", "1", "no", "  "} {
		t.Run("value="+value, func(t *testing.T) {
			env := map[string]string{"WEBKIT_DISABLE_DMABUF_RENDERER": value}
			got := namesOf(webkitEnvToSet(fakeEnv(env)))
			want := []string{"__NV_DISABLE_EXPLICIT_SYNC"}
			if !slices.Equal(got, want) {
				t.Fatalf("with DMABUF=%q: got %v, want %v", value, got, want)
			}
		})
	}
}

func TestWebKitEnvToSetOnAFullyConfiguredEnvironment(t *testing.T) {
	env := map[string]string{
		"WEBKIT_DISABLE_DMABUF_RENDERER": "0",
		"__NV_DISABLE_EXPLICIT_SYNC":     "0",
	}
	if got := webkitEnvToSet(fakeEnv(env)); len(got) != 0 {
		t.Fatalf("webkitEnvToSet(all set) = %v, want none", namesOf(got))
	}
}

// Every variable carries a reason, and no two share one. This is the
// guard the doc comment on webkitEnvVar describes: the two variables
// look interchangeable, and a copied-and-edited third entry that kept
// the first one's reason would be exactly the mistake that makes them
// look interchangeable.
func TestWebKitStartupEnvIsWellFormed(t *testing.T) {
	seenName := map[string]bool{}
	seenWhy := map[string]bool{}
	for _, v := range webkitStartupEnv {
		if v.name == "" || v.value == "" || v.why == "" {
			t.Errorf("incomplete entry: %+v", v)
		}
		if seenName[v.name] {
			t.Errorf("duplicate variable %q", v.name)
		}
		if seenWhy[v.why] {
			t.Errorf("variable %q repeats another's reason: %q", v.name, v.why)
		}
		seenName[v.name] = true
		seenWhy[v.why] = true
	}
}

func TestPrepareWebKitEnvironmentSetsWhatIsMissingAndReportsIt(t *testing.T) {
	// t.Setenv both isolates the process environment and fails the test
	// if it is ever run in parallel, which is what we want around a
	// function whose whole job is a process-wide side effect.
	t.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "0")
	// t.Setenv is called for its cleanup rather than its value: it
	// records whether the variable was present beforehand and restores
	// that exactly, including restoring it to absent. Unsetting it
	// afterwards is what the test actually needs, and doing it this way
	// means the test cannot leave a variable behind on the machine it
	// ran on — which is this phase's "leave every machine as you found
	// it" applied to the process.
	t.Setenv("__NV_DISABLE_EXPLICIT_SYNC", "restored-by-cleanup")
	if err := os.Unsetenv("__NV_DISABLE_EXPLICIT_SYNC"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}

	got := PrepareWebKitEnvironment()
	if !slices.Equal(got, []string{"__NV_DISABLE_EXPLICIT_SYNC"}) {
		t.Fatalf("PrepareWebKitEnvironment() = %v, want [__NV_DISABLE_EXPLICIT_SYNC]", got)
	}
	// The reported name is reported because it was really set...
	if v := os.Getenv("__NV_DISABLE_EXPLICIT_SYNC"); v != "1" {
		t.Errorf("__NV_DISABLE_EXPLICIT_SYNC = %q, want \"1\"", v)
	}
	// ...and the untouched one is really untouched.
	if v := os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER"); v != "0" {
		t.Errorf("WEBKIT_DISABLE_DMABUF_RENDERER = %q, want \"0\" — a user value was overwritten", v)
	}

	// Calling it twice sets nothing the second time: the first call has
	// made every variable present, and presence is the whole test.
	if again := PrepareWebKitEnvironment(); len(again) != 0 {
		t.Errorf("second PrepareWebKitEnvironment() = %v, want none", again)
	}
}
