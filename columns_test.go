package sanur_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
)

// contentStreams returns every page's content stream, in page order.
//
// A multi-column document is checked across sheets as well as across columns, so the
// single stream streamOf returns is not enough.
func contentStreams(t *testing.T, data []byte) []string {
	t.Helper()

	var streams []string
	for _, m := range regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`).FindAllSubmatch(data, -1) {
		body := string(m[1])
		if strings.Contains(body, "cm") || strings.Contains(body, "re") {
			streams = append(streams, body)
		}
	}

	if pages := countPages(data); len(streams) != pages {
		t.Fatalf("found %d content streams for %d pages", len(streams), pages)
	}
	return streams
}

// flowOrder returns the numbered paragraph markers a document drew, in the order the
// content streams place them.
//
// Reading order is the order ink is laid down, which is the only thing that says whether
// a column flowed into the next one or restarted. Poppler's own column detection is a
// heuristic and would be testing poppler.
func flowOrder(t *testing.T, data []byte) []int {
	t.Helper()

	marker := regexp.MustCompile(`\(#(\d+) `)

	var order []int
	for _, stream := range contentStreams(t, data) {
		for _, m := range marker.FindAllStringSubmatch(stream, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatal(err)
			}
			order = append(order, n)
		}
	}
	return order
}

// numbered builds a document whose paragraphs announce their own position, so that a
// missing, repeated or reordered one is visible in the output.
func numbered(t *testing.T, count int, setup func(*sanur.Page)) []byte {
	t.Helper()

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		setup(p)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(6)
			for i := 1; i <= count; i++ {
				// Long enough to wrap several times, so paragraphs also split across
				// column boundaries rather than only landing between them.
				c.Item().Text(fmt.Sprintf("#%d this paragraph is long enough to take "+
					"several lines in a narrow column, so that it splits across a "+
					"boundary rather than merely sitting on one side of it.", i))
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	return data
}

func TestColumnsFlowInReadingOrderWithoutLosingContent(t *testing.T) {
	// The load-bearing check. Content that flows through columns and then onto further
	// sheets is content passing through two nested continuations, and the failure that
	// matters is silent: a paragraph dropped between the last column of one sheet and
	// the first of the next leaves a document that looks entirely plausible.
	const count = 40

	data := numbered(t, count, func(p *sanur.Page) { p.Columns(3) })

	if pages := countPages(data); pages < 2 {
		t.Fatalf("expected the content to run past one sheet, got %d page(s)", pages)
	}

	order := flowOrder(t, data)

	// Every paragraph appears, once, in order. A split paragraph draws its marker on
	// the line the marker is on, so each contributes exactly one.
	if len(order) != count {
		t.Fatalf("drew %d markers for %d paragraphs: %v", len(order), count, order)
	}
	for i, got := range order {
		if got != i+1 {
			t.Fatalf("paragraph %d of the flow is #%d; order was %v", i+1, got, order)
		}
	}
}

func TestColumnsFillBeforeTheNextSheet(t *testing.T) {
	// Columns are only worth having if they are used before a new sheet is started.
	// Three columns of the same content must take about a third of the sheets one
	// column does.
	const count = 90

	single := countPages(numbered(t, count, func(p *sanur.Page) {}))
	triple := countPages(numbered(t, count, func(p *sanur.Page) { p.Columns(3) }))

	if single < 3 {
		t.Fatalf("the single-column control only took %d sheets; the test proves nothing", single)
	}
	if triple >= single {
		t.Errorf("three columns took %d sheets against one column's %d", triple, single)
	}
}

func TestColumnGeometry(t *testing.T) {
	// A4 is 595.28 wide; 40 of margin each side leaves 515.28. Two columns with 18
	// between them are (515.28-18)/2 = 248.64 wide, so the second starts 248.64+18 into
	// the content area — 306.64 from the left edge of the sheet, margin included.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			for i := 0; i < 100; i++ {
				c.Item().Text("A line of text in a column this narrow.")
			}
		})
	})

	// The content origin is the top-left margin, and each column is translated from it.
	if !strings.Contains(stream, "1 0 0 1 40 40 cm") {
		t.Errorf("the first column does not start at the margin; stream:\n%s", stream[:600])
	}
	if !strings.Contains(stream, "1 0 0 1 306.64 40 cm") {
		t.Errorf("the second column does not start at 306.64:\n%s", stream)
	}
}

