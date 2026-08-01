// Command newsletter produces a multi-column document: a full-width headline over three
// columns of text that flows through them and then onto the next sheet.
//
// Columns are a property of the page rather than of an element, because a column is a
// region content flows into and the engine already knows how to fill a region and carry
// on in the next one. A paragraph breaking between columns and a paragraph breaking
// between sheets are the same event, handled the same way — which is why nothing in this
// file mentions where a break should go.
//
// The full-width opening is the one thing columns cannot express on their own, since
// content that spans them cannot also flow through them. Spanning is the region for it.
package main

import (
	"fmt"
	"log"
	"os"

	sanur "github.com/aaripurna/sanur-pdf"
)

var (
	ink      = sanur.Hex("#111827")
	muted    = sanur.Hex("#6B7280")
	accent   = sanur.Hex("#B91C1C")
	hairline = sanur.Hex("#D1D5DB")
	panel    = sanur.Hex("#F3F4F6")
)

// article is one piece in the issue: a subheading and its paragraphs.
type article struct {
	heading string
	body    []string
}

var issue = []article{
	{
		"Measure, then draw",
		[]string{
			"Everything rests on two methods. Measure reports what an element would do " +
				"with a given amount of space, without producing any output. Draw then " +
				"commits to that answer. The engine only ever calls Draw after a " +
				"Measure it is happy with, so an element may assume the space it is " +
				"given has already been approved.",
			"Measure answers with one of three things: everything fits, this much fits " +
				"and call me again, or nothing useful fits here. The middle answer is " +
				"the one that matters. It is what lets a paragraph split between two " +
				"sheets, and it is the same answer that lets one split between two " +
				"columns, with no element aware that either exists.",
		},
	},
	{
		"Regions, not pages",
		[]string{
			"A page definition is not a page. It is a description that produces as many " +
				"sheets as its content needs, redrawing its header and footer on each. " +
				"Add columns and each sheet becomes several regions rather than one: " +
				"the content fills the first, carries on into the second, and only " +
				"when the last is full does a new sheet begin.",
			"That ordering is what makes the result readable. Software reading the " +
				"document follows the structure tree, which is built as the ink is " +
				"laid down, so the reading order is the order the columns were filled " +
				"in. A validator will confirm it; so will the tests, which check that " +
				"no paragraph goes missing between the last column of one sheet and " +
				"the first of the next.",
		},
	},
	{
		"What a rule is for",
		[]string{
			"The hairline between these columns carries no meaning. It is there to keep " +
				"the eye from crossing the gap mid-sentence, and a document that " +
				"announced it between every two paragraphs would be worse than one " +
				"with no rule at all. It is marked as decoration for exactly that " +
				"reason, and stops at the depth the text reached rather than running " +
				"to the bottom of a half-empty final sheet.",
		},
	},
	{
		"The two rules of an element",
		[]string{
			"Space on the cross axis passes through in full. A column reports the whole " +
				"width it was offered rather than shrinking to its widest child, which " +
				"is why a background spans its parent instead of hugging its text. In a " +
				"column layout that is what makes a tinted panel fill the track it sits " +
				"in without being told how wide the track is.",
			"And Measure is repeatable and free of side effects. Containers re-measure " +
				"their children while drawing, to recover the sizes they were promised, " +
				"so an element that advanced its own progress during a measurement " +
				"would render the wrong thing. Progress is advanced in Draw and nowhere " +
				"else — which is exactly what lets a column be measured, discarded, and " +
				"measured again when a first pass turns out to have been speculative.",
		},
	},
	{
		"Counting the sheets",
		[]string{
			"A footer reading page one of six needs the six, and the six is not known " +
				"until the document has been laid out. So it is laid out twice: once to " +
				"count, discarding the output, and once to render. The counting pass " +
				"draws the whole document onto a canvas that throws everything away, " +
				"which is why a canvas with nothing to write to still has to accept " +
				"every call.",
			"A table of contents printing page numbers is the same problem one turn " +
				"harder, because printing the numbers changes the widths involved and " +
				"can move the very pages being named. That one is settled by laying out " +
				"repeatedly until the answer stops moving, with a cap and an error " +
				"rather than a loop that never ends.",
		},
	},
	{
		"What a font can reach",
		[]string{
			"The built-in Helvetica and Courier need no files and are addressed one byte " +
				"at a time through WinAnsi, which stops at Western Europe. Register a " +
				"TrueType or OpenType face and a subset of it is embedded as a " +
				"composite font, which reaches every glyph the face has: Latin " +
				"Extended, Greek, Cyrillic, and Hebrew, Arabic, Persian and Urdu " +
				"complete with bidirectional reordering and letter shaping.",
			"Subsetting is where the care goes. A glyph referenced by a composite one " +
				"has to come with it, transitively; an outline copied into the subset " +
				"must be identical to the outline it came from; and the metrics table " +
				"must be exactly the size its header claims. A subsetter that shifts an " +
				"outline by a single byte produces a font that loads, reports plausible " +
				"metrics and draws the wrong shapes — far harder to notice than one " +
				"that simply fails.",
		},
	},
	{
		"Ink, not light",
		[]string{
			"A colour for a screen and a colour for a press are different quantities, " +
				"and converting between them is a decision rather than arithmetic. So " +
				"both are written as themselves: red as red, and a four-plate black as " +
				"four plates. A rich black asked for in CMYK arrives at the press as " +
				"the mixture that was specified, not as a conversion of a conversion.",
		},
	},
	{
		"Read out loud",
		[]string{
			"A PDF says where ink goes and nothing else. A heading is text that happens " +
				"to be large; a table is lines that happen to form a grid. Tagging is " +
				"the parallel structure that carries the meaning, and most of it can be " +
				"inferred — text is a paragraph, a header row heads its columns, a rule " +
				"is decoration a reader should skip.",
			"Two things cannot be inferred and so are declared: which text is a heading " +
				"and at what level, and what a picture shows. Generation fails without " +
				"them rather than producing a document that passes for accessible. The " +
				"validator found six defects that a suite of hand-written structural " +
				"checks could not see, which is the argument for having both.",
		},
	},
	{
		"Where it stops",
		[]string{
			"Columns divide the page, so every sheet of a definition has the same " +
				"number of them. An element wider than a single column has nowhere to " +
				"go, and generation says so rather than clipping it or quietly " +
				"leaving it out. A picture that has to run across the page belongs in " +
				"the spanning region above, or in a page definition of its own.",
			"Nor are the columns balanced. Each is filled before the next is started, " +
				"which is what a newspaper does and what makes the flow predictable; " +
				"the last column of the last sheet is as short as the content leaves " +
				"it. Evening them out would mean knowing the total before placing any " +
				"of it, which is the one thing a streaming layout cannot do.",
		},
	},
}

