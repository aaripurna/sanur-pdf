package chart

import (
	"math"

	"github.com/aaripurna/sanur-pdf/core"
)

// Bar draws a column or bar chart, grouping several series side by side within
// each category band.
//
// Unlike a line chart, bars are inset half a band from the plot edges, because a
// bar represents a bucket rather than a reading at an instant.
type Bar struct {
	Categories []string
	Series     []Series
	Style      Style

	// Horizontal runs the bars across the page instead of up it, which is what
	// long category names need — rotated labels are harder to read than a
	// sideways chart.
	Horizontal bool

	// CornerRadius rounds the far end of each bar. It is clamped to half the bar's
	// length, so a short bar cannot round into a lozenge.
	CornerRadius float64

	// BandFill is the fraction of each category band the bars occupy, between 0
	// and 1. Zero uses a default that leaves a readable gap between groups.
	BandFill float64

	// GroupGap is the space between bars of the same group, in points.
	GroupGap float64

	// MaxThickness caps how fat a single bar may become, which stops a two-category
	// chart from rendering as two enormous slabs.
	MaxThickness float64
}

// Default bar geometry.
const (
	defaultBandFill     = 0.62
	defaultGroupGap     = 2
	defaultMaxThickness = 46

	// valueLabelGap separates a value label from the bar it annotates.
	valueLabelGap = 4
)

func (b *Bar) Measure(available core.Size) core.SpacePlan {
	if !b.plottable() {
		return core.EmptyRender()
	}
	return core.FullRender(available)
}

func (b *Bar) plottable() bool {
	return len(b.Categories) > 0 && len(b.Series) > 0
}

func (b *Bar) Draw(canvas core.Canvas, available core.Size) {
	if !b.plottable() || available.IsEmpty() {
		return
	}

	style := b.Style.resolve()

	// Bars are always measured from zero; a bar chart whose axis started at the
	// smallest value would exaggerate every difference.
	f, ok := newFrame(available, frameOptions{
		style:       style,
		categories:  b.Categories,
		series:      b.Series,
		horizontal:  b.Horizontal,
		includeZero: true,
		reserveEnd:  b.labelReserve(style),
		// Only needed when a bar hangs the other side of the axis.
		reserveStart: b.negativeReserve(style),
	})
	if !ok {
		return
	}

	f.drawGrid(canvas)
	f.drawCategoryLabels(canvas, f.bandCentre)

	thickness, groupWidth := b.thickness(f)
	base := f.baseline()

	for category := range b.Categories {
		centre := f.bandCentre(category)
		start := centre - groupWidth/2

		for s, series := range b.Series {
			v, ok := series.At(category)
			if !ok {
				continue
			}

			offset := start + (thickness+b.gap())*float64(s) + thickness/2
			b.drawBar(canvas, f, style, s, series, v, offset, thickness, base)
		}
	}

	// The axis goes on top of the bars so that a bar sitting on the baseline does
	// not paint over it.
	f.drawAxis(canvas)

	drawFrameLegend(canvas, style, seriesLegend(style, b.Series), f, available)
}

// labelReserve is the room a value label needs beyond the end of its bar.
//
// Reserving the widest label's full width is deliberately generous: which bar is
// longest is known, but whether its label is the widest is not, since a negative
// value carries a sign and a rounded figure may be shorter than a smaller one.
func (b *Bar) labelReserve(style Style) float64 {
	if style.HideValueLabels {
		return 0
	}

	labels := make([]string, 0, len(b.Series)*len(b.Categories))
	for _, s := range b.Series {
		for _, v := range s.Values {
			labels = append(labels, style.Format(v))
		}
	}

	if b.Horizontal {
		return widestText(style.ValueLabel, labels) + valueLabelGap
	}
	// Vertically the label sits above the bar, so only its height is needed.
	return lineExtent(style.ValueLabel) + valueLabelGap
}

