package sanur

import (
	"codeberg.org/aaripurna/sanur/core"
	"codeberg.org/aaripurna/sanur/fonts"
)

// Family groups the four faces of a typeface so that Bold and Italic can be
// asked for by intent rather than by font name.
//
// PDF has no notion of a family: each weight and slant is a separate font
// program with its own name and metrics. Grouping them here means a document can
// say .Bold() once and get the real bold face, instead of a reader synthesising
// one by smearing the regular outlines.
type Family struct {
	Regular    core.Font
	Bold       core.Font
	Italic     core.Font
	BoldItalic core.Font
}

// NewFamily builds a family from registered fonts. Any face left nil falls back
// to the nearest available one.
func NewFamily(regular, bold, italic, boldItalic core.Font) Family {
	return Family{Regular: regular, Bold: bold, Italic: italic, BoldItalic: boldItalic}
}

// Pick selects the face for a weight and slant, degrading gracefully when the
// family is incomplete: a document asking for bold italic from a family that only
// has bold gets bold rather than an error or a missing glyph.
func (f Family) Pick(bold, italic bool) core.Font {
	switch {
	case bold && italic:
		return firstFont(f.BoldItalic, f.Bold, f.Italic, f.Regular)
	case bold:
		return firstFont(f.Bold, f.Regular)
	case italic:
		return firstFont(f.Italic, f.Regular)
	}
	return firstFont(f.Regular, f.Bold, f.Italic, f.BoldItalic)
}

func firstFont(candidates ...core.Font) core.Font {
	for _, c := range candidates {
		if c != nil {
			return c
		}
	}
	return nil
}

// HelveticaFamily and CourierFamily are the built-in typefaces, available with
// no font files present.
var (
	HelveticaFamily = Family{
		Regular:    fonts.MustStandard(fonts.Helvetica),
		Bold:       fonts.MustStandard(fonts.HelveticaBold),
		Italic:     fonts.MustStandard(fonts.HelveticaOblique),
		BoldItalic: fonts.MustStandard(fonts.HelveticaBoldOblique),
	}

	CourierFamily = Family{
		Regular:    fonts.MustStandard(fonts.Courier),
		Bold:       fonts.MustStandard(fonts.CourierBold),
		Italic:     fonts.MustStandard(fonts.CourierOblique),
		BoldItalic: fonts.MustStandard(fonts.CourierBoldOblique),
	}
)

// DefaultFontSize is used by any style that does not set one.
const DefaultFontSize = 11

// StyleBuilder composes a text style.
//
// Every method returns the builder so styles read as one expression, and none of
// them can fail: a family is always resolvable to some face, so there is no error
// to thread through a chain of setters.
type StyleBuilder struct {
	family Family
	bold   bool
	italic bool
	style  core.TextStyle
}

// TextStyle starts a style: Helvetica, 11pt, black.
func TextStyle() *StyleBuilder {
	return &StyleBuilder{
		family: HelveticaFamily,
		style: core.TextStyle{
			Size:   DefaultFontSize,
			Color:  Black,
			Weight: core.FontNormal,
		},
	}
}

// StyleFrom starts a style from an existing one, for local overrides.
func StyleFrom(base core.TextStyle) *StyleBuilder {
	b := &StyleBuilder{family: HelveticaFamily, style: base}
	b.bold = base.Weight >= core.FontSemiBold
	b.italic = base.Italic
	return b
}

// Family sets the typeface.
func (b *StyleBuilder) Family(f Family) *StyleBuilder {
	b.family = f
	return b
}

// Font sets one specific face, bypassing family resolution. Later calls to Bold
// or Italic would re-resolve from the family, so this is best used on its own.
func (b *StyleBuilder) Font(f core.Font) *StyleBuilder {
	b.family = Family{Regular: f}
	b.bold = false
	b.italic = false
	return b
}

// Mono switches to the built-in monospaced family.
func (b *StyleBuilder) Mono() *StyleBuilder { return b.Family(CourierFamily) }

// Size sets the font size in points.
func (b *StyleBuilder) Size(pt float64) *StyleBuilder {
	b.style.Size = pt
	return b
}

// Color sets the text colour.
func (b *StyleBuilder) Color(c core.Color) *StyleBuilder {
	b.style.Color = c
	return b
}

// Bold selects the family's bold face.
func (b *StyleBuilder) Bold() *StyleBuilder {
	b.bold = true
	b.style.Weight = core.FontBold
	return b
}

// Italic selects the family's italic face.
func (b *StyleBuilder) Italic() *StyleBuilder {
	b.italic = true
	b.style.Italic = true
	return b
}

// Weight sets a numeric weight. Anything semibold or heavier picks the bold face,
// since the built-in families have only two weights.
func (b *StyleBuilder) Weight(w core.FontWeight) *StyleBuilder {
	b.style.Weight = w
	b.bold = w >= core.FontSemiBold
	return b
}

// LineHeight multiplies the font's natural line spacing.
func (b *StyleBuilder) LineHeight(factor float64) *StyleBuilder {
	b.style.LineHeightFactor = factor
	return b
}

// LetterSpacing adds tracking, in points.
func (b *StyleBuilder) LetterSpacing(pt float64) *StyleBuilder {
	b.style.LetterSpacing = pt
	return b
}

// Underline draws a rule under the text.
func (b *StyleBuilder) Underline() *StyleBuilder {
	b.style.Underline = true
	return b
}

// Strikeout draws a rule through the text.
func (b *StyleBuilder) Strikeout() *StyleBuilder {
	b.style.Strikeout = true
	return b
}

// Build resolves the style.
func (b *StyleBuilder) Build() core.TextStyle {
	s := b.style
	s.Font = b.family.Pick(b.bold, b.italic)
	s.Italic = b.italic
	if s.Size <= 0 {
		s.Size = DefaultFontSize
	}
	return s
}
