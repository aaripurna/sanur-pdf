package sanur_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

// markersOf returns the text runs a document draws, in order.
//
// A list's markers are ordinary text, so this is how the scheme is checked: the numbering
// attribute in the structure says what a reader should announce, and these say what is
// actually on the page. Both matter, and they are independent — claiming Decimal while
// drawing bullets would satisfy either check alone.
func markersOf(t *testing.T, build func(*sanur.ListBuilder)) []string {
	t.Helper()

	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().List(build)
	})

	var runs []string
	for _, m := range regexp.MustCompile(`\(([^)]*)\) Tj`).FindAllStringSubmatch(stream, -1) {
		runs = append(runs, decodeLiteral(m[1]))
	}
	return runs
}

// decodeLiteral undoes the escaping a content-stream string uses, so an expectation can be
// written as the character it is.
//
// The bullet is the reason this is needed: it is 0x95 in WinAnsi, outside the printable
// range, so it reaches the stream as the octal escape \225. Comparing against that would
// work and would say nothing about what is drawn.
func decodeLiteral(s string) string {
	octal := regexp.MustCompile(`\\(\d{3})`)

	decoded := octal.ReplaceAllStringFunc(s, func(escape string) string {
		n, err := strconv.ParseUint(escape[1:], 8, 8)
		if err != nil {
			return escape
		}
		// The byte is a WinAnsi code, which is what a standard-14 font is addressed by.
		return string(fonts.RuneForWinAnsiCode(byte(n)))
	})

	return decoded
}

func TestListMarkerSchemes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*sanur.ListBuilder)
		want  []string
	}{
		{
			// A disc is the default, so a list that says nothing gets bullets.
			"default", func(l *sanur.ListBuilder) { l.Items("a", "b") },
			[]string{"•", "a", "•", "b"},
		},
		{
			"bulleted", func(l *sanur.ListBuilder) { l.Bulleted().Items("a", "b") },
			[]string{"•", "a", "•", "b"},
		},
		{
			"numbered", func(l *sanur.ListBuilder) { l.Numbered().Items("a", "b", "c") },
			[]string{"1.", "a", "2.", "b", "3.", "c"},
		},
		{
			"lettered", func(l *sanur.ListBuilder) { l.Lettered().Items("a", "b") },
			[]string{"a.", "a", "b.", "b"},
		},
		{
			// An unmarked list draws no marker at all, and is still a list: the structure
			// says so even though the gutter is empty.
			"unmarked", func(l *sanur.ListBuilder) { l.Unmarked().Items("a", "b") },
			[]string{"a", "b"},
		},
		{
			"custom", func(l *sanur.ListBuilder) {
				l.Marker(core.NumberingNone, func(i int) string { return fmt.Sprintf("[%d]", i) })
				l.Items("a", "b")
			},
			[]string{"[0]", "a", "[1]", "b"},
		},
	} {
		got := markersOf(t, tc.setup)

		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: drew %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLetteredListPassesTheTwentySixth(t *testing.T) {
	// The twenty-seventh item is where a plain base-26 conversion goes wrong: it gives ba
	// where a spreadsheet column gives aa. Lists rarely run that long, and the one that
	// does should not look like a bug.
	got := markersOf(t, func(l *sanur.ListBuilder) {
		l.Lettered()
		for i := 0; i < 28; i++ {
			l.Item().Text("x")
		}
	})

	// Markers and bodies alternate, so the markers are the even positions.
	var markers []string
	for i := 0; i < len(got); i += 2 {
		markers = append(markers, got[i])
	}

	for i, want := range map[int]string{0: "a.", 25: "z.", 26: "aa.", 27: "ab."} {
		if i >= len(markers) {
			t.Fatalf("only %d markers drawn", len(markers))
		}
		if markers[i] != want {
			t.Errorf("marker %d = %q, want %q", i, markers[i], want)
		}
	}
}

func TestListBodyHangsClearOfTheGutter(t *testing.T) {
	// The wrapped lines of an item have to align with its first line rather than running
	// back under the marker, which is the whole visual point of a list.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(0)
		p.Content().List(func(l *sanur.ListBuilder) {
			l.Numbered().Gutter(20).MarkerSpace(5)
			l.Item().Text("body")
		})
	})

	// The body is offset by the gutter plus the gap, so its text — including any wrapped
	// line — starts at 25 rather than at zero.
	if !strings.Contains(stream, "1 0 0 1 25 0 cm") {
		t.Errorf("the body is not offset by the gutter and gap; stream:\n%s", stream)
	}
}

func TestListMarkersAreFlushRightInTheGutter(t *testing.T) {
	// So that the numbers of a long list line up on their full stops instead of their
	// leading digits, which is what makes 9. and 10. read as a sequence.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(0)
		p.Content().List(func(l *sanur.ListBuilder) {
			l.Numbered()
			for i := 0; i < 10; i++ {
				l.Item().Text("x")
			}
		})
	})

	// A right-aligned marker is drawn at an offset; a left-aligned one would sit at zero.
	// The ten-item list has both one- and two-digit markers, so if they were left-aligned
	// every marker would share the same offset.
	offsets := regexp.MustCompile(`1 0 0 -1 ([\d.]+) [\d.]+ Tm`).FindAllStringSubmatch(stream, -1)
	if len(offsets) == 0 {
		t.Fatalf("no text placed; stream:\n%s", stream)
	}

	seen := map[string]bool{}
	for _, m := range offsets {
		seen[m[1]] = true
	}
	if len(seen) < 2 {
		t.Errorf("every run was placed at the same offset %v; markers are not flush right", seen)
	}
}

