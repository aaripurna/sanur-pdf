// Command accessible produces a tagged PDF: one that carries its own structure, so that
// software can tell a heading from a caption and a table's figures from its labels.
//
// An ordinary PDF says where ink goes and nothing more. A heading is text that happens to
// be large; a table is lines that happen to form a grid. Nothing in the file says so,
// which is why a PDF is opaque to a screen reader, cannot reflow onto a small display, and
// resists conversion into anything structured. It is also why tagged output is a
// procurement requirement for public-sector documents across much of the world.
//
// Most of the structure is inferred here — text is a paragraph, the table's header row
// heads its columns, the running footer is decoration a reader skips. Two things are
// declared, because no library can work them out: which text is a heading and at what
// level, and what a picture shows.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/render"
)

var (
	ink      = sanur.Hex("#1A1D29")
	muted    = sanur.Hex("#6B7280")
	accent   = sanur.Hex("#4F46E5")
	hairline = sanur.Hex("#E5E7EB")
	panel    = sanur.Hex("#F6F5FA")
)

type finding struct {
	area   string
	status string
	note   string
}

var findings = []finding{
	{"Headings", "Pass", "Levels descend without skipping"},
	{"Images", "Pass", "Every figure carries a description"},
	{"Tables", "Pass", "Header cells are marked as headers"},
	{"Links", "Pass", "Each link is reachable from the structure"},
	{"Language", "Pass", "Declared as en-GB"},
	{"Furniture", "Pass", "Running header and footer are artifacts"},
}

func main() {
	out := "accessible.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	// A chart stands in for any picture. It has to be described: a figure with nothing to
	// read out is exactly the gap tagging exists to close, and generation refuses without
	// one rather than producing a document that passes for accessible.
	logo, err := render.DecodeImage("logo", swatch())
	if err != nil {
		log.Fatalf("decoding the image: %v", err)
	}

	// Tagged is what switches all of this on, and the language is required: a reader that
	// does not know what language a document is in cannot pronounce it.
	doc := sanur.New().
		Title("Accessibility conformance summary").
		Author("sanur").
		Creator("sanur/examples/accessible").
		Tagged("en-GB")

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(sanur.Mm(18))
		p.DefaultTextStyle(sanur.TextStyle().Size(9.5).Color(ink))

		// Running furniture is decoration whatever it contains, and sanur marks it so
		// automatically. A header repeated on forty sheets is not forty paragraphs to
		// announce, and "Page 12 of 40" read out between every two paragraphs is worse
		// than silence.
		p.Footer().PaddingTop(10).Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("sanur/examples/accessible",
				sanur.TextStyle().Size(7.5).Color(muted))
			r.ConstantItem(110).AlignRight().
				DefaultTextStyle(sanur.TextStyle().Size(7.5).Color(muted)).
				PageNumber("Page {page} of {total}")
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(12)

			// Declared, not inferred. The level is the part that matters: an outline
			// that is confidently wrong is worse for a reader than none at all.
			c.Item().Tag(sanur.Heading1).StyledText("Accessibility conformance",
				sanur.TextStyle().Size(21).Bold().Color(ink))

			c.Item().Text("This document carries its own structure. Everything below is " +
				"reachable by a screen reader in reading order, and the parts that mean " +
				"nothing are marked so that a reader skips them.")

			c.Item().Tag(sanur.Heading2).StyledText("What is declared",
				sanur.TextStyle().Size(13).Bold().Color(ink))

			c.Item().Text("Two things cannot be worked out from a layout, so they are " +
				"stated. The first is a heading and its level: heading text is simply " +
				"text that happens to be large, and guessing would produce an outline " +
				"that is wrong rather than absent. The second is what a picture shows.")

			// A rule is decoration, and sanur marks rules as artifacts without being
			// asked — announcing one between every paragraph would make this document
			// worse than an untagged one.
			c.Item().LineHorizontal(1, hairline)

			c.Item().Tag(sanur.Heading2).StyledText("A described figure",
				sanur.TextStyle().Size(13).Bold().Color(ink))

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(12)
				// Describe supplies what a reader announces in place of the picture.
				r.ConstantItem(120).Describe(
					"A swatch of the four brand colours, in a horizontal band").
					Image(logo)
				r.RelativeItem(1).AlignMiddle().StyledText(
					"The description beside this swatch is what a screen reader reads "+
						"out. Without one, generation fails: an undescribed figure is "+
						"the gap tagging exists to close, and a document that passes "+
						"for accessible and is not would be the worse outcome.",
					sanur.TextStyle().Size(9).Color(muted).LineHeight(1.4))
			})

			c.Item().Tag(sanur.Heading2).StyledText("A table with real headers",
				sanur.TextStyle().Size(13).Bold().Color(ink))

			c.Item().Text("A table's meaning is which cell heads which column, and none " +
				"of that survives in the drawn rules. Declaring a header row is what " +
				"lets a reader say which column a figure sits under.")

			c.Item().Table(func(t *sanur.TableBuilder) {
				t.ColumnConstant(96)
				t.ColumnConstant(52)
				t.ColumnRelative(1)

				// The header row's cells become header cells in the structure, not
				// ordinary ones. That is the whole difference for a reader.
				t.HeaderRow(func(r *sanur.TableRowBuilder) {
					for _, heading := range []string{"Area", "Status", "Note"} {
						r.Cell().Background(accent).PaddingXY(8, 5).StyledText(heading,
							sanur.TextStyle().Size(7.5).Bold().Color(sanur.White))
					}
				})

				for i, f := range findings {
					background := sanur.White
					if i%2 == 1 {
						background = panel
					}

					t.Row(func(r *sanur.TableRowBuilder) {
						r.Cell().Background(background).PaddingXY(8, 5).AlignMiddle().
							StyledText(f.area, sanur.TextStyle().Size(8.5).Bold().Color(ink))
						r.Cell().Background(background).PaddingXY(8, 5).AlignMiddle().
							StyledText(f.status, sanur.TextStyle().Size(8.5).Color(accent))
						r.Cell().Background(background).PaddingXY(8, 5).AlignMiddle().
							StyledText(f.note, sanur.TextStyle().Size(8.5).Color(muted))
					})
				}
			})

			c.Item().Tag(sanur.Heading2).StyledText("Links",
				sanur.TextStyle().Size(13).Bold().Color(ink))

			c.Item().Text("A link in the body is reachable from the structure, so a " +
				"reader can offer it by the words it sits on rather than by its address:")

			c.Item().Link("https://www.w3.org/TR/WCAG22/").
				StyledText("Web Content Accessibility Guidelines 2.2",
					sanur.TextStyle().Size(9.5).Color(accent).Underline())

			c.Item().Text("The link in the footer of every page is clickable too, and " +
				"deliberately absent from the structure: the footer is decoration.")

			limitations(c)
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}
	fmt.Printf("wrote %s\n", out)
}