func TestColumnSpacingIsConfigurable(t *testing.T) {
	// The default is 18; a wider gap moves the second column and narrows both.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2).ColumnSpacing(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			for i := 0; i < 100; i++ {
				c.Item().Text("A line of text in a column this narrow.")
			}
		})
	})

	// (515.28-40)/2 = 237.64, so the second column starts 277.64 into the content area
	// and 317.64 from the edge of the sheet.
	if !strings.Contains(stream, "1 0 0 1 317.64 40 cm") {
		t.Errorf("the gap was not applied:\n%s", stream)
	}
}

func TestSingleColumnIsUnchanged(t *testing.T) {
	// Columns are a new region mechanism in the sheet loop, which every document now
	// goes through. A document that asked for nothing must come out exactly as before,
	// and one column has to be indistinguishable from not asking.
	build := func(setup func(*sanur.Page)) []byte {
		doc := sanur.New().Title("Same document").Uncompressed()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			setup(p)
			p.Footer().AlignCenter().PageNumber("{page} of {total}")
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 90; i++ {
					c.Item().Text("A paragraph that will run past the end of one sheet.")
				}
			})
		})

		data, err := doc.Bytes()
		if err != nil {
			t.Fatalf("generating: %v", err)
		}
		return data
	}

	plain := build(func(*sanur.Page) {})
	if countPages(plain) < 2 {
		t.Fatal("the control did not paginate")
	}

	if explicit := build(func(p *sanur.Page) { p.Columns(1) }); string(explicit) != string(plain) {
		t.Error("Columns(1) is not identical to the default")
	}

	// Spacing is meaningless with one column, so setting it must change nothing.
	if spaced := build(func(p *sanur.Page) { p.ColumnSpacing(100) }); string(spaced) != string(plain) {
		t.Error("column spacing altered a single-column document")
	}
}

func TestImpossibleColumnsAreReported(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*sanur.Page)
		want  string
	}{
		{
			"none", func(p *sanur.Page) { p.Columns(0) },
			"at least one column",
		},
		{
			"negative", func(p *sanur.Page) { p.Columns(-2) },
			"at least one column",
		},
		{
			"negative spacing", func(p *sanur.Page) { p.Columns(2).ColumnSpacing(-4) },
			"cannot be negative",
		},
		{
			// Forty columns of A4 with the default gap leave nothing per column, which
			// is a mistake worth naming rather than a division producing a negative
			// width and a stream of nonsense.
			"too many", func(p *sanur.Page) { p.Columns(40) },
			"leave no room",
		},
	} {
		doc := sanur.New()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			tc.setup(p)
			p.Content().Text("Anything.")
		})

		_, err := doc.Bytes()
		if err == nil {
			t.Errorf("%s: generating succeeded", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error is %q, want it to mention %q", tc.name, err, tc.want)
		}
	}
}