func TestListStructureIsTaggedProperly(t *testing.T) {
	// The structure is the reason this exists rather than leaving callers to compose a
	// column of rows: a reader announces "list of three items, item one" only if something
	// records the list, the item, and the marker as distinct from the body.
	face := embeddedFont(t, "ListStructure")

	doc := sanur.New().Title("Lists").Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(10))
		p.Content().List(func(l *sanur.ListBuilder) {
			l.Numbered().Items("one", "two")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	_, root := structureOf(t, data)

	want := strings.TrimSpace(`
Document
  L
    LI
      Lbl
      LBody
        P
    LI
      Lbl
      LBody
        P
`)
	if got := strings.TrimSpace(root.outline()); got != want {
		t.Errorf("structure:\n%s\nwant:\n%s", got, want)
	}
}

func TestListDeclaresItsNumberingScheme(t *testing.T) {
	// The markers are drawn as ordinary text, so nothing else in the file says whether "1."
	// is a list marker or the start of a sentence. This is what a reader uses to announce
	// the item number instead of reading the digits.
	face := embeddedFont(t, "ListNumbering")

	for _, tc := range []struct {
		setup func(*sanur.ListBuilder)
		want  string
	}{
		{func(l *sanur.ListBuilder) { l.Bulleted().Items("x") }, "/ListNumbering /Disc"},
		{func(l *sanur.ListBuilder) { l.Numbered().Items("x") }, "/ListNumbering /Decimal"},
		{func(l *sanur.ListBuilder) { l.Lettered().Items("x") }, "/ListNumbering /LowerAlpha"},
		{func(l *sanur.ListBuilder) { l.Unmarked().Items("x") }, "/ListNumbering /None"},
	} {
		doc := sanur.New().Tagged("en-GB").Uncompressed()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(10))
			p.Content().List(tc.setup)
		})

		data, err := doc.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), tc.want) {
			t.Errorf("no %q in the output", tc.want)
		}
	}
}

func TestNestedListNestsInTheStructure(t *testing.T) {
	// A sublist belongs to the item that introduces it, which is the shape a document
	// actually has: lead-in text, then the sublist beneath it.
	face := embeddedFont(t, "NestedList")

	doc := sanur.New().Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(10))
		p.Content().List(func(l *sanur.ListBuilder) {
			l.Item().Column(func(c *sanur.ColumnBuilder) {
				c.Item().Text("with a sublist:")
				c.Item().List(func(sub *sanur.ListBuilder) { sub.Items("inner") })
			})
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	_, root := structureOf(t, data)

	// The inner list sits inside the outer item's body, not beside it.
	want := strings.TrimSpace(`
Document
  L
    LI
      Lbl
      LBody
        P
        L
          LI
            Lbl
            LBody
              P
`)
	if got := strings.TrimSpace(root.outline()); got != want {
		t.Errorf("structure:\n%s\nwant:\n%s", got, want)
	}
}

func TestListPaginates(t *testing.T) {
	// A list is a column, so it splits like one. The marker sequence has to carry on across
	// the break rather than restarting, since the markers are generated once at build time
	// and the column merely draws what fits.
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().List(func(l *sanur.ListBuilder) {
			l.Numbered()
			for i := 0; i < 90; i++ {
				l.Item().Text("An item long enough that ninety of them will not fit on one sheet.")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if pages := countPages(data); pages < 2 {
		t.Fatalf("expected the list to paginate, got %d page(s)", pages)
	}
	// The last marker is drawn somewhere, so the numbering ran to the end rather than
	// restarting on the second sheet.
	if !strings.Contains(string(data), "(90.) Tj") {
		t.Error("the ninetieth marker is missing; the numbering may have restarted")
	}
	if strings.Count(string(data), "(1.) Tj") != 1 {
		t.Error("the first marker appears more than once; the numbering restarted")
	}
}

func TestListWorksWithoutTagging(t *testing.T) {
	// The structure is inert in an ordinary document, so a list costs nothing but its
	// layout and looks the same either way.
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().List(func(l *sanur.ListBuilder) { l.Numbered().Items("one", "two") })
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"(1.) Tj", "(one) Tj", "(2.) Tj", "(two) Tj"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("output is missing %q", want)
		}
	}
	if strings.Contains(string(data), "/ListNumbering") || strings.Contains(string(data), "BDC") {
		t.Error("an untagged list carried structure")
	}
}

func TestListMarkerStyleIsIndependent(t *testing.T) {
	// For a list whose bullets are a different colour from its text, which is common enough
	// in a designed document to be worth not requiring a custom element.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().List(func(l *sanur.ListBuilder) {
			l.MarkerStyle(sanur.TextStyle().Size(10).Color(sanur.Red))
			l.Item().Text("body in the inherited colour")
		})
	})

	// The marker is red and the body is not.
	if !strings.Contains(stream, "0.898 0.224 0.208 rg") {
		t.Errorf("the marker was not styled independently; stream:\n%s", stream)
	}
	if !strings.Contains(stream, "0 0 0 rg") {
		t.Errorf("the body did not keep the inherited colour; stream:\n%s", stream)
	}
}
