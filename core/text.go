package core

// FontWeight follows the CSS numeric weight scale.
type FontWeight int

const (
	FontThin       FontWeight = 100
	FontExtraLight FontWeight = 200
	FontLight      FontWeight = 300
	FontNormal     FontWeight = 400
	FontMedium     FontWeight = 500
	FontSemiBold   FontWeight = 600
	FontBold       FontWeight = 700
	FontExtraBold  FontWeight = 800
	FontBlack      FontWeight = 900
)

// Font supplies the glyph metrics the layout engine needs to measure text, and
// identifies itself to the PDF writer so the matching font resource is emitted.
//
// All metric methods take the font size in points and return points, so callers
// never deal with the font's internal units-per-em.
type Font interface {
	// Name is the stable key used to deduplicate font resources in the output
	// file. Two Fonts sharing a name must be interchangeable.
	Name() string

	// AdvanceOf returns the horizontal advance of a single rune. Runes the font
	// cannot represent report the width of its fallback glyph rather than zero,
	// so unsupported text still occupies believable space.
	AdvanceOf(r rune, size float64) float64

	// Measure returns the total advance width of a string.
	Measure(text string, size float64) float64

	// Ascent is the distance from the baseline to the top of the font's tallest
	// glyphs, as a positive number.
	Ascent(size float64) float64

	// Descent is the distance from the baseline down to the lowest descender,
	// as a positive number.
	Descent(size float64) float64

	// LineHeight is the default baseline-to-baseline distance.
	LineHeight(size float64) float64
}

// TextStyle is the full set of properties needed to measure and draw a run of
// text. It is a value type: elements copy and tweak it rather than mutating a
// shared instance, so a style override inside one container cannot leak out.
type TextStyle struct {
	Font   Font
	Size   float64
	Color  Color
	Weight FontWeight
	Italic bool

	// LineHeightFactor multiplies the font's natural line height. Zero means
	// "use the font's own value".
	LineHeightFactor float64

	// LetterSpacing is extra advance added after every glyph, in points.
	LetterSpacing float64

	// WordSpacing is extra advance added after every space character, in points.
	// Justified text sets it per line to stretch the gaps out to both margins;
	// PDF has a dedicated operator for exactly this, so justification costs one
	// number per line rather than a repositioned run per word.
	WordSpacing float64

	Underline bool
	Strikeout bool
}

// LineSpacing returns the baseline-to-baseline distance for this style.
func (s TextStyle) LineSpacing() float64 {
	base := s.Font.LineHeight(s.Size)
	if s.LineHeightFactor > 0 {
		return base * s.LineHeightFactor
	}
	return base
}

// MeasureText returns the advance width of text under this style, including
// letter and word spacing.
func (s TextStyle) MeasureText(text string) float64 {
	w := s.Font.Measure(text, s.Size)
	if s.LetterSpacing == 0 && s.WordSpacing == 0 {
		return w
	}
	for _, r := range text {
		if s.LetterSpacing != 0 {
			w += s.LetterSpacing
		}
		if s.WordSpacing != 0 && r == ' ' {
			w += s.WordSpacing
		}
	}
	return w
}
