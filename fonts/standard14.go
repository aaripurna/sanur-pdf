package fonts

import (
	"fmt"
	"strings"

	"codeberg.org/aaripurna/sanur/core"
)

// The standard-14 fonts are the typefaces every conforming PDF reader is
// required to provide, so a document using them embeds no font data at all.
// Sanur ships their metrics so that generating a PDF needs no font files on
// disk — a library that cannot draw text until the caller finds a .ttf is not
// much of a library.
//
// Only the families whose Adobe metrics are reproduced exactly are offered.
// Guessing widths would produce documents that measure correctly in Go and
// visibly wrong in a reader, so anything else requires RegisterTrueType.
const (
	Helvetica            = "Helvetica"
	HelveticaBold        = "Helvetica-Bold"
	HelveticaOblique     = "Helvetica-Oblique"
	HelveticaBoldOblique = "Helvetica-BoldOblique"

	Courier            = "Courier"
	CourierBold        = "Courier-Bold"
	CourierOblique     = "Courier-Oblique"
	CourierBoldOblique = "Courier-BoldOblique"
)

// unitsPerEm for the standard-14 metrics, which Adobe publishes in 1/1000 em.
const standardUnitsPerEm = 1000.0

// lineHeightFactor scales the font's ink height into a baseline-to-baseline
// distance. Neither the standard-14 metrics nor most TrueType hhea tables carry
// a usable line gap, so sanur applies one consistent typographic default rather
// than letting line spacing vary with how a font was compiled.
const lineHeightFactor = 1.2

// widthRun assigns one advance width to an inclusive range of WinAnsi codes.
// The tables below are written as runs because most of a font's width data is
// runs — digits, accented vowels, and the whole Courier repertoire are uniform,
// and collapsing them keeps the data short enough to proofread against the AFM.
type widthRun struct {
	from, to byte
	width    uint16
}

// standardFont is a standard-14 face: widths per WinAnsi code plus the vertical
// metrics needed to place baselines.
type standardFont struct {
	name    string
	widths  [256]uint16
	missing uint16
	ascent  float64
	descent float64
}

func (f *standardFont) Name() string { return f.name }

func (f *standardFont) AdvanceOf(r rune, size float64) float64 {
	w := f.missing
	if code, ok := WinAnsiCode(r); ok && f.widths[code] != 0 {
		w = f.widths[code]
	}
	return float64(w) / standardUnitsPerEm * size
}

func (f *standardFont) Measure(text string, size float64) float64 {
	var total float64
	for _, r := range text {
		total += f.AdvanceOf(r, size)
	}
	return total
}

func (f *standardFont) Ascent(size float64) float64 {
	return f.ascent / standardUnitsPerEm * size
}

func (f *standardFont) Descent(size float64) float64 {
	return f.descent / standardUnitsPerEm * size
}

func (f *standardFont) LineHeight(size float64) float64 {
	return (f.ascent + f.descent) / standardUnitsPerEm * size * lineHeightFactor
}

// build fills the width table from a list of runs.
func build(name string, ascent, descent float64, missing uint16, runs []widthRun) *standardFont {
	f := &standardFont{name: name, ascent: ascent, descent: descent, missing: missing}
	for _, r := range runs {
		for c := int(r.from); c <= int(r.to); c++ {
			f.widths[c] = r.width
		}
	}
	return f
}

