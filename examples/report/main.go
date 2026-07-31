// Command report builds a multi-section report that exercises the harder parts
// of the layout engine: a dashboard of stat tiles, charts drawn from primitives,
// a two-column article, a sidebar, nested tables, and a landscape sheet mixed in
// with portrait ones.
//
// The recurring theme is that sanur has no chart element, no stat-tile element
// and no sidebar element — and does not need them. Each is a small composition of
// rows, columns, backgrounds and lines, written once as a Go function and reused.
// A custom visual only needs a full core.Element implementation when it has to
// draw something the primitives cannot express, which the sparkline at the bottom
// of this file demonstrates.
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
)

// Brand colours, kept in one place so the composition helpers below agree.
var (
	ink      = sanur.Hex("#1A1D29")
	muted    = sanur.Hex("#6B7280")
	hairline = sanur.Hex("#E5E7EB")
	accent   = sanur.Hex("#4F46E5")
	positive = sanur.Hex("#059669")
	negative = sanur.Hex("#DC2626")
	surface  = sanur.Hex("#F9FAFB")
)

type metric struct {
	Label  string
	Value  string
	Change float64
	Series []float64
}

type region struct {
	Name     string
	Revenue  float64
	Share    float64
	Accounts int
	Growth   float64
}

func main() {
	out := "report.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	metrics := []metric{
		{"Revenue", "$4.82M", 12.4, []float64{3.1, 3.4, 3.2, 3.9, 4.1, 4.0, 4.5, 4.82}},
		{"Active accounts", "1,284", 8.1, []float64{980, 1010, 1060, 1090, 1140, 1180, 1240, 1284}},
		{"Gross margin", "61.3%", -1.8, []float64{63, 63.5, 62.8, 62.1, 62.4, 61.9, 61.5, 61.3}},
		{"Churn", "2.1%", -0.4, []float64{2.8, 2.7, 2.6, 2.5, 2.4, 2.3, 2.2, 2.1}},
	}

	regions := []region{
		{"South-East Asia", 1_940_000, 40.2, 486, 18.3},
		{"Oceania", 1_120_000, 23.2, 301, 9.7},
		{"South Asia", 830_000, 17.2, 224, 22.1},
		{"East Asia", 610_000, 12.7, 172, 4.4},
		{"Rest of world", 320_000, 6.7, 101, -2.8},
	}

	doc := sanur.New().
		Title("Quarterly Business Review").
		Author("Nuanu").
		Subject("Q2 2026").
		Creator("sanur/examples/report")

	// Geometry and furniture shared by every sheet in the document. EveryPage runs
	// this before each definition's own build, so the three definitions below only
	// describe what makes them different.
	//
	// It takes a function rather than a prepared element tree on purpose: elements
	// carry pagination state, so a single shared header instance would arrive at
	// the second definition believing it had already been drawn.
	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).MarginEach(sanur.Mm(16), sanur.Mm(15), sanur.Mm(14), sanur.Mm(15))
		p.DefaultTextStyle(sanur.TextStyle().Size(9.5).Color(ink))

		p.Header().Element(reportHeader("Quarterly Business Review", "Q2 2026 · Confidential"))
		p.Footer().Element(reportFooter())
	})

	// --- Sheet zero: title and contents ----------------------------------
	//
	// The contents entries link to destinations registered by the Bookmark calls
	// on the section headings below, which also populate the reader's outline
	// panel. A link may point forwards: nothing is resolved until every page has
	// been drawn.
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(26)

			c.Item().PaddingTop(60).Column(func(inner *sanur.ColumnBuilder) {
				inner.Spacing(6)
				inner.Item().StyledText("Quarterly Business Review",
					sanur.TextStyle().Size(26).Bold().Color(ink))
				inner.Item().StyledText("Q2 2026",
					sanur.TextStyle().Size(13).Color(muted))
			})

			c.Item().LineHorizontal(1, hairline)

			c.Item().Column(func(inner *sanur.ColumnBuilder) {
				inner.Spacing(3)
				inner.Item().PaddingBottom(6).StyledText("CONTENTS",
					sanur.TextStyle().Size(8).Bold().Color(muted).LetterSpacing(0.9))

				for _, entry := range []struct{ title, dest string }{
					{"Performance", "bookmark:Performance"},
					{"Operating review", "bookmark:Operating review"},
					{"Appendix A \u00b7 Monthly detail", "bookmark:Appendix A"},
				} {
					entry := entry
					// The whole row is the click target, not just the words, which is
					// what makes a contents list comfortable to use.
					inner.Item().LinkTo(entry.dest).PaddingXY(0, 5).
						BorderBottom(0.5, hairline).
						Row(func(r *sanur.RowBuilder) {
							r.RelativeItem(1).StyledText(entry.title,
								sanur.TextStyle().Size(11).Color(accent))
							// The page the section landed on. It is not known while
							// this row is being laid out — the section has not been
							// placed yet — so generation resolves it in an extra pass
							// and repeats until the answers stop moving.
							r.ConstantItem(26).AlignRight().AlignMiddle().
								DefaultTextStyle(sanur.TextStyle().Size(11).Color(accent)).
								PageRef(entry.dest)
						})
				}
			})

			c.Item().StyledText(
				"Each entry is clickable, prints the page its section landed on, and "+
					"appears in the reader's outline panel. The page numbers come from "+
					"the layout rather than from counting by hand.",
				sanur.TextStyle().Size(8).Color(muted))
		})
	})

	// --- Sheet one: dashboard --------------------------------------------
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(20)

			// Bookmark registers both an outline entry and a destination, so the
			// contents list on the title page can aim at the same spot.
			c.Item().Bookmark("Performance").Element(
				sectionTitle("Performance"))

			// A four-up tile row. Each tile is a relative item so the row divides
			// the width evenly however many tiles there are.
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(10)
				for _, m := range metrics {
					r.RelativeItem(1).Element(statTile(m))
				}
			})

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(18)

				// The chart takes two thirds, the commentary one third. Both are
				// relative, so the split holds at any page size.
				r.RelativeItem(2).Element(panel("Revenue by region",
					barChart(regions, 150)))

				r.RelativeItem(1).Element(panel("Commentary",
					commentary()))
			})

			c.Item().Element(panel("Regional detail", regionTable(regions)))
		})
	})

	// --- Sheet two: two-column article -----------------------------------
	doc.Page(func(p *sanur.Page) {
		// Only the differences from the template: looser leading for prose, and a
		// header naming this section.
		p.DefaultTextStyle(sanur.TextStyle().Size(9.5).Color(ink).LineHeight(1.35))
		p.Header().Element(reportHeader("Operating review", "Q2 2026 · Confidential"))

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(16)

			c.Item().Bookmark("Operating review").Element(
				sectionTitle("Operating review"))

			// A pull quote spanning the full width before the columns begin.
			c.Item().Background(surface).BorderLeft(3, accent).PaddingXY(16, 14).
				RichText(func(tb *sanur.TextBuilder) {
					tb.StyledSpan("“Margin compression is entirely mix-driven. ",
						sanur.TextStyle().Size(12).Italic().Color(ink))
					tb.StyledSpan("Unit economics in every established market improved "+
						"quarter on quarter.”",
						sanur.TextStyle().Size(12).Italic().Color(muted))
				})

			// Sanur has no multi-column text flow, so genuine newspaper columns
			// are not available. Splitting the prose into two column elements
			// side by side is the honest approximation: each side paginates
			// independently rather than the text snaking from one to the other.
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(20)
				r.RelativeItem(1).Element(article(leftColumnBody()))
				r.RelativeItem(1).Element(article(rightColumnBody()))
			})

			c.Item().LineHorizontal(1, hairline)

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(18)
				r.RelativeItem(1).Element(panel("Risks", bulletList([]string{
					"Concentration: the top five accounts are 31% of revenue.",
					"FX exposure on Oceania contracts is unhedged beyond Q3.",
					"Hiring plan assumes two senior engineers land by August.",
				})))
				r.RelativeItem(1).Element(panel("Next quarter", bulletList([]string{
					"Close the enterprise tier pricing review.",
					"Migrate remaining accounts off the legacy billing path.",
					"Publish the partner integration guide.",
				})))
			})
		})
	})

	// --- Sheet three: landscape appendix ---------------------------------
	doc.Page(func(p *sanur.Page) {
		// Overriding the size alone is enough to turn the sheet sideways; the
		// header and footer from the template still apply, and they reflow to the
		// wider measure because they were built as relative rows.
		p.Size(sanur.Landscape(sanur.A4)).Margin(sanur.Mm(14))
		p.DefaultTextStyle(sanur.TextStyle().Size(8.5).Color(ink))
		p.Header().Element(reportHeader("Appendix A · Monthly detail", "Q2 2026"))

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(14)

			c.Item().BookmarkNamed(0, "Appendix A \u00b7 Monthly detail", "bookmark:Appendix A").
				Element(sectionTitle("Appendix A \u00b7 Monthly detail"))

			c.Item().StyledText(
				"A wide table on a landscape sheet. Page size is per definition, so "+
					"orientation can change mid-document while the header and footer "+
					"stay the same.",
				sanur.TextStyle().Size(9).Color(muted))

			c.Item().Element(monthlyTable())
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("generating report: %v", err)
	}
	fmt.Printf("wrote %s\n", out)
}