// limitations sets out what tagging here does not cover, as ordinary tagged content —
// the honest place for it is in the document itself.
func limitations(c *sanur.ColumnBuilder) {
	c.Item().Background(panel).Padding(12).Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(7)
		col.Item().Tag(sanur.Heading2).StyledText("What this does not cover",
			sanur.TextStyle().Size(12).Bold().Color(ink))

		for _, note := range []string{
			"Tagging is not conformance. A document can carry a flawless structure and " +
				"still fail on contrast, or on colour used as the only way to tell two " +
				"things apart — neither of which a layout engine can judge.",
			"A paragraph split across a page break becomes two structure elements, so a " +
				"reader announces two paragraphs.",
			"Form fields, notes and highlights are absent, so a tagged document cannot " +
				"yet contain an accessible form.",
			"The suite verifies the structure against the object graph rather than with " +
				"a conformance checker: veraPDF is the validator, and it is not a Go tool.",
		} {
			col.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(6)
				// The bullet is decoration; the words are the content. Marking it so
				// keeps a reader from announcing a bullet before every item.
				r.ConstantItem(9).Decoration().StyledText("\u2022",
					sanur.TextStyle().Size(8.5).Color(muted))
				r.RelativeItem(1).StyledText(note,
					sanur.TextStyle().Size(8.5).Color(muted).LineHeight(1.4))
			})
		}
	})
}

// swatch builds a small PNG so the example needs no assets.
func swatch() []byte {
	const (
		width  = 160
		height = 40
	)

	colours := []color.RGBA{
		{0x4F, 0x46, 0xE5, 0xFF},
		{0x08, 0x91, 0xB2, 0xFF},
		{0x05, 0x96, 0x69, 0xFF},
		{0xD9, 0x77, 0x06, 0xFF},
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, colours[x*len(colours)/width])
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatalf("encoding the swatch: %v", err)
	}
	return buf.Bytes()
}
