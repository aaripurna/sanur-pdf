package fonts

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestParseSfntSlicesEveryTable(t *testing.T) {
	built := buildSfnt(versionTrueType, map[string][]byte{
		"head": make([]byte, 54),
		"maxp": {0, 1, 0, 0, 0, 7},
		"glyf": {1, 2, 3},
	})

	font, err := parseSfnt(built)
	if err != nil {
		t.Fatalf("parsing a font we just built: %v", err)
	}

	if font.version != versionTrueType {
		t.Errorf("version = %#08x", font.version)
	}
	if got := string(font.tables["glyf"]); got != "\x01\x02\x03" {
		t.Errorf("glyf = %q, want the three bytes written", got)
	}
	// The length recorded is the real one; the padding to a four-byte boundary must
	// not leak into the table a reader gets.
	if got := len(font.tables["glyf"]); got != 3 {
		t.Errorf("glyf is %d bytes, want 3 — padding leaked into the length", got)
	}
	if got := len(font.tables["maxp"]); got != 6 {
		t.Errorf("maxp is %d bytes, want 6", got)
	}
}

func TestBuildSfntAlignsTablesAndRecordsChecksums(t *testing.T) {
	tables := map[string][]byte{
		"head": make([]byte, 54),
		"aaaa": {1},          // one byte, so it needs three of padding
		"bbbb": {1, 2, 3, 4}, // already aligned
	}

	built := buildSfnt(versionTrueType, tables)

	count := int(binary.BigEndian.Uint16(built[4:]))
	if count != 3 {
		t.Fatalf("table count = %d, want 3", count)
	}

	for i := 0; i < count; i++ {
		entry := built[12+i*16:]
		tag := string(entry[0:4])
		recorded := binary.BigEndian.Uint32(entry[4:])
		offset := int(binary.BigEndian.Uint32(entry[8:]))
		length := int(binary.BigEndian.Uint32(entry[12:]))

		if offset%4 != 0 {
			t.Errorf("table %q starts at %d, which is not four-byte aligned", tag, offset)
		}
		if length != len(tables[tag]) {
			t.Errorf("table %q records length %d, want %d", tag, length, len(tables[tag]))
		}
		// head is rewritten after the directory is built, so its recorded checksum
		// is expected to be stale — the format defines it that way.
		if tag != "head" {
			if want := checksum(built[offset : offset+length]); recorded != want {
				t.Errorf("table %q checksum = %#08x, want %#08x", tag, recorded, want)
			}
		}
	}
}

func TestBuildSfntWritesTheWholeFileChecksum(t *testing.T) {
	// A validator recomputes this field, and a font that fails validation is a font
	// somebody is eventually told to stop using.
	built := buildSfnt(versionTrueType, map[string][]byte{
		"head": make([]byte, 54),
		"glyf": {9, 9, 9, 9},
	})

	font, err := parseSfnt(built)
	if err != nil {
		t.Fatal(err)
	}

	stored := binary.BigEndian.Uint32(font.tables["head"][8:])

	// The definition is the magic constant minus the checksum of the whole file
	// computed with this very field zeroed, so reproducing it means zeroing it again.
	zeroed := make([]byte, len(built))
	copy(zeroed, built)

	count := int(binary.BigEndian.Uint16(zeroed[4:]))
	for i := 0; i < count; i++ {
		entry := zeroed[12+i*16:]
		if string(entry[0:4]) != "head" {
			continue
		}
		offset := int(binary.BigEndian.Uint32(entry[8:]))
		copy(zeroed[offset+8:offset+12], []byte{0, 0, 0, 0})
	}

	if want := 0xB1B0AFBA - checksum(zeroed); stored != want {
		t.Errorf("checkSumAdjustment = %#08x, want %#08x", stored, want)
	}
}

