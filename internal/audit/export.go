package audit

import (
	"encoding/json"
	"fmt"
	"os"
)

// ExportReport is written alongside an export (F5 §8.4: "Export on user
// request writes a copy plus a verification report").
//
// EntryCount and Result are what they always were: how many entries were
// written, and the walk's overall verdict, with BrokenAt counted across
// the exported file's own lines so a message can point at a line a
// person has in front of them. Chains is the detail that verdict is
// assembled from — one entry per chain, each with its own result and,
// where there is one, the record of why that chain was started at all.
type ExportReport struct {
	EntryCount int          `json:"entryCount"`
	Result     VerifyResult `json:"result"`

	// Chains reports each chain separately: a store that has survived a
	// break holds more than one, and saying only "not OK" about the
	// whole of it would hide both which chain broke and that the others
	// are intact.
	Chains []ChainVerification `json:"chains"`
}

// Discontinuities returns every exported chain that began because an
// earlier one could not be continued.
func (r ExportReport) Discontinuities() []ChainVerification {
	var out []ChainVerification
	for _, c := range r.Chains {
		if c.Discontinuity != nil {
			out = append(out, c)
		}
	}
	return out
}

// Export writes every entry of every chain to dstEntries (one JSON
// object per line, the same shape the store itself uses) and a
// verification report to dstReport (F5 §8.4).
//
// Every chain, in order. A store that has survived a break holds more
// than one, and an export that wrote only the current one would leave
// behind exactly the evidence the break makes worth keeping. The
// entries' own lines carry their chain's discontinuity record where
// there is one, so the exported log is self-describing without the
// report beside it.
func (s *Store) Export(dstEntries, dstReport string) (ExportReport, error) {
	chains, err := s.Chains()
	if err != nil {
		return ExportReport{}, err
	}
	verification := VerifyChains(chains)

	f, err := os.Create(dstEntries)
	if err != nil {
		return ExportReport{}, err
	}
	defer func() { _ = f.Close() }()
	written := 0
	for _, c := range chains {
		for _, e := range c.Entries {
			b, err := json.Marshal(toJSONEntry(e))
			if err != nil {
				return ExportReport{}, err
			}
			if _, err := f.Write(append(b, '\n')); err != nil {
				return ExportReport{}, err
			}
			written++
		}
	}

	report := ExportReport{
		EntryCount: written,
		Result:     VerifyResult{OK: verification.OK, BrokenAt: verification.BrokenAt},
		Chains:     verification.Chains,
	}
	rb, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ExportReport{}, err
	}
	if err := os.WriteFile(dstReport, rb, 0o600); err != nil {
		return ExportReport{}, err
	}
	if !report.Result.OK {
		// Both files are still written: a chain that does not verify is
		// a finding about the log, not a failure to export it, and the
		// evidence is exactly what somebody will want to look at.
		if report.Result.BrokenAt >= 0 {
			return report, fmt.Errorf("audit: chain verification failed at entry %d", report.Result.BrokenAt)
		}
		return report, fmt.Errorf("audit: the log could not be read to its end")
	}
	return report, nil
}
