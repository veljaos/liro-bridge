//go:build linux

package ui

import (
	"os"
	"sort"
)

// webkitEnvVar is one environment variable WebKitGTK must see at start,
// with the reason it is there. The reason is carried in the code rather
// than left to a decision entry because the two variables look
// interchangeable and are not: they address different failures, and a
// future reader deleting the "wrong" one would find nothing broken on
// any machine that never had the hardware.
type webkitEnvVar struct {
	name  string
	value string
	why   string
}

// webkitStartupEnv is F12 §3.2's two variables.
//
// Neither was measured. This VM has no working GPU driver (D-324: the
// VirtualBox guest reports "VMware: No 3D enabled", so WebKitGTK runs
// on a software renderer and never took the DMABUF path at all), and it
// has no NVIDIA hardware. Both therefore go in on the advice, as §3.2
// directs, and are verified when hardware exists. D-324 records the
// prediction that failed here and why a byte-identical screenshot pair
// on this machine is not evidence that the DMABUF path is benign.
var webkitStartupEnv = []webkitEnvVar{
	{
		name:  "WEBKIT_DISABLE_DMABUF_RENDERER",
		value: "1",
		why:   "DMABUF renderer draws blank windows on KWin, NVIDIA and some Mesa stacks",
	},
	{
		name:  "__NV_DISABLE_EXPLICIT_SYNC",
		value: "1",
		why:   "NVIDIA on Wayland does not start at all without it",
	},
}

// webkitEnvToSet reports which of webkitStartupEnv are not already
// present in the environment lookup gives it, in the order they are
// declared.
//
// **A variable the user has set is left exactly as it is, including
// when they set it to "0" or to the empty string.** §3.2 asks for that
// in those words for the DMABUF variable, and it is the whole point of
// the pair: someone who has hardware this program has never run on must
// be able to turn a workaround off without editing the binary. So the
// test is presence, via lookup's second result, and never the value —
// os.Getenv cannot express the difference between "set to empty" and
// "not set", which is precisely the distinction a person writing
// WEBKIT_DISABLE_DMABUF_RENDERER= is reaching for.
//
// Whether WebKitGTK itself treats an empty value as off is its business
// and is not second-guessed here: this program's job is to supply a
// default, not to overrule an answer it was given.
func webkitEnvToSet(lookup func(string) (string, bool)) []webkitEnvVar {
	var missing []webkitEnvVar
	for _, v := range webkitStartupEnv {
		if _, ok := lookup(v.name); ok {
			continue
		}
		missing = append(missing, v)
	}
	return missing
}

// PrepareWebKitEnvironment sets F12 §3.2's two variables for any that
// the process does not already have, and returns the names it set,
// sorted, so a caller can log what it changed rather than what it
// intended to change.
//
// **It must be called before GTK initialises**, which in practice means
// before the first call into the GTK or WebKitGTK bindings from
// anywhere in the process — both variables are read once, at
// initialisation, and setting them afterwards does nothing silently.
// There is no GTK host in this package yet (F12 §3.1 settled the
// binding in D-327 and nothing is built on it), so nothing calls this
// today; the Linux window host calls it as its first statement when it
// lands.
//
// It is deliberately not an init(). An init() in this package would
// mutate the environment of every process that imports internal/ui —
// including the PKCS#11 worker and every test binary — for a reason
// none of them have, and this project has a standing preference for a
// call you can see over a side effect you cannot. The cost is that the
// window host can forget to call it; the comment above is the guard,
// and the exit checklist is where it gets checked.
func PrepareWebKitEnvironment() []string {
	var set []string
	for _, v := range webkitEnvToSet(os.LookupEnv) {
		// os.Setenv fails only on an invalid name, and both names are
		// compile-time constants in this file. Ignoring it would still
		// be wrong: a failure here means the workaround is not in
		// place, so the name is left out of the returned list and the
		// caller logs the truth.
		if err := os.Setenv(v.name, v.value); err != nil {
			continue
		}
		set = append(set, v.name)
	}
	sort.Strings(set)
	return set
}
