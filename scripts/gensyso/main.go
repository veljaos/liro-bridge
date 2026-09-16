// Command gensyso writes the Windows resource object that gives
// liro-bridge.exe an icon and a version resource.
//
// Go embeds no resources of its own, but the toolchain links any .syso file
// sitting beside the package's source automatically. This writes that file:
// a COFF object carrying one .rsrc section with RT_ICON, RT_GROUP_ICON and
// RT_VERSION in it, and nothing else — deliberately no RT_MANIFEST, because
// D-081 declares per-monitor DPI awareness programmatically and a manifest
// here would quietly take that decision over.
//
// It is generated at build time rather than committed, and the reason is not
// the binary-in-the-repository objection: it is that the version resource has
// to carry the version from the tag, exactly as -ldflags already stamps
// main.version, and a committed file cannot. The icon and the version are one
// resource, so the half that must change per release decides for both. See
// docs/decisions.md.
//
// Nothing outside the standard library is used, so "a tool CI must have" means
// a `go run` on a machine that already has Go in order to build Go — the same
// arrangement as scripts/checkdeps, scripts/genicon and seven others.
//
// The structures below are the PE/COFF and VERSIONINFO layouts, written out
// rather than taken from a library. Every offset that a reader might think is
// arbitrary carries the reason it is what it is, because getting one wrong
// does not produce a broken file — it produces one Windows loads happily and
// finds nothing in.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Resource type IDs (winuser.h).
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
)

// langEnglishUS is the language the resource tree files these under.
//
// It matches build/msi/liro-bridge.wxs's own Language="1033", and that file's
// comment explains the choice for both of them: the language of a resource is
// about which localised transform applies, and this product ships none. The
// strings themselves are in the language their audience reads, which is what
// the .wxs already does with a Serbian Description under Language 1033.
const langEnglishUS = 0x0409

// codepageUnicode is 1200, the codepage half of a VERSIONINFO translation
// pair. Every string below is UTF-16 regardless, so this is the only value
// that is not a lie.
const codepageUnicode = 0x04B0

// What Explorer's Details tab says about this program.
//
// ProductName and CompanyName are the same two strings build/msi/liro-bridge.wxs
// gives Windows Installer, and FileDescription is the same sentence its Package
// Description carries. That is one fact written in two files, which this project
// has had to unpick four times when the copies drifted ([[D-108]], [[D-124]],
// [[D-138]], [[D-183]]) — so main_test.go reads the .wxs and fails if they stop
// agreeing, rather than a comment asking somebody to remember.
const (
	defaultProduct     = "Liro Bridge"
	defaultCompany     = "Liro"
	defaultDescription = "Liro Bridge — potpisivanje PDF dokumenata kvalifikovanim elektronskim sertifikatom"
	defaultCopyright   = "Liro — Apache License 2.0"
)

func main() {
	var (
		iconPath      = flag.String("icon", "", "the .ico file to embed (required)")
		out           = flag.String("out", "", "the .syso to write (required)")
		version       = flag.String("version", "dev", "the version string, exactly as --version reports it")
		product       = flag.String("product", defaultProduct, "product name")
		company       = flag.String("company", defaultCompany, "company name")
		desc          = flag.String("description", defaultDescription, "file description, shown in Explorer's Details tab")
		copyrightFlag = flag.String("copyright", defaultCopyright, "legal copyright")
	)
	flag.Parse()

	if *iconPath == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "gensyso: -icon and -out are required")
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*iconPath, *out, *version, *product, *company, *desc, *copyrightFlag); err != nil {
		fmt.Fprintln(os.Stderr, "gensyso:", err)
		os.Exit(1)
	}
}

func run(iconPath, out, version, product, company, desc, copyright string) error {
	icoBytes, err := os.ReadFile(iconPath)
	if err != nil {
		return err
	}

	resources, err := iconResources(icoBytes)
	if err != nil {
		return fmt.Errorf("reading %s: %w", iconPath, err)
	}

	// The numeric version is four 16-bit fields and the string is whatever
	// --version reports, which for a development build is "dev" and parses to
	// nothing. Those are different facts and both are carried: a tool that
	// compares versions numerically gets 0.0.0.0, and a person reading the
	// Details tab gets the same word the command line would have told them.
	fileVersion := parseVersion(version)

	vi := versionResource(version, fileVersion, product, company, desc, copyright)
	resources = append(resources, resource{typ: rtVersion, id: 1, lang: langEnglishUS, data: vi})

	obj, err := coffObject(resources)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, obj, 0o644); err != nil {
		return err
	}

	fmt.Printf("gensyso: %s (%d bytes) — icon %d frames, version %q (%d.%d.%d.%d)\n",
		out, len(obj), len(resources)-2, version,
		fileVersion[0], fileVersion[1], fileVersion[2], fileVersion[3])
	return nil
}

