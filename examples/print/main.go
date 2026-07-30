// Command print demonstrates press-ready output: CMYK colour, tint ramps, crop
// marks and a registration bar.
//
// The point of the example is what does *not* happen to these colours. A CMYK
// colour is written to the file as CMYK, so the plates specified here are the
// plates the press lays down. Ghostscript's inkcov device will report ink on all
// four separations for this document, and the caption text on the black plate
// alone — which is the whole reason to say cmyk(0, 0, 0, 100) rather than #000000.
package main

import (
	"fmt"
	"log"
	"os"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/core"
)

// The inks. Named here rather than inline so a plate can be changed in one place,
// which is how a print job actually gets corrected.
var (
	// Single-plate black for text: one plate cannot misregister against itself, so
	// small type stays crisp. A four-plate black at 8pt fringes.
	textBlack = sanur.CMYK(0, 0, 0, 100)

	// Rich black for large solids, where a single plate looks washed out. The
	// under-colour keeps total ink under the 300% most presses accept.
	richBlack = sanur.CMYK(60, 40, 40, 100)

	cyan    = sanur.CMYK(100, 0, 0, 0)
	magenta = sanur.CMYK(0, 100, 0, 0)
	yellow  = sanur.CMYK(0, 0, 100, 0)

	// A duotone pair, the classic reason to specify CMYK by hand: two inks chosen
	// to separate cleanly rather than two colours picked on screen.
	deepTeal = sanur.CMYK(90, 30, 40, 10)
	warmSand = sanur.CMYK(0, 18, 45, 4)

	hairline = sanur.CMYK(0, 0, 0, 25)
	muted    = sanur.CMYK(0, 0, 0, 55)
)

// The sheet is built outwards from the trim size in two bands.
//
// Artwork runs past the trim line by bleed, so a guillotine that wanders by a
// point does not expose paper. Beyond that sits an empty band for the marks
// themselves, which have to be clear of the artwork to stay readable.
const (
	bleed    = 9  // 3mm, in points: how far artwork extends past trim
	markZone = 20 // empty band outside the bleed, where the marks live
	markLen  = 13 // how far each mark runs
	markGap  = 4  // clearance between the trim corner and the start of the mark
	markRule = 0.5

	// trimEdge is the distance from the sheet edge to the trim line, and so the
	// coordinate of the trim corner in every direction.
	trimEdge = bleed + markZone
)

