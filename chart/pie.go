package chart

import (
	"fmt"
	"math"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
)

// Slice is one wedge of a pie.
type Slice struct {
	Name  string
	Value float64

	// Color overrides the palette entry for this slice.
	Color core.Color
}

// Pie draws a pie chart, or a donut when InnerRadius is set.
//
// The legend sits to the right by default and carries each slice's share, because
// a wedge's angle is much harder to read as a percentage than a number is.
type Pie struct {
	Slices []Slice
	Style  Style

	// Radius caps the circle. Zero fills whatever the box allows.
	Radius float64

	// InnerRadius cuts a hole, turning the pie into a donut. It is clamped below
	// the outer radius.
	InnerRadius float64

	// CentreLabel and CentreNote are drawn in a donut's hole, typically the total
	// and what it counts.
	CentreLabel string
	CentreNote  string

	// CentreLabelStyle and CentreNoteStyle override the defaults, which are
	// derived from the value and label styles.
	CentreLabelStyle core.TextStyle
	CentreNoteStyle  core.TextStyle

	// HideShares drops the percentage from each legend row.
	HideShares bool

	// SeparatorWidth is the stroke between adjacent slices. Zero uses a hairline;
	// set a negative value for none.
	SeparatorWidth float64

	// SeparatorColor strokes between slices. Unset uses white, which reads as a
	// gap on a white page.
	SeparatorColor core.Color
}

// startAngle is twelve o'clock. Layout space grows downwards, so the positive X
// axis is three o'clock and a quarter turn back from it is straight up.
const startAngle = -90.0

func (p *Pie) Measure(available core.Size) core.SpacePlan {
	if p.total() <= 0 {
		return core.EmptyRender()
	}
	return core.FullRender(available)
}

// total sums the slices that can be drawn.
//
// Only positive values count. A wedge cannot sweep backwards and still tile the
// circle, so a negative slice has no representation here — which is why Draw
// refuses one outright rather than letting it fall out of the total silently.
func (p *Pie) total() float64 {
	total := 0.0
	for _, s := range p.Slices {
		if s.Value > 0 {
			total += s.Value
		}
	}
	return total
}

// negativeSlices names the slices that cannot be drawn.
func (p *Pie) negativeSlices() []string {
	var names []string
	for i, s := range p.Slices {
		if s.Value < 0 {
			name := s.Name
			if name == "" {
				name = fmt.Sprintf("slice %d", i)
			}
			names = append(names, fmt.Sprintf("%s (%g)", name, s.Value))
		}
	}
	return names
}

func (p *Pie) Draw(canvas core.Canvas, available core.Size) {
	// A negative slice is reported rather than skipped. Dropping it would rescale
	// the remaining slices to 100% and produce a chart that looks entirely
	// plausible while a portion of the data has vanished — the worst kind of
	// wrong. Failing here surfaces it from Bytes as a layout error instead.
	if negatives := p.negativeSlices(); len(negatives) > 0 {
		canvas.Fail(fmt.Errorf(
			"sanur/chart: a pie cannot show negative values: %s; "+
				"use a bar chart, or plot the magnitudes separately",
			strings.Join(negatives, ", ")))
		return
	}

	total := p.total()
	if total <= 0 || available.IsEmpty() {
		return
	}

	style := p.Style.resolve()
	if style.Legend == LegendTop || style.Legend == LegendBottom {
		// A pie is round; a strip legend above or below it wastes the width the
		// circle cannot use anyway. Only a column reads well beside one.
		style.Legend = LegendRight
	}

	entries := p.legendEntries(style, total)
	legend := legendExtent(style, entries)

	box := core.Size{Width: available.Width - legend.width, Height: available.Height}
	if box.Width <= 0 || box.Height <= 0 {
		return
	}

	radius := math.Min(box.Width, box.Height) / 2
	if p.Radius > 0 {
		radius = math.Min(radius, p.Radius)
	}
	if radius <= 0 {
		return
	}

	inner := math.Min(p.InnerRadius, radius*0.95)
	centre := core.Position{X: box.Width / 2, Y: available.Height / 2}

	p.drawSlices(canvas, style, centre, radius, inner, total)
	p.drawCentre(canvas, style, centre, inner)

	if style.Legend != LegendNone && len(entries) > 0 {
		drawLegendColumn(canvas, style, entries,
			centre.X+radius+style.LegendSpacing, centre.Y)
	}
}

