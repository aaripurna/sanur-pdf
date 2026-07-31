package sanur_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/elements"
)

// tocDocument builds a contents list followed by sections far enough apart to land on
// known sheets.
//
// The point of the shape is that the answers are not guessable: the contents list is on
// sheet one, and each section is pushed onto its own sheet by a page break, so a page
// number that came from anywhere but the layout would be wrong.
func tocDocument(t *testing.T, uncompressed bool) []byte {
	t.Helper()

	sections := []string{"Introduction", "Methods", "Results"}

	doc := sanur.New().Title("Contents")
	if uncompressed {
		doc = doc.Uncompressed()
	}

	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(6)
			c.Item().StyledText("Contents", sanur.TextStyle().Size(18).Bold())

			for _, name := range sections {
				c.Item().Row(func(r *sanur.RowBuilder) {
					r.RelativeItem(1).LinkTo("bookmark:" + name).Text(name)
					r.ConstantItem(30).AlignRight().PageRef("bookmark:" + name)
				})
			}

			// Each section on its own sheet, so the expected numbers are 2, 3 and 4.
			for _, name := range sections {
				c.Item().PageBreak()
				c.Item().Bookmark(name).StyledText(name, sanur.TextStyle().Size(15).Bold())
				c.Item().Text("Body text for " + name + ".")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}
	return data
}

func TestTableOfContentsPrintsResolvedPageNumbers(t *testing.T) {
	// The whole feature: an entry has to be able to say which sheet its section landed
	// on, which is not known while the entry itself is being laid out.
	data := tocDocument(t, true)

	stream := firstContentStream(t, data)

	// The contents list is on sheet one, and the three sections follow it on 2, 3 and 4.
	for _, want := range []string{"(2) Tj", "(3) Tj", "(4) Tj"} {
		if !strings.Contains(stream, want) {
			t.Errorf("the contents list does not print %q; got:\n%s", want, stream)
		}
	}

	// The placeholder that stands in during the unresolved passes must not survive into
	// the output — that would mean the final pass ran without the answers.
	if strings.Contains(stream, "(00) Tj") {
		t.Errorf("an unresolved placeholder reached the output:\n%s", stream)
	}
}

// firstContentStream returns the content stream of the first page.
func firstContentStream(t *testing.T, data []byte) string {
	t.Helper()

	for _, m := range regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`).FindAllSubmatch(data, -1) {
		body := string(m[1])
		if strings.Contains(body, "BT") {
			return body
		}
	}
	t.Fatalf("no content stream found in:\n%s", data)
	return ""
}

func TestPageRefRendersAPlaceholderForAnUnknownDestination(t *testing.T) {
	// A contents list naming a section that has not been written yet must still
	// generate. Failing would mean a document could not be built incrementally, and the
	// first pass has to succeed for there to be a second.
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().PageRef("nothing registers this")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}
	if !bytes.Contains(data, []byte("(00) Tj")) {
		t.Errorf("no placeholder was drawn:\n%s", data)
	}
}

func TestPageRefFormatSurroundsTheNumber(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().PageRefFormat("end", "see page {page}")
			c.Item().PageBreak()
			c.Item().Anchor("end").Text("Here")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("(see page 2) Tj")) {
		t.Errorf("the format was not applied:\n%s", data)
	}
}

func TestPageRefResolvesAgainstAnchorsAsWellAsBookmarks(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().PageRef("appendix")
			c.Item().PageBreak()
			c.Item().PageBreak()
			c.Item().Anchor("appendix").Text("Appendix")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("(3) Tj")) {
		t.Errorf("the anchor's page was not resolved:\n%s", data)
	}
}

func TestResolvingPageNumbersDoesNotChangeTheDocumentOtherwise(t *testing.T) {
	// The extra pass exists to fill in numbers, not to alter anything else. Generating
	// twice has to give identical bytes, which also proves the resolution loop is not
	// leaving state behind between passes.
	first := tocDocument(t, false)
	second := tocDocument(t, false)

	if !bytes.Equal(first, second) {
		t.Error("two runs produced different bytes")
	}
}

func TestTableOfContentsPassesGhostscript(t *testing.T) {
	checkWithGhostscript(t, tocDocument(t, false))
}

// TestPageNumbersConvergeWhenTheyChangeTheLayout is why resolution is a loop rather
// than a single extra pass.
//
// Printing a page number changes the width of the entry that prints it, which can wrap
// a title, which changes a height, which can push a section onto a different sheet —
// making the number that was just resolved wrong. The placeholder is deliberately far
// wider than any real number here, so the first pass lays the document out quite
// differently from the last and a single pass would record the wrong sheet.
func TestPageNumbersConvergeWhenTheyChangeTheLayout(t *testing.T) {
	build := func(unresolved string) []byte {
		t.Helper()

		doc := sanur.New().Uncompressed()
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Spacing(4)

				// An auto-width number column, so the title gets whatever is left and
				// wraps according to how wide the number turned out to be.
				c.Item().Row(func(r *sanur.RowBuilder) {
					ref := &elements.PageRef{
						Destination: "target",
						Style:       sanur.TextStyle().Size(11).Build(),
						Unresolved:  unresolved,
					}
					r.RelativeItem(1).Text(strings.Repeat("A long contents entry title. ", 3))
					r.AutoItem().Element(ref)
				})

				// Exactly enough body to put the anchor at the sheet boundary, so the
				// extra line the wide placeholder produces is what decides which sheet
				// it lands on. Found by sweeping the count: at 44 lines both
				// placeholders agree, at 46 both agree the other way, and only here does
				// the placeholder's own width change the answer.
				for i := 0; i < 45; i++ {
					c.Item().Text("Body line that occupies one line of the page.")
				}

				c.Item().Anchor("target").Text("Target section")
			})
		})

		data, err := doc.Bytes()
		if err != nil {
			t.Fatalf("generating with placeholder %q: %v", unresolved, err)
		}
		return data
	}

	// The narrow placeholder is close to the answer, so the first pass is nearly right.
	// The wide one is not, and the loop has to notice and try again.
	narrow := firstContentStream(t, build("0"))
	wide := firstContentStream(t, build(strings.Repeat("0", 30)))

	number := regexp.MustCompile(`\((\d+)\) Tj`)

	narrowMatch := number.FindStringSubmatch(narrow)
	wideMatch := number.FindStringSubmatch(wide)

	if narrowMatch == nil || wideMatch == nil {
		t.Fatalf("no resolved page number found\nnarrow:\n%s\nwide:\n%s", narrow, wide)
	}

	// Whatever the placeholder was, the answer has to be the same: the sheet the anchor
	// actually landed on in the document that was finally written. A single resolution
	// pass records the sheet from a layout that included the placeholder, and gets this
	// wrong.
	if narrowMatch[1] != wideMatch[1] {
		t.Errorf("the resolved page depends on the placeholder: %q with a narrow one, "+
			"%q with a wide one", narrowMatch[1], wideMatch[1])
	}
	if narrowMatch[1] != "1" {
		t.Errorf("the anchor resolved to sheet %s, want 1 — without the placeholder "+
			"taking up room it fits on the first sheet", narrowMatch[1])
	}
}
