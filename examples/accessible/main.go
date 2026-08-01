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
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/render"
)

// family is the registered font, resolved in main and read by the section builders. Every
// line of text uses the same one, so there is nothing to decide per section.
var family sanur.Family

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
	{"Lists", "Pass", "Markers are labels, not content"},
	{"Furniture", "Pass", "Running header and footer are artifacts"},
}

func main() {
	out := "accessible.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	// A conforming tagged document has to embed every font it uses, so the built-in
	// Helvetica will not do: the file only names it and the reader supplies the outlines.
	// Generation refuses rather than producing a document that is tagged and not
	// conformant, which is why this example needs a font file where the others do not.
	loaded, err := loadFamily()
	if err != nil {
		log.Fatal(err)
	}
	family = loaded

	// A swatch stands in for any picture. It has to be described: a figure with nothing to
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
		p.DefaultTextStyle(sanur.TextStyle().Family(family).Size(9.5).Color(ink))

		// Running furniture is decoration whatever it contains, and sanur marks it so
		// automatically. A header repeated on forty sheets is not forty paragraphs to
		// announce, and "Page 12 of 40" read out between every two paragraphs is worse
		// than silence.
		p.Footer().PaddingTop(10).Row(func(r *sanur.RowBuilder) {
			// Plain furniture text: decoration, and skipped.
			r.AutoItem().StyledText("sanur",
				sanur.TextStyle().Family(family).Size(7.5).Color(muted))
			// A link in the same footer: not decoration, because every link annotation
			// has to be reachable from the structure.
			r.RelativeItem(1).PaddingLeft(6).Link("https://verapdf.org/").
				StyledText("validated with veraPDF",
					sanur.TextStyle().Family(family).Size(7.5).Color(accent).Underline())
			r.ConstantItem(110).AlignRight().
				DefaultTextStyle(sanur.TextStyle().Family(family).Size(7.5).Color(muted)).
				PageNumber("Page {page} of {total}")
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(12)

			// Declared, not inferred. The level is the part that matters: an outline
			// that is confidently wrong is worse for a reader than none at all.
			c.Item().Tag(sanur.Heading1).StyledText("Accessibility conformance",
				sanur.TextStyle().Family(family).Size(21).Bold().Color(ink))

			c.Item().Text("This document carries its own structure. Everything below is " +
				"reachable by a screen reader in reading order, and the parts that mean " +
				"nothing are marked so that a reader skips them.")

			c.Item().Tag(sanur.Heading2).StyledText("What is declared",
				sanur.TextStyle().Family(family).Size(13).Bold().Color(ink))

			c.Item().Text("Two things cannot be worked out from a layout, so they are " +
				"stated. The first is a heading and its level: heading text is simply " +
				"text that happens to be large, and guessing would produce an outline " +
				"that is wrong rather than absent. The second is what a picture shows.")

			// A rule is decoration, and sanur marks rules as artifacts without being
			// asked — announcing one between every paragraph would make this document
			// worse than an untagged one.
			c.Item().LineHorizontal(1, hairline)

			c.Item().Tag(sanur.Heading2).StyledText("A described figure",
				sanur.TextStyle().Family(family).Size(13).Bold().Color(ink))

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
					sanur.TextStyle().Family(family).Size(9).Color(muted).LineHeight(1.4))
			})

			c.Item().Tag(sanur.Heading2).StyledText("A table with real headers",
				sanur.TextStyle().Family(family).Size(13).Bold().Color(ink))

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
							sanur.TextStyle().Family(family).Size(7.5).Bold().Color(sanur.White))
					}
				})

				for i, f := range findings {
					background := sanur.White
					if i%2 == 1 {
						background = panel
					}

					t.Row(func(r *sanur.TableRowBuilder) {
						r.Cell().Background(background).PaddingXY(8, 5).AlignMiddle().
							StyledText(f.area, sanur.TextStyle().Family(family).Size(8.5).Bold().Color(ink))
						r.Cell().Background(background).PaddingXY(8, 5).AlignMiddle().
							StyledText(f.status, sanur.TextStyle().Family(family).Size(8.5).Color(accent))
						r.Cell().Background(background).PaddingXY(8, 5).AlignMiddle().
							StyledText(f.note, sanur.TextStyle().Family(family).Size(8.5).Color(muted))
					})
				}
			})

		})
	})

	// A second page, so the closing sections are a deliberate part of the document rather
	// than a spill off the first.
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(12)

			c.Item().Tag(sanur.Heading2).StyledText("Lists",
				sanur.TextStyle().Family(family).Size(13).Bold().Color(ink))

			c.Item().Text("A list's markers are drawn as ordinary text, so nothing in a " +
				"file otherwise says whether \u201c1.\u201d begins an item or a sentence. " +
				"Building it as a list records three things a reader needs:")

			c.Item().PaddingLeft(4).List(func(l *sanur.ListBuilder) {
				l.Numbered().Spacing(6)
				l.MarkerStyle(sanur.TextStyle().Family(family).Size(9).Bold().Color(accent))

				l.Item().Text("That these items belong to one list, so a reader can say " +
					"how long it is and offer to skip it.")
				l.Item().Text("That each marker is a label rather than content, so the " +
					"digits are not read out as part of the sentence.")
				l.Item().Column(func(sub *sanur.ColumnBuilder) {
					sub.Spacing(6)
					sub.Item().Text("That a sublist belongs to the item introducing it:")
					sub.Item().List(func(inner *sanur.ListBuilder) {
						inner.Lettered().Spacing(4).Gutter(12)
						inner.MarkerStyle(
							sanur.TextStyle().Family(family).Size(8.5).Color(muted))
						inner.Item().StyledText("nested one level,",
							sanur.TextStyle().Family(family).Size(9).Color(muted))
						inner.Item().StyledText("and lettered rather than numbered.",
							sanur.TextStyle().Family(family).Size(9).Color(muted))
					})
				})
			})

			c.Item().Tag(sanur.Heading2).StyledText("Links",
				sanur.TextStyle().Family(family).Size(13).Bold().Color(ink))

			c.Item().Text("A link in the body is reachable from the structure, so a " +
				"reader can offer it by the words it sits on rather than by its address:")

			c.Item().Link("https://www.w3.org/TR/WCAG22/").
				StyledText("Web Content Accessibility Guidelines 2.2",
					sanur.TextStyle().Family(family).Size(9.5).Color(accent).Underline())

			c.Item().Text("The footer of every page carries one too. Running furniture is " +
				"decoration and stays out of the structure, but a link is not exempt: a " +
				"conforming document requires every link annotation to sit inside a Link " +
				"element, so the link escapes the artifact around it while the plain " +
				"footer text beside it does not.")

			limitations(c)
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}
	fmt.Printf("wrote %s\n", out)
}