func TestParseSfntRejectsWhatItCannotHandle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		data     []byte
		mentions string
	}{
		{"empty", nil, "too short"},
		{"truncated directory", []byte("\x00\x01\x00\x00\x00\x05"), "too short"},
		{"unknown version", append([]byte("xxxx"), make([]byte, 20)...), "sfnt version"},
		{
			// A collection holds several faces and has no single table directory,
			// so the error has to say what to do about it rather than complain
			// about a malformed font.
			name:     "collection",
			data:     append([]byte("ttcf"), make([]byte, 20)...),
			mentions: "collection",
		},
	} {
		_, err := parseSfnt(tc.data)
		if err == nil {
			t.Errorf("%s was accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.mentions) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err, tc.mentions)
		}
	}
}

func TestParseSfntRejectsATableOutsideTheFile(t *testing.T) {
	data := make([]byte, 12+16)
	binary.BigEndian.PutUint32(data, versionTrueType)
	binary.BigEndian.PutUint16(data[4:], 1)
	copy(data[12:16], "glyf")
	binary.BigEndian.PutUint32(data[20:], 1<<20) // offset far past the end

	if _, err := parseSfnt(data); err == nil {
		t.Error("a table starting past the end of the font was accepted")
	}
}

func TestParseSfntClampsATruncatedTable(t *testing.T) {
	// Fonts in the wild do end with a table whose recorded length runs a few bytes
	// past the file. Refusing them buys nothing; clamping keeps the rest readable.
	data := make([]byte, 12+16+4)
	binary.BigEndian.PutUint32(data, versionTrueType)
	binary.BigEndian.PutUint16(data[4:], 1)
	copy(data[12:16], "glyf")
	binary.BigEndian.PutUint32(data[20:], 12+16)
	binary.BigEndian.PutUint32(data[24:], 999)

	font, err := parseSfnt(data)
	if err != nil {
		t.Fatalf("a truncated final table was refused: %v", err)
	}
	if got := len(font.tables["glyf"]); got != 4 {
		t.Errorf("clamped table is %d bytes, want 4", got)
	}
}

func TestMissingTableIsReportedByName(t *testing.T) {
	font := &sfntFile{tables: map[string][]byte{"glyf": {}}}

	if font.has("glyf") {
		t.Error("an empty table should not count as present")
	}

	_, err := font.table("loca")
	if err == nil {
		t.Fatal("a missing table was not reported")
	}
	if !strings.Contains(err.Error(), "loca") {
		t.Errorf("error %q does not name the table", err)
	}
}

func TestCFFIsDetectedFromEitherSignal(t *testing.T) {
	for _, tc := range []struct {
		name string
		font *sfntFile
		want bool
	}{
		{"OTTO version", &sfntFile{version: versionOpenTypeCFF, tables: map[string][]byte{}}, true},
		{"CFF table", &sfntFile{version: versionTrueType, tables: map[string][]byte{"CFF ": {1}}}, true},
		{"CFF2 table", &sfntFile{version: versionTrueType, tables: map[string][]byte{"CFF2": {1}}}, true},
		{"glyf outlines", &sfntFile{version: versionTrueType, tables: map[string][]byte{"glyf": {1}}}, false},
	} {
		if got := tc.font.isCFF(); got != tc.want {
			t.Errorf("%s: isCFF = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChecksumTreatsTheTailAsPadded(t *testing.T) {
	// The specification defines the sum over 32-bit words with the final partial word
	// padded with zeros, so a trailing byte contributes to the top of a word rather
	// than the bottom.
	if got, want := checksum([]byte{0x01}), uint32(0x01000000); got != want {
		t.Errorf("checksum of one byte = %#08x, want %#08x", got, want)
	}
	if got, want := checksum([]byte{0x00, 0x00, 0x00, 0x02, 0x03}), uint32(0x03000002); got != want {
		t.Errorf("checksum = %#08x, want %#08x", got, want)
	}
	if got := checksum(nil); got != 0 {
		t.Errorf("checksum of nothing = %#08x, want 0", got)
	}
}

func TestAlign4(t *testing.T) {
	for in, want := range map[int]int{0: 0, 1: 4, 3: 4, 4: 4, 5: 8} {
		if got := align4(in); got != want {
			t.Errorf("align4(%d) = %d, want %d", in, got, want)
		}
	}
}