// justified adds a paragraph set flush on both edges, which is what a narrow column
// wants: a ragged right edge next to a hairline reads as a mistake.
//
// Alignment belongs to the paragraph rather than to a run of text, which is why it is set
// on the rich-text builder and not in a style.
func justified(c *sanur.ColumnBuilder, paragraph string) {
	c.Item().RichText(func(t *sanur.TextBuilder) {
		t.Align(sanur.AlignJustify)
		t.Span(paragraph)
	})
}

func main() {
	out := "newsletter.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	doc := sanur.New().
		Title("The Sanur Review").
		Author("sanur").
		Creator("sanur/examples/newsletter")

	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).MarginXY(sanur.Mm(16), sanur.Mm(14))

		// Three columns with a hairline in each gap. The gap is set explicitly here
		// because 16 suits this measure; left alone it would be 18.
		p.Columns(3).ColumnSpacing(16).ColumnRule(0.5, hairline)

		p.DefaultTextStyle(sanur.TextStyle().Size(8.6).Color(ink).LineHeight(1.35))

		p.Footer().PaddingTop(8).Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("The Sanur Review",
				sanur.TextStyle().Size(7).Color(muted))
			r.ConstantItem(90).AlignRight().
				DefaultTextStyle(sanur.TextStyle().Size(7).Color(muted)).
				PageNumber("Page {page} of {total}")
		})

		// The masthead spans all three columns, and is drawn on the first sheet only.
		// A header would repeat it on every sheet, which is the difference between
		// furniture and content that happens to come first.
		p.Spanning().PaddingBottom(12).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(6)

			c.Item().Tag(sanur.Heading1).StyledText("The Sanur Review",
				sanur.TextStyle().Size(30).Bold().Color(ink))

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).StyledText("On laying out a document you cannot see",
					sanur.TextStyle().Size(10).Italic().Color(accent))
				r.AutoItem().AlignRight().StyledText("Issue one",
					sanur.TextStyle().Size(8).Color(muted))
			})

			c.Item().LineHorizontal(1.5, ink)
		})

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(9)

			// A panel inside a column, to show that a column is an ordinary region:
			// anything that fits in one can go in it.
			c.Item().Background(panel).Padding(9).Column(func(box *sanur.ColumnBuilder) {
				box.Spacing(5)
				box.Item().Tag(sanur.Heading2).StyledText("In this issue",
					sanur.TextStyle().Size(9.5).Bold().Color(ink))
				box.Item().List(func(l *sanur.ListBuilder) {
					l.Numbered().Spacing(4).Gutter(11).MarkerSpace(4)
					l.MarkerStyle(sanur.TextStyle().Size(8).Bold().Color(accent).
						LineHeight(1.3))
					for _, piece := range issue {
						l.Item().StyledText(piece.heading,
							sanur.TextStyle().Size(8).Color(muted).LineHeight(1.3))
					}
				})
			})

			for _, piece := range issue {
				c.Item().Tag(sanur.Heading2).StyledText(piece.heading,
					sanur.TextStyle().Size(11).Bold().Color(ink))

				for _, paragraph := range piece.body {
					justified(c, paragraph)
				}
			}

			c.Item().PaddingTop(3).LineHorizontal(0.5, hairline)
			c.Item().StyledText(
				"Set by sanur, in three columns of one page definition. No template "+
					"file, no cursor arithmetic, and no line of this issue knows which "+
					"column it landed in.",
				sanur.TextStyle().Size(7.5).Italic().Color(muted).LineHeight(1.35))
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}
	fmt.Printf("wrote %s\n", out)
}
