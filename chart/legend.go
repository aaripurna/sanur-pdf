package chart

import (
	"math"

	"github.com/aaripurna/sanur-pdf/core"
)

// Legend swatch geometry, in points.
const (
	swatchWidth  = 16
	swatchHeight = 5
	swatchRadius = 2.5
	swatchGap    = 5
)

// legendEntry is one key row: a colour and the name it stands for.
//
// The legend takes entries rather than series so that a pie, whose slices are not
// series, shares the same code. Both build the list themselves, which also
// guarantees a swatch shows the colour the chart actually drew with, including any
// per-series override.
type legendEntry struct {
	name   string
	colour core.Color
}

// legendSize is how much room a legend needs.
type legendSize struct {
	width  float64
	height float64
}

// seriesLegend builds entries from a series list, skipping unnamed series.
func seriesLegend(style Style, series []Series) []legendEntry {
	var entries []legendEntry
	for i, s := range series {
		if s.Name == "" {
			continue
		}
		entries = append(entries, legendEntry{name: s.Name, colour: style.colorFor(i, s)})
	}
	return entries
}

// legendExtent measures the legend without drawing it, so a plot can be laid out
// around it.
func legendExtent(style Style, entries []legendEntry) legendSize {
	if style.Legend == LegendNone || len(entries) == 0 {
		return legendSize{}
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}

	if style.Legend == LegendRight {
		// A right-hand legend is a column: as wide as its longest entry, and as
		// tall as it likes, since the plot does not have to reserve that.
		return legendSize{
			width: swatchWidth + swatchGap +
				widestText(style.LegendLabel, names) + style.LegendSpacing,
		}
	}

	// A horizontal legend occupies one strip. Entries are never wrapped: a chart
	// with enough series to overflow the width wants a right-hand legend instead,
	// and wrapping would eat into the plot by an amount the layout cannot predict.
	return legendSize{
		height: math.Max(lineExtent(style.LegendLabel), swatchHeight) + style.LegendSpacing/2,
	}
}

// legendRowHeight is the vertical pitch of a stacked legend.
func legendRowHeight(style Style) float64 {
	return math.Max(lineExtent(style.LegendLabel), swatchHeight) + 5
}

// drawLegendStrip lays entries out left to right on one line, centred on centreY.
func drawLegendStrip(canvas core.Canvas, style Style, entries []legendEntry, startX, centreY float64) {
	x := startX

	for _, e := range entries {
		drawSwatch(canvas, e.colour, x, centreY)

		canvas.DrawText(e.name, core.Position{
			X: x + swatchWidth + swatchGap,
			Y: centredBaseline(style.LegendLabel, centreY),
		}, style.LegendLabel)

		x += swatchWidth + swatchGap +
			style.LegendLabel.MeasureText(e.name) + style.LegendSpacing
	}
}

// drawLegendColumn stacks entries downwards from x, centred as a block on centreY
// so the key reads as belonging to the plot beside it.
func drawLegendColumn(canvas core.Canvas, style Style, entries []legendEntry, x, centreY float64) {
	pitch := legendRowHeight(style)
	y := centreY - float64(len(entries))*pitch/2 + pitch/2

	for _, e := range entries {
		drawSwatch(canvas, e.colour, x, y)

		canvas.DrawText(e.name, core.Position{
			X: x + swatchWidth + swatchGap,
			Y: centredBaseline(style.LegendLabel, y),
		}, style.LegendLabel)

		y += pitch
	}
}

// drawSwatch paints one colour chip, vertically centred on centreY.
func drawSwatch(canvas core.Canvas, colour core.Color, x, centreY float64) {
	swatch := core.RoundedRect(
		core.Position{X: x, Y: centreY - swatchHeight/2},
		core.Size{Width: swatchWidth, Height: swatchHeight},
		swatchRadius)
	canvas.DrawPath(swatch, core.Filled(colour))
}

// drawFrameLegend places a cartesian chart's legend relative to its plot.
func drawFrameLegend(canvas core.Canvas, style Style, entries []legendEntry, f frame, available core.Size) {
	if style.Legend == LegendNone || len(entries) == 0 {
		return
	}

	switch style.Legend {
	case LegendRight:
		drawLegendColumn(canvas, style, entries,
			f.right()+style.LegendSpacing, f.origin.Y+f.size.Height/2)
	case LegendBottom:
		drawLegendStrip(canvas, style, entries,
			f.origin.X, available.Height-lineExtent(style.LegendLabel)/2)
	default:
		drawLegendStrip(canvas, style, entries,
			f.origin.X, lineExtent(style.LegendLabel)/2)
	}
}
