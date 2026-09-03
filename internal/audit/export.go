package audit

import (
	"encoding/json"
	"fmt"
	"os"
)

// ExportReport is written alongside an export (F5 §8.4: "Export on user
// request writes a copy plus a verification report").
type ExportReport struct {
	EntryCount int          `json:"entryCount"`
	Result     VerifyResult `json:"result"`
}

// Export writes every entry to dstEntries (one JSON object per line,
// the same shape the store itself uses) and a verification report to
// dstReport (F5 §8.4).
func (s *Store) Export(dstEntries, dstReport string) (ExportReport, error) {
	entries, err := s.All()
	if err != nil {
		return ExportReport{}, err
	}
	result := Verify(entries)

	f, err := os.Create(dstEntries)
	if err != nil {
		return ExportReport{}, err
	}
	defer func() { _ = f.Close() }()
	for _, e := range entries {
		b, err := json.Marshal(toJSONEntry(e))
		if err != nil {
			return ExportReport{}, err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return ExportReport{}, err
		}
	}

	report := ExportReport{EntryCount: len(entries), Result: result}
	rb, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ExportReport{}, err
	}
	if err := os.WriteFile(dstReport, rb, 0o600); err != nil {
		return ExportReport{}, err
	}
	if !result.OK {
		return report, fmt.Errorf("audit: chain verification failed at entry %d", result.BrokenAt)
	}
	return report, nil
}
