package windowscng

import "testing"

func TestOnHardware(t *testing.T) {
	cases := []struct {
		provider string
		want     bool
	}{
		{"Microsoft Smart Card Key Storage Provider", true},
		{"Microsoft Base Smart Card Crypto Provider", true},
		{"Microsoft Software Key Storage Provider", false},
		{"", false},
		{"Some Future Vendor KSP", false}, // unknown providers log, but count as software
	}
	for _, c := range cases {
		if got := onHardware(c.provider); got != c.want {
			t.Errorf("onHardware(%q) = %v, want %v", c.provider, got, c.want)
		}
	}
}