func TestContentTooTallForAColumnIsReported(t *testing.T) {
	// An element taller than a column can never be placed, and every column on every
	// sheet is the same size — so this has to be an error rather than a document that
	// silently omits it or a loop that produces sheets until the cap.
	//
	// The failure is found partway through a sheet, once the first column has taken what
	// it can, and the message says which box the content would not go in. Both are worth
	// pinning: leaving it to the next sheet's first column would also fail, but it would
	// report the whole content area as the space available when the space that matters is
	// one column of it.
	build := func(columns int) error {
		doc := sanur.New()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Columns(columns)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				// Fills the first column, so the failure happens partway through a
				// sheet rather than on the very first measurement.
				for i := 0; i < 30; i++ {
					c.Item().Text("A paragraph that helps fill the first column of the sheet.")
				}
				c.Item().Height(2000).Text("Taller than any column.")
			})
		})

		_, err := doc.Bytes()
		return err
	}

	err := build(2)
	if err == nil {
		t.Fatal("generating succeeded")
	}

	// A4 less 40 of margin each side is 515.28; two columns with the default 18 between
	// them are 248.6 wide. Naming the column's own width is the whole point — the
	// content area's 515.3 would send a reader looking for a 515-point element.
	if !strings.Contains(err.Error(), "does not fit in a 248.6x761.9 column") {
		t.Errorf("error is %q, want it to name the column the content would not fit", err)
	}

	// The control: with one column there is no in-sheet boundary to fail at, and the
	// pre-existing check reports the content area instead.
	single := build(1)
	if single == nil {
		t.Fatal("the single-column control succeeded")
	}
	if !strings.Contains(single.Error(), "does not fit in the available 515.3x761.9") {
		t.Errorf("the single-column error changed: %q", single)
	}
}

func TestColumnRuleIsDrawnBetweenColumns(t *testing.T) {
	// Two columns have one gap, three have two, and the rule sits in the middle of each.
	for _, tc := range []struct{ columns, lines int }{{2, 1}, {3, 2}, {4, 3}} {
		stream := streamOf(t, func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Columns(tc.columns).ColumnSpacing(20).ColumnRule(0.5, sanur.Grey300)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 80; i++ {
					c.Item().Text("Text to fill the columns of this sheet.")
				}
			})
		})

		// A stroked line is a moveto, a lineto and a stroke, all on one line.
		if got := strings.Count(stream, " l S"); got != tc.lines {
			t.Errorf("%d columns drew %d rules, want %d", tc.columns, got, tc.lines)
		}
	}
}

func TestColumnRuleSitsInTheGapAndStopsAtTheText(t *testing.T) {
	// Centred in the gap, or it reads as an underline on one column; and stopping at
	// the depth the text reached, or a half-empty final sheet gets a rule down the rest
	// of the page.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2).ColumnSpacing(20).ColumnRule(0.5, sanur.Black)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Text("A single short line, so the columns are barely filled.")
		})
	})

	// (515.28-20)/2 = 247.64 per column, so the gap runs 247.64..267.64 and its middle
	// is 257.64.
	line := regexp.MustCompile(`([\d.]+) ([\d.]+) m ([\d.]+) ([\d.]+) l S`).FindStringSubmatch(stream)
	if line == nil {
		t.Fatalf("no rule drawn:\n%s", stream)
	}

	if line[1] != "257.64" || line[3] != "257.64" {
		t.Errorf("the rule is at x=%s..%s, want 257.64 (the middle of the gap)", line[1], line[3])
	}

	// The single line of text is nowhere near the full 715 points of content height.
	depth, err := strconv.ParseFloat(line[4], 64)
	if err != nil {
		t.Fatal(err)
	}
	if depth <= 0 || depth > 60 {
		t.Errorf("the rule runs %.1f points deep for one line of text", depth)
	}
}

func TestNoColumnRuleUnlessAskedFor(t *testing.T) {
	// The rule is opt-in, and a single column has no gap to put one in.
	for _, tc := range []struct {
		name  string
		setup func(*sanur.Page)
	}{
		{"not requested", func(p *sanur.Page) { p.Columns(2) }},
		{"zero width", func(p *sanur.Page) { p.Columns(2).ColumnRule(0, sanur.Black) }},
		{"transparent", func(p *sanur.Page) {
			p.Columns(2).ColumnRule(1, sanur.RGBA(0, 0, 0, 0))
		}},
		{"single column", func(p *sanur.Page) { p.Columns(1).ColumnRule(1, sanur.Black) }},
	} {
		stream := streamOf(t, func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			tc.setup(p)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 60; i++ {
					c.Item().Text("Text to fill the columns of this sheet.")
				}
			})
		})

		if strings.Contains(stream, " l S") {
			t.Errorf("%s: a rule was drawn anyway", tc.name)
		}
	}
}

