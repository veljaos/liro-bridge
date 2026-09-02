package platform

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func encodeMultiString(parts ...string) []uint16 {
	var out []uint16
	for _, p := range parts {
		out = append(out, utf16.Encode([]rune(p))...)
		out = append(out, 0)
	}
	out = append(out, 0) // final terminator
	return out
}

func TestParseMultiString(t *testing.T) {
	cases := []struct {
		name string
		in   []uint16
		want []string
	}{
		{"empty", encodeMultiString(), []string{}},
		{"one reader", encodeMultiString("Generic Smart Card Reader"), []string{"Generic Smart Card Reader"}},
		{"three readers", encodeMultiString("Reader A", "Reader B", "Reader C"), []string{"Reader A", "Reader B", "Reader C"}},
		{"trailing nulls", append(encodeMultiString("Reader A"), 0, 0, 0), []string{"Reader A"}},
		{"non-ASCII name", encodeMultiString("Čitač kartica"), []string{"Čitač kartica"}},
		{"just terminators", []uint16{0, 0}, []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMultiString(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseMultiString(%q) = %#v, want %#v", c.name, got, c.want)
			}
		})
	}
}
