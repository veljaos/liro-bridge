// Command signrelease builds a release manifest from the artefacts in
// a directory and signs it with the release private key (SPEC §15.2).
//
// It writes release.json and release.json.sig beside them. The private
// key arrives in an environment variable — never on the command line,
// where every other process running as this user could read it
// (D-224), and never as a file path CI would have to write a secret to
// first.
//
// Usage:
//
//	LIRO_RELEASE_SIGNING_KEY=<base64 private key> \
//	  go run ./scripts/signrelease --dir dist/release --version 1.2.0
//
// Without the variable it exits non-zero and signs nothing. There is
// deliberately no "skip signing" flag: an unsigned release is one no
// agent will ever accept, so producing one quietly is worse than
// failing the build that tried.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/update"
)

// keyEnv is where the private key comes from. Named once, here,
// because the workflow and this tool have to agree on it.
const keyEnv = "LIRO_RELEASE_SIGNING_KEY"

func main() {
	dir := flag.String("dir", "", "the directory holding the release artefacts (required)")
	version := flag.String("version", "", "the released version, without a leading v (required)")
	notes := flag.String("notes", "", "the release page URL (defaults to the tag's own page)")
	flag.Parse()

	if err := run(*dir, *version, *notes); err != nil {
		fmt.Fprintln(os.Stderr, "signrelease:", err)
		os.Exit(1)
	}
}

func run(dir, version, notes string) error {
	if dir == "" || version == "" {
		return fmt.Errorf("--dir and --version are both required")
	}
	if _, err := update.ParseVersion(version); err != nil {
		return err
	}
	if notes == "" {
		notes = "https://github.com/veljaos/liro-bridge/releases/tag/v" + version
	}

	priv, err := privateKeyFromEnv()
	if err != nil {
		return err
	}

	artefacts, err := describe(dir)
	if err != nil {
		return err
	}
	if len(artefacts) == 0 {
		return fmt.Errorf("%s holds no .msi or .exe to sign", dir)
	}

	m := update.Manifest{
		Version: version,
		// Truncated to the second: RFC 3339 with nanoseconds in it
		// would make the manifest's bytes depend on how fast the
		// machine was, which is a needless difference between two
		// otherwise identical builds.
		Released:  time.Now().UTC().Truncate(time.Second),
		NotesURL:  notes,
		Artefacts: artefacts,
	}
	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	manifestBytes = append(manifestBytes, '\n')

	// Parse what was just built before signing it. A manifest this tool
	// produces and the agent then refuses is a release nobody can
	// install, found out by a user rather than by the build.
	if _, err := update.ParseManifest(manifestBytes); err != nil {
		return fmt.Errorf("the manifest this tool just built is not one the agent would accept: %w", err)
	}

	sig, err := update.Sign(priv, manifestBytes)
	if err != nil {
		return err
	}

	// Verify it exactly as the agent will, with the agent's own embedded
	// trust set — which is what catches a key that is not the one this
	// build's binaries were meant to be verified against.
	//
	// Before writing anything, not after. This check used to run after
	// both files were on disk, so signing with a key the agent does not
	// trust left a release.json and a release.json.sig beside the
	// artefacts that verified against nothing — measured, by signing a
	// directory with a freshly generated key: the tool exited 1 and both
	// files were there afterwards. The release workflow happens to catch
	// that one step later, but a tool that fails should not leave its
	// output behind for the next thing along to pick up.
	if _, err := update.VerifyManifest(manifestBytes, []byte(sig), update.TrustedKeys()); err != nil {
		return fmt.Errorf("the signature this tool just built does not verify against the keys the agent embeds: %w", err)
	}

	manifestPath := filepath.Join(dir, "release.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath+".sig", []byte(sig), 0o644); err != nil {
		return err
	}

	fmt.Printf("release.json and release.json.sig written to %s\n", dir)
	for _, a := range artefacts {
		fmt.Printf("  %-40s %10d bytes  %s\n", a.Name, a.Size, a.SHA256)
	}
	fmt.Println("verified against the trust set this agent embeds")
	return nil
}

// privateKeyFromEnv reads and clears the key. Clearing it means a
// child process this tool never starts cannot inherit it either — this
// tool starts none, so it is belt and braces, and it costs one line.
func privateKeyFromEnv() (ed25519.PrivateKey, error) {
	raw := os.Getenv(keyEnv)
	_ = os.Unsetenv(keyEnv)
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%s is not set; a release is not published unsigned", keyEnv)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		// Deliberately does not echo the value.
		return nil, fmt.Errorf("%s is not base64", keyEnv)
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%s decodes to %d bytes, want %d", keyEnv, len(b), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(b), nil
}

// describe hashes every artefact in dir. Only .msi and .exe are
// described: those are the two SPEC §15 says a release publishes, and
// a manifest that listed whatever else happened to be in the directory
// would sign over build leftovers.
func describe(dir string) ([]update.Artefact, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []update.Artefact
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var kind string
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".msi":
			kind = update.KindMSI
		case ".exe":
			kind = update.KindEXE
		default:
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		out = append(out, update.Artefact{
			Name:   e.Name(),
			Kind:   kind,
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(b)),
		})
	}
	return out, nil
}