// Helvetica metrics, from Adobe's Helvetica.afm / Helvetica-Bold.afm. The
// oblique faces are shears of the uprights and share their advance widths
// exactly, so they reuse the same tables under a different PDF base font name.
var helveticaWidths = []widthRun{
	{0x20, 0x20, 278}, {0x21, 0x21, 278}, {0x22, 0x22, 355}, {0x23, 0x23, 556},
	{0x24, 0x24, 556}, {0x25, 0x25, 889}, {0x26, 0x26, 667}, {0x27, 0x27, 191},
	{0x28, 0x28, 333}, {0x29, 0x29, 333}, {0x2A, 0x2A, 389}, {0x2B, 0x2B, 584},
	{0x2C, 0x2C, 278}, {0x2D, 0x2D, 333}, {0x2E, 0x2E, 278}, {0x2F, 0x2F, 278},
	{0x30, 0x39, 556}, // digits
	{0x3A, 0x3B, 278}, {0x3C, 0x3E, 584}, {0x3F, 0x3F, 556}, {0x40, 0x40, 1015},
	{0x41, 0x42, 667}, {0x43, 0x44, 722}, {0x45, 0x45, 667}, {0x46, 0x46, 611},
	{0x47, 0x47, 778}, {0x48, 0x48, 722}, {0x49, 0x49, 278}, {0x4A, 0x4A, 500},
	{0x4B, 0x4B, 667}, {0x4C, 0x4C, 556}, {0x4D, 0x4D, 833}, {0x4E, 0x4E, 722},
	{0x4F, 0x4F, 778}, {0x50, 0x50, 667}, {0x51, 0x51, 778}, {0x52, 0x52, 722},
	{0x53, 0x53, 667}, {0x54, 0x54, 611}, {0x55, 0x55, 722}, {0x56, 0x56, 667},
	{0x57, 0x57, 944}, {0x58, 0x59, 667}, {0x5A, 0x5A, 611},
	{0x5B, 0x5D, 278}, {0x5E, 0x5E, 469}, {0x5F, 0x5F, 556}, {0x60, 0x60, 333},
	{0x61, 0x62, 556}, {0x63, 0x63, 500}, {0x64, 0x65, 556}, {0x66, 0x66, 278},
	{0x67, 0x68, 556}, {0x69, 0x6A, 222}, {0x6B, 0x6B, 500}, {0x6C, 0x6C, 222},
	{0x6D, 0x6D, 833}, {0x6E, 0x71, 556}, {0x72, 0x72, 333}, {0x73, 0x73, 500},
	{0x74, 0x74, 278}, {0x75, 0x75, 556}, {0x76, 0x76, 500}, {0x77, 0x77, 722},
	{0x78, 0x7A, 500}, {0x7B, 0x7B, 334}, {0x7C, 0x7C, 260}, {0x7D, 0x7D, 334},
	{0x7E, 0x7E, 584},

	// WinAnsi punctuation block.
	{0x80, 0x80, 556},  // euro
	{0x82, 0x82, 222},  // quotesinglbase
	{0x83, 0x83, 556},  // florin
	{0x84, 0x84, 333},  // quotedblbase
	{0x85, 0x85, 1000}, // ellipsis
	{0x86, 0x87, 556},  // dagger, daggerdbl
	{0x88, 0x88, 333},  // circumflex
	{0x89, 0x89, 1000}, // perthousand
	{0x8A, 0x8A, 667},  // Scaron
	{0x8B, 0x8B, 333},  // guilsinglleft
	{0x8C, 0x8C, 1000}, // OE
	{0x8E, 0x8E, 611},  // Zcaron
	{0x91, 0x92, 222},  // quoteleft, quoteright
	{0x93, 0x94, 333},  // quotedblleft, quotedblright
	{0x95, 0x95, 350},  // bullet
	{0x96, 0x96, 556},  // endash
	{0x97, 0x97, 1000}, // emdash
	{0x98, 0x98, 333},  // tilde
	{0x99, 0x99, 1000}, // trademark
	{0x9A, 0x9A, 500},  // scaron
	{0x9B, 0x9B, 333},  // guilsinglright
	{0x9C, 0x9C, 944},  // oe
	{0x9E, 0x9E, 500},  // zcaron
	{0x9F, 0x9F, 667},  // Ydieresis

	// Latin-1 supplement.
	{0xA0, 0xA0, 278}, // no-break space
	{0xA1, 0xA1, 333}, {0xA2, 0xA5, 556}, {0xA6, 0xA6, 260}, {0xA7, 0xA7, 556},
	{0xA8, 0xA8, 333}, {0xA9, 0xA9, 737}, {0xAA, 0xAA, 370}, {0xAB, 0xAB, 556},
	{0xAC, 0xAC, 584}, {0xAD, 0xAD, 333}, {0xAE, 0xAE, 737}, {0xAF, 0xAF, 333},
	{0xB0, 0xB0, 400}, {0xB1, 0xB1, 584}, {0xB2, 0xB4, 333}, {0xB5, 0xB5, 556},
	{0xB6, 0xB6, 537}, {0xB7, 0xB7, 278}, {0xB8, 0xB9, 333}, {0xBA, 0xBA, 365},
	{0xBB, 0xBB, 556}, {0xBC, 0xBE, 834}, {0xBF, 0xBF, 611},
	{0xC0, 0xC5, 667}, // Agrave..Aring
	{0xC6, 0xC6, 1000}, {0xC7, 0xC7, 722},
	{0xC8, 0xCB, 667}, // Egrave..Edieresis
	{0xCC, 0xCF, 278}, // Igrave..Idieresis
	{0xD0, 0xD1, 722}, {0xD2, 0xD6, 778}, {0xD7, 0xD7, 584}, {0xD8, 0xD8, 778},
	{0xD9, 0xDC, 722}, {0xDD, 0xDE, 667}, {0xDF, 0xDF, 611},
	{0xE0, 0xE5, 556}, // agrave..aring
	{0xE6, 0xE6, 889}, {0xE7, 0xE7, 500},
	{0xE8, 0xEB, 556}, // egrave..edieresis
	{0xEC, 0xEF, 222}, // igrave..idieresis
	{0xF0, 0xF6, 556}, {0xF7, 0xF7, 584}, {0xF8, 0xF8, 611}, {0xF9, 0xFC, 556},
	{0xFD, 0xFD, 500}, {0xFE, 0xFE, 556}, {0xFF, 0xFF, 500},
}