// --- Page furniture --------------------------------------------------------

// reportHeader is a title on the left, context on the right, over a rule.
func reportHeader(title, context string) core.Element {
	row := elements.NewRow(10,
		elements.Relative(1, elements.NewText(title,
			sanur.TextStyle().Size(10).Bold().Color(ink).LetterSpacing(0.3).Build())),
		elements.Auto(elements.NewText(context,
			sanur.TextStyle().Size(8.5).Color(muted).Build())),
	)

	col := &elements.Column{Spacing: 7}
	col.Add(row)
	col.Add(&elements.Line{Width: 1, Color: hairline})

	return &elements.Padding{Bottom: 16, Child: col}
}

// reportFooter is a rule with a caption and a page number under it.
func reportFooter() core.Element {
	row := elements.NewRow(10,
		elements.Relative(1, elements.NewText("Generated with sanur",
			sanur.TextStyle().Size(7.5).Color(muted).Build())),
		elements.Constant(110, &elements.Aligned{
			Horizontal: core.AlignRight,
			Child: elements.NewPageNumber("Page {page} of {total}",
				sanur.TextStyle().Size(7.5).Color(muted).Build()),
		}),
	)

	col := &elements.Column{Spacing: 6}
	col.Add(&elements.Line{Width: 1, Color: hairline})
	col.Add(row)

	return &elements.Padding{Top: 12, Child: col}
}

