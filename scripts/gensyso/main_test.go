package main

import (
	"encoding/binary"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The checks in this file pin the invariants whose violation is silent.
//
// None of them is the real verification, and that is deliberate: a writer
// checked by a reader from the same hand proves nothing (D-044). What
// establishes that this object is correct is Windows itself — LoadLibraryEx,
// EnumResourceTypes, FindResource, GetFileVersionInfo and SHGetFileInfo
// against a linked binary, recorded in the decision entry. These exist
// because every one of the mistakes below produces a file that loads cleanly
// and is missing something, which is the shape no amount of looking at the
// output catches.

// TestTheStringsAgreeWithTheInstallersOwn keeps one fact from being written
// twice without anything noticing when the copies drift. The .wxs is the other
// place the product name, the company and the description live.
func TestTheStringsAgreeWithTheInstallersOwn(t *testing.T) {
	b, err := os.ReadFile("../../build/msi/liro-bridge.wxs")
	if err != nil {
		t.Fatalf("reading the installer source: %v", err)
	}
	wxs := string(b)

	// Name carries a preprocessor suffix for the per-machine package; the
	// product name is what comes before it.
	name := firstGroup(t, wxs, `Name="([^"]*)\$\(var\.ProductSuffix\)"`)
	if name != defaultProduct {
		t.Errorf("product name: the installer says %q, gensyso says %q", name, defaultProduct)
	}

	manufacturer := firstGroup(t, wxs, `Manufacturer="([^"]+)"`)
	if manufacturer != defaultCompany {
		t.Errorf("company: the installer says %q, gensyso says %q", manufacturer, defaultCompany)
	}

	description := firstGroup(t, wxs, `Description="(Liro Bridge[^"]+)"`)
	if description != defaultDescription {
		t.Errorf("description:\n  the installer says %q\n  gensyso says      %q", description, defaultDescription)
	}
}

func firstGroup(t *testing.T, s, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("the installer source has nothing matching %s", pattern)
	}
	return m[1]
}

