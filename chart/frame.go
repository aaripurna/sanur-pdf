package chart

import (
	"math"

	"github.com/aaripurna/sanur-pdf/core"
)

// labelGap is the space between a label and the plot edge it annotates.
const labelGap = 6

// frame is the resolved geometry of a cartesian plot: where the plotting area
// sits once the axis furniture has taken its share, and how data maps into it.
//
// Gutters are derived from the labels that will occupy them rather than fixed in
// advance. A hardcoded left gutter is wrong twice over — too wide for single
// digits, too narrow the moment a value reaches seven figures, at which point the
// label silently overlaps the plot.
type frame struct {
	style Style

	// origin and size bound the plotting area, excluding all furniture.
	origin core.Position
	size   core.Size

	// horizontal reports whether categories run down the side and values across
	// the bottom, as they do for a bar chart.
	horizontal bool

	categories []string
	ticks      []float64

	// categoryGap is the distance from the plot edge to the category labels. It
	// exceeds labelGap when value labels may hang past the low end of the value
	// axis, which is what keeps a negative bar's label clear of them.
	categoryGap float64

	// value maps a data value onto the axis it belongs to.
	value Scale
}

// frameOptions configures a plot's layout.
//
// This is a struct rather than a parameter list because the flags read as noise at
// a call site — newFrame(size, style, cats, series, true, true) says nothing about
// which true is which.
type frameOptions struct {
	style      Style
	categories []string
	series     []Series

	// horizontal swaps the axes, putting categories down the side.
	horizontal bool

	// includeZero forces the value axis to span zero, which bars and areas need
	// so they are measured from a real baseline rather than from their own
	// minimum.
	includeZero bool

	// reserveEnd is space kept clear beyond the largest value, for labels drawn
	// past the end of a bar. Without it, a bar reaching the top of its axis puts
	// its own label outside the plot.
	reserveEnd float64

	// reserveStart does the same at the low end, which only matters when the data
	// crosses zero: a negative bar's label hangs below it, and with nothing
	// reserved it lands on top of the category labels.
	reserveStart float64
}

// newFrame lays out a cartesian plot inside the available space.
//
// It returns false when the furniture leaves no room to plot in, which happens
// with a box a few points tall rather than through any fault of the caller's
// data; the chart then draws nothing instead of emitting inverted geometry.
func newFrame(available core.Size, opts frameOptions) (frame, bool) {
	style := opts.style
	categories := opts.categories
	horizontal := opts.horizontal

	f := frame{
		style:      style,
		horizontal: horizontal,
		categories: categories,
	}

	low, high := bounds(opts.series, opts.includeZero)
	f.ticks = Ticks(low, high, style.TickCount)
	axisLow, axisHigh := f.ticks[0], f.ticks[len(f.ticks)-1]

	// The legend is reserved first: it sits outside the plot entirely, so
	// everything below divides what it leaves behind.
	legend := legendExtent(style, seriesLegend(style, opts.series))

	box := core.Size{Width: available.Width, Height: available.Height}
	offset := core.Position{}

	switch style.Legend {
	case LegendTop:
		box.Height -= legend.height
		offset.Y += legend.height
	case LegendBottom:
		box.Height -= legend.height
	case LegendRight:
		box.Width -= legend.width
	}

	labelHeight := lineExtent(style.Label)

	// A label hanging past the low end of the value axis lands where the category
	// labels go, so the reservation belongs in the gap between the plot and those
	// labels — not in the plot itself. Shrinking the plot would move both up
	// together and leave them just as close.
	f.categoryGap = labelGap + opts.reserveStart

	var gutterLeft, gutterBottom float64
	if horizontal {
		// Categories label the vertical axis, so the left gutter has to fit the
		// longest of them plus whatever hangs to the left of the axis.
		gutterLeft = widestText(style.Label, categories) + f.categoryGap
		gutterBottom = labelHeight + labelGap
	} else {
		gutterLeft = widestTick(style, f.ticks) + labelGap
		gutterBottom = labelHeight + f.categoryGap
	}

	f.origin = core.Position{X: offset.X + gutterLeft, Y: offset.Y}
	f.size = core.Size{
		Width:  box.Width - gutterLeft,
		Height: box.Height - gutterBottom,
	}

	// The high-end reservation does come out of the plot, because nothing else
	// occupies the space beyond it: the right edge when values run across, the top
	// when they run up.
	if opts.reserveEnd > 0 {
		if horizontal {
			f.size.Width -= opts.reserveEnd
		} else {
			f.origin.Y += opts.reserveEnd
			f.size.Height -= opts.reserveEnd
		}
	}

	if f.size.Width <= 0 || f.size.Height <= 0 {
		return f, false
	}

	if horizontal {
		f.value = Scale{Low: axisLow, High: axisHigh, From: f.origin.X, To: f.origin.X + f.size.Width}
	} else {
		// The vertical scale runs from the bottom of the plot to the top, which in
		// a Y-down space means From is the larger coordinate.
		f.value = Scale{Low: axisLow, High: axisHigh, From: f.origin.Y + f.size.Height, To: f.origin.Y}
	}

	return f, true
}

// bottom returns the coordinate of the plot's lower edge.
func (f frame) bottom() float64 { return f.origin.Y + f.size.Height }

// right returns the coordinate of the plot's right edge.
func (f frame) right() float64 { return f.origin.X + f.size.Width }

// baseline returns the coordinate of the zero line, or the axis minimum when the
// data does not cross zero.
func (f frame) baseline() float64 {
	if f.value.Low <= 0 && f.value.High >= 0 {
		return f.value.At(0)
	}
	return f.value.At(f.value.Low)
}

