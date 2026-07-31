package text

import "golang.org/x/text/unicode/bidi"

// This file resolves Unicode embedding levels: the Unicode Bidirectional Algorithm,
// UAX #9, sections 3.3.3 to 3.4.
//
// golang.org/x/text/unicode/bidi runs the algorithm internally but exposes only the
// resulting runs and their directions, in logical order, with no levels. Levels are
// what rule L2 reverses, so without them the best available is a two-level
// approximation: reverse the run sequence for a right-to-left paragraph, reverse
// characters within right-to-left runs. That is exact when the paragraph direction
// matches its content and wrong in a way worth caring about when it does not — a
// number inside an Arabic phrase quoted in an English sentence lands in the wrong
// place, and "עמוד 12" comes out as " דומע12" instead of "12 דומע".
//
// What the package does export is the character classes, and those are the hard part
// of the data. The resolution rules on top of them are mechanical, so they are
// implemented here and checked against fribidi.
//
// Two rules are left out.
//
// The explicit embedding, override and isolate controls (U+202A to U+202E and U+2066
// to U+2069) are rules X1 to X10, and are treated here as removed characters. Nothing
// sanur generates emits them, and honouring them means tracking a stack of directional
// status that nothing else in this file needs.
//
// Mirroring under rule L4 covers the paired brackets rather than the whole
// Bidi_Mirrored property, because that property is not exposed anywhere reachable. An
// angle bracket or a mathematical relation inside right-to-left text is therefore drawn
// as it was written.

// level is an embedding level: even is left-to-right, odd is right-to-left.
type level int

// isRTL reports whether a level runs right to left.
func (l level) isRTL() bool { return l&1 == 1 }

// direction returns the class a level corresponds to.
func (l level) direction() bidi.Class {
	if l.isRTL() {
		return bidi.R
	}
	return bidi.L
}

// maxDepth is the algorithm's own limit on nesting, used here only to bound the
// reversal loop.
const maxDepth = 125

// resolveLevels returns the embedding level of every rune, and the paragraph level.
func resolveLevels(runes []rune, base Direction) ([]level, level) {
	classes := make([]bidi.Class, len(runes))
	original := make([]bidi.Class, len(runes))

	for i, r := range runes {
		p, _ := bidi.LookupRune(r)
		classes[i] = p.Class()

		// The explicit controls are dropped rather than acted on. Keeping them as
		// neutrals would let them influence the surrounding text, which is worse than
		// ignoring them: a stray control would move words that were fine.
		switch classes[i] {
		case bidi.LRO, bidi.RLO, bidi.LRE, bidi.RLE, bidi.PDF,
			bidi.LRI, bidi.RLI, bidi.FSI, bidi.PDI:
			classes[i] = bidi.BN
		}
		original[i] = classes[i]
	}

	paragraph := paragraphLevel(classes, base)

	// Rules W1 to W7, N0 to N2 and I1 to I2 operate on one isolating run sequence.
	// With the explicit controls removed there is exactly one, spanning the whole
	// paragraph, and its boundaries take the paragraph's own direction (rule X10).
	sos := paragraph.direction()
	eos := sos

	resolveWeak(classes, sos, eos)
	resolveBrackets(runes, classes, paragraph, sos)
	resolveNeutral(classes, paragraph, sos, eos)

	levels := resolveImplicit(classes, paragraph)
	resolveSeparators(levels, original, paragraph)

	return levels, paragraph
}

// paragraphLevel implements rules P2 and P3: the first strong character decides.
func paragraphLevel(classes []bidi.Class, base Direction) level {
	switch base {
	case DirectionLeftToRight:
		return 0
	case DirectionRightToLeft:
		return 1
	}

	for _, class := range classes {
		switch class {
		case bidi.L:
			return 0
		case bidi.R, bidi.AL:
			return 1
		}
	}
	return 0
}

