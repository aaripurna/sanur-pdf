package fonts

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// This file is a minimal reader and writer for the sfnt container that both
// TrueType and OpenType files use.
//
// golang.org/x/image/font/sfnt already parses these files, and it is what supplies
// metrics and glyph indices. What it does not expose is the raw table bytes, and
// subsetting is fundamentally an operation on those: keep some tables, rewrite
// three of them, drop the rest, and recompute the checksums. That is the whole job
// of this file — it reads no glyph outlines and understands no glyph semantics.

// sfntVersion values distinguish the two outline formats a font can carry.
const (
	// versionTrueType marks glyf-based outlines. Some older fonts write the ASCII
	// tag "true" instead, which means the same thing.
	versionTrueType = 0x00010000
	versionTrue     = 0x74727565 // "true"

	// versionOpenTypeCFF marks CFF (PostScript) outlines, in a table named "CFF ".
	versionOpenTypeCFF = 0x4F54544F // "OTTO"

	// versionCollection marks a font collection, which holds several faces and has
	// no single table directory of its own.
	versionCollection = 0x74746366 // "ttcf"
)

// sfntFile is a parsed table directory: the tables themselves, keyed by tag.
type sfntFile struct {
	version uint32
	tables  map[string][]byte
}

// parseSfnt reads the table directory and slices out every table.
//
// The slices alias the input rather than copying it. Nothing here mutates them,
// and a 23MB font would otherwise be duplicated for no reason.
func parseSfnt(data []byte) (*sfntFile, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("sanur/fonts: font is %d bytes, too short for a table directory", len(data))
	}

	version := binary.BigEndian.Uint32(data)
	switch version {
	case versionTrueType, versionTrue, versionOpenTypeCFF:
	case versionCollection:
		return nil, fmt.Errorf("sanur/fonts: font collections (.ttc) are not supported; " +
			"extract the face you want first")
	default:
		return nil, fmt.Errorf("sanur/fonts: unrecognised sfnt version %#08x", version)
	}

	count := int(binary.BigEndian.Uint16(data[4:]))
	if 12+count*16 > len(data) {
		return nil, fmt.Errorf("sanur/fonts: table directory claims %d tables, "+
			"which does not fit in %d bytes", count, len(data))
	}

	file := &sfntFile{version: version, tables: make(map[string][]byte, count)}

	for i := 0; i < count; i++ {
		entry := data[12+i*16:]
		tag := string(entry[0:4])
		offset := int(binary.BigEndian.Uint32(entry[8:]))
		length := int(binary.BigEndian.Uint32(entry[12:]))

		// A truncated final table is common enough in fonts in the wild that
		// clamping is kinder than refusing the file. Only a length that starts past
		// the end is unrecoverable.
		if offset < 0 || offset > len(data) {
			return nil, fmt.Errorf("sanur/fonts: table %q starts at %d, past the end of the font", tag, offset)
		}
		if end := offset + length; end > len(data) {
			length = len(data) - offset
		}

		file.tables[tag] = data[offset : offset+length]
	}

	return file, nil
}

// has reports whether a table is present and non-empty.
func (f *sfntFile) has(tag string) bool { return len(f.tables[tag]) > 0 }

// table returns a table, or an error naming it. Every caller needs the table it
// asks for, so a missing one is always fatal and always wants the same message.
func (f *sfntFile) table(tag string) ([]byte, error) {
	if data, ok := f.tables[tag]; ok && len(data) > 0 {
		return data, nil
	}
	return nil, fmt.Errorf("sanur/fonts: font has no %q table", tag)
}

// isCFF reports whether the outlines are PostScript rather than TrueType.
//
// This decides how the program is embedded — /FontFile3 against /FontFile2 — and
// whether it can be subsetted at all, since CFF charstrings are a different format
// from glyf outlines.
func (f *sfntFile) isCFF() bool {
	return f.version == versionOpenTypeCFF || f.has("CFF ") || f.has("CFF2")
}

// buildSfnt assembles a font file from a set of tables.
//
// Tables are written in tag order, which is not required by the specification but
// makes the output reproducible and easy to diff. Each table is padded to a
// four-byte boundary; the padding is included in the offsets of later tables but
// not in the recorded length, which is what the format requires.
func buildSfnt(version uint32, tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	directorySize := 12 + 16*len(tags)

	total := directorySize
	for _, tag := range tags {
		total += align4(len(tables[tag]))
	}

	out := make([]byte, directorySize, total)

	binary.BigEndian.PutUint32(out, version)
	binary.BigEndian.PutUint16(out[4:], uint16(len(tags)))

	// searchRange, entrySelector and rangeShift describe a binary search over the
	// directory. Readers compute them themselves in practice, but a validator will
	// object if they disagree with the table count.
	entrySelector := uint16(0)
	for 1<<(entrySelector+1) <= len(tags) {
		entrySelector++
	}
	searchRange := uint16(16) << entrySelector
	binary.BigEndian.PutUint16(out[6:], searchRange)
	binary.BigEndian.PutUint16(out[8:], entrySelector)
	binary.BigEndian.PutUint16(out[10:], uint16(16*len(tags))-searchRange)

	for i, tag := range tags {
		body := tables[tag]
		offset := len(out)

		out = append(out, body...)
		for len(out) < offset+align4(len(body)) {
			out = append(out, 0)
		}

		entry := out[12+i*16:]
		copy(entry[0:4], tag)
		binary.BigEndian.PutUint32(entry[4:], checksum(body))
		binary.BigEndian.PutUint32(entry[8:], uint32(offset))
		binary.BigEndian.PutUint32(entry[12:], uint32(len(body)))
	}

	setHeadChecksumAdjustment(out, tables)
	return out
}

// setHeadChecksumAdjustment stores the whole-file checksum inside the head table.
//
// The field is defined as a magic constant minus the checksum of the entire file
// computed with the field itself zeroed, so it can only be filled in once
// everything else is in place. No reader sanur is likely to meet verifies it, but
// font validators do, and a font that fails validation is a font somebody will
// eventually be told to stop using.
func setHeadChecksumAdjustment(font []byte, tables map[string][]byte) {
	head, ok := tables["head"]
	if !ok || len(head) < 12 {
		return
	}

	// Find where head landed, so the field can be zeroed in place before summing.
	count := int(binary.BigEndian.Uint16(font[4:]))
	for i := 0; i < count; i++ {
		entry := font[12+i*16:]
		if string(entry[0:4]) != "head" {
			continue
		}

		offset := int(binary.BigEndian.Uint32(entry[8:]))
		if offset+12 > len(font) {
			return
		}

		field := font[offset+8 : offset+12]
		copy(field, []byte{0, 0, 0, 0})
		binary.BigEndian.PutUint32(field, 0xB1B0AFBA-checksum(font))
		return
	}
}

// checksum is the sum of a table's contents read as big-endian 32-bit words, with
// the tail treated as if padded with zeros.
func checksum(data []byte) uint32 {
	var sum uint32

	full := len(data) / 4 * 4
	for i := 0; i < full; i += 4 {
		sum += binary.BigEndian.Uint32(data[i:])
	}

	if rest := data[full:]; len(rest) > 0 {
		var word [4]byte
		copy(word[:], rest)
		sum += binary.BigEndian.Uint32(word[:])
	}

	return sum
}

func align4(n int) int { return (n + 3) & ^3 }
