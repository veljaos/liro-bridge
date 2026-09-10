// Command verifyrelease checks a built release exactly the way an
// installed agent will: the signature over release.json against the
// public keys the agent embeds, and then every artefact's own digest
// against the manifest that has just verified.
//
// It shares no reasoning with scripts/signrelease — it calls the same
// package the agent calls, which is the point. A release the agent
// would refuse must not reach a release page, and the build is where a
// failure costs nobody an afternoon.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/update"
)

func main() {
	dir := flag.String("dir", "", "the directory holding the release artefacts and release.json (required)")
	flag.Parse()

	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "verifyrelease:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	if dir == "" {
		return fmt.Errorf("--dir is required")
	}
	manifestPath := filepath.Join(dir, "release.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	sigBytes, err := os.ReadFile(manifestPath + ".sig")
	if err != nil {
		return err
	}

	m, err := update.VerifyManifest(manifestBytes, sigBytes, update.TrustedKeys())
	if err != nil {
		return fmt.Errorf("this release is not one the agent would accept: %w", err)
	}
	fmt.Printf("release.json verifies against the keys this agent embeds (version %s)\n", m.Version)

	// Every artefact the manifest names must be there and must hash to
	// what it says. This is the check the agent makes after it
	// downloads one; making it here means a release cannot be published
	// with a manifest that describes a file it does not ship.
	for _, a := range m.Artefacts {
		path := filepath.Join(dir, a.Name)
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s is named in the manifest and is not here: %w", a.Name, err)
		}
		if int64(len(b)) != a.Size {
			return fmt.Errorf("%s is %d bytes, the manifest says %d", a.Name, len(b), a.Size)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != a.SHA256 {
			return fmt.Errorf("%s hashes to %s, the manifest says %s", a.Name, got, a.SHA256)
		}
		fmt.Printf("  %-46s %10d bytes  ok\n", a.Name, a.Size)
	}

	// And an MSI in particular, because that is the one the agent can
	// install: a release with none is one every installed agent will
	// report as available and be unable to act on.
	if _, ok := m.Artefact(update.KindMSI); !ok {
		return fmt.Errorf("this release publishes no MSI, so no installed agent could update to it")
	}
	fmt.Println("every artefact matches its signed digest")
	return nil
}
