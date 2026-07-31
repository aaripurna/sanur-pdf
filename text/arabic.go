// Package text prepares a logical-order string for drawing.
//
// Two transformations live here, and they are not interchangeable. Shape substitutes
// the contextual forms Arabic orthography requires, which changes the glyphs and
// therefore the widths, so it has to run before anything is measured. Reorder puts
// runes into the order they are drawn in, which changes nothing about the glyphs, so
// it has to run per line after line breaking — bidirectional order is defined per
// line, and a paragraph reordered as a whole would scramble its wrapped lines.
//
// Neither one is a full text-shaping engine. What is missing is set out in Shape and
// in Reorder, and the honest summary is that left-to-right alphabetic scripts and
// ordinary Arabic and Hebrew running text are handled, while Indic and the other
// scripts that need glyph reordering and conjunct formation are not.
package text

import "unicode"

// Arabic characters that carry no forms of their own but decide how their
// neighbours join.
const (
	// tatweel is the elongation stroke. It has no shape to change, and it lets the
	// letters on either side connect through it, which is how Arabic text is
	// stretched to fill a line by hand.
	tatweel = 0x0640

	// zeroWidthJoiner forces a connection where there would not otherwise be one,
	// used to show a letter in a particular form in isolation.
	zeroWidthJoiner = 0x200D

	// zeroWidthNonJoiner forbids one, which is what keeps a Persian prefix visually
	// separate from the word it attaches to.
	zeroWidthNonJoiner = 0x200C

	// lam is the one letter with a mandatory ligature.
	lam = 0x0644
)

// shapingClass returns how r joins to its neighbours.
func shapingClass(r rune) joining {
	if entry, ok := arabicShaping[r]; ok {
		return entry.class
	}

	switch r {
	case tatweel, zeroWidthJoiner:
		return joinCausing
	case zeroWidthNonJoiner:
		// Explicitly non-joining rather than transparent: its whole purpose is to
		// break a connection, and a transparent character would be looked straight
		// through.
		return joinNone
	}

	// Marks and format characters are transparent: a vowel sign sits above a letter
	// without interrupting the stroke that runs beneath it, so the letters on either
	// side of it are still neighbours.
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return joinTransparent
	}

	return joinNone
}

// joinsForward reports whether a character can connect to the one after it.
func joinsForward(class joining) bool {
	return class == joinDual || class == joinCausing
}

// joinsBackward reports whether a character can connect to the one before it.
//
// Right-joining letters count here and not in joinsForward, which is the whole
// asymmetry of the script: alef connects to what precedes it and leaves the next
// letter to start a new cluster.
func joinsBackward(class joining) bool {
	return class == joinDual || class == joinRight || class == joinCausing
}

// Shape substitutes the contextual forms of Arabic letters.
//
// Arabic letters change shape according to their neighbours — four forms for most
// letters, two for those that only join backwards — and text set without the
// substitution reads as a row of disconnected letters. Unicode assigns every form its
// own codepoint in the presentation-form blocks, so the substitution is a table
// lookup and needs none of the font's own glyph substitution machinery.
//
// available reports whether the font can draw a given rune. A form the font lacks
// falls back to the base letter, which is wrong but legible; substituting a glyph
// that is not there would draw a question mark or nothing at all. Passing nil assumes
// every form is available.
//
// What this does not do:
//
//   - Mark positioning. A vowel sign is placed by the font's own metrics, so it sits
//     at a default offset rather than centred over the letter it belongs to. Getting
//     that right needs the GPOS table.
//   - Ligatures beyond lam-alef. Those are optional in Arabic; lam-alef is not, and
//     is handled.
//   - Kashida justification, which stretches words by elongating strokes rather than
//     by widening spaces.
//   - Any other script. Devanagari and its relatives need glyph reordering and
//     conjunct formation, which cannot be expressed as a codepoint substitution.
//
// Text with no Arabic in it is returned unchanged, and the scan that establishes that
// is the only cost for the overwhelming majority of documents.
func Shape(s string, available func(rune) bool) string {
	if !hasArabic(s) {
		return s
	}
	if available == nil {
		available = func(rune) bool { return true }
	}

	runes := []rune(s)
	out := make([]rune, 0, len(runes))

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		entry, shapeable := arabicShaping[r]
		if !shapeable {
			out = append(out, r)
			continue
		}

		prev := joinsForward(shapingClass(previousVisible(runes, i)))
		next := joinsBackward(shapingClass(nextVisible(runes, i)))

		// Lam followed by alef is written as one glyph, so the pair is consumed
		// together and the alef never reaches the output.
		if r == lam {
			if after := nextVisibleIndex(runes, i); after >= 0 {
				if ligature, ok := lamAlef[runes[after]]; ok {
					form := ligature.isolated
					if prev {
						form = ligature.final
					}
					if available(form) {
						out = append(out, form)
						// Anything transparent between the two — a vowel mark on the
						// lam — is carried across so it is not silently dropped.
						out = append(out, runes[i+1:after]...)
						i = after
						continue
					}
				}
			}
		}

		out = append(out, selectForm(entry, prev, next, available, r))
	}

	return string(out)
}