var helveticaBoldWidths = []widthRun{
	{0x20, 0x20, 278}, {0x21, 0x21, 333}, {0x22, 0x22, 474}, {0x23, 0x23, 556},
	{0x24, 0x24, 556}, {0x25, 0x25, 889}, {0x26, 0x26, 722}, {0x27, 0x27, 238},
	{0x28, 0x29, 333}, {0x2A, 0x2A, 389}, {0x2B, 0x2B, 584}, {0x2C, 0x2C, 278},
	{0x2D, 0x2D, 333}, {0x2E, 0x2F, 278},
	{0x30, 0x39, 556}, // digits
	{0x3A, 0x3B, 333}, {0x3C, 0x3E, 584}, {0x3F, 0x3F, 611}, {0x40, 0x40, 975},
	{0x41, 0x44, 722}, {0x45, 0x45, 667}, {0x46, 0x46, 611}, {0x47, 0x47, 778},
	{0x48, 0x48, 722}, {0x49, 0x49, 278}, {0x4A, 0x4A, 556}, {0x4B, 0x4B, 722},
	{0x4C, 0x4C, 611}, {0x4D, 0x4D, 833}, {0x4E, 0x4E, 722}, {0x4F, 0x4F, 778},
	{0x50, 0x50, 667}, {0x51, 0x51, 778}, {0x52, 0x52, 722}, {0x53, 0x53, 667},
	{0x54, 0x54, 611}, {0x55, 0x55, 722}, {0x56, 0x56, 667}, {0x57, 0x57, 944},
	{0x58, 0x59, 667}, {0x5A, 0x5A, 611},
	{0x5B, 0x5B, 333}, {0x5C, 0x5C, 278}, {0x5D, 0x5D, 333}, {0x5E, 0x5E, 584},
	{0x5F, 0x5F, 556}, {0x60, 0x60, 333},
	{0x61, 0x61, 556}, {0x62, 0x62, 611}, {0x63, 0x63, 556}, {0x64, 0x64, 611},
	{0x65, 0x65, 556}, {0x66, 0x66, 333}, {0x67, 0x68, 611}, {0x69, 0x6A, 278},
	{0x6B, 0x6B, 556}, {0x6C, 0x6C, 278}, {0x6D, 0x6D, 889}, {0x6E, 0x6F, 611},
	{0x70, 0x71, 611}, {0x72, 0x72, 389}, {0x73, 0x73, 556}, {0x74, 0x74, 333},
	{0x75, 0x75, 611}, {0x76, 0x76, 556}, {0x77, 0x77, 778}, {0x78, 0x79, 556},
	{0x7A, 0x7A, 500}, {0x7B, 0x7B, 389}, {0x7C, 0x7C, 280}, {0x7D, 0x7D, 389},
	{0x7E, 0x7E, 584},

	{0x80, 0x80, 556}, {0x82, 0x82, 278}, {0x83, 0x83, 556}, {0x84, 0x84, 500},
	{0x85, 0x85, 1000}, {0x86, 0x87, 556}, {0x88, 0x88, 333}, {0x89, 0x89, 1000},
	{0x8A, 0x8A, 667}, {0x8B, 0x8B, 333}, {0x8C, 0x8C, 1000}, {0x8E, 0x8E, 611},
	{0x91, 0x92, 278}, {0x93, 0x94, 500}, {0x95, 0x95, 350}, {0x96, 0x96, 556},
	{0x97, 0x97, 1000}, {0x98, 0x98, 333}, {0x99, 0x99, 1000}, {0x9A, 0x9A, 556},
	{0x9B, 0x9B, 333}, {0x9C, 0x9C, 889}, {0x9E, 0x9E, 500}, {0x9F, 0x9F, 667},

	{0xA0, 0xA0, 278}, {0xA1, 0xA1, 333}, {0xA2, 0xA5, 556}, {0xA6, 0xA6, 280},
	{0xA7, 0xA7, 556}, {0xA8, 0xA8, 333}, {0xA9, 0xA9, 737}, {0xAA, 0xAA, 370},
	{0xAB, 0xAB, 556}, {0xAC, 0xAC, 584}, {0xAD, 0xAD, 333}, {0xAE, 0xAE, 737},
	{0xAF, 0xAF, 333}, {0xB0, 0xB0, 400}, {0xB1, 0xB1, 584}, {0xB2, 0xB4, 333},
	{0xB5, 0xB5, 611}, {0xB6, 0xB6, 556}, {0xB7, 0xB7, 278}, {0xB8, 0xB9, 333},
	{0xBA, 0xBA, 365}, {0xBB, 0xBB, 556}, {0xBC, 0xBE, 834}, {0xBF, 0xBF, 611},
	{0xC0, 0xC5, 722}, {0xC6, 0xC6, 1000}, {0xC7, 0xC7, 722}, {0xC8, 0xCB, 667},
	{0xCC, 0xCF, 278}, {0xD0, 0xD1, 722}, {0xD2, 0xD6, 778}, {0xD7, 0xD7, 584},
	{0xD8, 0xD8, 778}, {0xD9, 0xDC, 722}, {0xDD, 0xDE, 667}, {0xDF, 0xDF, 611},
	{0xE0, 0xE5, 556}, {0xE6, 0xE6, 889}, {0xE7, 0xE7, 556}, {0xE8, 0xEB, 556},
	{0xEC, 0xEF, 278}, {0xF0, 0xF6, 611}, {0xF7, 0xF7, 584}, {0xF8, 0xF8, 611},
	{0xF9, 0xFC, 611}, {0xFD, 0xFD, 556}, {0xFE, 0xFE, 611}, {0xFF, 0xFF, 556},
}