func TestFurnitureStaysFullWidthAcrossColumns(t *testing.T) {
	// The header and footer belong to the sheet, not to a column: a running header cut
	// to a third of the page would be a surprising reading of "two columns".
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(3)
		p.Header().AlignRight().Text("R")
		p.Footer().AlignRight().Text("F")
		p.Content().Text("Body.")
	})

	// Aligning right translates by the width left over, so the header's offset says
	// what width it was aligned within: nearly the full 515.28, rather than the 159.76
	// of a column. Two of them, for the header and the footer.
	var wide int
	for _, m := range regexp.MustCompile(`1 0 0 1 ([\d.]+) 0 cm`).FindAllStringSubmatch(stream, -1) {
		offset, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatal(err)
		}
		if offset > 400 {
			wide++
		}
	}

	if wide != 2 {
		t.Errorf("%d pieces of furniture were aligned within the full width, want 2:\n%s",
			wide, stream)
	}
}

func TestColumnDocumentIsAcceptedByGhostscript(t *testing.T) {
	checkWithGhostscript(t, numbered(t, 40, func(p *sanur.Page) {
		p.Columns(2).ColumnRule(0.5, sanur.Grey300)
	}))
}

func TestColumnsAreDrawnLeftToRight(t *testing.T) {
	// Flow order is only reading order if the columns are laid down in the order they
	// are read. The flow test above proves the paragraphs are drawn in sequence; this
	// proves that sequence runs across the page rather than back up it.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(3)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			for i := 0; i < 140; i++ {
				c.Item().Text("A line in a narrow column.")
			}
		})
	})

	// Three columns of 159.76 starting at 40, 217.76 and 395.52 from the sheet's edge.
	var previous int
	for i, offset := range []string{"1 0 0 1 40 40 cm", "1 0 0 1 217.76 40 cm", "1 0 0 1 395.52 40 cm"} {
		at := strings.Index(stream, offset)
		if at < 0 {
			t.Fatalf("column %d (%q) was never drawn", i+1, offset)
		}
		if at < previous {
			t.Errorf("column %d is drawn before column %d", i+1, i)
		}
		previous = at
	}
}

func TestLinkInASecondColumnLandsInIt(t *testing.T) {
	// A link's rectangle is absolute on the sheet while the element that asks for it
	// knows only its own coordinates, so a column's translate has to reach it. Getting
	// this wrong puts a clickable box over the wrong column, which nothing in the
	// rendered page shows.
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			for i := 0; i < 80; i++ {
				c.Item().Text("A line that helps fill the first column.")
			}
			c.Item().Link("https://example.com").Text("In the second column")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	rect := regexp.MustCompile(`/Rect \[([\d.]+) [\d.]+ ([\d.]+) [\d.]+\]`).FindStringSubmatch(string(data))
	if rect == nil {
		t.Fatalf("no link annotation in:\n%s", data)
	}

	left, err := strconv.ParseFloat(rect[1], 64)
	if err != nil {
		t.Fatal(err)
	}

	// The second column starts 306.64 from the left edge of the sheet.
	if left < 300 {
		t.Errorf("the link starts at x=%.1f, which is in the first column", left)
	}
}

func TestTaggedColumnsKeepReadingOrder(t *testing.T) {
	// Marked content is numbered in the order it is drawn and the structure tree is
	// built in the same pass, so a reader follows the tree rather than the geometry.
	// Both have to agree, and a column boundary is where they would come apart.
	face := embeddedFont(t, "ColumnFace")

	doc := sanur.New().Title("Columns").Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2).ColumnRule(0.5, sanur.Grey300)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(10))
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Tag(sanur.Heading1).Text("Across two columns")
			for i := 0; i < 60; i++ {
				c.Item().Text("A paragraph of body text in one of the two columns.")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if pages := countPages(data); pages != 1 {
		t.Fatalf("expected one sheet of two columns, got %d", pages)
	}

	_, root := structureOf(t, data)

	// Walking the tree in order must give sequence numbers that only ever go up: the
	// structure's order is the reading order, and the numbers are the drawing order.
	var mcids []int
	var walk func(*node)
	walk = func(n *node) {
		mcids = append(mcids, n.mcids...)
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)

	if len(mcids) < 60 {
		t.Fatalf("only %d marked sequences in the tree", len(mcids))
	}
	for i := 1; i < len(mcids); i++ {
		if mcids[i] <= mcids[i-1] {
			t.Fatalf("sequence %d follows %d in the tree: reading order does not "+
				"match the order the columns were drawn", mcids[i], mcids[i-1])
		}
	}
}

