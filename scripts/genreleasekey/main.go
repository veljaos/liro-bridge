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
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		k := storedKey{
			Role:       role,
			PublicKey:  base64.StdEncoding.EncodeToString(pub),
			PrivateKey: base64.StdEncoding.EncodeToString(priv),
		}
		// The half that is printed and the half that is stored are two
		// different encodings made at two different moments, and the
		// consequence of their disagreeing is not a failed build: it
		// is a public key embedded in every installed agent whose
		// private half nobody has. Check the pair the caller will
		// actually end up with — decoded back out of the two strings
		// about to be written and printed — rather than the values in
		// hand.
		if err := roundTrips(k); err != nil {
			fmt.Fprintf(os.Stderr, "genreleasekey: the %s key does not round-trip: %v\n", role, err)
			fmt.Fprintln(os.Stderr, "genreleasekey: nothing was written.")
			os.Exit(1)
		}
		file.Keys = append(file.Keys, k)
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
	fmt.Println("  1. Paste the public halves above into internal/update/trustedkeys.go.")
	fmt.Println("  2. Put the primary's private half in the LIRO_RELEASE_SIGNING_KEY secret of")
	fmt.Println("     the `release` ENVIRONMENT -- Settings -> Environments -> release ->")
	fmt.Println("     Environment secrets. Not a repository secret: the environment is what")
	fmt.Println("     restricts the key to a release tag, and a repository secret is readable")
	fmt.Println("     from any branch this workflow can be run on.")
	fmt.Println("  3. Move the spare's private half somewhere the build machine is not, and")
	fmt.Println("     never put it in CI until the primary has to be retired. A spare that")
	fmt.Println("     lives beside the primary is not a spare.")
	fmt.Println("  4. Delete this file once both halves are where they belong.")
}

// roundTrips decodes both halves back out of the exact strings that
// will be written to the file and printed to the terminal, signs a
// fixed message with the private one and verifies it with the public
// one, and checks that the public half is the one the private half
// itself derives. It returns an error rather than panicking so the
// caller can say that nothing was written.
func roundTrips(k storedKey) error {
	pubRaw, err := base64.StdEncoding.DecodeString(k.PublicKey)
	if err != nil {
		return fmt.Errorf("the public half is not base64: %w", err)
	}
	privRaw, err := base64.StdEncoding.DecodeString(k.PrivateKey)
	if err != nil {
		return fmt.Errorf("the private half is not base64: %w", err)
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return fmt.Errorf("the public half is %d bytes, want %d", len(pubRaw), ed25519.PublicKeySize)
	}
	if len(privRaw) != ed25519.PrivateKeySize {
		return fmt.Errorf("the private half is %d bytes, want %d", len(privRaw), ed25519.PrivateKeySize)
	}
	pub := ed25519.PublicKey(pubRaw)
	priv := ed25519.PrivateKey(privRaw)

	derived, ok := priv.Public().(ed25519.PublicKey)
	if !ok || !derived.Equal(pub) {
		return errors.New("the printed public half is not the one this private half derives")
	}
	// ed25519.Verify takes (publicKey, message, sig) in that order, not
	// (publicKey, sig, message). Both orders compile, because the last
	// two parameters are []byte, and the wrong one rejects everything —
	// including RFC 8032's own test vector, which is how this was
	// caught here rather than by a key that could not be used.
	msg := []byte("liro-bridge release key self-check")
	if !ed25519.Verify(pub, msg, ed25519.Sign(priv, msg)) {
		return errors.New("a signature made with the private half does not verify against the public half")
	}
	return nil
}

// inRepository is a deliberately blunt check: a path under the working
// directory's own module root. It exists to catch the obvious mistake
// (--out internal/update/key.json), not to be a security boundary.
//
// A path is outside the root exactly when the relative path to it is
// ".." or begins with "../". The earlier form of this test asked
// instead whether the relative path *started* with a dot, which is
// true of every dot-prefixed directory as well: measured, --out
// .github/probe.json wrote a key pair into the repository and
// `git check-ignore` confirmed nothing would have stopped `git add .`
// from staging it. The check must be about leaving the root, not about
// the first character of a name.
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
		// Not expressible relative to the root at all — a different
		// volume, which is outside it by construction.
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}
