// Command charts demonstrates the chart package.
//
// Every chart here is a core.Element, so it goes wherever any other element goes:
// in a row beside a table, inside a bordered panel, in a column that paginates.
// The one thing a chart needs from its caller is a height, because a plot has no
// natural size of its own.
package main

import (
	"fmt"
	"log"
	"os"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/chart"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
)

var (
	ink      = sanur.Hex("#1A1D29")
	muted    = sanur.Hex("#6B7280")
	hairline = sanur.Hex("#E5E7EB")
)

func main() {
	out := "charts.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	doc := sanur.New().
		Title("Charts").
		Author("sanur").
		Creator("sanur/examples/charts")

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(sanur.Mm(16))
		p.DefaultTextStyle(sanur.TextStyle().Size(9).Color(ink))

		p.Header().PaddingBottom(14).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).StyledText("Charts", sanur.TextStyle().Size(11).Bold())
				r.AutoItem().StyledText("package chart",
					sanur.TextStyle().Mono().Size(8.5).Color(muted))
			})
			c.Item().LineHorizontal(1, hairline)
		})

		p.Footer().PaddingTop(10).Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("sanur/examples/charts",
				sanur.TextStyle().Size(7.5).Color(muted))
			r.ConstantItem(100).AlignRight().
				DefaultTextStyle(sanur.TextStyle().Size(7.5).Color(muted)).
				PageNumber("Page {page} of {total}")
		})
	})

	// --- Sheet one: line, area, pie --------------------------------------
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(18)

			c.Item().Element(heading("Line"))
			c.Item().Element(note(
				"Two series, points spread edge to edge. Axis ticks land on round " +
					"numbers, and the left gutter is measured from the widest label " +
					"rather than fixed, so a jump to seven figures cannot overlap the plot."))

			c.Item().Height(175).Element(&chart.Line{
				Categories: months(),
				Series: []chart.Series{
					{Name: "Revenue", Values: []float64{31, 34, 32, 39, 41, 40, 45, 48}},
					{Name: "Costs", Values: []float64{22, 23, 24, 24, 26, 27, 28, 29}},
				},
			})

			c.Item().Element(heading("Area"))
			c.Item().Element(note(
				"The same element with Area set. A single series is the intended use; " +
					"several translucent fills overlapping quickly stop being readable."))

			c.Item().Height(150).Element(&chart.Line{
				Categories: months(),
				Series: []chart.Series{
					{Name: "Active accounts", Values: []float64{980, 1010, 1060, 1090, 1140, 1180, 1240, 1284}},
				},
				Area: true,
			})

			c.Item().Element(heading("Pie and donut"))

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(14)

				r.RelativeItem(1).Height(150).Element(&chart.Pie{
					Slices: regions(),
				})

				r.RelativeItem(1).Height(150).Element(&chart.Pie{
					Slices:      regions(),
					InnerRadius: 32,
					CentreLabel: "4.82M",
					CentreNote:  "total",
				})
			})
		})
	})

	// --- Sheet two: bars -------------------------------------------------
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(18)

			c.Item().Element(heading("Columns"))
			c.Item().Element(note(
				"One series, rounded tops. Bars are inset half a band from the plot " +
					"edges, because a bar is a bucket rather than a reading at an instant."))

			c.Item().Height(165).Element(&chart.Bar{
				Categories:   []string{"SE Asia", "Oceania", "S Asia", "E Asia", "Other"},
				Series:       []chart.Series{{Values: []float64{1940, 1120, 830, 610, 320}}},
				CornerRadius: 3,
			})

			c.Item().Element(heading("Grouped columns"))
			c.Item().Element(note(
				"Several series share each band. The legend moves to the right here, " +
					"which suits long names better than a strip does."))

			c.Item().Height(175).Element(&chart.Bar{
				Categories: []string{"Q1", "Q2", "Q3", "Q4"},
				Series: []chart.Series{
					{Name: "Plan", Values: []float64{820, 910, 980, 1100}},
					{Name: "Actual", Values: []float64{790, 965, 1020, 1045}},
					{Name: "Prior year", Values: []float64{700, 740, 815, 880}},
				},
				CornerRadius: 2,
				Style:        chart.Style{Legend: chart.LegendRight},
			})

			c.Item().Element(heading("Horizontal bars"))
			c.Item().Element(note(
				"Long category names read better sideways than rotated. The gutter " +
					"grows to fit them automatically."))

			c.Item().Height(150).Element(&chart.Bar{
				Categories: []string{
					"South-East Asia", "Oceania", "South Asia", "East Asia", "Rest of world",
				},
				Series:       []chart.Series{{Values: []float64{1940, 1120, 830, 610, 320}}},
				Horizontal:   true,
				CornerRadius: 2,
			})
		})
	})

	// --- Sheet three: negative values ------------------------------------
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(16)

			c.Item().Element(heading("Negative values"))
			c.Item().Element(note(
				"Bars and areas always include zero in their axis, so a series that " +
					"crosses it gets a real baseline to be measured against. The axis " +
					"line is drawn at zero rather than at the bottom of the plot."))

			crossing := []float64{80, -45, 30, -60, 95, -20}
			months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"}

			c.Item().Element(subheading("Line and area crossing zero"))
			c.Item().Element(note(
				"An area fills between the series and zero, so the lobes above and " +
					"below the axis are shaded on their own sides."))

			c.Item().Height(135).Element(&chart.Line{
				Categories: months,
				Series:     []chart.Series{{Name: "Net flow", Values: crossing}},
				Area:       true,
			})

			c.Item().Element(subheading("Columns crossing zero"))
			c.Item().Element(note(
				"Negative bars hang below the axis. Their labels go underneath, and " +
					"the plot reserves room so they cannot land on the category names."))

			c.Item().Height(150).Element(&chart.Bar{
				Categories:   months,
				Series:       []chart.Series{{Name: "Net flow", Values: crossing}},
				CornerRadius: 2,
			})

			c.Item().Element(subheading("All negative, and horizontal"))

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(14)

				r.RelativeItem(1).Height(140).Element(&chart.Bar{
					Categories: []string{"Q1", "Q2", "Q3", "Q4"},
					Series: []chart.Series{
						{Values: []float64{-10, -25, -40, -15}},
					},
					CornerRadius: 2,
					Style:        chart.Style{Legend: chart.LegendNone},
				})

				r.RelativeItem(1).Height(140).Element(&chart.Bar{
					Categories: []string{"Q1", "Q2", "Q3", "Q4"},
					Series: []chart.Series{
						{Values: []float64{80, -45, 30, -60}},
					},
					Horizontal:   true,
					CornerRadius: 2,
					Style:        chart.Style{Legend: chart.LegendNone},
				})
			})

			c.Item().Background(sanur.Hex("#FFF8E1")).BorderLeft(3, sanur.Hex("#D97706")).
				Padding(11).RichText(func(tb *sanur.TextBuilder) {
				tb.StyledSpan("Pies are the exception. ",
					sanur.TextStyle().Size(8.5).Bold())
				tb.StyledSpan("A wedge cannot sweep backwards and still tile a circle, "+
					"so a negative slice has no representation. Rather than dropping it — "+
					"which would rescale the rest to 100% and quietly lose data — a pie "+
					"reports the slice and generation fails with an error naming it.",
					sanur.TextStyle().Size(8.5).Color(muted))
			})
		})
	})

	// --- Sheet four: styling ---------------------------------------------
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(18)

			c.Item().Element(heading("Styling"))
			c.Item().Element(note(
				"A zero Style resolves to the defaults, so a chart needs no " +
					"configuration. Fields set on it override one at a time — here the " +
					"palette, the gridlines and the tick count."))

			c.Item().Height(165).Element(&chart.Line{
				Categories: months(),
				Series: []chart.Series{
					{Name: "Latency p50", Values: []float64{120, 118, 125, 119, 112, 108, 105, 102}},
					{Name: "Latency p99", Values: []float64{410, 395, 440, 420, 380, 360, 344, 330}},
				},
				Style: chart.Style{
					Palette:   []core.Color{sanur.Hex("#0F766E"), sanur.Hex("#B45309")},
					GridDash:  []float64{1, 3},
					TickCount: 4,
					Legend:    chart.LegendBottom,
					Format:    func(v float64) string { return fmt.Sprintf("%.0fms", v) },
				},
				HideMarkers: true,
				Width:       1.4,
			})

			c.Item().Element(heading("Inside other layout"))
			c.Item().Element(note(
				"A chart is an element, so it composes: here two sit in bordered " +
					"panels within a row."))

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(12)

				r.RelativeItem(1).Border(1, hairline).Padding(10).
					Column(func(inner *sanur.ColumnBuilder) {
						inner.Spacing(6)
						inner.Item().StyledText("Conversion",
							sanur.TextStyle().Size(9).Bold())
						inner.Item().Height(90).Element(&chart.Line{
							Categories: []string{"W1", "W2", "W3", "W4", "W5"},
							Series:     []chart.Series{{Values: []float64{3.1, 3.4, 3.2, 3.9, 4.2}}},
							Area:       true,
							Style:      chart.Style{Legend: chart.LegendNone, TickCount: 3},
						})
					})

				r.RelativeItem(1).Border(1, hairline).Padding(10).
					Column(func(inner *sanur.ColumnBuilder) {
						inner.Spacing(6)
						inner.Item().StyledText("Channel mix",
							sanur.TextStyle().Size(9).Bold())
						inner.Item().Height(90).Element(&chart.Pie{
							Slices: []chart.Slice{
								{Name: "Direct", Value: 48},
								{Name: "Partner", Value: 31},
								{Name: "Organic", Value: 21},
							},
							InnerRadius: 18,
						})
					})
			})
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("generating charts: %v", err)
	}
	fmt.Printf("wrote %s\n", out)
}

func months() []string {
	return []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug"}
}

func regions() []chart.Slice {
	return []chart.Slice{
		{Name: "South-East Asia", Value: 1940},
		{Name: "Oceania", Value: 1120},
		{Name: "South Asia", Value: 830},
		{Name: "East Asia", Value: 610},
		{Name: "Rest of world", Value: 320},
	}
}

// heading is a section title over a rule.
func heading(title string) core.Element {
	col := &elements.Column{Spacing: 5}
	col.Add(elements.NewText(title, sanur.TextStyle().Size(12).Bold().Color(ink).Build()))
	col.Add(&elements.Line{Width: 0.75, Color: hairline})
	return col
}

// note is explanatory copy under a heading.
func note(text string) core.Element {
	return elements.NewText(text, sanur.TextStyle().Size(8.5).Color(muted).Build())
}

// subheading is a smaller title used within a section.
func subheading(title string) core.Element {
	return elements.NewText(title, sanur.TextStyle().Size(9.5).Bold().Color(ink).Build())
}