// candidates are the faces commonly installed on a development machine.
var candidates = []struct{ regular, bold string }{
	{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
	},
	{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
	},
	{
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
	},
	{"C:/Windows/Fonts/arial.ttf", "C:/Windows/Fonts/arialbd.ttf"},
}

// loadFamily finds and registers a font, since a conforming tagged document has to embed
// one. No font is shipped with sanur: system fonts are licensed, and vendoring one is a
// decision each project makes for itself.
func loadFamily() (sanur.Family, error) {
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate.regular); err != nil {
			continue
		}

		regular, err := fonts.LoadTrueTypeFile("", candidate.regular)
		if err != nil {
			return sanur.Family{}, err
		}

		var bold core.Font
		if _, err := os.Stat(candidate.bold); err == nil {
			bold, _ = fonts.LoadTrueTypeFile("", candidate.bold)
		}
		return sanur.NewFamily(regular, bold, nil, nil), nil
	}

	return sanur.Family{}, fmt.Errorf(
		"no system font found to embed; a tagged document cannot use the built-in "+
			"faces, so install one or edit the candidate list — for example %s",
		candidates[0].regular)
}

// limitations sets out what tagging here does not cover, as ordinary tagged content —
// the honest place for it is in the document itself.
func limitations(c *sanur.ColumnBuilder) {
	c.Item().Background(panel).Padding(12).Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(7)
		col.Item().Tag(sanur.Heading2).StyledText("What this does not cover",
			sanur.TextStyle().Family(family).Size(12).Bold().Color(ink))

		// A list, rather than a column of rows with a bullet in the left one. The
		// difference is entirely in the structure: this way a reader announces "list of
		// four items, item one" and can skip the rest, where the hand-built version
		// would be four unrelated paragraphs with a decorative dot beside each.
		col.Item().List(func(l *sanur.ListBuilder) {
			l.Bulleted().Spacing(7).Gutter(9).MarkerSpace(5)
			// The same line height as the bodies below: a marker is text, so its line
			// height is what puts its baseline on the item's first line rather than
			// above it.
			l.MarkerStyle(sanur.TextStyle().Family(family).Size(8.5).Color(muted).
				LineHeight(1.4))

			for _, note := range []string{
				"Tagging is not conformance. A document can carry a flawless structure " +
					"and still fail on contrast, or on colour used as the only way to " +
					"tell two things apart — neither of which a layout engine can judge.",
				"A paragraph split across a page break becomes two structure elements, " +
					"so a reader announces two paragraphs.",
				"Form fields, notes and highlights are absent, so a tagged document " +
					"cannot yet contain an accessible form.",
				"Everything above is checked two ways: sanur's own tests read the " +
					"structure back out of the object graph, and veraPDF validates the " +
					"file against PDF/UA-1. The second found six defects the first " +
					"could not see.",
			} {
				l.Item().StyledText(note,
					sanur.TextStyle().Family(family).Size(8.5).Color(muted).LineHeight(1.4))
			}
		})
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