// resolveWeak applies rules W1 to W7, which turn the weak classes — combining marks,
// digits and the separators between them — into strong ones.
func resolveWeak(classes []bidi.Class, sos, eos bidi.Class) {
	// W1: a combining mark takes the class of what it attaches to.
	previous := sos
	for i, class := range classes {
		if class == bidi.NSM {
			classes[i] = previous
			// A mark on a mark inherits through, which is why previous is not
			// updated to NSM here.
			continue
		}
		if class != bidi.BN {
			previous = class
		}
	}

	// W2: a European digit becomes an Arabic one when the last strong class before it
	// was Arabic, because the digit belongs to the Arabic number that surrounds it.
	strong := sos
	for i, class := range classes {
		switch class {
		case bidi.L, bidi.R, bidi.AL:
			strong = class
		case bidi.EN:
			if strong == bidi.AL {
				classes[i] = bidi.AN
			}
		}
	}

	// W3: Arabic letters are right-to-left from here on; the distinction has done its
	// work in W2.
	for i, class := range classes {
		if class == bidi.AL {
			classes[i] = bidi.R
		}
	}

	// W4: a single separator between two digits of the same kind joins them.
	for i := 1; i < len(classes)-1; i++ {
		before, after := neighbours(classes, i)
		switch classes[i] {
		case bidi.ES:
			if before == bidi.EN && after == bidi.EN {
				classes[i] = bidi.EN
			}
		case bidi.CS:
			if before == after && (before == bidi.EN || before == bidi.AN) {
				classes[i] = before
			}
		}
	}

	// W5: a run of terminators adjacent to European digits joins them, which is what
	// makes a currency symbol or a percent sign part of its number.
	for i := 0; i < len(classes); i++ {
		if classes[i] != bidi.ET {
			continue
		}

		start, end := i, i
		for end < len(classes) && (classes[end] == bidi.ET || classes[end] == bidi.BN) {
			end++
		}

		adjacent := (start > 0 && previousClass(classes, start) == bidi.EN) ||
			(end < len(classes) && classes[end] == bidi.EN)

		if adjacent {
			for j := start; j < end; j++ {
				if classes[j] == bidi.ET {
					classes[j] = bidi.EN
				}
			}
		}
		i = end - 1
	}

	// W6: any separator or terminator still standing is neutral.
	for i, class := range classes {
		switch class {
		case bidi.ES, bidi.ET, bidi.CS:
			classes[i] = bidi.ON
		}
	}

	// W7: a European digit in left-to-right context is left-to-right, so it does not
	// drag its surroundings into a right-to-left run.
	strong = sos
	for i, class := range classes {
		switch class {
		case bidi.L, bidi.R:
			strong = class
		case bidi.EN:
			if strong == bidi.L {
				classes[i] = bidi.L
			}
		}
	}
}

// neighbours returns the classes either side of i, skipping removed characters.
func neighbours(classes []bidi.Class, i int) (before, after bidi.Class) {
	return previousClass(classes, i), nextClass(classes, i)
}

func previousClass(classes []bidi.Class, i int) bidi.Class {
	for j := i - 1; j >= 0; j-- {
		if classes[j] != bidi.BN {
			return classes[j]
		}
	}
	return bidi.ON
}

func nextClass(classes []bidi.Class, i int) bidi.Class {
	for j := i + 1; j < len(classes); j++ {
		if classes[j] != bidi.BN {
			return classes[j]
		}
	}
	return bidi.ON
}

// isNeutral reports whether a class is a neutral or isolate, which rules N1 and N2
// resolve together.
func isNeutral(class bidi.Class) bool {
	switch class {
	case bidi.B, bidi.S, bidi.WS, bidi.ON,
		bidi.LRI, bidi.RLI, bidi.FSI, bidi.PDI:
		return true
	}
	return false
}

// strongOf maps a resolved class to the direction it counts as for rules N1 and N2,
// where a number behaves as right-to-left.
func strongOf(class bidi.Class) bidi.Class {
	switch class {
	case bidi.L:
		return bidi.L
	case bidi.R, bidi.EN, bidi.AN:
		return bidi.R
	}
	return bidi.ON
}