// parseVersion turns "0.9.1" or "1.2.3-rc.1" into the four 16-bit fields
// VS_FIXEDFILEINFO carries. Anything it cannot read — "dev", most obviously —
// is 0.0.0.0, which is what the MSI stamps for the same build (build.ps1's
// own $msiVersion does the same thing to the same input).
func parseVersion(s string) [4]uint16 {
	var v [4]uint16
	s = strings.SplitN(s, "-", 2)[0]
	s = strings.SplitN(s, "+", 2)[0]
	for i, part := range strings.Split(s, ".") {
		if i >= 4 {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 0xFFFF {
			return [4]uint16{}
		}
		v[i] = uint16(n)
	}
	return v
}

// ---------------------------------------------------------------- the icon

// resource is one leaf of the three-level resource tree: type, then id, then
// language.
type resource struct {
	typ  uint32
	id   uint32
	lang uint32
	data []byte
}

// iconResources turns a .ico file into one RT_ICON per frame plus the single
// RT_GROUP_ICON that names them.
//
// The two directory entries are deliberately *not* the same shape, and that is
// the mistake this function exists to avoid: an ICONDIRENTRY in a file ends
// with a 4-byte offset to the image, and a GRPICONDIRENTRY in a resource ends
// with a 2-byte resource id instead. Copying the 16 bytes across verbatim
// produces a group whose entries are two bytes too long each, which Windows
// reads as a valid directory describing frames that are not there.
func iconResources(ico []byte) ([]resource, error) {
	const (
		iconDirSize      = 6
		iconDirEntrySize = 16
		grpDirEntrySize  = 14
	)
	if len(ico) < iconDirSize {
		return nil, fmt.Errorf("not an icon: %d bytes", len(ico))
	}
	if binary.LittleEndian.Uint16(ico[0:]) != 0 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil, fmt.Errorf("not an icon: bad ICONDIR header")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:]))
	if count == 0 {
		return nil, fmt.Errorf("icon holds no frames")
	}
	if len(ico) < iconDirSize+count*iconDirEntrySize {
		return nil, fmt.Errorf("icon truncated: %d frames declared, %d bytes", count, len(ico))
	}

	group := new(bytes.Buffer)
	group.Write(ico[0:iconDirSize]) // reserved, type and count are the same in both

	out := make([]resource, 0, count+1)
	for i := 0; i < count; i++ {
		e := ico[iconDirSize+i*iconDirEntrySize:][:iconDirEntrySize]
		size := binary.LittleEndian.Uint32(e[8:])
		offset := binary.LittleEndian.Uint32(e[12:])
		if uint64(offset)+uint64(size) > uint64(len(ico)) {
			return nil, fmt.Errorf("frame %d runs past the end of the file", i)
		}

		// Frame ids start at 1: zero is not a usable resource id.
		id := uint32(i + 1)

		group.Write(e[0:8]) // width, height, colours, reserved, planes, bit count
		_ = binary.Write(group, binary.LittleEndian, size)
		_ = binary.Write(group, binary.LittleEndian, uint16(id))

		out = append(out, resource{
			typ:  rtIcon,
			id:   id,
			lang: langEnglishUS,
			data: ico[offset : offset+size],
		})
	}

	if group.Len() != iconDirSize+count*grpDirEntrySize {
		return nil, fmt.Errorf("group icon came out %d bytes, want %d",
			group.Len(), iconDirSize+count*grpDirEntrySize)
	}

	// The shell picks the application icon by taking the numerically lowest
	// RT_GROUP_ICON id, so this one is 1 and there is only one.
	out = append(out, resource{typ: rtGroupIcon, id: 1, lang: langEnglishUS, data: group.Bytes()})
	return out, nil
}

// ------------------------------------------------------------- the version

// verBlock is one node of VERSIONINFO's nested block format. Every block is
// [wLength][wValueLength][wType][key as UTF-16 + NUL][pad][value][children],
// where each of pad, value and every child starts on a 4-byte boundary
// measured from the start of the block — and wLength counts all of it.
//
// A block whose length is wrong by the size of one pad does not fail to load.
// VerQueryValue simply returns false for that one key and true for its
// siblings, which reads as a string somebody forgot to set.
type verBlock struct {
	key      string
	value    []byte
	valueLen uint16 // characters for text, bytes for binary — the format's own inconsistency
	text     bool
	children []*verBlock
}

