package text

import (
	"strings"
	"testing"
)

// codepoints renders a string as hex, since Arabic presentation forms are impossible
// to tell apart by eye and a test failure has to say which form was chosen.
func codepoints(s string) string {
	var parts []string
	for _, r := range s {
		parts = append(parts, sprintHex(r))
	}
	return strings.Join(parts, " ")
}

func sprintHex(r rune) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{
		digits[r>>12&0xF], digits[r>>8&0xF], digits[r>>4&0xF], digits[r&0xF],
	})
}

func TestShapeSelectsContextualForms(t *testing.T) {
	// Every expectation is written as codepoints from the Unicode presentation-form
	// blocks. Comparing rendered strings would pass whatever the shaper produced, since
	// four forms of the same letter look alike at a glance.
	for _, tc := range []struct {
		name, in, want string
	}{
		{
			// Beh joins on both sides, so on its own it is isolated.
			"lone letter", "ب", "FE8F",
		},
		{
			// Two beh: the first opens a cluster, the second closes it.
			"two dual-joining letters", "بب", "FE91 FE90",
		},
		{
			"three dual-joining letters", "ببب", "FE91 FE92 FE90",
		},
		{
			// Alef joins only backwards, so nothing after it can connect: it takes a
			// final form and the letter after it starts again.
			"right-joining breaks the cluster", "باب", "FE91 FE8E FE8F",
		},
		{
			// Reh is right-joining and sits between two joinable letters. Its medial
			// form does not exist, and the answer is final, not isolated — the bug the
			// first version of selectForm had. Hah then stands alone, because nothing
			// can connect forward out of a reh.
			"right-joining letter mid-word", "مرح", "FEE3 FEAE FEA1",
		},
		{
			"marhaba", "مرحبا", "FEE3 FEAE FEA3 FE92 FE8E",
		},
		{
			// A vowel mark is transparent: the letters either side of it are still
			// neighbours, so beh stays initial and yeh stays medial.
			"marks do not break joining", "بَيْت",
			"FE91 064E FEF4 0652 FE96",
		},
		{
			// Persian letters live in the other presentation-form block.
			"persian", "پچ", "FB58 FB7B",
		},
		{
			"nothing arabic", "Hello, world", "0048 0065 006C 006C 006F 002C 0020 0077 006F 0072 006C 0064",
		},
	} {
		if got := codepoints(Shape(tc.in, nil)); got != tc.want {
			t.Errorf("%s: Shape(%q)\n  got  %s\n  want %s", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestShapeFormsTheLamAlefLigature(t *testing.T) {
	// Lam followed by alef has to be written as one glyph. Setting the two letters side
	// by side is as wrong as putting a space inside an English word, which is why this
	// is the one ligature that is not optional.
	for _, tc := range []struct {
		name, in, want string
	}{
		{"isolated", "لا", "FEFB"},
		{"final, after a joining letter", "بلا", "FE91 FEFC"},
		{"alef with hamza above", "لأ", "FEF7"},
		{"alef with madda above", "لآ", "FEF5"},
		{"in a word", "السلام", "FE8D FEDF FEB4 FEFC FEE1"},
	} {
		if got := codepoints(Shape(tc.in, nil)); got != tc.want {
			t.Errorf("%s: Shape(%q)\n  got  %s\n  want %s", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestShapeFallsBackWhenTheFontLacksAForm(t *testing.T) {
	// Some fonts cover Arabic but ship the contextual forms only in their glyph
	// substitution tables, with nothing at the presentation-form codepoints. Asking for
	// a glyph that is not there would draw a question mark; the base letter is unjoined
	// but readable.
	noForms := func(r rune) bool { return r < 0xFB50 }

	if got := codepoints(Shape("بب", noForms)); got != "0628 0628" {
		t.Errorf("with no presentation forms available, got %s, want the base letters", got)
	}

	// A font with the forms still gets them.
	if got := codepoints(Shape("بب", nil)); got != "FE91 FE90" {
		t.Errorf("with the forms available, got %s", got)
	}

	// The ligature falls back to shaping the two letters separately.
	if got := codepoints(Shape("لا", noForms)); got != "0644 0627" {
		t.Errorf("lam-alef with no forms available, got %s", got)
	}
}

func TestShapeHonoursTheJoiningControls(t *testing.T) {
	// The zero-width non-joiner exists to break a connection that would otherwise be
	// made, which is what keeps a Persian prefix visually separate from its stem.
	joined := codepoints(Shape("بب", nil))
	broken := codepoints(Shape("ب‌ب", nil))

	if joined == broken {
		t.Error("the zero-width non-joiner did not break the connection")
	}
	if !strings.HasPrefix(broken, "FE8F") {
		t.Errorf("before a non-joiner beh should be isolated; got %s", broken)
	}

	// Tatweel is join-causing: it lets the letters either side connect through it.
	stretched := codepoints(Shape("بـب", nil))
	if !strings.HasPrefix(stretched, "FE91") {
		t.Errorf("before a tatweel beh should be initial; got %s", stretched)
	}
}

func TestShapeLeavesOtherScriptsAlone(t *testing.T) {
	for _, in := range []string{
		"The quick brown fox",
		"Съешь же ещё этих мягких булок",
		"דג סקרן שט בים",
		"Ξεσκεπάζω",
		"",
	} {
		if got := Shape(in, nil); got != in {
			t.Errorf("Shape(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestBaseRunesUndoesShaping(t *testing.T) {
	// This is what keeps shaped text searchable: a document reporting presentation
	// forms as its content cannot be searched for the words as anyone would type them.
	//
	// None of these contains a lam-alef, because that one cannot round-trip: its two
	// characters are reported in drawing order rather than writing order, for the reason
	// given in TestBaseRunesReversesTheLigature.
	for _, in := range []string{
		"مرحبا",   // marhaba
		"العربية", // al-arabiyya
		"بَيْت",   // with vowel marks
		"دنيا",    // dunya
		"شكرا",    // shukran
	} {
		shaped := Shape(in, nil)

		var restored strings.Builder
		for _, r := range shaped {
			for _, base := range BaseRunes(r) {
				restored.WriteRune(base)
			}
		}

		if got := restored.String(); got != in {
			t.Errorf("shaping %q and undoing it gave %q\n  shaped: %s", in, got, codepoints(shaped))
		}
	}
}

func TestBaseRunesReversesTheLigature(t *testing.T) {
	// The ligature's two characters are reported in the order they are drawn, not the
	// order they are written, because everything downstream of shaping is in visual
	// order. A reader reverses right-to-left runs to recover the text and would
	// otherwise transpose the pair.
	got := BaseRunes(0xFEFB)

	if len(got) != 2 || got[0] != 0x0627 || got[1] != 0x0644 {
		t.Errorf("BaseRunes(lam-alef) = %04X, want alef then lam", got)
	}
}

func TestBaseRunesPassesThroughOrdinaryCharacters(t *testing.T) {
	for _, r := range []rune{'a', 'Z', 'д', 0x0628, ' '} {
		got := BaseRunes(r)
		if len(got) != 1 || got[0] != r {
			t.Errorf("BaseRunes(%q) = %04X, want the character itself", r, got)
		}
	}
}

func TestShapingClassifiesCharacters(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want joining
	}{
		{0x0628, joinDual},        // beh
		{0x0627, joinRight},       // alef
		{0x0640, joinCausing},     // tatweel
		{0x200D, joinCausing},     // zero-width joiner
		{0x200C, joinNone},        // zero-width non-joiner
		{0x064E, joinTransparent}, // fatha
		{'a', joinNone},
		{' ', joinNone},
	} {
		if got := shapingClass(tc.r); got != tc.want {
			t.Errorf("shapingClass(%04X) = %d, want %d", tc.r, got, tc.want)
		}
	}
}