// resolveNeutral applies rules N1 and N2: a run of neutrals between two strong
// characters of the same direction takes that direction, and otherwise takes the
// paragraph's.
func resolveNeutral(classes []bidi.Class, paragraph level, sos, eos bidi.Class) {
	embedding := paragraph.direction()

	for i := 0; i < len(classes); i++ {
		if !isNeutral(classes[i]) {
			continue
		}

		start, end := i, i
		for end < len(classes) && (isNeutral(classes[end]) || classes[end] == bidi.BN) {
			end++
		}

		before := sos
		if start > 0 {
			before = strongOf(previousClass(classes, start))
		}
		after := eos
		if end < len(classes) {
			after = strongOf(classes[end])
		}

		resolved := embedding
		if before == after && (before == bidi.L || before == bidi.R) {
			resolved = before
		}

		for j := start; j < end; j++ {
			if classes[j] != bidi.BN {
				classes[j] = resolved
			}
		}
		i = end - 1
	}
}

// resolveImplicit applies rules I1 and I2, turning resolved classes into levels.
func resolveImplicit(classes []bidi.Class, paragraph level) []level {
	levels := make([]level, len(classes))

	for i, class := range classes {
		l := paragraph

		if paragraph.isRTL() {
			// At an odd level, anything left-to-right or numeric steps up one.
			switch class {
			case bidi.L, bidi.EN, bidi.AN:
				l = paragraph + 1
			}
		} else {
			// At an even level, right-to-left steps up one and numbers step up two,
			// which is what puts a number inside an Arabic phrase at its own level and
			// keeps it reading left to right inside a line that does not.
			switch class {
			case bidi.R:
				l = paragraph + 1
			case bidi.EN, bidi.AN:
				l = paragraph + 2
			}
		}
		levels[i] = l
	}

	// A removed character takes the level of what precedes it, so it travels with its
	// neighbours rather than splitting a run in two.
	for i := range levels {
		if classes[i] == bidi.BN && i > 0 {
			levels[i] = levels[i-1]
		}
	}

	return levels
}

// resolveSeparators applies rule L1: paragraph and segment separators, and any
// whitespace before them or at the end of the line, revert to the paragraph level.
//
// This is what keeps a trailing space from being dragged to the far side of a
// right-to-left line, where it would push the text away from the margin.
func resolveSeparators(levels []level, original []bidi.Class, paragraph level) {
	resetting := true

	for i := len(levels) - 1; i >= 0; i-- {
		switch original[i] {
		case bidi.B, bidi.S:
			levels[i] = paragraph
			resetting = true
		case bidi.WS, bidi.BN,
			bidi.LRI, bidi.RLI, bidi.FSI, bidi.PDI:
			if resetting {
				levels[i] = paragraph
			}
		default:
			resetting = false
		}
	}
}

// reorderLevels applies rule L2 and returns the visual order as indices.
//
// From the highest level down to the lowest odd one, every contiguous stretch of
// characters at or above that level is reversed. Applying it level by level is what
// produces nesting: an inner run reversed twice comes back to its own order inside an
// outer run that did not.
func reorderLevels(levels []level) []int {
	order := make([]int, len(levels))
	for i := range order {
		order[i] = i
	}
	if len(levels) == 0 {
		return order
	}

	highest := level(0)
	lowestOdd := level(maxDepth + 1)

	for _, l := range levels {
		if l > highest {
			highest = l
		}
		if l.isRTL() && l < lowestOdd {
			lowestOdd = l
		}
	}

	for l := highest; l >= lowestOdd; l-- {
		for i := 0; i < len(levels); i++ {
			if levels[i] < l {
				continue
			}

			start := i
			for i < len(levels) && levels[i] >= l {
				i++
			}
			reverse(order[start:i])
		}
	}

	return order
}

// bracketPair is a matched pair of brackets, by index into the sequence.
type bracketPair struct{ open, close int }

// bracketStackSize is the limit rule BD16 puts on nesting. Past it the algorithm
// stops looking for pairs rather than growing without bound.
const bracketStackSize = 63