func TestTaggedColumnDocumentIsPDFUAConformant(t *testing.T) {
	// The rule between columns is ink with no meaning, which is exactly what a
	// validator objects to when it is left unmarked.
	verapdf, err := exec.LookPath("verapdf")
	if err != nil {
		t.Skip("verapdf not installed")
	}

	face := embeddedFont(t, "ColumnUAFace")

	doc := sanur.New().Title("Two columns").Tagged("en-GB")
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2).ColumnRule(0.5, sanur.Grey300)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(10))
		p.Footer().AlignCenter().PageNumber("{page}")
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Tag(sanur.Heading1).Text("Across two columns")
			for i := 0; i < 60; i++ {
				c.Item().Text("A paragraph of body text in one of the two columns.")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "columns.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(verapdf, "--flavour", "ua1", "--format", "text", path).Output()
	if err != nil {
		t.Fatalf("verapdf failed: %v", err)
	}
	if !strings.HasPrefix(string(out), "PASS") {
		detail, _ := exec.Command(verapdf, "--flavour", "ua1", path).Output()
		t.Errorf("veraPDF rejected the document:\n%s\n%s", out, detail)
	}
}

// spanningDocument is a two-column document with a full-width lead-in and enough body to
// run past one sheet.
func spanningDocument(t *testing.T, setup func(*sanur.Page)) []byte {
	t.Helper()

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2)
		if setup != nil {
			setup(p)
		}
		p.Spanning().PaddingBottom(10).StyledText("HEADLINE",
			sanur.TextStyle().Size(20).Bold())
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			// Enough to fill both columns of two sheets, so the second sheet has a
			// second column to compare against.
			for i := 0; i < 240; i++ {
				c.Item().Text("A line of body text in one of the columns.")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	return data
}

func TestSpanningContentIsDrawnOnceAtFullWidth(t *testing.T) {
	// A headline over the columns is content, not furniture: repeating it on every sheet
	// is what a header does, and would be a different feature.
	data := spanningDocument(t, nil)

	if pages := countPages(data); pages < 2 {
		t.Fatalf("expected more than one sheet, got %d", pages)
	}
	if got := strings.Count(string(data), "(HEADLINE) Tj"); got != 1 {
		t.Errorf("the lead-in was drawn %d times, want once", got)
	}

	streams := contentStreams(t, data)
	if !strings.Contains(streams[0], "1 0 0 1 40 40 cm") {
		t.Errorf("the lead-in does not start at the margin:\n%s", streams[0][:400])
	}
}

func TestSpanningContentSpansTheFullWidth(t *testing.T) {
	// Spanning the columns is the whole point, so it is laid out against the sheet's
	// inner width rather than one column of it. Aligning right is how to see which:
	// the translate is the space left over, so it says what width the content was
	// aligned within.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(3)
		p.Spanning().PaddingBottom(8).AlignRight().Text("R")
		p.Content().Text("Body.")
	})

	offsets := regexp.MustCompile(`1 0 0 1 ([\d.]+) 0 cm`).FindAllStringSubmatch(stream, -1)
	if len(offsets) == 0 {
		t.Fatalf("nothing was aligned:\n%s", stream)
	}

	// Three columns of A4 are 159.76 wide, so a column-width box would put the offset
	// around 152 rather than around 507.
	offset, err := strconv.ParseFloat(offsets[0][1], 64)
	if err != nil {
		t.Fatal(err)
	}
	if offset < 400 {
		t.Errorf("the lead-in was aligned within %.1f points, which is a column, not "+
			"the full width", offset+8)
	}
}

