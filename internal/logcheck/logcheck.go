// Package logcheck finds personal data in text the agent writes where a
// person did not put it: the log, and the notes and errors that reach a
// log or a calling program (SPEC §18.3, D-482).
//
// It exists because nothing did (D-480): the agent's own code wrote a
// signer's name, and on a MUP card their JMBG, into bridge.log, and no
// test, review or rule would have found it. The audit log has had a
// check of this shape for a long time; the agent's log had none.
//
// What it finds:
//   - a run of 13 or more decimal digits that is not part of a longer
//     hexadecimal token, which is the shape of a JMBG (a thumbprint is
//     40 hex characters and is not flagged for the digits inside it);
//   - "SERIALNUMBER=" or "2.5.4.5=", the attribute a JMBG is carried in
//     when a certificate subject is written out;
//   - any of the known values the caller passes: a test passes every
//     certificate's subject, CommonName and serial-number attribute.
//
// What it misses, said here so nobody reads it as more:
//   - a JMBG glued to a letter a–f ("…c0101990710006"), or split by
//     spaces or dashes;
//   - a name that is not one of the known values: it cannot know a
//     person's name it was not given;
//   - email addresses, file names and document contents, which SPEC
//     §18.3 also forbids and which it does not look for;
//   - any path the calling test does not drive. It reads text; it does
//     not find the code that wrote it.
package logcheck

import (
	"regexp"
	"strconv"
	"strings"
)

var hexToken = regexp.MustCompile(`[0-9A-Fa-f]+`)

// Find returns one line per piece of personal data found in text, or
// nothing.
func Find(text string, known ...string) []string {
	var found []string
	for _, tok := range hexToken.FindAllString(text, -1) {
		if len(tok) >= 13 && strings.Trim(tok, "0123456789") == "" {
			found = append(found, "a run of "+strconv.Itoa(len(tok))+" digits, the shape of a JMBG: "+tok)
		}
	}
	for _, marker := range []string{"SERIALNUMBER=", "2.5.4.5="} {
		if strings.Contains(text, marker) {
			found = append(found, "a certificate subject's serial-number attribute ("+marker+")")
		}
	}
	for _, k := range known {
		if k != "" && strings.Contains(text, k) {
			found = append(found, "a known certificate value: "+k)
		}
	}
	return found
}