// --- Composition helpers ---------------------------------------------------

// panel is a titled card: heading, rule, then arbitrary content.
func panel(title string, body core.Element) core.Element {
	col := &elements.Column{Spacing: 9}
	col.Add(elements.NewText(strings.ToUpper(title),
		sanur.TextStyle().Size(7.5).Bold().Color(muted).LetterSpacing(0.9).Build()))
	col.Add(&elements.Line{Width: 1, Color: hairline})
	col.Add(body)
	return col
}

// statTile is a headline figure with a delta and a sparkline.
//
// The tile is vertically greedy on purpose: every tile in the row extends to the
// tallest, so their backgrounds line up even though their labels wrap to
// different numbers of lines.
func statTile(m metric) core.Element {
	// The sign is spelled with ASCII rather than a triangle glyph. This document uses
	// the built-in faces, which are addressed through WinAnsi and have no geometric
	// shapes, so a triangle would be substituted with a question mark. Marks like
	// these need a registered TrueType font — see examples/scripts.
	deltaColor := positive
	sign := "+"
	if m.Change < 0 {
		deltaColor = negative
		sign = "-"
	}

	col := &elements.Column{Spacing: 5}
	col.Add(elements.NewText(strings.ToUpper(m.Label),
		sanur.TextStyle().Size(7).Bold().Color(muted).LetterSpacing(0.7).Build()))
	col.Add(elements.NewText(m.Value,
		sanur.TextStyle().Size(17).Bold().Color(ink).Build()))
	col.Add(elements.NewText(
		fmt.Sprintf("%s%.1f%% vs Q1", sign, math.Abs(m.Change)),
		sanur.TextStyle().Size(7.5).Color(deltaColor).Build()))
	col.Add(&elements.Constrained{
		MinHeight: 18, MaxHeight: 18,
		Child: &sparkline{values: m.Series, color: accent, width: 1.2},
	})

	return &elements.Extend{
		Vertical: true,
		Child: &elements.Border{
			Top:    elements.BorderSide{Width: 1, Color: hairline},
			Right:  elements.BorderSide{Width: 1, Color: hairline},
			Bottom: elements.BorderSide{Width: 1, Color: hairline},
			Left:   elements.BorderSide{Width: 1, Color: hairline},
			Child: &elements.Background{
				Color: sanur.White,
				Child: elements.UniformPadding(11, col),
			},
		},
	}
}

