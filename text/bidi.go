package text

import "golang.org/x/text/unicode/bidi"

// Direction is the reading direction of a run of text.
type Direction int

const (
	// DirectionNeutral means nothing in the text settles the question: digits and
	// punctuation on their own take the direction of whatever surrounds them.
	DirectionNeutral Direction = iota

	// DirectionLeftToRight is Latin, Greek, Cyrillic, CJK.
	DirectionLeftToRight

	// DirectionRightToLeft is Hebrew, Arabic, Syriac, Thaana.
	DirectionRightToLeft
)

func (d Direction) String() string {
	switch d {
	case DirectionLeftToRight:
		return "left-to-right"
	case DirectionRightToLeft:
		return "right-to-left"
	}
	return "neutral"
}

// DirectionOf reports the paragraph direction of s.
//
// The rule is Unicode's P2 and P3: the first strong directional character decides, and
// text with none is neutral. It is what a caller needs in order to align a paragraph,
// since a right-to-left paragraph set flush left reads correctly and looks wrong.
//
// Note that golang.org/x/text's Paragraph.Direction is not this. It reports the
// direction of the first run, so "12 דג" comes back left-to-right from it — the digits
// are a run of their own and are not strong. The paragraph is right-to-left.
func DirectionOf(s string) Direction {
	if s == "" {
		return DirectionNeutral
	}

	for _, r := range s {
		switch strongDirectionOf(r) {
		case DirectionLeftToRight:
			return DirectionLeftToRight
		case DirectionRightToLeft:
			return DirectionRightToLeft
		}
	}
	return DirectionNeutral
}

// strongDirectionOf returns the direction a single character settles, or neutral for
// the characters that settle nothing: digits, spaces and punctuation.
func strongDirectionOf(r rune) Direction {
	p, _ := bidi.LookupRune(r)

	switch p.Class() {
	case bidi.L:
		return DirectionLeftToRight
	case bidi.R, bidi.AL:
		return DirectionRightToLeft
	}
	return DirectionNeutral
}

// Glyph is one rune as it is drawn: which rune to draw, and where in the input it
// came from.
//
// The position matters as much as the rune. A line of text is usually several styled
// runs, and once the line has been rearranged each run has to be able to find its own
// characters again — so reordering cannot simply hand back a string.
type Glyph struct {
	// Rune is the character to draw, mirrored where the context calls for it: an
	// opening bracket inside right-to-left text is drawn as a closing one, because
	// what it enclosed is now on the other side of it.
	Rune rune

	// From is the index of this character in []rune of the input.
	From int
}

// Reorder returns s with its runes in the order they are drawn, taking the paragraph
// direction from s itself.
func Reorder(s string) string { return ReorderIn(s, DirectionNeutral) }

// ReorderIn is Reorder with the paragraph direction supplied.
func ReorderIn(s string, base Direction) string {
	glyphs := VisualRunes(s, base)

	out := make([]rune, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.Rune
	}
	return string(out)
}

// VisualRunes returns s's characters in the order and the form they are drawn.
//
// base is the direction of the paragraph s belongs to, or DirectionNeutral to take it
// from s. Supplying it matters for wrapped text: direction is a property of the
// paragraph, and a line that happens to begin with a Latin word inside an Arabic
// paragraph would otherwise be laid out as though it were English.
//
// # What this implements
//
// The Unicode Bidirectional Algorithm, UAX #9: the paragraph level (P2, P3), the weak
// and neutral resolution rules (W1 to W7, N1, N2), the implicit levels (I1, I2) and
// the display rules (L1 to L4). Output is checked character for character against
// fribidi across a corpus and several hundred randomly generated strings.
//
// # Where it stops
//
// The explicit embedding, override and isolate controls — U+202A to U+202E and U+2066
// to U+2069 — are treated as removed rather than acted on, so text that uses them to
// force a nesting it would not otherwise have will not get it. Rule N0, which gives a
// matched pair of brackets the direction of what they enclose, is also absent;
// brackets resolve as ordinary neutrals, which agrees with the reference on every
// bracket case tested. Mirroring covers the paired brackets rather than the whole
// Bidi_Mirrored property, so an angle bracket or a mathematical relation inside
// right-to-left text is drawn unmirrored.
//
// Text with nothing right-to-left in it is returned in order, and the scan that
// establishes that is the only cost for a document in a left-to-right script.
func VisualRunes(s string, base Direction) []Glyph {
	runes := []rune(s)

	identity := func() []Glyph {
		glyphs := make([]Glyph, len(runes))
		for i, r := range runes {
			glyphs[i] = Glyph{Rune: r, From: i}
		}
		return glyphs
	}

	// Nothing right-to-left means every level resolves to the paragraph's own, so
	// there is nothing to reverse and nothing to mirror.
	if !hasRightToLeft(s) && base != DirectionRightToLeft {
		return identity()
	}

	levels, _ := resolveLevels(runes, base)

	order := reorderLevels(levels)
	orderMarks(order, runes)

	// A permutation that lost or gained a character would silently drop text, so the
	// input order is the safer answer if anything above went wrong.
	if len(order) != len(runes) {
		return identity()
	}

	glyphs := make([]Glyph, len(order))
	for i, from := range order {
		r := runes[from]
		if levels[from].isRTL() {
			r = mirrored(r)
		}
		glyphs[i] = Glyph{Rune: r, From: from}
	}
	return glyphs
}