func main() {
	out := "print.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	doc := sanur.New().
		Title("Press check").
		Author("sanur").
		Creator("sanur/examples/print").
		Subject("CMYK colour, tint ramps and crop marks")

	doc.Page(func(p *sanur.Page) {
		// The sheet is the trim size plus bleed on every edge, and the page carries
		// no margin of its own: the furniture that has to reach the paper edge is
		// positioned against the full sheet, and the content insets itself.
		p.Size(core.Size{
			Width:  sanur.A4.Width + 2*trimEdge,
			Height: sanur.A4.Height + 2*trimEdge,
		})
		p.Margin(0)
		p.DefaultTextStyle(sanur.TextStyle().Size(9).Color(textBlack))

		// Extend, because a layer stack is only as big as its content and the bleed
		// and the marks have to reach the paper edge regardless of how much the
		// column happens to fill.
		p.Content().Extend().Layers(func(l *sanur.LayersBuilder) {
			// Beneath everything: the bleed itself. A background that stops at the
			// trim line leaves a white hairline wherever the guillotine wanders.
			l.Below().Padding(markZone).Extend().
				Background(sanur.CMYK(0, 3, 8, 0)).Empty()

			l.Content().Padding(trimEdge).Padding(sanur.Mm(15)).Column(body)

			// Above everything: the marks. They are drawn last so no panel can cover
			// them, and they live outside the trim area so they vanish when the sheet
			// is cut.
			l.Above().Element(&cropMarks{})
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}
	fmt.Printf("wrote %s\n", out)
}

func body(c *sanur.ColumnBuilder) {
	c.Spacing(18)

	c.Item().Row(func(r *sanur.RowBuilder) {
		r.RelativeItem(1).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(3)
			c.Item().StyledText("Press check",
				sanur.TextStyle().Size(20).Bold().Color(textBlack))
			c.Item().StyledText("Every colour on this sheet is specified as plates.",
				sanur.TextStyle().Size(9).Color(muted))
		})
		r.ConstantItem(120).AlignRight().AlignMiddle().
			StyledText("A4 + 3mm bleed", sanur.TextStyle().Size(8).Mono().Color(muted))
	})

	c.Item().LineHorizontal(markRule, hairline)

	// --- the four process inks, solid ----------------------------------------

	c.Item().StyledText("Process inks", heading())
	c.Item().Row(func(r *sanur.RowBuilder) {
		r.Spacing(10)
		for _, ink := range []struct {
			name   string
			colour core.Color
		}{
			{"Cyan", cyan},
			{"Magenta", magenta},
			{"Yellow", yellow},
			{"Black", textBlack},
		} {
			r.RelativeItem(1).Column(func(c *sanur.ColumnBuilder) {
				c.Spacing(4)
				c.Item().Height(52).Background(ink.colour).Empty()
				c.Item().StyledText(ink.name, label())
				c.Item().StyledText(ink.colour.String(), mono())
			})
		}
	})

	// --- tint ramps -----------------------------------------------------------

	// A ramp is the quickest way to see whether a press is holding its dot: the
	// steps should read as even, and the light end should not disappear.
	c.Item().StyledText("Tint ramps", heading())
	c.Item().Column(func(c *sanur.ColumnBuilder) {
		c.Spacing(6)
		for _, ramp := range []struct {
			name string
			ink  func(percent float64) core.Color
		}{
			{"Cyan", func(v float64) core.Color { return sanur.CMYK(v, 0, 0, 0) }},
			{"Magenta", func(v float64) core.Color { return sanur.CMYK(0, v, 0, 0) }},
			{"Yellow", func(v float64) core.Color { return sanur.CMYK(0, 0, v, 0) }},
			{"Black", func(v float64) core.Color { return sanur.CMYK(0, 0, 0, v) }},
		} {
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.ConstantItem(56).AlignMiddle().StyledText(ramp.name, label())
				for _, percent := range []float64{5, 10, 20, 40, 60, 80, 100} {
					r.RelativeItem(1).Height(20).Background(ramp.ink(percent)).Empty()
				}
			})
		}
	})

	// --- two blacks -----------------------------------------------------------

	c.Item().StyledText("Two blacks", heading())
	c.Item().Row(func(r *sanur.RowBuilder) {
		r.Spacing(10)
		r.RelativeItem(1).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(4)
			c.Item().Height(64).Background(textBlack).Padding(10).
				StyledText("100% K", sanur.TextStyle().Size(11).Bold().Color(sanur.CMYK(0, 0, 0, 0)))
			c.Item().StyledText("Text and small type. One plate, so nothing can misregister.", note())
		})
		r.RelativeItem(1).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(4)
			c.Item().Height(64).Background(richBlack).Padding(10).
				StyledText("Rich black", sanur.TextStyle().Size(11).Bold().Color(sanur.CMYK(0, 0, 0, 0)))
			c.Item().StyledText("Large solids. 240% total ink, within what a sheet-fed press takes.", note())
		})
	})

	// Both of these are #000000 in RGB, which is exactly why the space is carried
	// through to the file rather than normalised away.
	c.Item().StyledText(
		"Both panels above are #000000 once converted to RGB. Only the separations tell them apart.",
		note())

	// --- duotone --------------------------------------------------------------

	c.Item().StyledText("Duotone pair", heading())
	c.Item().Row(func(r *sanur.RowBuilder) {
		r.Spacing(0)
		for _, step := range []float64{100, 80, 60, 40, 20} {
			r.RelativeItem(1).Height(34).Background(tint(deepTeal, step)).Empty()
		}
		for _, step := range []float64{20, 40, 60, 80, 100} {
			r.RelativeItem(1).Height(34).Background(tint(warmSand, step)).Empty()
		}
	})

	// --- mixing spaces --------------------------------------------------------

	// Mixing is legitimate and common: the brand ink is specified for the press, the
	// chart came from somewhere that only speaks RGB. PDF selects a colour space per
	// drawing operation, so both survive on the same page unconverted.
	c.Item().StyledText("Mixed spaces on one page", heading())
	c.Item().Row(func(r *sanur.RowBuilder) {
		r.Spacing(10)
		r.RelativeItem(1).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(4)
			c.Item().Height(40).Background(deepTeal).Empty()
			c.Item().StyledText("cmyk(90, 30, 40, 10) — emitted as k", mono())
		})
		r.RelativeItem(1).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(4)
			c.Item().Height(40).Background(sanur.Hex("#0F766E")).Empty()
			c.Item().StyledText("#0F766E — emitted as rg", mono())
		})
	})
}