// barChart draws a horizontal bar per region.
//
// There is no chart element; a bar is a background of a fixed width inside a row.
// Because the bar's width is a relative share of the track, the chart rescales
// with the page rather than being pinned to pixel positions.
func barChart(regions []region, trackWidth float64) core.Element {
	var maxRevenue float64
	for _, r := range regions {
		maxRevenue = math.Max(maxRevenue, r.Revenue)
	}

	col := &elements.Column{Spacing: 9}
	for _, r := range regions {
		fraction := r.Revenue / maxRevenue

		// The filled portion and the empty remainder are two relative items, so
		// their ratio is the data. Weights of zero are avoided because a
		// zero-weight item would take no space and collapse the track.
		bar := elements.NewRow(0,
			elements.Relative(math.Max(fraction, 0.001), &elements.Background{
				Color:  accent,
				Radius: 1.5,
				Child:  &elements.Spacer{Height: 9},
			}),
			elements.Relative(math.Max(1-fraction, 0.001), &elements.Spacer{Height: 9}),
		)

		row := elements.NewRow(10,
			elements.Constant(84, &elements.Aligned{
				Vertical: core.AlignMiddle,
				Child: elements.NewText(r.Name,
					sanur.TextStyle().Size(8).Color(ink).Build()),
			}),
			elements.Relative(1, &elements.Aligned{Vertical: core.AlignMiddle, Child: bar}),
			elements.Constant(52, &elements.Aligned{
				Horizontal: core.AlignRight,
				Vertical:   core.AlignMiddle,
				Child: elements.NewText(money(r.Revenue),
					sanur.TextStyle().Size(8).Bold().Color(ink).Build()),
			}),
		)
		col.Add(row)
	}
	return col
}

// commentary is a short stack of label/value pairs plus a note.
func commentary() core.Element {
	col := &elements.Column{Spacing: 7}

	for _, pair := range [][2]string{
		{"Bookings", "$5.41M"},
		{"Net retention", "114%"},
		{"CAC payback", "14 mo"},
		{"Runway", "26 mo"},
	} {
		col.Add(elements.NewRow(6,
			elements.Relative(1, elements.NewText(pair[0],
				sanur.TextStyle().Size(8.5).Color(muted).Build())),
			elements.Auto(elements.NewText(pair[1],
				sanur.TextStyle().Size(8.5).Bold().Color(ink).Build())),
		))
	}

	col.Add(&elements.Spacer{Height: 3})
	col.Add(elements.NewText(
		"South Asia is the fastest-growing region at 22.1%, though off a small base.",
		sanur.TextStyle().Size(8).Color(muted).Build()))

	return col
}

// regionTable is a banded table with a right-aligned numeric block.
func regionTable(regions []region) core.Element {
	header := sanur.TextStyle().Size(7).Bold().Color(muted).LetterSpacing(0.7).Build()
	body := sanur.TextStyle().Size(8.5).Color(ink).Build()

	col := &elements.Column{}

	col.Add(tableRow(hairline, 0.75,
		cell(elements.NewText("REGION", header), core.AlignLeft),
		cell(elements.NewText("REVENUE", header), core.AlignRight),
		cell(elements.NewText("SHARE", header), core.AlignRight),
		cell(elements.NewText("ACCOUNTS", header), core.AlignRight),
		cell(elements.NewText("GROWTH", header), core.AlignRight),
	))

	for _, r := range regions {
		growthStyle := body
		growthStyle.Color = positive
		if r.Growth < 0 {
			growthStyle.Color = negative
		}

		col.Add(tableRow(hairline, 0.5,
			cell(elements.NewText(r.Name, body), core.AlignLeft),
			cell(elements.NewText(money(r.Revenue), body), core.AlignRight),
			cell(elements.NewText(fmt.Sprintf("%.1f%%", r.Share), body), core.AlignRight),
			cell(elements.NewText(fmt.Sprintf("%d", r.Accounts), body), core.AlignRight),
			cell(elements.NewText(fmt.Sprintf("%+.1f%%", r.Growth), growthStyle), core.AlignRight),
		))
	}

	return col
}

// tableRow lays five cells across and rules the bottom edge, which is how a
// table gets horizontal separators without a dedicated grid element.
func tableRow(ruleColor core.Color, ruleWidth float64, cells ...core.Element) core.Element {
	weights := []float64{3, 1.4, 1, 1.2, 1.1}

	items := make([]elements.RowItem, 0, len(cells))
	for i, c := range cells {
		items = append(items, elements.Relative(weights[i], c))
	}

	return &elements.Border{
		Bottom: elements.BorderSide{Width: ruleWidth, Color: ruleColor},
		Child:  elements.NewRow(8, items...),
	}
}

