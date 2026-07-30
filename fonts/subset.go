package fonts

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// subsetGlyf builds a font containing only the glyphs in keep.
//
// Glyph IDs are preserved rather than renumbered. That is the decision everything
// else here follows from: the PDF font is a CIDFontType2 with /CIDToGIDMap /Identity,
// so the IDs in the content stream index this font's glyphs directly. Renumbering
// would save a little space in the loca table and cost a mapping stream, a
// translation layer on every draw call, and a class of bug where the two disagree.
//
// Unused glyphs become zero-length entries in loca, which is how the format spells
// "no outline". The tables are then trimmed to the highest glyph actually used,
// since a Latin subset of a CJK font otherwise carries twenty thousand empty slots.
func subsetGlyf(font *sfntFile, keep map[uint16]bool) ([]byte, error) {
	head, err := font.table("head")
	if err != nil {
		return nil, err
	}
	if len(head) < 54 {
		return nil, fmt.Errorf("sanur/fonts: head table is %d bytes, too short", len(head))
	}

	maxp, err := font.table("maxp")
	if err != nil {
		return nil, err
	}
	if len(maxp) < 6 {
		return nil, fmt.Errorf("sanur/fonts: maxp table is %d bytes, too short", len(maxp))
	}

	hhea, err := font.table("hhea")
	if err != nil {
		return nil, err
	}
	if len(hhea) < 36 {
		return nil, fmt.Errorf("sanur/fonts: hhea table is %d bytes, too short", len(hhea))
	}

	glyf, err := font.table("glyf")
	if err != nil {
		return nil, err
	}

	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))

	offsets, err := readLoca(font, numGlyphs, int16(binary.BigEndian.Uint16(head[50:])))
	if err != nil {
		return nil, err
	}

	// A composite glyph is drawn out of other glyphs, so keeping it means keeping
	// everything it refers to, transitively.
	wanted, err := closeOverComponents(glyf, offsets, keep, numGlyphs)
	if err != nil {
		return nil, err
	}

	highest := 0
	for gid := range wanted {
		if int(gid) > highest {
			highest = int(gid)
		}
	}
	kept := highest + 1

	newGlyf, newOffsets := rebuildGlyf(glyf, offsets, wanted, kept)

	// The long format is only needed once offsets stop fitting in the halved
	// sixteen-bit form, and a Latin subset almost never gets there.
	longLoca := newOffsets[len(newOffsets)-1] > 0x1FFFF
	newLoca := writeLoca(newOffsets, longLoca)

	hmtx, err := font.table("hmtx")
	if err != nil {
		return nil, err
	}
	newHmtx := rebuildHmtx(hmtx, int(binary.BigEndian.Uint16(hhea[34:])), numGlyphs, kept)

	tables := map[string][]byte{
		"head": patched(head, func(t []byte) {
			format := uint16(0)
			if longLoca {
				format = 1
			}
			binary.BigEndian.PutUint16(t[50:], format)
		}),
		"maxp": patched(maxp, func(t []byte) {
			binary.BigEndian.PutUint16(t[4:], uint16(kept))
		}),
		"hhea": patched(hhea, func(t []byte) {
			// Every retained glyph gets its own metric entry, so the compressed tail
			// the format allows is simply not used.
			binary.BigEndian.PutUint16(t[34:], uint16(kept))
		}),
		"hmtx": newHmtx,
		"loca": newLoca,
		"glyf": newGlyf,
	}

	// The hinting tables are copied through when present. They are small, they are
	// what makes a font look right at small sizes on a screen, and dropping them
	// would be a visible regression for the sake of a few hundred bytes.
	//
	// cmap, name, post and the layout tables are deliberately absent: with
	// Identity-H the PDF addresses glyphs by ID, so nothing consults a character
	// map, and no reader needs the font's own names.
	for _, tag := range []string{"cvt ", "fpgm", "prep", "gasp"} {
		if font.has(tag) {
			tables[tag] = font.tables[tag]
		}
	}

	return buildSfnt(versionTrueType, tables), nil
}