// Courier is monospaced: every glyph in the family advances 600 units, which is
// why it needs no width table at all.
const courierAdvance = 600

var courierWidths = []widthRun{
	{0x20, 0x7E, courierAdvance},
	{0x80, 0x80, courierAdvance}, {0x82, 0x8C, courierAdvance},
	{0x8E, 0x8E, courierAdvance}, {0x91, 0x9C, courierAdvance},
	{0x9E, 0x9F, courierAdvance}, {0xA0, 0xFF, courierAdvance},
}

// standard14 indexes the built-in faces by their PDF base font name.
var standard14 = map[string]*standardFont{}

func init() {
	// Helvetica: Ascender 718, Descender -207 (Helvetica.afm).
	for _, name := range []string{Helvetica, HelveticaOblique} {
		standard14[name] = build(name, 718, 207, 278, helveticaWidths)
	}
	for _, name := range []string{HelveticaBold, HelveticaBoldOblique} {
		standard14[name] = build(name, 718, 207, 278, helveticaBoldWidths)
	}
	// Courier: Ascender 629, Descender -157 (Courier.afm).
	for _, name := range []string{Courier, CourierBold, CourierOblique, CourierBoldOblique} {
		standard14[name] = build(name, 629, 157, courierAdvance, courierWidths)
	}
}

// Standard returns a built-in font by name, e.g. fonts.Helvetica.
func Standard(name string) (core.Font, error) {
	if f, ok := standard14[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf(
		"sanur/fonts: %q is not a built-in font (available: %s); "+
			"for other typefaces use RegisterTrueType",
		name, strings.Join(StandardNames(), ", "))
}

// MustStandard is Standard for the constants in this package, where a miss is a
// programming error rather than a runtime condition.
func MustStandard(name string) core.Font {
	f, err := Standard(name)
	if err != nil {
		panic(err)
	}
	return f
}

// StandardNames lists the built-in font names in a stable order.
func StandardNames() []string {
	return []string{
		Helvetica, HelveticaBold, HelveticaOblique, HelveticaBoldOblique,
		Courier, CourierBold, CourierOblique, CourierBoldOblique,
	}
}

// StandardFamily picks the right built-in face for a weight and slant, so
// callers can ask for "bold Helvetica" without knowing PDF's naming scheme.
func StandardFamily(family string, weight core.FontWeight, italic bool) (core.Font, error) {
	bold := weight >= core.FontSemiBold

	var name string
	switch strings.ToLower(family) {
	case "helvetica", "arial", "sans", "sans-serif", "":
		switch {
		case bold && italic:
			name = HelveticaBoldOblique
		case bold:
			name = HelveticaBold
		case italic:
			name = HelveticaOblique
		default:
			name = Helvetica
		}
	case "courier", "mono", "monospace":
		switch {
		case bold && italic:
			name = CourierBoldOblique
		case bold:
			name = CourierBold
		case italic:
			name = CourierOblique
		default:
			name = Courier
		}
	default:
		return nil, fmt.Errorf(
			"sanur/fonts: no built-in family %q; use RegisterTrueType to supply it", family)
	}
	return Standard(name)
}
