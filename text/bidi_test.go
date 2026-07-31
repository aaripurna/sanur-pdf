package text

import (
	"strings"
	"testing"
)

func TestDirectionOfFindsTheFirstStrongCharacter(t *testing.T) {
	// Unicode's rule, and the one a caller needs in order to align a paragraph: a
	// right-to-left paragraph set flush left reads correctly and looks wrong.
	for _, tc := range []struct {
		in   string
		want Direction
	}{
		{"", DirectionNeutral},
		{"12 34", DirectionNeutral},
		{" .,!? ", DirectionNeutral},
		{"hello", DirectionLeftToRight},
		{"דג סקרן", DirectionRightToLeft},
		{"مرحبا", DirectionRightToLeft},

		// The first strong character decides, not the majority.
		{"Hebrew: דג סקרן שט בים מאוד מאוד", DirectionLeftToRight},
		{"דג סקרן and a great deal of English after it", DirectionRightToLeft},

		// Digits and punctuation are not strong, so they do not settle it.
		{"12 דג", DirectionRightToLeft},
		{"(דג)", DirectionRightToLeft},
		{"12 abc", DirectionLeftToRight},
	} {
		if got := DirectionOf(tc.in); got != tc.want {
			t.Errorf("DirectionOf(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDirectionString(t *testing.T) {
	for _, tc := range []struct {
		d    Direction
		want string
	}{
		{DirectionNeutral, "neutral"},
		{DirectionLeftToRight, "left-to-right"},
		{DirectionRightToLeft, "right-to-left"},
		{Direction(99), "neutral"},
	} {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("Direction(%d).String() = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestReorderLeavesLeftToRightTextAlone(t *testing.T) {
	// The fast path, and the one that matters for performance: every line of every
	// document in a Latin script goes through it.
	for _, in := range []string{
		"The quick brown fox",
		"Съешь же ещё этих мягких булок",
		"Ξεσκεπάζω την ψυχοφθόρα",
		"Zażółć gęślą jaźń",
		"1,234.56 (net) — 50%",
		"",
	} {
		if got := Reorder(in); got != in {
			t.Errorf("Reorder(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestReorderReversesRightToLeftRuns(t *testing.T) {
	// Hebrew needs nothing but this: its letters do not join, so reordering is the
	// whole of what makes it correct.
	if got, want := Reorder("דג סקרן שט בים"), "םיב טש ןרקס גד"; got != want {
		t.Errorf("Reorder = %q, want %q", got, want)
	}

	// Inside an English sentence only the Hebrew reverses.
	if got, want := Reorder("Title: דג סקרן"), "Title: ןרקס גד"; got != want {
		t.Errorf("Reorder = %q, want %q", got, want)
	}
}

func TestReorderKeepsNumbersReadingLeftToRight(t *testing.T) {
	// A number embedded in right-to-left text still reads left to right, and it takes
	// its own embedding level to get there. This is the case a two-level approximation
	// gets wrong, and the reason the algorithm is implemented properly.
	if got, want := ReorderIn("עמוד 12", DirectionRightToLeft), "12 דומע"; got != want {
		t.Errorf("Reorder = %q, want %q", got, want)
	}
}

func TestReorderMirrorsBrackets(t *testing.T) {
	// An opening bracket inside right-to-left text is drawn as a closing one, because
	// what it encloses is now on the other side of it. Rule L4.
	got := ReorderIn("(דג)", DirectionRightToLeft)

	if strings.Count(got, "(") != 1 || strings.Count(got, ")") != 1 {
		t.Errorf("Reorder = %q, want one bracket of each kind", got)
	}
	if !strings.HasPrefix(got, "(") {
		t.Errorf("Reorder = %q, want it to open with a bracket that faces the text", got)
	}
}

func TestReorderKeepsMarksAfterTheirBase(t *testing.T) {
	// Rule L3. Reversing a run puts a combining mark in front of the letter it belongs
	// to, and every mark then renders against the wrong letter — the letters are in the
	// right order, which makes the damage easy to miss and impossible to read.
	const in = "בָ" // bet with a qamats under it

	want := []rune(in)
	got := []rune(ReorderIn(in, DirectionRightToLeft))

	if len(got) != len(want) {
		t.Fatalf("Reorder gave %d characters, want %d", len(got), len(want))
	}
	// The cluster comes back in its original order: the base first, then its mark.
	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Reorder = %04X, want %04X — the base letter has to come first", got, want)
	}
}

func TestBaseDirectionOverridesDetection(t *testing.T) {
	// Direction belongs to the paragraph, not the line. A wrapped line that happens to
	// begin with a Latin word inside an Arabic paragraph is still part of that
	// paragraph, and laying it out on its own evidence puts its clauses back to front.
	const line = "Go بالعالم"

	detected := Reorder(line)
	forced := ReorderIn(line, DirectionRightToLeft)

	if detected == forced {
		t.Errorf("supplying a base direction made no difference: %q", detected)
	}
	if detected != ReorderIn(line, DirectionLeftToRight) {
		t.Error("detection should have settled on left-to-right for a line starting with Latin")
	}
}

// TestVisualRunesIsAlwaysAPermutation covers the inputs the reference comparison
// deliberately excludes.
//
// Improperly nested brackets and combining marks with no base character are where this
// implementation and fribidi part company, so their exact order is not pinned. What
// still has to hold is that no character is lost, duplicated or invented, because
// silently dropping text is the one failure that would not be noticed.
func TestVisualRunesIsAlwaysAPermutation(t *testing.T) {
	for _, in := range []string{
		// Mismatched and improperly nested brackets.
		"[a(b]",
		")(",
		"][דג",
		"((((דג",
		"א (ב [ג ד) ה]",

		// Marks with nothing to attach to.
		"ַّדג",
		"ַ",
		"ּּּ",
		" ַ ",

		// The controls that are treated as removed.
		"a‫דג‬ b",
		"⁦דג⁩",

		// Degenerate but legal.
		"",
		"\t\n",
		"דג\n\nסקרן",
	} {
		for _, base := range []Direction{DirectionNeutral, DirectionLeftToRight, DirectionRightToLeft} {
			glyphs := VisualRunes(in, base)
			runes := []rune(in)

			if len(glyphs) != len(runes) {
				t.Errorf("VisualRunes(%q, %v) gave %d glyphs for %d runes",
					in, base, len(glyphs), len(runes))
				continue
			}

			// Every input position appears exactly once.
			seen := make([]bool, len(runes))
			for _, glyph := range glyphs {
				if glyph.From < 0 || glyph.From >= len(runes) {
					t.Errorf("VisualRunes(%q, %v) reported position %d, out of range",
						in, base, glyph.From)
					continue
				}
				if seen[glyph.From] {
					t.Errorf("VisualRunes(%q, %v) reported position %d twice",
						in, base, glyph.From)
				}
				seen[glyph.From] = true
			}
			for i, ok := range seen {
				if !ok {
					t.Errorf("VisualRunes(%q, %v) dropped position %d", in, base, i)
				}
			}

			// A glyph is either the character it came from or its mirror, never
			// anything else.
			for _, glyph := range glyphs {
				original := runes[glyph.From]
				if glyph.Rune != original && glyph.Rune != mirrored(original) {
					t.Errorf("VisualRunes(%q, %v) turned %04X into %04X",
						in, base, original, glyph.Rune)
				}
			}
		}
	}
}

func TestVisualRunesIsDeterministic(t *testing.T) {
	// Output has to be byte-identical between runs, and the resolution walks maps and
	// stacks that could easily leak an order.
	for _, in := range []string{"الصفحة 12 من 34", "abc דג 12 def", "[a(b]"} {
		first := ReorderIn(in, DirectionNeutral)
		for i := 0; i < 5; i++ {
			if got := ReorderIn(in, DirectionNeutral); got != first {
				t.Fatalf("Reorder(%q) is not stable: %q then %q", in, first, got)
			}
		}
	}
}

func TestReorderHandlesLongText(t *testing.T) {
	// The reversal loop runs once per embedding level, and a bug in its bounds would
	// show up as a panic on text long enough to have many runs rather than on a phrase.
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("שלום ")
		b.WriteString("world ")
		b.WriteString("12 ")
	}

	in := b.String()
	got := Reorder(in)

	if len([]rune(got)) != len([]rune(in)) {
		t.Errorf("reordering %d runes produced %d", len([]rune(in)), len([]rune(got)))
	}
}