func TestParseVersion(t *testing.T) {
	for _, c := range []struct {
		in   string
		want [4]uint16
	}{
		{"0.9.1", [4]uint16{0, 9, 1, 0}},
		{"1.2.3.4", [4]uint16{1, 2, 3, 4}},
		{"1.0.0-rc.1", [4]uint16{1, 0, 0, 0}}, // a prerelease is that release's numbers
		{"2.0.0+build7", [4]uint16{2, 0, 0, 0}},
		{"dev", [4]uint16{}},       // the development build, which the MSI also stamps 0.0.0
		{"", [4]uint16{}},          // and anything else unreadable
		{"1.99999.0", [4]uint16{}}, // a field that will not fit 16 bits is not a version
	} {
		if got := parseVersion(c.in); got != c.want {
			t.Errorf("parseVersion(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestGroupIconEntriesAreFourteenBytesNotSixteen pins the difference between
// the two directory entry shapes. A file's ICONDIRENTRY ends with a 4-byte
// offset; a resource's GRPICONDIRENTRY ends with a 2-byte id. Copying the
// entry across whole produces a group Windows reads as valid and that
// describes frames which are not there.
func TestGroupIconEntriesAreFourteenBytesNotSixteen(t *testing.T) {
	const frames = 3
	ico := synthIcon(frames)

	res, err := iconResources(ico)
	if err != nil {
		t.Fatalf("iconResources: %v", err)
	}
	if len(res) != frames+1 {
		t.Fatalf("got %d resources, want %d icons plus one group", len(res), frames)
	}

	var group []byte
	icons := 0
	for _, r := range res {
		switch r.typ {
		case rtGroupIcon:
			group = r.data
		case rtIcon:
			icons++
		}
	}
	if icons != frames {
		t.Errorf("got %d RT_ICON resources, want %d", icons, frames)
	}
	if want := 6 + frames*14; len(group) != want {
		t.Fatalf("group icon is %d bytes, want %d (6-byte header plus %d 14-byte entries)", len(group), want, frames)
	}
	if n := binary.LittleEndian.Uint16(group[4:]); int(n) != frames {
		t.Errorf("group icon declares %d frames, want %d", n, frames)
	}
	// Every entry's id must name an RT_ICON that exists.
	for i := 0; i < frames; i++ {
		id := binary.LittleEndian.Uint16(group[6+i*14+12:])
		found := false
		for _, r := range res {
			if r.typ == rtIcon && r.id == uint32(id) {
				found = true
			}
		}
		if !found {
			t.Errorf("group entry %d names icon id %d, which is not among the RT_ICON resources", i, id)
		}
	}
}

// TestVersionBlockLengthsCoverTheirOwnPadding walks the VERSIONINFO tree the
// way VerQueryValue does and checks the two things that make it readable:
// every block's wLength accounts for everything inside it, and every block
// starts on a 4-byte boundary. Getting either wrong does not produce a
// malformed file — it produces one where a single key reads as missing.
func TestVersionBlockLengthsCoverTheirOwnPadding(t *testing.T) {
	data := versionResource("0.9.1", [4]uint16{0, 9, 1, 0},
		defaultProduct, defaultCompany, defaultDescription, defaultCopyright)

	if len(data)%4 != 0 {
		t.Errorf("the whole resource is %d bytes, which is not a multiple of 4", len(data))
	}
	if n := int(binary.LittleEndian.Uint16(data[0:])); n != len(data) {
		t.Fatalf("root wLength is %d, resource is %d bytes", n, len(data))
	}

	var walk func(b []byte, at int, path string)
	walk = func(b []byte, at int, path string) {
		if at%4 != 0 {
			t.Errorf("%s starts at offset %d, which is not 4-aligned", path, at)
		}
		if len(b) < 6 {
			t.Fatalf("%s is %d bytes, too short for a block header", path, len(b))
			return
		}
		length := int(binary.LittleEndian.Uint16(b[0:]))
		valueLen := int(binary.LittleEndian.Uint16(b[2:]))
		isText := binary.LittleEndian.Uint16(b[4:]) == 1
		if length > len(b) {
			t.Fatalf("%s claims %d bytes but only %d remain", path, length, len(b))
			return
		}
		b = b[:length]

		// key, NUL-terminated UTF-16, then padding to 4
		off := 6
		key := ""
		for off+1 < len(b) {
			c := binary.LittleEndian.Uint16(b[off:])
			off += 2
			if c == 0 {
				break
			}
			key += string(rune(c))
		}
		for off%4 != 0 {
			off++
		}

		valueBytes := valueLen
		if isText {
			valueBytes = valueLen * 2 // text counts characters, binary counts bytes
		}
		if off+valueBytes > len(b) {
			t.Errorf("%s/%s: value of %d bytes does not fit in the block's own %d",
				path, key, valueBytes, len(b))
			return
		}
		off += valueBytes

		for off < len(b) {
			for off%4 != 0 {
				off++
			}
			if off >= len(b) {
				break
			}
			childLen := int(binary.LittleEndian.Uint16(b[off:]))
			if childLen == 0 {
				t.Errorf("%s/%s: a child block of length 0 — the walk cannot advance", path, key)
				return
			}
			if off+childLen > len(b) {
				t.Errorf("%s/%s: a child claims %d bytes at offset %d of a %d-byte block",
					path, key, childLen, off, len(b))
				return
			}
			walk(b[off:off+childLen], at+off, path+"/"+key)
			off += childLen
		}
	}
	walk(data, 0, "")

	// And the strings really are in there, in UTF-16.
	for _, want := range []string{defaultProduct, defaultCompany, defaultCopyright, "liro-bridge.exe", "0.9.1"} {
		if !strings.Contains(string(data), utf16Marker(want)) {
			t.Errorf("the version resource does not contain %q", want)
		}
	}
}

func utf16Marker(s string) string {
	var b []byte
	for _, r := range s {
		if r > 0xFFFF {
			continue
		}
		b = append(b, byte(r), byte(r>>8))
	}
	return string(b)
}

// TestEveryDataEntryHasARelocation is the one with no visible failure mode at
// all: without the relocations the object still links and the image still
// loads, and every resource pointer in it is short by the section's address,
// so FindResource finds nothing.
func TestEveryDataEntryHasARelocation(t *testing.T) {
	res, err := iconResources(synthIcon(2))
	if err != nil {
		t.Fatal(err)
	}
	res = append(res, resource{typ: rtVersion, id: 1, lang: langEnglishUS,
		data: versionResource("dev", [4]uint16{}, defaultProduct, defaultCompany, defaultDescription, defaultCopyright)})

	section, relocs, err := buildRSRC(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(relocs) != len(res) {
		t.Errorf("got %d relocations for %d resources; every data entry needs one", len(relocs), len(res))
	}
	for i, off := range relocs {
		if off%4 != 0 {
			t.Errorf("relocation %d targets offset %d, which is not 4-aligned", i, off)
		}
		if int(off)+4 > len(section) {
			t.Errorf("relocation %d targets offset %d, past the %d-byte section", i, off, len(section))
			continue
		}
		// The field it points at holds a section-relative offset that must
		// land inside the section, since the linker only adds a base to it.
		target := binary.LittleEndian.Uint32(section[off:])
		if int(target) >= len(section) {
			t.Errorf("relocation %d points at offset %d, outside the %d-byte section", i, target, len(section))
		}
	}
}

// TestDirectoryEntriesAreSortedAscending matters because Windows binary-searches
// them. An unsorted directory does not fail; it fails to find things, and which
// things depends on where the search happens to land.
func TestDirectoryEntriesAreSortedAscending(t *testing.T) {
	// Deliberately handed in an order no caller would use.
	res := []resource{
		{typ: rtVersion, id: 1, lang: langEnglishUS, data: []byte{1, 2, 3, 4}},
		{typ: rtIcon, id: 9, lang: langEnglishUS, data: []byte{5}},
		{typ: rtIcon, id: 2, lang: langEnglishUS, data: []byte{6}},
		{typ: rtGroupIcon, id: 1, lang: langEnglishUS, data: []byte{7}},
	}
	section, _, err := buildRSRC(res)
	if err != nil {
		t.Fatal(err)
	}
	checkDir(t, section, 0, "root")
}

func checkDir(t *testing.T, section []byte, at uint32, path string) {
	t.Helper()
	if int(at)+16 > len(section) {
		t.Fatalf("%s: directory header runs past the section", path)
	}
	named := binary.LittleEndian.Uint16(section[at+12:])
	ids := binary.LittleEndian.Uint16(section[at+14:])
	if named != 0 {
		t.Errorf("%s: %d named entries; this project uses ids only", path, named)
	}
	prev := int64(-1)
	for i := 0; i < int(ids); i++ {
		e := at + 16 + uint32(i)*8
		id := binary.LittleEndian.Uint32(section[e:])
		off := binary.LittleEndian.Uint32(section[e+4:])
		if int64(id) <= prev {
			t.Errorf("%s: entry %d has id %d after %d — not ascending", path, i, id, prev)
		}
		prev = int64(id)
		if off&0x80000000 != 0 {
			checkDir(t, section, off&0x7FFFFFFF, path+"/"+itoa(id))
		}
	}
}

func itoa(u uint32) string {
	if u == 0 {
		return "0"
	}
	var b []byte
	for u > 0 {
		b = append([]byte{byte('0' + u%10)}, b...)
		u /= 10
	}
	return string(b)
}

// synthIcon builds a .ico with n one-byte frames — enough structure to
// exercise the directory arithmetic without carrying a real image.
func synthIcon(n int) []byte {
	head := make([]byte, 6)
	binary.LittleEndian.PutUint16(head[2:], 1) // type: icon
	binary.LittleEndian.PutUint16(head[4:], uint16(n))

	entries := make([]byte, 0, n*16)
	body := make([]byte, 0, n)
	base := 6 + n*16
	for i := 0; i < n; i++ {
		e := make([]byte, 16)
		e[0] = 32                                // width
		e[1] = 32                                // height
		binary.LittleEndian.PutUint16(e[4:], 1)  // planes
		binary.LittleEndian.PutUint16(e[6:], 32) // bit count
		binary.LittleEndian.PutUint32(e[8:], 1)  // bytes in res
		binary.LittleEndian.PutUint32(e[12:], uint32(base+i))
		entries = append(entries, e...)
		body = append(body, byte(0xA0+i))
	}
	return append(append(head, entries...), body...)
}
