package fonts

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/aaripurna/sanur-pdf/core"
)

// trueTypeFont is a parsed .ttf/.otf face.
//
// Advance widths are read once in font units and cached per rune. sfnt resolves
// an advance through the cmap and hmtx tables on every call and needs a scratch
// buffer that cannot be shared across goroutines; measuring a paragraph touches
// the same few dozen runes thousands of times, so caching turns the hot path
// into a map lookup and confines the locking to first sight of each rune.
type trueTypeFont struct {
	name string
	data []byte
	sf   *sfnt.Font

	upem       float64
	ascent     float64 // positive, font units
	descent    float64 // positive, font units
	capHeight  float64
	bbox       [4]float64
	fixedPitch bool
	italic     bool
	bold       bool

	mu       sync.Mutex
	buf      sfnt.Buffer
	advances map[rune]float64 // font units
	hasGlyph map[rune]bool

	// missing is the advance used for runes the font has no glyph for.
	missing float64
}

// RegisterTrueType parses a TrueType or OpenType font from memory.
//
// name is the identity used to deduplicate the font in the output file and
// becomes the PDF base font name; it does not have to match the font's internal
// name, but it must be unique per distinct font program.
func RegisterTrueType(name string, data []byte) (core.Font, error) {
	if name == "" {
		return nil, fmt.Errorf("sanur/fonts: font name must not be empty")
	}
	if _, taken := standard14[name]; taken {
		return nil, fmt.Errorf(
			"sanur/fonts: %q collides with a built-in font name; choose another", name)
	}

	// sfnt keeps a reference to data rather than copying it, and the same bytes
	// are later embedded verbatim in the PDF. Copying up front means a caller
	// reusing its buffer cannot corrupt an already-registered font.
	owned := make([]byte, len(data))
	copy(owned, data)

	sf, err := sfnt.Parse(owned)
	if err != nil {
		return nil, fmt.Errorf("sanur/fonts: parsing %q: %w", name, err)
	}

	f := &trueTypeFont{
		name:     name,
		data:     owned,
		sf:       sf,
		upem:     float64(sf.UnitsPerEm()),
		advances: make(map[rune]float64),
		hasGlyph: make(map[rune]bool),
	}
	if f.upem == 0 {
		return nil, fmt.Errorf("sanur/fonts: %q reports zero units per em", name)
	}

	// advanceUnits falls back to f.missing for absent glyphs and caches the
	// result, so missing needs a sane value before the first lookup below. Half
	// an em is the seed; it is refined to the font's own space width once that
	// can be read.
	f.missing = f.upem / 2

	// Asking sfnt for metrics at a ppem equal to the em size makes it return
	// values already in font units, which is the space the PDF descriptor wants.
	ppem := fixed.Int26_6(sf.UnitsPerEm() << 6)

	metrics, err := sf.Metrics(&f.buf, ppem, font.HintingNone)
	if err != nil {
		return nil, fmt.Errorf("sanur/fonts: reading metrics of %q: %w", name, err)
	}
	f.ascent = fixedToFloat(metrics.Ascent)
	f.descent = fixedToFloat(metrics.Descent)
	f.capHeight = fixedToFloat(metrics.CapHeight)
	if f.capHeight <= 0 {
		f.capHeight = f.ascent
	}

	bounds, err := sf.Bounds(&f.buf, ppem, font.HintingNone)
	if err != nil {
		return nil, fmt.Errorf("sanur/fonts: reading bounds of %q: %w", name, err)
	}
	f.bbox = [4]float64{
		fixedToFloat(bounds.Min.X),
		// sfnt reports bounds in a y-down space while PDF's FontBBox is y-up,
		// so the vertical extremes swap and negate.
		-fixedToFloat(bounds.Max.Y),
		fixedToFloat(bounds.Max.X),
		-fixedToFloat(bounds.Min.Y),
	}

	// A font that advances 'i' and 'M' identically is monospaced. Reading the
	// two advances is cheaper and more reliable than looking for a post table
	// flag that many fonts set carelessly.
	iAdv, iOK := f.advanceUnits('i')
	mAdv, mOK := f.advanceUnits('M')
	f.fixedPitch = iOK && mOK && iAdv == mAdv

	lower := strings.ToLower(name)
	subfamily := strings.ToLower(nameOrEmpty(sf, &f.buf, sfnt.NameIDSubfamily))
	f.italic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique") ||
		strings.Contains(subfamily, "italic") || strings.Contains(subfamily, "oblique")
	f.bold = strings.Contains(lower, "bold") || strings.Contains(subfamily, "bold")

	if adv, ok := f.advanceUnits(' '); ok {
		f.missing = adv
	}

	return f, nil
}