// patched returns a mutable copy of a table with an edit applied, so the original
// font's bytes are never written to.
func patched(table []byte, edit func([]byte)) []byte {
	out := make([]byte, len(table))
	copy(out, table)
	edit(out)
	return out
}

// readLoca returns the numGlyphs+1 glyph offsets into the glyf table.
func readLoca(font *sfntFile, numGlyphs int, format int16) ([]uint32, error) {
	loca, err := font.table("loca")
	if err != nil {
		return nil, err
	}

	offsets := make([]uint32, numGlyphs+1)

	switch format {
	case 0:
		if len(loca) < 2*(numGlyphs+1) {
			return nil, fmt.Errorf("sanur/fonts: short loca holds %d of %d entries",
				len(loca)/2, numGlyphs+1)
		}
		for i := range offsets {
			// The short form stores halved offsets, which is why it can address
			// twice what sixteen bits otherwise would.
			offsets[i] = uint32(binary.BigEndian.Uint16(loca[i*2:])) * 2
		}
	case 1:
		if len(loca) < 4*(numGlyphs+1) {
			return nil, fmt.Errorf("sanur/fonts: long loca holds %d of %d entries",
				len(loca)/4, numGlyphs+1)
		}
		for i := range offsets {
			offsets[i] = binary.BigEndian.Uint32(loca[i*4:])
		}
	default:
		return nil, fmt.Errorf("sanur/fonts: unknown loca format %d", format)
	}

	return offsets, nil
}

func writeLoca(offsets []uint32, long bool) []byte {
	if long {
		out := make([]byte, 4*len(offsets))
		for i, off := range offsets {
			binary.BigEndian.PutUint32(out[i*4:], off)
		}
		return out
	}

	out := make([]byte, 2*len(offsets))
	for i, off := range offsets {
		binary.BigEndian.PutUint16(out[i*2:], uint16(off/2))
	}
	return out
}

// glyphAt returns the outline bytes for one glyph, or nil when it has none.
func glyphAt(glyf []byte, offsets []uint32, gid int) []byte {
	if gid < 0 || gid+1 >= len(offsets) {
		return nil
	}

	start, end := int(offsets[gid]), int(offsets[gid+1])
	if end <= start || start > len(glyf) {
		return nil
	}
	if end > len(glyf) {
		end = len(glyf)
	}
	return glyf[start:end]
}

// rebuildGlyf writes the kept outlines back to back and returns the new offsets.
func rebuildGlyf(glyf []byte, offsets []uint32, wanted map[uint16]bool, kept int) ([]byte, []uint32) {
	out := make([]byte, 0, len(glyf)/4)
	newOffsets := make([]uint32, kept+1)

	for gid := 0; gid < kept; gid++ {
		newOffsets[gid] = uint32(len(out))

		if !wanted[uint16(gid)] {
			continue
		}

		out = append(out, glyphAt(glyf, offsets, gid)...)

		// Offsets must be even for the short loca format to be able to express
		// them, and four-byte alignment is what every other tool emits.
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}

	newOffsets[kept] = uint32(len(out))
	return out, newOffsets
}

// rebuildHmtx writes one full metric entry per retained glyph.
//
// The format allows a run of trailing glyphs to share the last advance width and
// record only their left side bearing. Expanding that to full entries costs two
// bytes per glyph and removes the only place where hmtx and hhea can fall out of
// step, which is a trade worth making in a subsetter that has to be right.
func rebuildHmtx(hmtx []byte, numberOfHMetrics, numGlyphs, kept int) []byte {
	if numberOfHMetrics <= 0 {
		numberOfHMetrics = 1
	}

	read := func(gid int) (advance uint16, bearing int16) {
		if gid < numberOfHMetrics {
			if off := gid * 4; off+4 <= len(hmtx) {
				return binary.BigEndian.Uint16(hmtx[off:]),
					int16(binary.BigEndian.Uint16(hmtx[off+2:]))
			}
			return 0, 0
		}

		// Past the metric count every glyph repeats the final advance and carries
		// only its own bearing, in an array that follows the pairs.
		last := uint16(0)
		if off := (numberOfHMetrics - 1) * 4; off+2 <= len(hmtx) {
			last = binary.BigEndian.Uint16(hmtx[off:])
		}

		off := numberOfHMetrics*4 + (gid-numberOfHMetrics)*2
		if off+2 <= len(hmtx) {
			return last, int16(binary.BigEndian.Uint16(hmtx[off:]))
		}
		return last, 0
	}

	out := make([]byte, 4*kept)
	for gid := 0; gid < kept && gid < numGlyphs; gid++ {
		advance, bearing := read(gid)
		binary.BigEndian.PutUint16(out[gid*4:], advance)
		binary.BigEndian.PutUint16(out[gid*4+2:], uint16(bearing))
	}
	return out
}