// cell pads a value and aligns it horizontally.
func cell(content core.Element, align core.HorizontalAlign) core.Element {
	return &elements.Padding{
		Top: 7, Bottom: 7,
		Child: &elements.Aligned{Horizontal: align, Child: content},
	}
}

// article renders justified prose with a hanging first heading.
func article(paragraphs []string) core.Element {
	col := &elements.Column{Spacing: 9}

	for _, text := range paragraphs {
		// A line ending in a colon is treated as a subheading, which keeps the
		// copy itself free of markup.
		if strings.HasSuffix(text, ":") {
			col.Add(elements.NewText(strings.TrimSuffix(text, ":"),
				sanur.TextStyle().Size(10).Bold().Color(ink).Build()))
			continue
		}

		para := elements.NewText(text, sanur.TextStyle().Size(9).Color(ink).LineHeight(1.4).Build())
		para.Align = core.AlignJustify
		col.Add(para)
	}
	return col
}

// bulletList renders a marker beside each item, aligned so wrapped lines indent.
func bulletList(items []string) core.Element {
	col := &elements.Column{Spacing: 7}

	for _, text := range items {
		// A constant-width marker column is what makes the hanging indent work:
		// the prose is a relative item, so its wrapped lines start under the
		// first line rather than under the bullet.
		col.Add(elements.NewRow(6,
			elements.Constant(10, elements.NewText("•",
				sanur.TextStyle().Size(9).Color(accent).Build())),
			elements.Relative(1, elements.NewText(text,
				sanur.TextStyle().Size(8.5).Color(ink).LineHeight(1.35).Build())),
		))
	}
	return col
}

// monthlyTable is a wide grid sized for a landscape sheet.
func monthlyTable() core.Element {
	months := []string{"Apr", "May", "Jun"}
	lines := []string{
		"New bookings", "Expansion", "Churned", "Net revenue",
		"Gross margin", "Operating expense", "Operating income", "Headcount",
	}

	header := sanur.TextStyle().Size(7).Bold().Color(sanur.White).LetterSpacing(0.7).Build()
	body := sanur.TextStyle().Size(8).Color(ink).Build()

	// Every row shares this weighting, which is what keeps the columns aligned
	// down the sheet.
	weights := append([]float64{2.4}, repeat(1.0, len(months)*3)...)

	col := &elements.Column{}

	// Two header rows: a spanning group label over each month, then the
	// per-metric labels. Spanning is achieved by giving the group cell a weight
	// equal to the sum of the columns it covers.
	groupItems := []elements.RowItem{elements.Relative(2.4, &elements.Empty{})}
	for _, m := range months {
		groupItems = append(groupItems, elements.Relative(3, &elements.Background{
			Color: accent,
			Child: &elements.Padding{
				Top: 5, Bottom: 5,
				Child: &elements.Aligned{
					Horizontal: core.AlignCenter,
					Child:      elements.NewText(strings.ToUpper(m), header),
				},
			},
		}))
	}
	col.Add(elements.NewRow(4, groupItems...))
	col.Add(&elements.Spacer{Height: 2})

	subHeader := sanur.TextStyle().Size(6.5).Bold().Color(muted).LetterSpacing(0.5).Build()
	subItems := []elements.RowItem{elements.Relative(2.4, &elements.Empty{})}
	for range months {
		for _, label := range []string{"PLAN", "ACTUAL", "VAR"} {
			subItems = append(subItems, elements.Relative(1, &elements.Padding{
				Top: 4, Bottom: 4,
				Child: &elements.Aligned{
					Horizontal: core.AlignRight,
					Child:      elements.NewText(label, subHeader),
				},
			}))
		}
	}
	col.Add(&elements.Border{
		Bottom: elements.BorderSide{Width: 0.75, Color: hairline},
		Child:  elements.NewRow(4, subItems...),
	})

	for i, line := range lines {
		band := sanur.White
		if i%2 == 1 {
			band = surface
		}

		items := []elements.RowItem{elements.Relative(weights[0], &elements.Padding{
			Top: 6, Bottom: 6, Left: 6,
			Child: elements.NewText(line, body),
		})}

		for month := range months {
			// Deterministic pseudo-data, so the example output is reproducible.
			plan := 100 + float64((i*17+month*23)%180)
			actual := plan * (0.9 + float64((i*7+month*13)%25)/100)
			variance := actual - plan

			varStyle := body
			varStyle.Color = positive
			if variance < 0 {
				varStyle.Color = negative
			}

			for _, value := range []struct {
				text  string
				style core.TextStyle
			}{
				{fmt.Sprintf("%.0f", plan), body},
				{fmt.Sprintf("%.0f", actual), body},
				{fmt.Sprintf("%+.0f", variance), varStyle},
			} {
				items = append(items, elements.Relative(1, &elements.Padding{
					Top: 6, Bottom: 6, Right: 6,
					Child: &elements.Aligned{
						Horizontal: core.AlignRight,
						Child:      elements.NewText(value.text, value.style),
					},
				}))
			}
		}

		col.Add(&elements.Background{Color: band, Child: elements.NewRow(4, items...)})
	}

	return col
}