// findBracketPairs implements rule BD16.
//
// Only brackets still neutral after the weak rules count, and a closing bracket
// matches the most recent unmatched opening bracket of the right kind — searching down
// the stack rather than just its top, so that "[a(b]" pairs the square brackets and
// discards the stray parenthesis.
func findBracketPairs(runes []rune, classes []bidi.Class) []bracketPair {
	type opening struct {
		at     int
		expect rune
	}

	var (
		stack []opening
		pairs []bracketPair
	)

	for i, r := range runes {
		if classes[i] != bidi.ON {
			continue
		}

		p, _ := bidi.LookupRune(r)
		if !p.IsBracket() {
			continue
		}

		if p.IsOpeningBracket() {
			if len(stack) == bracketStackSize {
				// BD16 says to abandon the search entirely rather than to carry on
				// with a truncated stack, since the pairs found after an overflow
				// would be arbitrary.
				return sortPairs(pairs)
			}
			stack = append(stack, opening{at: i, expect: mirrored(r)})
			continue
		}

		// From the top of the stack downwards, which is what BD16 means by advancing
		// to the next element deeper in the stack, and what the implementation in
		// golang.org/x/text does — the one that is run against Unicode's own
		// conformance data. fribidi pairs a closing bracket with the *earliest*
		// matching opener instead, which gives a different answer for brackets that
		// nest improperly, as in "[a(b]". Well-formed nesting is unaffected, and text
		// with mismatched brackets has no correct reading to get right.
		for depth := len(stack) - 1; depth >= 0; depth-- {
			if stack[depth].expect != r {
				continue
			}
			pairs = append(pairs, bracketPair{open: stack[depth].at, close: i})
			// Everything opened inside the pair just closed is unmatched.
			stack = stack[:depth]
			break
		}
	}

	return sortPairs(pairs)
}

// sortPairs orders pairs by opening position, which is the order rule N0 processes
// them in.
func sortPairs(pairs []bracketPair) []bracketPair {
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0 && pairs[j].open < pairs[j-1].open; j-- {
			pairs[j], pairs[j-1] = pairs[j-1], pairs[j]
		}
	}
	return pairs
}

// resolveBrackets applies rule N0: a matched pair of brackets takes the direction of
// what it encloses.
//
// The rule exists because a bracket is a neutral, and left to rules N1 and N2 it takes
// its direction from whatever happens to sit either side of it — so "[English] עברית"
// can put the closing bracket on the wrong side of the phrase it closes. It was left
// out of the first version of this file on the evidence of a hand-written corpus, where
// nothing depended on it. A randomised comparison against fribidi found a case in
// under a second.
func resolveBrackets(runes []rune, classes []bidi.Class, paragraph level, sos bidi.Class) {
	embedding := paragraph.direction()
	opposite := bidi.L
	if embedding == bidi.L {
		opposite = bidi.R
	}

	for _, pair := range findBracketPairs(runes, classes) {
		var foundEmbedding, foundOpposite bool

		for i := pair.open + 1; i < pair.close; i++ {
			switch strongOf(classes[i]) {
			case embedding:
				foundEmbedding = true
			case opposite:
				foundOpposite = true
			}
		}

		var resolved bidi.Class

		switch {
		case foundEmbedding:
			// Something inside runs the same way as the text around it, so the
			// brackets follow.
			resolved = embedding

		case foundOpposite:
			// Only the opposite direction inside. The brackets go with it when the
			// text before them runs that way too, and otherwise stay with the
			// paragraph — which is what keeps a bracketed foreign phrase from
			// dragging its brackets across the words beside it.
			resolved = embedding
			if precedingStrong(classes, pair.open, sos) == opposite {
				resolved = opposite
			}

		default:
			// Nothing strong inside: the brackets are left to N1 and N2.
			continue
		}

		classes[pair.open] = resolved
		classes[pair.close] = resolved

		// A combining mark on a bracket whose class just changed has to change with
		// it; W1 gave it the bracket's old class.
		for _, at := range []int{pair.open, pair.close} {
			for j := at + 1; j < len(runes); j++ {
				p, _ := bidi.LookupRune(runes[j])
				if p.Class() != bidi.NSM {
					break
				}
				classes[j] = resolved
			}
		}
	}
}

// precedingStrong returns the direction of the last strong class before i, or sos when
// there is none.
func precedingStrong(classes []bidi.Class, i int, sos bidi.Class) bidi.Class {
	for j := i - 1; j >= 0; j-- {
		if strong := strongOf(classes[j]); strong != bidi.ON {
			return strong
		}
	}
	return sos
}
