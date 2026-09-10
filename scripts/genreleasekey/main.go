// Command genreleasekey generates the Ed25519 key pair a release is
// signed with (SPEC §15.2: "signed package" means our own signature,
// because there is no code-signing certificate).
//
// It is a developer tool. It is never imported by anything that ships,
// and it prints the public half only — the private half is written to
// a file the caller names, outside this repository, and is never
// echoed, logged or committed.
//
// Usage:
//
//	go run ./scripts/genreleasekey --out <path>            # one key
//	go run ./scripts/genreleasekey --out <path> --pair     # primary + spare
//
// The two-key form is what F10 §4.1 asks for an answer to. Every
// installed agent trusts the keys embedded in the build it was
// installed from and can never be told to stop trusting one; a spare
// public key embedded from the first release is what makes a lost or
// compromised primary recoverable without every user reinstalling by
// hand. The spare's private half must be kept somewhere the build
// machine is not — that is the whole of what makes it a spare.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type keyFile struct {
	Generated time.Time   `json:"generated"`
	Note      string      `json:"note"`
	Keys      []storedKey `json:"keys"`
}

type storedKey struct {
	Role       string `json:"role"`
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

func main() {
	out := flag.String("out", "", "where to write the private key file (required; must not be inside this repository)")
	pair := flag.Bool("pair", false, "generate a primary and a spare rather than one key")
	flag.Parse()

	if *out == "" {
		fmt.Fprintln(os.Stderr, "genreleasekey: --out is required, and it must be a path outside this repository")
		os.Exit(2)
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genreleasekey:", err)
		os.Exit(1)
	}
	if inRepository(abs) {
		fmt.Fprintf(os.Stderr, "genreleasekey: %s is inside this repository; a signing key never goes there\n", abs)
		os.Exit(2)
	}
	if _, err := os.Stat(abs); err == nil {
		fmt.Fprintf(os.Stderr, "genreleasekey: %s already exists; refusing to overwrite a key file\n", abs)
		os.Exit(2)
	}

	roles := []string{"primary"}
	if *pair {
		roles = append(roles, "spare")
	}

	file := keyFile{
		Generated: time.Now().UTC().Truncate(time.Second),
		Note: "Liro Bridge release signing keys. The private halves never go into the repository, " +
			"into a build artefact, or into a log. The primary belongs in the repository's " +
			"LIRO_RELEASE_SIGNING_KEY Actions secret; the spare belongs somewhere the build " +
			"machine is not.",
	}
	for _, role := range roles {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fmt.Fprintln(os.Stderr, "genreleasekey:", err)
			os.Exit(1)
		}
		file.Keys = append(file.Keys, storedKey{
			Role:       role,
			PublicKey:  base64.StdEncoding.EncodeToString(pub),
			PrivateKey: base64.StdEncoding.EncodeToString(priv),
		})
	}

	b, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "genreleasekey:", err)
		os.Exit(1)
	}
	// 0600 on POSIX; on Windows the mode is advisory and the file's
	// protection is the user profile it is written into. The caller is
	// told where it is and what to do with it, which is the part that
	// actually matters.
	if err := os.WriteFile(abs, append(b, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "genreleasekey:", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d private key(s) to %s\n\n", len(file.Keys), abs)
	fmt.Println("Public halves — paste these into internal/update/trustedkeys.go:")
	for _, k := range file.Keys {
		fmt.Printf("  %-8s %s\n", k.Role, k.PublicKey)
	}
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  1. Put the primary's private half in the repository's LIRO_RELEASE_SIGNING_KEY")
	fmt.Println("     Actions secret (Settings -> Secrets and variables -> Actions).")
	fmt.Println("  2. Move the spare's private half somewhere the build machine is not, and")
	fmt.Println("     never put it in CI until the primary has to be retired.")
	fmt.Println("  3. Delete this file once both halves are where they belong.")
}

// inRepository is a deliberately blunt check: a path under the working
// directory's own module root. It exists to catch the obvious mistake
// (--out internal/update/key.json), not to be a security boundary.
func inRepository(abs string) bool {
	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return false
		}
		root = parent
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !filepath.IsAbs(rel) && rel[0] != '.'
}