// Composite glyph flags, from the glyf table specification.
const (
	compositeArgsAreWords    = 0x0001
	compositeHaveScale       = 0x0008
	compositeMoreComponents  = 0x0020
	compositeHaveXYScale     = 0x0040
	compositeHaveTwoByTwo    = 0x0080
	compositeComponentHeader = 10 // numberOfContours plus the bounding box
)

// closeOverComponents expands a glyph set to include everything its composites
// refer to.
//
// An accented letter is usually stored as a reference to the base letter plus a
// reference to the accent. Keeping only the composite would leave both parts
// blank, and the glyph would render as nothing at all — which is the kind of bug
// that appears in one language and not another.
func closeOverComponents(glyf []byte, offsets []uint32, keep map[uint16]bool, numGlyphs int) (map[uint16]bool, error) {
	wanted := make(map[uint16]bool, len(keep)+8)

	// Glyph zero is .notdef, drawn for anything the font cannot represent. It is
	// always kept so a missing character shows as a box rather than nothing.
	pending := []uint16{0}
	wanted[0] = true

	for gid := range keep {
		if int(gid) < numGlyphs && !wanted[gid] {
			wanted[gid] = true
			pending = append(pending, gid)
		}
	}

	for len(pending) > 0 {
		gid := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		components, err := componentsOf(glyphAt(glyf, offsets, int(gid)))
		if err != nil {
			return nil, fmt.Errorf("sanur/fonts: glyph %d: %w", gid, err)
		}

		for _, component := range components {
			if int(component) >= numGlyphs || wanted[component] {
				continue
			}
			wanted[component] = true
			pending = append(pending, component)
		}
	}

	return wanted, nil
}

// componentsOf returns the glyph IDs a composite glyph is built from, or nil for a
// simple glyph.
func componentsOf(glyph []byte) ([]uint16, error) {
	if len(glyph) < compositeComponentHeader {
		return nil, nil
	}
	if contours := int16(binary.BigEndian.Uint16(glyph)); contours >= 0 {
		return nil, nil
	}

	var components []uint16

	pos := compositeComponentHeader
	for {
		if pos+4 > len(glyph) {
			return nil, fmt.Errorf("composite glyph ends mid-component")
		}

		flags := binary.BigEndian.Uint16(glyph[pos:])
		components = append(components, binary.BigEndian.Uint16(glyph[pos+2:]))
		pos += 4

		if flags&compositeArgsAreWords != 0 {
			pos += 4
		} else {
			pos += 2
		}

		switch {
		case flags&compositeHaveTwoByTwo != 0:
			pos += 8
		case flags&compositeHaveXYScale != 0:
			pos += 4
		case flags&compositeHaveScale != 0:
			pos += 2
		}

		if flags&compositeMoreComponents == 0 {
			return components, nil
		}
	}
}

// SortedGlyphIDs returns a set of glyph IDs in ascending order.
//
// Every consumer needs them ordered: the width array groups consecutive runs, the
// ToUnicode map is written in code order, and the subset tag hashes them. Map
// iteration is randomised, so sorting here is also what keeps the output of two
// identical runs byte-identical.
func SortedGlyphIDs(set map[uint16]bool) []uint16 {
	out := make([]uint16, 0, len(set))
	for gid := range set {
		out = append(out, gid)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
