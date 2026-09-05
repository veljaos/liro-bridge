// Command genseed regenerates the Trusted List embedded in the binary
// (internal/trust/tsl/seed/TSL-RS.xml).
//
// The seed is the one artefact in this repository that decides which
// certificates the agent will ever call qualified (SPEC §11.1), so it is
// never edited by hand and never copied in without proof. This tool
// fetches (or reads) a candidate list, strips the server's leading UTF-8
// BOM, verifies its XML-DSig signature against the pinned Ministry
// signer certificates (tsl.PinnedSigners), parses it, and only then
// writes it out — printing the sequence number, issue date, provider
// count and SHA-256 it is about to commit to.
//
// Run with no flags to check the committed seed without touching it:
//
//	go run ./scripts/genseed -check
//
// Re-seed from the Ministry, refusing to write anything that does not
// verify or that would move the sequence number backwards:
//
//	go run ./scripts/genseed -write
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// defaultOut is the embedded seed's path, relative to the module root.
const defaultOut = "internal/trust/tsl/seed/TSL-RS.xml"

// utf8BOM is what the Ministry's server prefixes its XML with. It is
// stripped before the bytes are committed: the embedded seed is the
// document, not the transfer encoding of it. Everything downstream
// (encoding/xml, the C14N implementation) treats a BOM as content, so
// leaving it in would change the canonical form.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func main() {
	var (
		from    = flag.String("from", "", "read the candidate list from this file instead of fetching it")
		url     = flag.String("url", tsl.DefaultURL, "fetch the candidate list from this URL")
		out     = flag.String("out", defaultOut, "path of the embedded seed to write")
		check   = flag.Bool("check", false, "report on the committed seed and exit without writing")
		write   = flag.Bool("write", false, "write the candidate list to -out")
		timeout = flag.Duration("timeout", 30*time.Second, "network timeout when fetching")
	)
	flag.Parse()

	if err := run(*from, *url, *out, *check, *write, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "genseed:", err)
		os.Exit(1)
	}
}

func run(from, url, out string, check, write bool, timeout time.Duration) error {
	if check && write {
		return fmt.Errorf("-check and -write are mutually exclusive")
	}

	committed, committedErr := os.ReadFile(out)
	if committedErr == nil {
		fmt.Println("committed seed:")
		if err := describe(committed, "  "); err != nil {
			return fmt.Errorf("the committed seed does not verify: %w", err)
		}
	} else if !check {
		fmt.Printf("committed seed: not readable (%v)\n", committedErr)
	} else {
		return committedErr
	}

	if check {
		return nil
	}

	candidate, err := load(from, url, timeout)
	if err != nil {
		return err
	}
	candidate = stripBOM(candidate)

	fmt.Println("candidate:")
	if err := describe(candidate, "  "); err != nil {
		// Refusing here is the whole point of the tool: an unverified
		// seed is worse than a stale one (SPEC §11.1).
		return fmt.Errorf("the candidate does not verify against the pinned signers, refusing to write: %w", err)
	}

	if committedErr == nil {
		oldList, err := tsl.Parse(committed)
		if err != nil {
			return err
		}
		newList, err := tsl.Parse(candidate)
		if err != nil {
			return err
		}
		if newList.Sequence < oldList.Sequence {
			return fmt.Errorf("candidate sequence %d is older than the committed seed's %d, refusing to write",
				newList.Sequence, oldList.Sequence)
		}
	}

	if !write {
		fmt.Println("\nnothing written (pass -write to replace the seed)")
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, candidate, 0o644); err != nil {
		return err
	}
	fmt.Printf("\nwrote %s (%d bytes)\n", out, len(candidate))
	fmt.Println("update seedSHA256 in internal/trust/tsl/seed.go to the digest above.")
	return nil
}

// load reads the candidate list from a file or fetches it over HTTPS.
func load(from, url string, timeout time.Duration) ([]byte, error) {
	if from != "" {
		return os.ReadFile(from)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	fmt.Printf("\nfetching %s\n", url)
	return tsl.HTTPFetcher(ctx, url)
}

func stripBOM(b []byte) []byte {
	if len(b) >= len(utf8BOM) && string(b[:len(utf8BOM)]) == string(utf8BOM) {
		return b[len(utf8BOM):]
	}
	return b
}

// describe verifies raw against the pinned signers, parses it, and
// prints everything a human needs to decide whether to commit it.
func describe(raw []byte, indent string) error {
	sum := sha256.Sum256(raw)
	fmt.Printf("%ssha256      %s\n", indent, hex.EncodeToString(sum[:]))
	fmt.Printf("%sbytes       %d\n", indent, len(raw))
	fmt.Printf("%sBOM         %t\n", indent, len(raw) >= 3 && string(raw[:3]) == string(utf8BOM))
	fmt.Printf("%sline ending %s\n", indent, lineEnding(raw))

	if err := tsl.Verify(raw); err != nil {
		return err
	}
	fmt.Printf("%ssignature   verified against a pinned Ministry signer\n", indent)

	list, err := tsl.Parse(raw)
	if err != nil {
		return err
	}
	fmt.Printf("%ssequence    %d\n", indent, list.Sequence)
	fmt.Printf("%sissued      %s\n", indent, list.IssuedAt.Format(time.RFC3339))
	fmt.Printf("%sproviders   %d\n", indent, len(list.Providers))
	return nil
}

// lineEnding reports which line terminator the document uses. It exists
// because getting this wrong is exactly how the committed seed's digest
// stopped matching the Ministry's published one — see docs/decisions.md.
func lineEnding(raw []byte) string {
	var cr, lf int
	for i, b := range raw {
		switch b {
		case '\r':
			cr++
		case '\n':
			lf++
			if i > 0 && raw[i-1] == '\r' {
				continue
			}
		}
	}
	switch {
	case cr == 0 && lf == 0:
		return "none"
	case cr == 0:
		return fmt.Sprintf("LF (%d)", lf)
	case cr == lf:
		return fmt.Sprintf("CRLF (%d)", lf)
	default:
		return fmt.Sprintf("mixed (CR %d, LF %d)", cr, lf)
	}
}
