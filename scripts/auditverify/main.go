// Command auditverify re-verifies an audit log's hash chain against the code in
// this working tree.
//
// It is a developer tool and it is read-only. It exists for one situation: a
// change to audit.Entry's canonical form is a change to what every entry
// already on disk hashes to, so the only honest way to land one is to take a
// copy of a real log and show that the new code still verifies it. A unit test
// over entries the test itself built cannot answer that — it would be checking
// the new code against the new code.
//
//	go run ./scripts/auditverify <directory>
//
// Point it at a COPY. It opens nothing for writing and appends nothing, but a
// tool aimed at the one file whose value is that only signing writes to it
// should be aimed at a copy on principle rather than on inspection of its
// source.
package main

import (
	"fmt"
	"os"

	"github.com/veljaos/liro-bridge/internal/audit"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: auditverify <directory holding the audit files>")
		os.Exit(2)
	}
	store, err := audit.NewStore(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "opening the store:", err)
		os.Exit(1)
	}
	chains, err := store.Chains()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reading the chains:", err)
		os.Exit(1)
	}
	if len(chains) == 0 {
		// Said out loud rather than passing silently: "nothing to verify" and
		// "everything verified" must not look the same from outside.
		fmt.Println("NO CHAINS FOUND. This proves nothing.")
		os.Exit(3)
	}
	result := audit.VerifyChains(chains)
	total := 0
	for _, c := range result.Chains {
		total += c.EntryCount
		status := "OK"
		if !c.Result.OK {
			status = fmt.Sprintf("BROKEN at entry %d", c.Result.BrokenAt)
		}
		if c.TruncatedFile != "" {
			// Said, because a chain that could not be read to its end is not a
			// chain that verified: the entries after the stop were never walked.
			status += fmt.Sprintf(" (reading stopped in %s at line %d: %v)",
				c.TruncatedFile, c.TruncatedAtLine, c.TruncatedReason)
		}
		fmt.Printf("chain %d: %d entries, %s\n", c.Chain, c.EntryCount, status)
	}
	fmt.Printf("%d entries across %d chain(s)\n", total, len(result.Chains))
	if !result.OK {
		fmt.Println("RESULT: the chain does NOT verify against this working tree.")
		os.Exit(1)
	}
	fmt.Println("RESULT: every entry verifies against this working tree.")
}
