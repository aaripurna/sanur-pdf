package text

import (
	"math/rand"
	"os/exec"
	"strings"
	"testing"
)

// This file checks the bidirectional algorithm against fribidi, the reference
// implementation the rest of the world uses.
//
// It earns its keep. The first version of the reordering here read the bidi package's
// run positions as byte offsets when they are rune indices, which left every Hebrew
// string containing a digit silently unreordered — and it looked right in a rendered
// page, because the words were individually correct. Nothing but a comparison against
// a known-good implementation was going to catch that.

// reference returns fribidi's visual order for s.
//
// fribidi shapes Arabic on the way out and pads a ligature with a zero-width no-break
// space to keep the character count, so this side is shaped too and the padding is
// removed before comparing.
func reference(t *testing.T, s string, base Direction) (string, bool) {
	t.Helper()

	fribidi, err := exec.LookPath("fribidi")
	if err != nil {
		return "", false
	}

	args := []string{"--nopad", "--nobreak"}
	switch base {
	case DirectionLeftToRight:
		args = append(args, "--ltr")
	case DirectionRightToLeft:
		args = append(args, "--rtl")
	}

	cmd := exec.Command(fribidi, args...)
	cmd.Stdin = strings.NewReader(s + "\n")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fribidi failed on %q: %v", s, err)
	}

	return strings.ReplaceAll(strings.TrimRight(string(out), "\n"), "\ufeff", ""), true
}

// corpus covers the shapes real bidirectional text takes: pure runs, a phrase of one
// direction quoted inside the other, numbers next to right-to-left words — which is
// where naive implementations fail — brackets, punctuation and currency.
var corpus = []string{
	"",
	"plain ascii text",
	"12 34",
	"Total: 1,234.56",

	// Pure right-to-left.
	"דג סקרן שט בים",
	"مرحبا بالعالم",
	"السلام عليكم ورحمة الله وبركاته",
	"لا إله إلا الله",
	"سلام دنیا",

	// Right-to-left with numbers, the case that needs real embedding levels.
	"עמוד 12",
	"الصفحة 12 من 34",
	"בשנת 2024 היו 15 אירועים",
	"اسمي John وعمري 30 سنة",
	"Total: 1,234.56 ريال",
	"‏12.5% من الطلاب",
	"رقم الهاتف 555-1234",

	// A right-to-left phrase inside a left-to-right sentence, and the reverse.
	"Hebrew: דג סקרן שט בים",
	"Mixed: الصفحة 12 من 34",
	"Page: مرحبا Go بالعالم",
	"abc דג 12 def",
	"شكرا Thank you جزيلا",
	"The word שלום means peace",
	"قال Hello ثم غادر",

	// Brackets and quotes, which rule N0 would otherwise handle.
	"abc (דג) def",
	"מה (this) אומר",
	"العنوان (Main Street) هنا",
	"\"דג סקרן\" הוא ביטוי",
	"[12] דג סקרן",
	"א (ב [ג] ד) ה",

	// Punctuation and separators at the edges, which rule L1 resets.
	"דג סקרן   ",
	"   דג סקרן",
	"דג, סקרן; שט!",
	"مرحبا؟",
	"עמוד 12.",

	// Combining marks, which rule W1 resolves.
	"بَيْت",
	"שָׁלוֹם",
	"مُحَمَّد",

	// Newlines and tabs, which are separators of their own.
	"דג\tסקרן",
	"a\tדג\tb",
}

func TestBidiMatchesFribidi(t *testing.T) {
	if _, ok := reference(t, "a", DirectionNeutral); !ok {
		t.Skip("fribidi not installed")
	}

	for _, in := range corpus {
		for _, base := range []Direction{DirectionNeutral, DirectionLeftToRight, DirectionRightToLeft} {
			want, _ := reference(t, in, base)

			// Both sides are shaped, because fribidi's visual output is shaped and the
			// two transformations have to agree about which glyphs there are before
			// their order can be compared.
			got := ReorderIn(Shape(in, nil), base)

			if got != want {
				t.Errorf("base %v, input %q\n  got  %q\n  want %q", base, in, got, want)
			}
		}
	}
}

