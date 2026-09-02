package pdf

import "testing"

// FuzzParse feeds foreign input straight to Parse (F3 §2.5/§16.5,
// required by SPEC §16.5, not optional). The parser accepts files from
// outside the agent's control, so this must never panic, never allocate
// unboundedly and never loop forever — Parse itself must simply return
// an error for garbage input, exactly like any other malformed document.
func FuzzParse(f *testing.F) {
	f.Add(buildClassicFixture())
	f.Add(buildStreamFixture())
	f.Add([]byte("%PDF-1.4\n"))
	f.Add([]byte(""))
	f.Add([]byte("not a pdf at all"))

	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil || doc == nil {
			return
		}
		// A successful parse must also survive a Get() walk over every
		// declared object number without panicking.
		for num := range doc.xref {
			_ = doc.Get(num)
		}
	})
}