// selectForm picks the form a letter takes in its context.
//
// The cascade matters more than it looks. A right-joining letter has no medial form,
// so when it sits between two joinable letters the medial slot is empty — and the
// answer is its final form, not its isolated one, because it still connects to what
// precedes it. Falling straight to isolated instead is subtly wrong in a way that
// only shows in mid-word: reh in مرحبا would break the stroke that should reach it.
func selectForm(entry arabicForms, prev, next bool, available func(rune) bool, base rune) rune {
	var form rune

	switch {
	case prev && next && entry.medial != 0:
		form = entry.medial
	case prev && entry.final != 0:
		form = entry.final
	case next && entry.initial != 0:
		form = entry.initial
	default:
		form = entry.isolated
	}

	if form == 0 || !available(form) {
		// A font that covers Arabic but ships no presentation forms — some modern
		// ones put the forms in GSUB instead — still draws the base letters, which is
		// unjoined but readable. Reaching for a glyph that is not there would not be.
		return base
	}
	return form
}

// previousVisible returns the character before i that is not transparent, or 0.
func previousVisible(runes []rune, i int) rune {
	for j := i - 1; j >= 0; j-- {
		if shapingClass(runes[j]) != joinTransparent {
			return runes[j]
		}
	}
	return 0
}

// nextVisible returns the character after i that is not transparent, or 0.
func nextVisible(runes []rune, i int) rune {
	if j := nextVisibleIndex(runes, i); j >= 0 {
		return runes[j]
	}
	return 0
}

func nextVisibleIndex(runes []rune, i int) int {
	for j := i + 1; j < len(runes); j++ {
		if shapingClass(runes[j]) != joinTransparent {
			return j
		}
	}
	return -1
}

// hasArabic reports whether s contains anything the shaper would change.
func hasArabic(s string) bool {
	for _, r := range s {
		// The Arabic blocks proper, then the supplements and extended ranges that
		// carry the Persian, Urdu and African letters.
		if r >= 0x0600 && r <= 0x08FF {
			return true
		}
		if r >= 0xFB50 && r <= 0xFDFF {
			return true
		}
		if r >= 0xFE70 && r <= 0xFEFF {
			return true
		}
	}
	return false
}

// decomposed maps every presentation form back to the characters it stands for.
//
// Built once from the shaping tables rather than written out, so the two directions
// cannot disagree.
var decomposed = buildDecomposition()

func buildDecomposition() map[rune][]rune {
	out := make(map[rune][]rune, len(arabicShaping)*4+len(lamAlef)*2)

	for base, forms := range arabicShaping {
		for _, form := range []rune{forms.isolated, forms.final, forms.initial, forms.medial} {
			if form != 0 {
				out[form] = []rune{base}
			}
		}
	}

	// A ligature stands for two characters, which is why the values are slices — and
	// they are listed alef first, which is the reverse of how they are written.
	//
	// Everything downstream of shaping is in visual order: the glyphs go into the
	// content stream left to right, so the characters a glyph stands for have to be
	// listed the same way. A reader expanding this map walks the glyphs as they were
	// drawn and then reverses the right-to-left runs to recover the text, which
	// reverses the pair along with everything around it. Listing them in writing order
	// instead puts them back transposed: "السلام" extracts as "السالم".
	for alef, forms := range lamAlef {
		for _, form := range []rune{forms.isolated, forms.final} {
			if form != 0 {
				out[form] = []rune{alef, lam}
			}
		}
	}

	return out
}

// BaseRunes returns the characters a shaped Arabic glyph stands for, or r alone if it
// is not a presentation form.
//
// This is what keeps shaped text searchable. Shaping replaces ب with one of four
// presentation-form codepoints, and a PDF that reports those as the text it contains
// cannot be searched for the word as anyone would type it — nor copied out of, since
// what comes back is a form nothing else uses. Mapping each glyph back to its base
// letter is the difference between a document that is text and one that merely looks
// like it.
//
// The lam-alef ligature decomposes to two characters, which is the reason for the
// slice: one glyph on the page stands for two in the text.
func BaseRunes(r rune) []rune {
	if base, ok := decomposed[r]; ok {
		return base
	}
	return []rune{r}
}