// negativeReserve is the room a label below a negative bar needs. It is zero for
// all-positive data, so an ordinary chart loses no plot area to it.
func (b *Bar) negativeReserve(style Style) float64 {
	if style.HideValueLabels || !b.hasNegative() {
		return 0
	}
	return b.labelReserve(style)
}

func (b *Bar) hasNegative() bool {
	for _, s := range b.Series {
		for _, v := range s.Values {
			if v < 0 {
				return true
			}
		}
	}
	return false
}

func (b *Bar) gap() float64 {
	if b.GroupGap > 0 {
		return b.GroupGap
	}
	if len(b.Series) <= 1 {
		return 0
	}
	return defaultGroupGap
}

// thickness resolves how fat each bar is, and how wide a whole group is.
func (b *Bar) thickness(f frame) (bar, group float64) {
	fill := b.BandFill
	if fill <= 0 || fill > 1 {
		fill = defaultBandFill
	}

	maxThickness := b.MaxThickness
	if maxThickness <= 0 {
		maxThickness = defaultMaxThickness
	}

	group = f.slot() * fill
	count := float64(len(b.Series))

	gaps := b.gap() * (count - 1)
	bar = (group - gaps) / count

	if bar > maxThickness {
		bar = maxThickness
		group = bar*count + gaps
	}
	// A band too narrow to hold the group at all still gets a hairline per bar,
	// which reads as a dense chart rather than as missing data.
	if bar < 0.5 {
		bar = 0.5
		group = bar*count + gaps
	}
	return bar, group
}

// drawBar paints one bar and its value label.
func (b *Bar) drawBar(
	canvas core.Canvas,
	f frame,
	style Style,
	index int,
	series Series,
	value, centre, thickness, base float64,
) {
	at := f.value.At(value)

	var pos core.Position
	var size core.Size

	if b.Horizontal {
		pos = core.Position{X: math.Min(base, at), Y: centre - thickness/2}
		size = core.Size{Width: math.Abs(at - base), Height: thickness}
	} else {
		pos = core.Position{X: centre - thickness/2, Y: math.Min(base, at)}
		size = core.Size{Width: thickness, Height: math.Abs(at - base)}
	}

	if size.Width <= 0 || size.Height <= 0 {
		// A zero value has no bar to draw, but its label is still worth placing.
		b.drawValueLabel(canvas, style, value, at, centre, base)
		return
	}

	radius := b.CornerRadius
	if radius > 0 {
		// Clamping to half the bar's length keeps a short bar a bar.
		if b.Horizontal {
			radius = math.Min(radius, size.Width/2)
		} else {
			radius = math.Min(radius, size.Height/2)
		}
		canvas.DrawPath(core.RoundedRect(pos, size, radius),
			core.Filled(style.colorFor(index, series)))
	} else {
		canvas.DrawRect(pos, size, style.colorFor(index, series))
	}

	b.drawValueLabel(canvas, style, value, at, centre, base)
}

// drawValueLabel puts the figure just past the end of a bar.
func (b *Bar) drawValueLabel(canvas core.Canvas, style Style, value, at, centre, base float64) {
	if style.HideValueLabels {
		return
	}

	text := style.Format(value)
	if text == "" {
		return
	}

	var pos core.Position
	if b.Horizontal {
		x := at + valueLabelGap
		if at < base {
			x = at - valueLabelGap - style.ValueLabel.MeasureText(text)
		}
		pos = core.Position{X: x, Y: centredBaseline(style.ValueLabel, centre)}
	} else {
		y := at - valueLabelGap
		if at > base {
			// A negative bar hangs below the axis, so its label goes underneath.
			y = at + valueLabelGap + style.ValueLabel.Font.Ascent(style.ValueLabel.Size)
		}
		pos = core.Position{X: centre - style.ValueLabel.MeasureText(text)/2, Y: y}
	}
	canvas.DrawText(text, pos, style.ValueLabel)
}

var _ core.Element = (*Bar)(nil)
