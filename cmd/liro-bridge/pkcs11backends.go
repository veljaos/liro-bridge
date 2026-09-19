//go:build windows || softtoken

package main

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11/worker"
)

// pkcs11Backends is this process's PKCS#11 modules: discovered once, each held
// open in a child that lives as long as the agent does.
//
// # Why the workers are process-wide and not per signature
//
// A Worker is a child process with a vendor module loaded in it, and
// C_Initialize is the call D-272 measured a real module dying inside. Paying it
// per signature would roll that die every time somebody signs — which the
// worker would absorb, since absorbing it is what the worker is for, but at the
// cost of three process spawns and a visible pause on the unlucky one in a
// hundred. internal/keysource/pkcs11's own LiveModule doc already made this
// argument for the module: *"a session has to survive many calls, and paying
// C_Initialize per call rolls the same dice every time."*
//
// The other half of the reason is a deadline nobody should have to invent.
// keysource.Session.Close takes no context, so a worker closed with its session
// would need a shutdown budget chosen here — and D-297 left the per-request
// deadline deliberately open, as a question with its own justification and its
// own owner. Closing at process exit means the bound is the agent's own
// shutdown, which already exists.
//
// # What it costs, named rather than glossed
//
// One child process per discovered module, alive for the agent's lifetime, each
// holding a vendor DLL loaded. That is not free: a module's DllMain runs in
// that child and stays run, and Nexus's personal64.dll is already known to
// write to standard error from inside it (D-303). The children are cheap, they
// are reaped at exit, and what they are holding is the thing that must not be
// re-initialised. It is a deliberate trade and D-311 is where it is argued.
//
// # Discovery is lazy
//
// Nothing here runs until something needs a certificate. Discovery spawns a
// probe child per candidate (~30 ms each), and paying that on `liro-bridge
// --version` would be a cost with no reader.
type pkcs11Backends struct {
	mu       sync.Mutex
	done     bool
	sources  []worker.Source
	failures []pkcs11.Failure
}

// modules is the agent's set. A package-level value because the lifetime being
// managed is the process's, and threading it through every call site that might
// eventually want a certificate would put a parameter into a dozen signatures
// to express "there is one of these per program".
var modules pkcs11Backends

// ensure discovers the modules on this machine, once, and returns the sources
// and the candidates that were not modules.
//
// configured is the person's own path from their config file and may be empty.
// It comes from there and from nowhere else — see config.PKCS11ModulePath and
// pkcs11.Candidates, which both say so, because neither place is sufficient on
// its own.
//
// The sources come back with no PIN entry attached. The screen belongs to one
// window and one moment and the child does not; callers add theirs with
// WithPINEntry.
func (b *pkcs11Backends) ensure(configured string, log *slog.Logger) ([]worker.Source, []pkcs11.Failure) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done {
		return b.sources, b.failures
	}
	b.done = true

	// The children's standard error goes to the log rather than to this
	// process's, because a vendor module writing from inside DllMain would
	// otherwise print over whatever the agent is showing (D-303).
	b.sources, b.failures = worker.Sources(configured, nil, func(modulePath string) io.Writer {
		return &logWriter{log: log, path: modulePath}
	})

	for _, f := range b.failures {
		// Every one of these is something to log and carry past, never
		// something to stop for (F11 §3). A known path that is simply not
		// installed never reaches here — Candidates only offers paths that
		// exist — so a failure means a file that is there and did not work,
		// which is worth a line.
		log.Info("pkcs11: a module candidate was not usable",
			slog.String("path", f.Candidate.Path),
			slog.String("origin", f.Candidate.Origin.String()),
			slog.String("error", f.Err.Error()))
	}
	for _, s := range b.sources {
		log.Info("pkcs11: module available", slog.String("path", s.ModulePath()))
	}
	return b.sources, b.failures
}

// close shuts every worker down. It is safe to call when nothing was ever
// discovered, which is the ordinary case for a command that never asked for a
// certificate.
func (b *pkcs11Backends) close(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.done {
		return nil
	}
	return worker.CloseAll(ctx, b.sources)
}

// logWriter puts a worker child's standard error into the agent's log, one
// record per write.
//
// A vendor module is not required to be quiet and one of them is measured not
// to be, so this has to go somewhere that is not this process's own standard
// error — where it would print over a running command's output and be attached
// to nothing. Tagged with the module path, because otherwise a line from a
// module says nothing about which module said it.
type logWriter struct {
	log  *slog.Logger
	path string
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.log.Info("pkcs11 worker said", slog.String("module", w.path), slog.String("text", string(p)))
	return len(p), nil
}
