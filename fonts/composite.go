package fonts

import (
	"encoding/binary"
	"fmt"
)

// GlyphSource is implemented by fonts that can be addressed by glyph ID.
//
// This is what makes text outside WinAnsi possible. A PDF simple font selects
// glyphs with one byte, so it can reach at most 256 of them through an encoding
// table — and WinAnsi, the only encoding whose repertoire every reader agrees on,
// stops at Western Europe. A composite font indexes the font's glyphs directly with
// two bytes, which reaches all of them and needs no encoding table at all.
//
// The interface is asserted rather than folded into core.Font, for the same reason
// Programmable is: core describes layout and knows nothing about PDF.
type GlyphSource interface {
	// GlyphID maps a rune to a glyph ID in this font, reporting false when the
	// font has no glyph for it.
	GlyphID(r rune) (uint16, bool)

	// SubstituteGlyph returns the glyph drawn for a rune the font cannot
	// represent — visibly wrong, rather than silently absent.
	SubstituteGlyph() uint16

	// GlyphWidth returns a glyph's advance width in 1/1000 em, the unit PDF
	// width arrays are defined in.
	GlyphWidth(gid uint16) int

	// Subset returns a font program holding only the given glyphs.
	Subset(gids map[uint16]bool) (Subset, error)
}

// Subset is a font program reduced to the glyphs a document actually used.
type Subset struct {
	// Data is the program to embed.
	Data []byte

	// Tag is the six-letter prefix PDF requires on a subsetted font's name, or
	// empty when the whole program was embedded.
	//
	// The tag exists so that two documents carrying different subsets of the same
	// typeface cannot be mistaken for the same font when they are merged, which
	// would leave one of them drawing blanks.
	Tag string

	// CFF marks a PostScript-outline program. It changes how the program is
	// embedded and which flavour of CID font describes it, because a CFF program is
	// not a TrueType program and a reader told otherwise will refuse it.
	CFF bool
}

// BaseName returns the PDF /BaseFont value for a subset of the named font.
func (s Subset) BaseName(name string) string {
	if s.Tag == "" {
		return name
	}
	return s.Tag + "+" + name
}

// GlyphSourceOf extracts the glyph-level interface for a font, reporting whether
// the font supports it.
func GlyphSourceOf(f interface{}) (GlyphSource, bool) {
	source, ok := f.(GlyphSource)
	return source, ok
}

// GlyphID maps a rune to a glyph ID.
func (f *trueTypeFont) GlyphID(r rune) (uint16, bool) {
	gid, ok := f.glyphIndex(r)
	return gid, ok
}

// SubstituteGlyph returns the glyph used for runes this font cannot represent.
func (f *trueTypeFont) SubstituteGlyph() uint16 { return f.substitute }

// GlyphWidth returns a glyph's advance in 1/1000 em.
func (f *trueTypeFont) GlyphWidth(gid uint16) int {
	return int(f.glyphAdvanceUnits(gid) / f.upem * 1000)
}

// Subset reduces the font program to the glyphs given.
//
// A CFF program is returned whole. Subsetting one means rewriting PostScript
// charstrings and the private dictionaries they depend on, which is a different and
// much larger job than trimming a glyf table, and getting it subtly wrong produces
// a font that renders incorrectly rather than one that fails to load. Embedding the
// whole program is honest, correct, and the same thing every other Go PDF library
// does; the size cost is called out in the README.
func (f *trueTypeFont) Subset(gids map[uint16]bool) (Subset, error) {
	container, err := parseSfnt(f.data)
	if err != nil {
		return Subset{}, fmt.Errorf("sanur/fonts: subsetting %q: %w", f.name, err)
	}

	if container.isCFF() {
		return Subset{Data: f.data, CFF: true}, nil
	}

	data, err := subsetGlyf(container, gids)
	if err != nil {
		return Subset{}, fmt.Errorf("sanur/fonts: subsetting %q: %w", f.name, err)
	}

	return Subset{Data: data, Tag: subsetTag(f.name, SortedGlyphIDs(gids))}, nil
}

// subsetTag derives the six uppercase letters PDF wants in front of a subsetted
// font name.
//
// It has to be stable across runs, because sanur guarantees that generating the
// same document twice produces identical bytes, and it has to differ when the glyph
// set differs, because that is the only thing the tag is for. A hash of the name
// and the glyph list gives both.
func subsetTag(name string, gids []uint16) string {
	// FNV-1a, written out rather than imported: hash/fnv would work, but the two
	// constants are the whole algorithm and this keeps the derivation visible next
	// to the guarantee it supports.
	const (
		offset = 2166136261
		prime  = 16777619
	)

	hash := uint32(offset)
	mix := func(b byte) {
		hash ^= uint32(b)
		hash *= prime
	}

	for i := 0; i < len(name); i++ {
		mix(name[i])
	}
	for _, gid := range gids {
		var pair [2]byte
		binary.BigEndian.PutUint16(pair[:], gid)
		mix(pair[0])
		mix(pair[1])
	}

	var tag [6]byte
	for i := range tag {
		tag[i] = byte('A' + hash%26)
		hash /= 26
	}
	return string(tag[:])
}

var _ GlyphSource = (*trueTypeFont)(nil)