// slot is the width or height allotted to each category band.
//
// Bars sit inside a band, centred; line and area points sit on the band's centre
// too, except that a line chart with a single point per edge looks wrong unless
// the first and last points touch the plot edges. pointAt handles that difference.
func (f frame) slot() float64 {
	if len(f.categories) == 0 {
		return 0
	}
	if f.horizontal {
		return f.size.Height / float64(len(f.categories))
	}
	return f.size.Width / float64(len(f.categories))
}

// bandCentre returns the middle of category i's band, on the category axis.
func (f frame) bandCentre(i int) float64 {
	slot := f.slot()
	if f.horizontal {
		return f.origin.Y + slot*(float64(i)+0.5)
	}
	return f.origin.X + slot*(float64(i)+0.5)
}

// edgeAt returns category i's position when points are spread edge to edge,
// which is how a line chart is read: the first value at the axis, the last at the
// far side, with no half-band inset.
func (f frame) edgeAt(i int) float64 {
	n := len(f.categories)
	if n <= 1 {
		if f.horizontal {
			return f.origin.Y + f.size.Height/2
		}
		return f.origin.X + f.size.Width/2
	}

	fraction := float64(i) / float64(n-1)
	if f.horizontal {
		return f.origin.Y + fraction*f.size.Height
	}
	return f.origin.X + fraction*f.size.Width
}

// drawGrid strokes the gridlines and their tick labels.
func (f frame) drawGrid(canvas core.Canvas) {
	if f.style.HideGrid {
		return
	}

	for _, tick := range f.ticks {
		at := f.value.At(tick)

		var from, to core.Position
		if f.horizontal {
			from = core.Position{X: at, Y: f.origin.Y}
			to = core.Position{X: at, Y: f.bottom()}
		} else {
			from = core.Position{X: f.origin.X, Y: at}
			to = core.Position{X: f.right(), Y: at}
		}
		canvas.DrawPath(core.Polyline(from, to), f.style.gridStyle())

		f.drawTickLabel(canvas, tick, at)
	}
}

// drawTickLabel places one value label beside the plot.
func (f frame) drawTickLabel(canvas core.Canvas, tick, at float64) {
	label := f.style.Format(tick)
	if label == "" {
		return
	}

	var pos core.Position
	if f.horizontal {
		// Value labels run along the bottom, centred on their gridline.
		pos = core.Position{
			X: at - f.style.Label.MeasureText(label)/2,
			Y: f.bottom() + labelGap + f.style.Label.Font.Ascent(f.style.Label.Size),
		}
	} else {
		// Value labels sit to the left, vertically centred on their gridline.
		pos = core.Position{
			X: f.origin.X - labelGap - f.style.Label.MeasureText(label),
			Y: centredBaseline(f.style.Label, at),
		}
	}
	canvas.DrawText(label, pos, f.style.Label)
}

// drawAxis strokes the baseline.
func (f frame) drawAxis(canvas core.Canvas) {
	if f.style.HideAxis {
		return
	}

	at := f.baseline()

	var from, to core.Position
	if f.horizontal {
		from = core.Position{X: at, Y: f.origin.Y}
		to = core.Position{X: at, Y: f.bottom()}
	} else {
		from = core.Position{X: f.origin.X, Y: at}
		to = core.Position{X: f.right(), Y: at}
	}
	canvas.DrawPath(core.Polyline(from, to), f.style.axisStyle())
}

// drawCategoryLabels places the category names along their axis.
//
// positions supplies each label's coordinate on the category axis, so a line
// chart can spread its labels edge to edge while a bar chart centres them on
// their bands.
func (f frame) drawCategoryLabels(canvas core.Canvas, positions func(int) float64) {
	for i, name := range f.categories {
		if name == "" {
			continue
		}
		at := positions(i)

		var pos core.Position
		if f.horizontal {
			pos = core.Position{
				X: f.origin.X - f.categoryGap - f.style.Label.MeasureText(name),
				Y: centredBaseline(f.style.Label, at),
			}
		} else {
			pos = core.Position{
				X: at - f.style.Label.MeasureText(name)/2,
				Y: f.bottom() + f.categoryGap + f.style.Label.Font.Ascent(f.style.Label.Size),
			}
		}
		canvas.DrawText(name, pos, f.style.Label)
	}
}

// --- text metrics -----------------------------------------------------------

// centredBaseline returns the baseline that visually centres text on y.
//
// Ink spans from one ascent above the baseline to one descent below it, so its
// midpoint sits half their difference above the baseline. Placing the baseline at
// y plus that half-difference puts the ink's centre on y.
func centredBaseline(style core.TextStyle, y float64) float64 {
	return y + (style.Font.Ascent(style.Size)-style.Font.Descent(style.Size))/2
}

// lineExtent is the vertical room one line of text needs.
func lineExtent(style core.TextStyle) float64 {
	return style.Font.Ascent(style.Size) + style.Font.Descent(style.Size)
}

// widestText measures the longest of several strings.
func widestText(style core.TextStyle, values []string) float64 {
	widest := 0.0
	for _, v := range values {
		widest = math.Max(widest, style.MeasureText(v))
	}
	return widest
}

// widestTick measures the longest formatted tick label.
func widestTick(style Style, ticks []float64) float64 {
	labels := make([]string, len(ticks))
	for i, t := range ticks {
		labels[i] = style.Format(t)
	}
	return widestText(style.Label, labels)
}