func TestSpanningContentKnowsItsPage(t *testing.T) {
	// It is drawn inside the page loop like any other content, so a page reference in it
	// has to resolve — and it is the one region that would be easy to leave out of the
	// context every other part receives.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2)
		p.Spanning().PaddingBottom(8).PageNumber("Sheet {page} of {total}")
		p.Content().Text("Body.")
	})

	if !strings.Contains(stream, "(Sheet 1 of 1) Tj") {
		t.Errorf("the lead-in did not resolve its page number:\n%s", stream)
	}
}

func TestSpanningContentReservesSpaceOnItsSheetOnly(t *testing.T) {
	// The columns start below the lead-in on the first sheet and at the top of the
	// content area on every other one, or the second sheet would carry a gap where the
	// headline was.
	streams := contentStreams(t, spanningDocument(t, nil))

	// A 20-point line plus 10 of padding puts the columns about 34 points down, so the
	// first sheet's second column is translated to (306.64, 40+34) and the second
	// sheet's to (306.64, 40).
	second := regexp.MustCompile(`1 0 0 1 306\.64 ([\d.]+) cm`)

	first := second.FindStringSubmatch(streams[0])
	if first == nil {
		t.Fatalf("no second column on the first sheet")
	}
	top, err := strconv.ParseFloat(first[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	if top <= 41 {
		t.Errorf("the columns start at y=%.1f, so the lead-in reserved nothing", top)
	}

	rest := second.FindStringSubmatch(streams[1])
	if rest == nil {
		t.Fatalf("no second column on the second sheet")
	}
	if rest[1] != "40" {
		t.Errorf("the second sheet's columns start at y=%s, want the top margin at 40",
			rest[1])
	}
}

func TestSpanningContentThatCannotFitIsReported(t *testing.T) {
	// It cannot be split between columns — that is what spanning them means — so there
	// is nowhere for it to overflow to.
	for _, tc := range []struct {
		name   string
		height float64
		want   string
	}{
		{"taller than the sheet", 2000, "does not fit"},
		{"exactly the content area", 761.89, "leaving them nothing"},
	} {
		doc := sanur.New()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Columns(2)
			p.Spanning().Height(tc.height).Background(sanur.Grey100).Text("Too big.")
			p.Content().Text("Body.")
		})

		_, err := doc.Bytes()
		if err == nil {
			t.Errorf("%s: generating succeeded", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error is %q, want it to mention %q", tc.name, err, tc.want)
		}
	}
}

func TestUnusedSpanningRegionChangesNothing(t *testing.T) {
	// Every document now goes past the lead-in, so one that never asks for it has to be
	// byte-identical to what it was.
	build := func(touch bool) []byte {
		doc := sanur.New().Title("Same").Uncompressed()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			if touch {
				// Asking for the container without putting anything in it.
				p.Spanning()
			}
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 90; i++ {
					c.Item().Text("A paragraph that runs past one sheet.")
				}
			})
		})

		data, err := doc.Bytes()
		if err != nil {
			t.Fatalf("generating: %v", err)
		}
		return data
	}

	if string(build(true)) != string(build(false)) {
		t.Error("an empty spanning region altered the output")
	}
}

func TestSpanningContentIsTaggedAheadOfTheColumns(t *testing.T) {
	// It is content, so it belongs in the structure — and it belongs first, because that
	// is the order it is read in.
	face := embeddedFont(t, "SpanningFace")

	doc := sanur.New().Title("Spanning").Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Columns(2)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(10))
		p.Spanning().PaddingBottom(8).Tag(sanur.Heading1).Text("Headline")
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			for i := 0; i < 60; i++ {
				c.Item().Text("A paragraph of body text in one of the two columns.")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	_, root := structureOf(t, data)

	if len(root.children) == 0 {
		t.Fatal("the structure has no children")
	}
	if got := root.children[0].role; got != "H1" {
		t.Errorf("the first element in reading order is %q, want the spanning heading", got)
	}
}
