package fonts

import "github.com/aaripurna/sanur-pdf/core"

// FontProgram is everything the PDF writer needs to emit a font resource, apart
// from the program bytes of an embedded font.
//
// It exists so the writer never has to know whether a face came from the built-in
// metric tables or a parsed .ttf. The two kinds are genuinely different objects in
// a PDF — a bare Type1 dictionary resolved by name against a bare Type0 font with a
// descendant CID font, a descriptor and an embedded program — and the two flags
// below are how the writer tells them apart.
type FontProgram struct {
	// BaseName is the PDF /BaseFont value, before any subset tag is applied.
	BaseName string

	// Standard14 marks a reader-provided font, emitted as a bare Type1 dictionary
	// with no descriptor and no embedded program.
	Standard14 bool

	// Composite marks a font emitted as a Type0 font with Identity-H encoding: two
	// bytes per glyph, addressing the font's glyphs directly instead of through a
	// 256-entry encoding table. This is what lets a document contain any script the
	// registered font covers.
	//
	// The program bytes are not carried here, because they depend on which glyphs
	// the document used. The writer obtains them from GlyphSource.Subset once every
	// page has been drawn.
	Composite bool

	// DefaultWidth is the advance assumed for a glyph absent from the width array,
	// emitted as /DW.
	DefaultWidth int

	// Widths holds advance widths in 1/1000 em indexed by WinAnsi code, for
	// codes FirstChar..LastChar inclusive. Simple fonts only.
	Widths    [256]int
	FirstChar int
	LastChar  int

	// Descriptor fields, all in 1/1000 em where applicable.
	Flags       int
	BBox        [4]int
	ItalicAngle int
	Ascent      int
	Descent     int // negative, as PDF requires
	CapHeight   int
	StemV       int
}

// PDF font descriptor flag bits (PDF 32000-1 table 123).
const (
	FlagFixedPitch  = 1 << 0
	FlagSerif       = 1 << 1
	FlagSymbolic    = 1 << 2
	FlagScript      = 1 << 3
	FlagNonsymbolic = 1 << 5
	FlagItalic      = 1 << 6
	FlagForceBold   = 1 << 18
)

// Programmable is implemented by fonts that can describe themselves to the PDF
// writer. Every font sanur produces satisfies it; the interface is asserted
// rather than folded into core.Font so that core stays free of PDF concepts.
type Programmable interface {
	Program() FontProgram
}

// ProgramOf extracts the PDF program for a font, reporting whether the font
// supports it.
func ProgramOf(f core.Font) (FontProgram, bool) {
	p, ok := f.(Programmable)
	if !ok {
		return FontProgram{}, false
	}
	return p.Program(), true
}

// Program implements Programmable for the built-in faces.
func (f *standardFont) Program() FontProgram {
	p := FontProgram{
		BaseName:   f.name,
		Standard14: true,
		FirstChar:  0x20,
		LastChar:   0xFF,
		Flags:      FlagNonsymbolic,
		Ascent:     int(f.ascent),
		Descent:    -int(f.descent),
		CapHeight:  int(f.ascent),
		StemV:      80,
	}
	for c := p.FirstChar; c <= p.LastChar; c++ {
		w := f.widths[c]
		if w == 0 {
			w = f.missing
		}
		p.Widths[c] = int(w)
	}
	return p
}