func repeat(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// money formats an amount compactly, since a table column has no room for
// thousands separators on every figure.
func money(v float64) string {
	if v >= 1_000_000 {
		return fmt.Sprintf("$%.2fM", v/1_000_000)
	}
	return fmt.Sprintf("$%.0fk", v/1000)
}

// --- A custom element ------------------------------------------------------

// sparkline plots a series as a polyline.
//
// This is the case where composition runs out: a line through arbitrary points
// cannot be expressed as nested boxes, so it implements core.Element directly.
// Doing so needs only the two methods — Measure to claim a box, Draw to paint
// inside it — and the result composes with every other element exactly like a
// built-in one.
type sparkline struct {
	values []float64
	color  core.Color
	width  float64
}

func (s *sparkline) Measure(available core.Size) core.SpacePlan {
	if len(s.values) < 2 {
		return core.EmptyRender()
	}
	// The sparkline fills whatever box it is given, so it takes the offered
	// space rather than asking for a size of its own. A caller decides the
	// height by wrapping it in a constraint.
	return core.FullRender(available)
}

func (s *sparkline) Draw(canvas core.Canvas, available core.Size) {
	if len(s.values) < 2 || available.Width <= 0 || available.Height <= 0 {
		return
	}

	min, max := s.values[0], s.values[0]
	for _, v := range s.values {
		min = math.Min(min, v)
		max = math.Max(max, v)
	}

	// A flat series would divide by zero when normalised, so it is drawn along
	// the vertical centre instead.
	span := max - min
	flat := span < 1e-9

	step := available.Width / float64(len(s.values)-1)
	point := func(i int) core.Position {
		v := 0.5
		if !flat {
			v = (s.values[i] - min) / span
		}
		// Y is inverted because layout space grows downwards while a chart's
		// values grow upwards.
		return core.Position{
			X: float64(i) * step,
			Y: available.Height - v*available.Height,
		}
	}

	for i := 1; i < len(s.values); i++ {
		canvas.DrawLine(point(i-1), point(i), s.color, s.width)
	}
}

// --- Copy ------------------------------------------------------------------

func leftColumnBody() []string {
	return []string{
		"Demand:",
		"Bookings grew 12.4% against Q1 on broadly flat sales headcount, which is " +
			"the first quarter in six where growth has come from conversion rather " +
			"than from adding capacity. The improvement is concentrated in the " +
			"mid-market segment, where the shortened trial has lifted close rates " +
			"by roughly nine points.",
		"Enterprise remains lumpy. Two of the three deals slipped from June into " +
			"July on procurement timelines rather than on price, and both have since " +
			"closed at or above the original contract value.",
		"Retention:",
		"Net revenue retention held at 114%. Gross churn improved to 2.1%, its " +
			"lowest level on record, helped by the migration of the last cohort of " +
			"accounts off the legacy billing path.",
	}
}

func rightColumnBody() []string {
	return []string{
		"Margin:",
		"Gross margin fell 180 basis points to 61.3%. The entire movement is " +
			"attributable to mix: the fastest-growing regions carry a higher " +
			"proportion of hosted deployments, which are structurally lower margin " +
			"than the self-managed tier.",
		"On a like-for-like basis, unit economics improved in every established " +
			"market. Hosting cost per account fell 6% following the storage tiering " +
			"work completed in May.",
		"Efficiency:",
		"Operating expense grew 4% while revenue grew 12%, giving the first " +
			"quarter of genuine operating leverage since the platform rebuild. CAC " +
			"payback shortened to 14 months.",
	}
}

// sectionTitle is a heading large enough to introduce a sheet.
func sectionTitle(title string) core.Element {
	return elements.NewText(title, sanur.TextStyle().Size(19).Bold().Color(ink).Build())
}
