package main

import (
	"bufio"
	"crypto/x509"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A snapshot is the small durable record of one day's list. The 30 MB list
// itself is not kept, because what a later run needs from it is only:
//
//   - the list's own identity — issuer, thisUpdate, nextUpdate, size, count —
//     so two runs can be shown to be looking at different lists rather than
//     the same one twice;
//   - whether our serial was on it, which is the measurement; and
//   - a sample of serials that *were* on it, which is the only way to answer
//     "does this issuer drop an entry once the certificate expires" from
//     anything less than the whole 30 MB.
//
// That last question matters here specifically: the certificate being watched
// expires two days after it is cancelled, so whether MUP purges at expiry
// decides whether the entry can ever be seen at all.
type snapshot struct {
	Path       string
	Issuer     string
	ThisUpdate time.Time
	NextUpdate time.Time
	Bytes      int
	Entries    int
	Serial     string
	Present    bool
	RevokedAt  time.Time
	Sample     []string // uppercase hex, in list order
}

// sampleSize is a compromise with one job: large enough that a purge of a few
// per cent is visible above sampling noise, small enough that the file stays
// readable by a person and reviewable in a diff.
const sampleSize = 500

func write(dir string, l *x509.RevocationList, serial string, entry *x509.RevocationListEntry, size int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(dir, "crl-"+l.ThisUpdate.UTC().Format("2006-01-02T150405Z")+".txt")
	// Refusing to overwrite is deliberate. A snapshot is a dated measurement,
	// and a second run on the same list must not silently replace the first —
	// if they disagreed, the disagreement is the finding.
	if _, err := os.Stat(name); err == nil {
		fmt.Printf("  snapshot %s exists already; this run did not overwrite it\n\n", filepath.Base(name))
		return nil
	}
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	_, _ = fmt.Fprintf(w, "taken       %s\n", time.Now().Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "issuer      %s\n", l.Issuer.CommonName)
	_, _ = fmt.Fprintf(w, "thisUpdate  %s\n", l.ThisUpdate.UTC().Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "nextUpdate  %s\n", l.NextUpdate.UTC().Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "bytes       %d\n", size)
	_, _ = fmt.Fprintf(w, "entries     %d\n", len(l.RevokedCertificateEntries))
	_, _ = fmt.Fprintf(w, "serial      %s\n", serial)
	if entry != nil {
		_, _ = fmt.Fprintf(w, "present     YES  revoked at %s\n", entry.RevocationTime.UTC().Format(time.RFC3339))
	} else {
		_, _ = fmt.Fprintf(w, "present     no\n")
	}
	// An evenly spaced sample rather than a random one: it needs no seed to be
	// reproducible, and it spreads across the whole list. A CRL is ordered
	// roughly by issuance, so a sample drawn from one end would answer the
	// purging question about one cohort only.
	step := len(l.RevokedCertificateEntries) / sampleSize
	if step < 1 {
		step = 1
	}
	_, _ = fmt.Fprintf(w, "sample      every %d-th entry\n", step)
	_, _ = fmt.Fprintln(w, "--- serials ---")
	for i := 0; i < len(l.RevokedCertificateEntries); i += step {
		e := l.RevokedCertificateEntries[i]
		_, _ = fmt.Fprintf(w, "%X %s\n", e.SerialNumber, e.RevocationTime.UTC().Format(time.RFC3339))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("  snapshot written to %s\n\n", name)
	return nil
}

// compare reports this list against the most recent earlier snapshot. It is
// what makes the committed baseline an instrument rather than a data file:
// without something that reads it, a snapshot nobody opens is D-247's shape
// applied to evidence.
func compare(dir string, l *x509.RevocationList) {
	prev, err := newest(dir, l.ThisUpdate)
	if err != nil {
		fmt.Printf("  compare  no earlier snapshot to compare against (%v)\n", err)
		fmt.Printf("           this run is the baseline. Nothing is established by it\n")
		fmt.Printf("           alone; what it makes possible is the next one.\n\n")
		return
	}
	fmt.Printf("  compare  against %s\n", filepath.Base(prev.Path))
	fmt.Printf("           thisUpdate  %s -> %s  (%v later)\n",
		prev.ThisUpdate.Format(time.RFC3339), l.ThisUpdate.UTC().Format(time.RFC3339),
		l.ThisUpdate.Sub(prev.ThisUpdate).Round(time.Minute))
	fmt.Printf("           entries     %d -> %d  (%+d)\n",
		prev.Entries, len(l.RevokedCertificateEntries), len(l.RevokedCertificateEntries)-prev.Entries)

	if len(prev.Sample) == 0 {
		fmt.Printf("           the earlier snapshot carries no sample, so nothing can be\n")
		fmt.Printf("           said about whether entries are dropped.\n\n")
		return
	}
	survivors := 0
	for _, hex := range prev.Sample {
		n, ok := new(big.Int).SetString(hex, 16)
		if !ok {
			continue
		}
		if find(l, n) != nil {
			survivors++
		}
	}
	gone := len(prev.Sample) - survivors
	fmt.Printf("           of %d serials sampled then, %d are still listed and %d are gone\n",
		len(prev.Sample), survivors, gone)
	switch {
	case survivors == 0:
		// Fail closed. A whole sample vanishing is far more likely to be a
		// broken comparison than an issuer purging its entire list, and an
		// instrument that reports the exciting reading without saying so is
		// the failure D-296 and D-304 are about.
		fmt.Printf("           !! NONE of them survived. Treat this as the comparison being\n")
		fmt.Printf("           !! broken rather than as the list being purged, until something\n")
		fmt.Printf("           !! independent says otherwise.\n")
	case gone == 0:
		fmt.Printf("           nothing was dropped, so on this evidence entries persist and a\n")
		fmt.Printf("           revocation published before expiry stays visible after it.\n")
	default:
		fmt.Printf("           entries ARE dropped between lists. A revocation that is not\n")
		fmt.Printf("           published before the certificate expires may never be seen.\n")
	}
	fmt.Println()
}

// newest returns the most recent snapshot in dir strictly older than notAfter,
// so a re-run against the same list compares with the previous day rather than
// with itself.
func newest(dir string, before time.Time) (*snapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "crl-") && strings.HasSuffix(e.Name(), ".txt") {
			names = append(names, e.Name())
		}
	}
	// The filename carries an RFC3339-ish UTC timestamp, so lexical order is
	// chronological order and no parsing is needed to sort.
	sort.Strings(names)
	for i := len(names) - 1; i >= 0; i-- {
		s, err := read(filepath.Join(dir, names[i]))
		if err != nil {
			continue
		}
		if s.ThisUpdate.Before(before) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no snapshot in %s older than %s", dir, before.UTC().Format(time.RFC3339))
}

func read(path string) (*snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	s := &snapshot{Path: path}
	sc := bufio.NewScanner(f)
	inSample := false
	for sc.Scan() {
		line := sc.Text()
		if inSample {
			if hex, _, ok := strings.Cut(line, " "); ok && hex != "" {
				s.Sample = append(s.Sample, hex)
			}
			continue
		}
		if line == "--- serials ---" {
			inSample = true
			continue
		}
		key, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		switch key {
		case "issuer":
			s.Issuer = rest
		case "thisUpdate":
			s.ThisUpdate, _ = time.Parse(time.RFC3339, rest)
		case "nextUpdate":
			s.NextUpdate, _ = time.Parse(time.RFC3339, rest)
		case "serial":
			s.Serial = rest
		case "bytes":
			if s.Bytes, err = strconv.Atoi(rest); err != nil {
				return nil, fmt.Errorf("%s: bytes is not a number: %q", path, rest)
			}
		case "entries":
			if s.Entries, err = strconv.Atoi(rest); err != nil {
				return nil, fmt.Errorf("%s: entries is not a number: %q", path, rest)
			}
		case "present":
			s.Present = strings.HasPrefix(rest, "YES")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if s.ThisUpdate.IsZero() {
		return nil, fmt.Errorf("%s has no thisUpdate", path)
	}
	return s, nil
}
