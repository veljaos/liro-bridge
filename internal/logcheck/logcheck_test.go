package logcheck

import "testing"

func TestAJMBGIsFoundAndAThumbprintIsNot(t *testing.T) {
	if got := Find(`"serial":"PNORS-0101990710006"`); len(got) == 0 {
		t.Error("a JMBG after a dash was not found")
	}
	if got := Find(`subject="Петар Петровић 0101990710006"`); len(got) == 0 {
		t.Error("a JMBG after a space was not found")
	}
	// A thumbprint is hexadecimal and may hold long runs of digits.
	if got := Find(`"thumbprint":"7C8B6A805F174D9CE4BD0A2A5BC1548083838FE5"`); len(got) != 0 {
		t.Errorf("a thumbprint was flagged: %v", got)
	}
	if got := Find(`"thumbprint":"A1234567890123456789012345678901234567BC"`); len(got) != 0 {
		t.Errorf("a thumbprint with 37 digits inside it was flagged: %v", got)
	}
	if got := Find(`"bytes":30017248,"cap":5242880`); len(got) != 0 {
		t.Errorf("sizes were flagged: %v", got)
	}
}

func TestASubjectIsFound(t *testing.T) {
	if got := Find(`subject="SERIALNUMBER=PNORS-x,CN=y"`); len(got) == 0 {
		t.Error("a written-out subject was not found")
	}
	if got := Find(`note: the evidence says "Petar Petrović"`, "Petar Petrović"); len(got) == 0 {
		t.Error("a known name was not found")
	}
	if got := Find(`nothing personal here`, "Petar Petrović", ""); len(got) != 0 {
		t.Errorf("found something in clean text: %v", got)
	}
}