// TestBidiMatchesFribidiOnRandomText fuzzes the algorithm.
//
// The fixed corpus above covers the cases somebody thought of. This covers the ones
// nobody did: strings are assembled from every class the algorithm distinguishes —
// strong, weak, neutral, numeric, marks — in random order, which is where the
// interaction between rules W4, W5, N1 and L1 goes wrong. It found the missing bracket
// rule N0 within a second of being written.
//
// Brackets are generated properly nested. Improperly nested ones — "[a(b]" — are the
// one place this implementation and fribidi disagree: BD16 says to match a closing
// bracket against the most recent matching opener, which is what golang.org/x/text
// does and what is checked against Unicode's own conformance data, while fribidi
// matches the earliest. Text like that has no correct reading, so generating it would
// only measure which reference the test happened to pick.
func TestBidiMatchesFribidiOnRandomText(t *testing.T) {
	if _, ok := reference(t, "a", DirectionNeutral); !ok {
		t.Skip("fribidi not installed")
	}

	// One character from each bidi class that matters, so a generated string exercises
	// the rules rather than just the alphabet.
	bases := []rune{
		'a', 'Z', // L
		'\u05d3', '\u05dd', // R, Hebrew
		'\u0645', '\u0628', // AL, Arabic
		'1', '9', // EN
		'\u0664', '\u0667', // AN, Arabic-Indic digits
	}
	neutrals := []rune{
		'+', '-', // ES
		'$', '%', // ET
		',', '.', // CS
		' ', '\t', // WS, S
		'"', '!', '?', ':', // ON
	}
	// Combining marks, emitted only after something they can attach to.
	marks := []rune{'\u0651', '\u05b7'} // Arabic shadda, Hebrew patah

	// Bracket kinds, inserted as balanced pairs.
	brackets := [][2]rune{{'(', ')'}, {'[', ']'}, {'{', '}'}}

	// A fixed seed: a failure has to be reproducible, and a fuzz test that finds a
	// different bug on every run is a test nobody can act on.
	rng := rand.New(rand.NewSource(20260731))

	for i := 0; i < 600; i++ {
		in := randomBidiText(rng, bases, neutrals, marks, brackets, 1+rng.Intn(24))

		for _, base := range []Direction{DirectionNeutral, DirectionLeftToRight, DirectionRightToLeft} {
			want, _ := reference(t, in, base)
			got := ReorderIn(Shape(in, nil), base)

			if got != want {
				t.Fatalf("base %v, input %q (% X)\n  got  %q\n  want %q",
					base, in, []rune(in), got, want)
			}
		}
	}
}

// randomBidiText builds a random string that is well-formed text: brackets nest
// properly, and a combining mark only ever follows something it can attach to.
//
// Both constraints exist because they mark the boundary of what this implementation
// promises. Improperly nested brackets and a mark with no base character are the two
// places it and fribidi part company, and neither is text — generating them would only
// measure which of two references the test happened to prefer.
func randomBidiText(rng *rand.Rand, bases, neutrals, marks []rune, brackets [][2]rune, length int) string {
	var (
		b        strings.Builder
		open     []rune // the closers owed, innermost last
		attached bool   // whether the last character can carry a mark
	)

	emit := func(r rune, canCarryMark bool) {
		b.WriteRune(r)
		attached = canCarryMark
	}

	for i := 0; i < length; i++ {
		switch {
		// Close what is open, sometimes, and always by the end.
		case len(open) > 0 && (i == length-1 || rng.Intn(4) == 0):
			emit(open[len(open)-1], false)
			open = open[:len(open)-1]

		case len(open) < 3 && rng.Intn(8) == 0:
			pair := brackets[rng.Intn(len(brackets))]
			emit(pair[0], false)
			open = append(open, pair[1])

		case attached && rng.Intn(5) == 0:
			// A mark keeps the base it attaches to, so more may follow.
			emit(marks[rng.Intn(len(marks))], true)

		case rng.Intn(3) == 0:
			emit(neutrals[rng.Intn(len(neutrals))], false)

		default:
			emit(bases[rng.Intn(len(bases))], true)
		}
	}

	// Anything still open is closed, so the nesting is always well formed.
	for i := len(open) - 1; i >= 0; i-- {
		b.WriteRune(open[i])
	}

	return b.String()
}