// tint returns a percentage of a colour's plates, which is how a tint is specified
// for print — not by mixing it with white, which is what lightening an RGB colour
// does and which no press can reproduce.
func tint(c core.Color, percent float64) core.Color {
	cy, m, y, k := c.CMYKComponents()
	scale := percent / 100

	return sanur.CMYK(cy*100*scale, m*100*scale, y*100*scale, k*100*scale)
}

func heading() *sanur.StyleBuilder {
	return sanur.TextStyle().Size(11).Bold().Color(textBlack)
}

func label() *sanur.StyleBuilder {
	return sanur.TextStyle().Size(8.5).Color(textBlack)
}

func mono() *sanur.StyleBuilder {
	return sanur.TextStyle().Size(7.5).Mono().Color(muted)
}

func note() *sanur.StyleBuilder {
	return sanur.TextStyle().Size(7.5).Color(muted)
}

// cropMarks draws trim marks in the four corners of the sheet.
//
// It is a custom core.Element rather than anything built from the layout
// vocabulary, because marks are positioned against the sheet in absolute terms:
// they belong to the paper, not to the content. Fifteen lines is the whole
// interface — Measure to claim the space it was offered, Draw to put ink down.
type cropMarks struct{}

func (m *cropMarks) Measure(available core.Size) core.SpacePlan {
	// The marks fill whatever they are given and never paginate, so they are always
	// a full render of the box on offer. Nothing here depends on the content, which
	// is why they can sit in an overlay drawn after everything else.
	return core.FullRender(available)
}

func (m *cropMarks) Draw(canvas core.Canvas, available core.Size) {
	w, h := available.Width, available.Height

	// Registration colour: all four plates at 100%, so a plate out of alignment
	// shows up on the mark itself rather than only in the artwork.
	ink := sanur.Registration

	for _, corner := range []struct{ x, y, dx, dy float64 }{
		{trimEdge, trimEdge, -1, -1},       // top left
		{w - trimEdge, trimEdge, 1, -1},    // top right
		{trimEdge, h - trimEdge, -1, 1},    // bottom left
		{w - trimEdge, h - trimEdge, 1, 1}, // bottom right
	} {
		// Each corner gets two marks — one horizontal, one vertical — running
		// outwards from the trim point, with a gap so they do not cross inside the
		// finished area.
		canvas.DrawLine(
			core.Position{X: corner.x + corner.dx*markGap, Y: corner.y},
			core.Position{X: corner.x + corner.dx*(markGap+markLen), Y: corner.y},
			ink, markRule)

		canvas.DrawLine(
			core.Position{X: corner.x, Y: corner.y + corner.dy*markGap},
			core.Position{X: corner.x, Y: corner.y + corner.dy*(markGap+markLen)},
			ink, markRule)
	}
}

var _ core.Element = (*cropMarks)(nil)