// orderMarks applies rule L3: a combining mark has to follow the character it
// attaches to, and reversing a right-to-left run puts it in front.
//
// Without this, a vowel-marked Arabic or Hebrew word comes out with every mark
// attached to the wrong letter — the letters are in the right order, so the damage is
// easy to miss and impossible to read.
func orderMarks(order []int, runes []rune) {
	isMark := func(at int) bool {
		p, _ := bidi.LookupRune(runes[at])
		return p.Class() == bidi.NSM
	}

	for i := 0; i < len(order); {
		if !isMark(order[i]) {
			i++
			continue
		}

		// A reversed cluster appears as its marks in descending order followed by
		// their base, which is the character logically before the last of them.
		end := i
		for end+1 < len(order) && isMark(order[end+1]) && order[end+1] == order[end]-1 {
			end++
		}

		if end+1 < len(order) && !isMark(order[end+1]) && order[end+1] == order[end]-1 {
			reverse(order[i : end+2])
			i = end + 2
			continue
		}
		i = end + 1
	}
}

// mirrored returns the glyph a character is drawn as inside right-to-left text.
//
// Only the paired brackets are handled, using the table golang.org/x/text maintains.
// The full Bidi_Mirrored property covers rather more — angle brackets, mathematical
// relations — and is not exposed anywhere reachable, so those are drawn as they were
// written. In running prose it is the parentheses that matter.
func mirrored(r rune) rune {
	p, _ := bidi.LookupRune(r)
	if !p.IsBracket() {
		return r
	}

	// ReverseString reverses a string and swaps its brackets for their counterparts.
	// Given one character there is no order to reverse, so what comes back is the
	// mirror on its own.
	swapped := []rune(bidi.ReverseString(string(r)))
	if len(swapped) != 1 {
		return r
	}
	return swapped[0]
}

func reverse(v []int) {
	for a, b := 0, len(v)-1; a < b; a, b = a+1, b-1 {
		v[a], v[b] = v[b], v[a]
	}
}

// hasRightToLeft reports whether s contains any character that could make the text
// bidirectional.
//
// This is the fast path, and it matters: every line of every document would otherwise
// run the bidi algorithm to be told it is Latin. The ranges are deliberately generous
// — a false positive costs one wasted analysis, a false negative would leave Hebrew in
// logical order.
func hasRightToLeft(s string) bool {
	for _, r := range s {
		switch {
		case r < 0x0590:
			// Latin, Greek, Cyrillic and everything below Hebrew.
			continue
		case r <= 0x08FF:
			// Hebrew, Arabic, Syriac, Thaana, NKo, Samaritan, Mandaic and the
			// Arabic supplements.
			return true
		case r >= 0xFB1D && r <= 0xFDFF:
			// Hebrew and Arabic presentation forms.
			return true
		case r >= 0xFE70 && r <= 0xFEFF:
			// Arabic presentation forms-B.
			return true
		case r >= 0x10800 && r <= 0x10FFF:
			// The historic right-to-left scripts: Phoenician, Kharoshthi, Avestan.
			return true
		case r >= 0x1E800 && r <= 0x1EFFF:
			// Mende Kikakui, Adlam, and the Arabic mathematical alphabets.
			return true
		}
	}
	return false
}