func (b *verBlock) bytes() []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{0, 0, 0, 0, 0, 0}) // wLength, wValueLength, wType — filled in below
	writeUTF16z(buf, b.key)
	pad4(buf)
	buf.Write(b.value)
	for _, c := range b.children {
		pad4(buf)
		buf.Write(c.bytes())
	}

	out := buf.Bytes()
	typ := uint16(0)
	if b.text {
		typ = 1
	}
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	binary.LittleEndian.PutUint16(out[2:], b.valueLen)
	binary.LittleEndian.PutUint16(out[4:], typ)
	return out
}

func versionResource(versionString string, v [4]uint16, product, company, desc, copyright string) []byte {
	// VS_FIXEDFILEINFO, 52 bytes, the binary half that tools compare
	// numerically. The string half below is what a person reads.
	ffi := new(bytes.Buffer)
	put32 := func(x uint32) { _ = binary.Write(ffi, binary.LittleEndian, x) }
	ms := uint32(v[0])<<16 | uint32(v[1])
	ls := uint32(v[2])<<16 | uint32(v[3])
	put32(0xFEEF04BD) // dwSignature
	put32(0x00010000) // dwStrucVersion 1.0
	put32(ms)         // dwFileVersionMS
	put32(ls)         // dwFileVersionLS
	put32(ms)         // dwProductVersionMS — the same number; this product has one version
	put32(ls)         // dwProductVersionLS
	put32(0x0000003F) // dwFileFlagsMask: all six flags are meaningful
	put32(0)          // dwFileFlags: not a debug, prerelease or patched build
	put32(0x00000004) // dwFileOS = VOS__WINDOWS32
	put32(0x00000001) // dwFileType = VFT_APP
	put32(0)          // dwFileSubtype: none for an application
	put32(0)          // dwFileDateMS
	put32(0)          // dwFileDateLS

	str := func(k, val string) *verBlock {
		return &verBlock{
			key:      k,
			value:    utf16z(val),
			valueLen: uint16(len([]rune(val))) + 1, // characters, including the NUL
			text:     true,
		}
	}

	// The key is language and codepage as eight hex digits. It has to agree
	// with the Translation value below or VerQueryValue finds the table and
	// then cannot name it.
	table := &verBlock{
		key:  fmt.Sprintf("%04X%04X", langEnglishUS, codepageUnicode),
		text: true,
		children: []*verBlock{
			str("CompanyName", company),
			str("FileDescription", desc),
			str("FileVersion", versionString),
			str("InternalName", "liro-bridge"),
			str("LegalCopyright", copyright),
			str("OriginalFilename", "liro-bridge.exe"),
			str("ProductName", product),
			str("ProductVersion", versionString),
		},
	}

	translation := new(bytes.Buffer)
	_ = binary.Write(translation, binary.LittleEndian, uint16(langEnglishUS))
	_ = binary.Write(translation, binary.LittleEndian, uint16(codepageUnicode))

	root := &verBlock{
		key:      "VS_VERSION_INFO",
		value:    ffi.Bytes(),
		valueLen: uint16(ffi.Len()), // bytes, because this block is binary
		children: []*verBlock{
			{key: "StringFileInfo", text: true, children: []*verBlock{table}},
			{key: "VarFileInfo", text: true, children: []*verBlock{
				{key: "Translation", value: translation.Bytes(), valueLen: uint16(translation.Len())},
			}},
		},
	}
	return root.bytes()
}

func utf16z(s string) []byte {
	buf := new(bytes.Buffer)
	writeUTF16z(buf, s)
	return buf.Bytes()
}

func writeUTF16z(buf *bytes.Buffer, s string) {
	for _, c := range utf16.Encode([]rune(s)) {
		_ = binary.Write(buf, binary.LittleEndian, c)
	}
	buf.Write([]byte{0, 0})
}

func pad4(buf *bytes.Buffer) {
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}
}

// ------------------------------------------------- the resource directory