// LoadTrueTypeFile reads and registers a font from disk, naming it after the
// file if name is empty.
func LoadTrueTypeFile(name, path string) (core.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sanur/fonts: reading %s: %w", path, err)
	}
	if name == "" {
		base := path
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		name = strings.TrimSuffix(strings.TrimSuffix(base, ".ttf"), ".otf")
	}
	return RegisterTrueType(name, data)
}

func nameOrEmpty(sf *sfnt.Font, buf *sfnt.Buffer, id sfnt.NameID) string {
	s, err := sf.Name(buf, id)
	if err != nil {
		return ""
	}
	return s
}

func fixedToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64
}

// advanceUnits returns the advance width of r in font units, and whether the
// font actually has a glyph for it.
func (f *trueTypeFont) advanceUnits(r rune) (float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if adv, ok := f.advances[r]; ok {
		return adv, f.hasGlyph[r]
	}

	ppem := fixed.Int26_6(f.sf.UnitsPerEm() << 6)

	gid, err := f.sf.GlyphIndex(&f.buf, r)
	if err != nil || gid == 0 {
		// Glyph index 0 is .notdef: the font has no glyph for this rune.
		f.advances[r] = f.missing
		f.hasGlyph[r] = false
		return f.missing, false
	}

	adv, err := f.sf.GlyphAdvance(&f.buf, gid, ppem, font.HintingNone)
	if err != nil {
		f.advances[r] = f.missing
		f.hasGlyph[r] = false
		return f.missing, false
	}

	units := fixedToFloat(adv)
	f.advances[r] = units
	f.hasGlyph[r] = true
	return units, true
}

func (f *trueTypeFont) Name() string { return f.name }

func (f *trueTypeFont) AdvanceOf(r rune, size float64) float64 {
	units, _ := f.advanceUnits(r)
	return units / f.upem * size
}

func (f *trueTypeFont) Measure(text string, size float64) float64 {
	var units float64
	for _, r := range text {
		adv, _ := f.advanceUnits(r)
		units += adv
	}
	return units / f.upem * size
}

func (f *trueTypeFont) Ascent(size float64) float64 {
	return f.ascent / f.upem * size
}

func (f *trueTypeFont) Descent(size float64) float64 {
	return f.descent / f.upem * size
}

func (f *trueTypeFont) LineHeight(size float64) float64 {
	return (f.ascent + f.descent) / f.upem * size * lineHeightFactor
}

// Program implements Programmable, converting the font's own units into the
// 1/1000 em space PDF descriptors are defined in.
func (f *trueTypeFont) Program() FontProgram {
	scale := func(v float64) int { return int(v / f.upem * 1000) }

	p := FontProgram{
		BaseName:  f.name,
		Data:      f.data,
		FirstChar: 0x20,
		LastChar:  0xFF,
		Flags:     FlagNonsymbolic,
		BBox: [4]int{
			scale(f.bbox[0]), scale(f.bbox[1]),
			scale(f.bbox[2]), scale(f.bbox[3]),
		},
		Ascent:    scale(f.ascent),
		Descent:   -scale(f.descent),
		CapHeight: scale(f.capHeight),
		StemV:     80,
	}

	if f.fixedPitch {
		p.Flags |= FlagFixedPitch
	}
	if f.italic {
		p.Flags |= FlagItalic
		// ItalicAngle is negative for a right-leaning slant. sfnt does not
		// expose the post table's value, and 12 degrees matches the standard
		// oblique used across the common families closely enough for the only
		// thing a reader does with it: synthesising a substitute if the
		// embedded program ever fails to load.
		p.ItalicAngle = -12
	}
	if f.bold {
		p.StemV = 160
	}

	for c := p.FirstChar; c <= p.LastChar; c++ {
		r := RuneForWinAnsiCode(byte(c))
		if r == undefinedRune {
			continue
		}
		units, _ := f.advanceUnits(r)
		p.Widths[c] = scale(units)
	}
	return p
}