func (p *Pie) drawSlices(
	canvas core.Canvas,
	style Style,
	centre core.Position,
	radius, inner, total float64,
) {
	separator := core.PathStyle{}
	if p.SeparatorWidth >= 0 {
		width := p.SeparatorWidth
		if width == 0 {
			width = 1.5
		}
		colour := p.SeparatorColor
		if !colour.Visible() {
			colour = core.RGB(255, 255, 255)
		}
		separator.Stroke = colour
		separator.Width = width
	}

	angle := startAngle

	for i, s := range p.Slices {
		// Zero-value slices are simply absent; negatives never reach here, having
		// been reported by Draw.
		if s.Value <= 0 {
			continue
		}
		sweep := s.Value / total * 360

		var path *core.Path
		if inner > 0 {
			// A donut segment is bounded by two arcs. Sweeping the inner one
			// backwards is what joins its ends to the outer arc's; swept the same
			// way, the path would cross itself and fill as a bow tie.
			path = core.NewPath().
				Arc(centre, radius, angle, sweep).
				Arc(centre, inner, angle+sweep, -sweep).
				Close()
		} else {
			path = core.NewPath().
				MoveTo(centre).
				Arc(centre, radius, angle, sweep).
				Close()
		}

		fill := separator
		fill.Fill = style.colorFor(i, Series{Color: s.Color})
		canvas.DrawPath(path, fill)

		angle += sweep
	}
}

// drawCentre puts the summary text in a donut's hole.
func (p *Pie) drawCentre(canvas core.Canvas, style Style, centre core.Position, inner float64) {
	if p.CentreLabel == "" || inner <= 0 {
		return
	}

	label := resolveText(p.CentreLabelStyle, centreLabelDefault(style))
	note := resolveText(p.CentreNoteStyle, style.Label)

	// With a note beneath it, the pair is centred as a block; alone, the label
	// centres on the hole itself.
	labelY := centre.Y
	if p.CentreNote != "" {
		labelY = centre.Y - lineExtent(note)/2
	}

	canvas.DrawText(p.CentreLabel, core.Position{
		X: centre.X - label.MeasureText(p.CentreLabel)/2,
		Y: centredBaseline(label, labelY),
	}, label)

	if p.CentreNote != "" {
		canvas.DrawText(p.CentreNote, core.Position{
			X: centre.X - note.MeasureText(p.CentreNote)/2,
			Y: centredBaseline(note, labelY+lineExtent(label)/2+lineExtent(note)/2),
		}, note)
	}
}

// centreLabelDefault scales the value style up, since a donut's centre figure is
// the chart's headline rather than an annotation.
func centreLabelDefault(style Style) core.TextStyle {
	s := style.ValueLabel
	s.Size = style.ValueLabel.Size * 1.6
	return s
}

// legendEntries builds the key, appending each slice's share.
func (p *Pie) legendEntries(style Style, total float64) []legendEntry {
	var entries []legendEntry

	for i, s := range p.Slices {
		if s.Value <= 0 || s.Name == "" {
			continue
		}

		name := s.Name
		if !p.HideShares {
			name = fmt.Sprintf("%s  %.0f%%", s.Name, s.Value/total*100)
		}

		entries = append(entries, legendEntry{
			name:   name,
			colour: style.colorFor(i, Series{Color: s.Color}),
		})
	}
	return entries
}

var _ core.Element = (*Pie)(nil)