// buildRSRC lays out the three-level resource directory and returns the
// section's bytes together with the offsets of every field that needs a
// relocation.
//
// The relocations are the part with no visible failure mode. Each
// IMAGE_RESOURCE_DATA_ENTRY's OffsetToData is written here as an offset from
// the start of the section, and has to become a whole-image RVA once the
// linker has placed the section. That is what the relocation does. Omit them
// and the object still links, the file still loads, and FindResource returns
// nothing because every pointer in the tree is short by the section's address.
func buildRSRC(resources []resource) (section []byte, relocOffsets []uint32, err error) {
	// Entries at every level must be sorted ascending: Windows binary-searches
	// them, so an unsorted directory does not fail, it fails to find things.
	sort.SliceStable(resources, func(i, j int) bool {
		a, b := resources[i], resources[j]
		if a.typ != b.typ {
			return a.typ < b.typ
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return a.lang < b.lang
	})

	type idNode struct {
		id    uint32
		langs []resource
	}
	type typeNode struct {
		typ uint32
		ids []*idNode
	}

	var types []*typeNode
	for _, r := range resources {
		var tn *typeNode
		if len(types) > 0 && types[len(types)-1].typ == r.typ {
			tn = types[len(types)-1]
		} else {
			tn = &typeNode{typ: r.typ}
			types = append(types, tn)
		}
		var in *idNode
		if len(tn.ids) > 0 && tn.ids[len(tn.ids)-1].id == r.id {
			in = tn.ids[len(tn.ids)-1]
		} else {
			in = &idNode{id: r.id}
			tn.ids = append(tn.ids, in)
		}
		in.langs = append(in.langs, r)
	}

	const dirHeader = 16 // IMAGE_RESOURCE_DIRECTORY
	const dirEntry = 8   // IMAGE_RESOURCE_DIRECTORY_ENTRY
	const dataEntry = 16 // IMAGE_RESOURCE_DATA_ENTRY

	// Pass one: where everything goes.
	dirSize := dirHeader + dirEntry*len(types)
	for _, tn := range types {
		dirSize += dirHeader + dirEntry*len(tn.ids)
		for _, in := range tn.ids {
			dirSize += dirHeader + dirEntry*len(in.langs)
		}
	}
	leaves := 0
	for _, tn := range types {
		for _, in := range tn.ids {
			leaves += len(in.langs)
		}
	}

	dataEntriesAt := uint32(dirSize)
	blobsAt := align(dataEntriesAt+uint32(dataEntry*leaves), 8)

	// Pass two: write it.
	buf := new(bytes.Buffer)
	writeDirHeader := func(n int) {
		buf.Write(make([]byte, 8))                            // Characteristics, TimeDateStamp
		_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // MajorVersion
		_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // MinorVersion
		_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // NumberOfNamedEntries — this project uses ids only
		_ = binary.Write(buf, binary.LittleEndian, uint16(n)) // NumberOfIdEntries
	}
	writeDirEntry := func(id uint32, offset uint32, isDir bool) {
		_ = binary.Write(buf, binary.LittleEndian, id)
		if isDir {
			offset |= 0x80000000 // the high bit is what says "this points at another directory"
		}
		_ = binary.Write(buf, binary.LittleEndian, offset)
	}

	// Offsets of each child directory, computed in the same order they are written.
	typeDirAt := make([]uint32, len(types))
	idDirAt := make([][]uint32, len(types))
	next := uint32(dirHeader + dirEntry*len(types))
	for i, tn := range types {
		typeDirAt[i] = next
		next += uint32(dirHeader + dirEntry*len(tn.ids))
	}
	for i, tn := range types {
		idDirAt[i] = make([]uint32, len(tn.ids))
		for j, in := range tn.ids {
			idDirAt[i][j] = next
			next += uint32(dirHeader + dirEntry*len(in.langs))
		}
	}

	writeDirHeader(len(types))
	for i, tn := range types {
		writeDirEntry(tn.typ, typeDirAt[i], true)
	}
	for i, tn := range types {
		writeDirHeader(len(tn.ids))
		for j, in := range tn.ids {
			writeDirEntry(in.id, idDirAt[i][j], true)
		}
	}
	leafIndex := 0
	for _, tn := range types {
		for _, in := range tn.ids {
			writeDirHeader(len(in.langs))
			for _, r := range in.langs {
				writeDirEntry(r.lang, dataEntriesAt+uint32(leafIndex*dataEntry), false)
				leafIndex++
			}
		}
	}
	if uint32(buf.Len()) != dataEntriesAt {
		return nil, nil, fmt.Errorf("directory came out %d bytes, computed %d", buf.Len(), dataEntriesAt)
	}

	// The data entries, and the blobs they point at.
	blobOffset := blobsAt
	for _, tn := range types {
		for _, in := range tn.ids {
			for _, r := range in.langs {
				relocOffsets = append(relocOffsets, uint32(buf.Len()))
				_ = binary.Write(buf, binary.LittleEndian, blobOffset)          // OffsetToData — relocated
				_ = binary.Write(buf, binary.LittleEndian, uint32(len(r.data))) // Size
				_ = binary.Write(buf, binary.LittleEndian, uint32(0))           // CodePage
				_ = binary.Write(buf, binary.LittleEndian, uint32(0))           // Reserved
				blobOffset = align(blobOffset+uint32(len(r.data)), 8)
			}
		}
	}
	for uint32(buf.Len()) < blobsAt {
		buf.WriteByte(0)
	}
	for _, tn := range types {
		for _, in := range tn.ids {
			for _, r := range in.langs {
				buf.Write(r.data)
				for buf.Len()%8 != 0 {
					buf.WriteByte(0)
				}
			}
		}
	}

	return buf.Bytes(), relocOffsets, nil
}

func align(x, to uint32) uint32 {
	if r := x % to; r != 0 {
		return x + (to - r)
	}
	return x
}

// --------------------------------------------------------- the COFF object

// coffObject wraps the .rsrc section in the object file the Go linker will
// pick up. It is x86-64 only, which is what this project ships (build.ps1
// sets GOARCH=amd64), and the filename the caller writes it to is what keeps
// it out of every other platform's build.
func coffObject(resources []resource) ([]byte, error) {
	section, relocs, err := buildRSRC(resources)
	if err != nil {
		return nil, err
	}

	const (
		fileHeaderSize    = 20
		sectionHeaderSize = 40
		relocSize         = 10
		symbolSize        = 18
	)
	rawDataAt := uint32(fileHeaderSize + sectionHeaderSize)
	relocsAt := rawDataAt + uint32(len(section))
	symbolsAt := relocsAt + uint32(len(relocs)*relocSize)

	buf := new(bytes.Buffer)

	// IMAGE_FILE_HEADER
	_ = binary.Write(buf, binary.LittleEndian, uint16(0x8664)) // Machine = AMD64
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))      // NumberOfSections
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))      // TimeDateStamp: zero, so the output is reproducible
	_ = binary.Write(buf, binary.LittleEndian, symbolsAt)      // PointerToSymbolTable
	_ = binary.Write(buf, binary.LittleEndian, uint32(1))      // NumberOfSymbols
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))      // SizeOfOptionalHeader: an object has none
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))      // Characteristics

	// IMAGE_SECTION_HEADER
	name := make([]byte, 8)
	copy(name, ".rsrc")
	buf.Write(name)
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))            // VirtualSize
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))            // VirtualAddress
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(section))) // SizeOfRawData
	_ = binary.Write(buf, binary.LittleEndian, rawDataAt)            // PointerToRawData
	_ = binary.Write(buf, binary.LittleEndian, relocsAt)             // PointerToRelocations
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))            // PointerToLinenumbers
	_ = binary.Write(buf, binary.LittleEndian, uint16(len(relocs)))  // NumberOfRelocations
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))            // NumberOfLinenumbers
	// CNT_INITIALIZED_DATA | ALIGN_8BYTES | MEM_READ
	_ = binary.Write(buf, binary.LittleEndian, uint32(0x40)|uint32(0x00400000)|uint32(0x40000000))

	buf.Write(section)

	// The relocations. ADDR32NB is "32-bit address with no image base", which
	// is exactly an RVA: the linker adds the placed section's address to the
	// section-relative offset already stored at the target.
	const imageRelAmd64Addr32NB = 0x0003
	for _, off := range relocs {
		_ = binary.Write(buf, binary.LittleEndian, off)                           // VirtualAddress
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))                     // SymbolTableIndex — the .rsrc symbol below
		_ = binary.Write(buf, binary.LittleEndian, uint16(imageRelAmd64Addr32NB)) // Type
	}

	// One symbol: the section itself, which is what the relocations above are
	// relative to.
	buf.Write(name)                                       // ".rsrc", already 8 bytes
	_ = binary.Write(buf, binary.LittleEndian, uint32(0)) // Value
	_ = binary.Write(buf, binary.LittleEndian, int16(1))  // SectionNumber, 1-based
	_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // Type
	buf.WriteByte(3)                                      // StorageClass = IMAGE_SYM_CLASS_STATIC
	buf.WriteByte(0)                                      // NumberOfAuxSymbols

	// The string table is mandatory even when empty, and its first field is
	// its own length — so four bytes reading 4.
	_ = binary.Write(buf, binary.LittleEndian, uint32(4))

	return buf.Bytes(), nil
}
