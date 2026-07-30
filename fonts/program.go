package fonts

import "github.com/aaripurna/sanur-pdf/core"

// FontProgram is everything the PDF writer needs to emit a font resource.
//
// It exists so the writer never has to know whether a face came from the
// built-in metric tables or a parsed .ttf: both produce a FontProgram, and the
// writer branches only on Standard14 to decide whether to embed the bytes.
type FontProgram struct {
	// BaseName is the PDF /BaseFont value.
	BaseName string

	// Standard14 marks a reader-provided font, which is emitted as a bare
	// Type1 dictionary with no descriptor and no embedded program.
	Standard14 bool

	// Data is the raw TrueType file, embedded as /FontFile2. Nil for
	// standard-14 fonts.
	Data []byte

	// Widths holds advance widths in 1/1000 em indexed by WinAnsi code, for
	// codes FirstChar..LastChar inclusive.
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
