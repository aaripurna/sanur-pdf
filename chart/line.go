package chart

import (
	"github.com/aaripurna/sanur-pdf/core"
)

// Line plots one or more series against shared categories, optionally filling the
// area beneath them.
//
// Points are spread edge to edge: the first value sits on the value axis and the
// last on the far side of the plot. That reads as a continuous quantity over time,
// which is what a line chart is for — inset half a band, as bars are, it would
// read as a set of discrete buckets instead.
type Line struct {
	Categories []string
	Series     []Series
	Style      Style

	// Area fills beneath every series.
	//
	// It fills all of them rather than only the first, because singling one out
	// would be an arbitrary rule to remember. With more than one series the
	// overlap muddies quickly, so an area chart is usually a single-series chart;
	// leave this off and use the stroke alone when comparing several.
	Area bool

	// AreaOpacity is the fill alpha. Zero uses a light default.
	AreaOpacity uint8

	// Width is the stroke width in points. Zero uses a sensible default.
	Width float64

	// HideMarkers omits the dot on each data point, which is worth doing once a
	// series has enough points that the markers merge into a thick line.
	HideMarkers bool

	// MarkerRadius overrides the dot size.
	MarkerRadius float64
}

// Default line geometry, in points.
const (
	defaultLineWidth    = 1.8
	defaultMarkerRadius = 2.6
	defaultAreaOpacity  = 38
)

func (l *Line) Measure(available core.Size) core.SpacePlan {
	if !l.plottable() {
		return core.EmptyRender()
	}
	// A plot has no natural size, so it takes the box it is offered.
	return core.FullRender(available)
}

// plottable reports whether there is anything to draw. A single category cannot
// make a line, only a point, so two is the minimum.
func (l *Line) plottable() bool {
	return len(l.Categories) >= 2 && len(l.Series) > 0
}

func (l *Line) Draw(canvas core.Canvas, available core.Size) {
	if !l.plottable() || available.IsEmpty() {
		return
	}

	style := l.Style.resolve()

	// An area chart is measured from zero: filling down to the smallest value
	// instead would leave a band of colour floating above the axis.
	f, ok := newFrame(available, frameOptions{
		style:       style,
		categories:  l.Categories,
		series:      l.Series,
		includeZero: true,
	})
	if !ok {
		return
	}

	f.drawGrid(canvas)
	f.drawAxis(canvas)
	f.drawCategoryLabels(canvas, f.edgeAt)

	points := l.resolvePoints(f)

	// Areas go down first, then every stroke, then every marker. Drawing each
	// series completely in turn would let the second series' fill wash over the
	// first one's line.
	if l.Area {
		for i := range l.Series {
			l.drawArea(canvas, f, style, i, points[i])
		}
	}
	for i := range l.Series {
		l.drawStroke(canvas, style, i, points[i])
	}
	if !l.HideMarkers {
		for i := range l.Series {
			l.drawMarkers(canvas, style, i, points[i])
		}
	}

	drawFrameLegend(canvas, style, seriesLegend(style, l.Series), f, available)
}

// resolvePoints maps every series onto plot coordinates.
//
// A series shorter than the category list contributes only the points it has, so
// an incomplete final period renders as a line that stops rather than dropping to
// zero — which would read as a real collapse in the data.
func (l *Line) resolvePoints(f frame) [][]core.Position {
	out := make([][]core.Position, len(l.Series))

	for i, s := range l.Series {
		points := make([]core.Position, 0, len(l.Categories))
		for c := range l.Categories {
			v, ok := s.At(c)
			if !ok {
				break
			}
			points = append(points, core.Position{X: f.edgeAt(c), Y: f.value.At(v)})
		}
		out[i] = points
	}
	return out
}

func (l *Line) drawArea(canvas core.Canvas, f frame, style Style, index int, points []core.Position) {
	if len(points) < 2 {
		return
	}

	opacity := l.AreaOpacity
	if opacity == 0 {
		opacity = defaultAreaOpacity
	}

	base := f.baseline()

	// The area is the series closed down to the baseline at both ends.
	outline := make([]core.Position, 0, len(points)+2)
	outline = append(outline, core.Position{X: points[0].X, Y: base})
	outline = append(outline, points...)
	outline = append(outline, core.Position{X: points[len(points)-1].X, Y: base})

	canvas.DrawPath(core.Polygon(outline...),
		core.Filled(fade(style.colorFor(index, l.Series[index]), opacity)))
}

func (l *Line) drawStroke(canvas core.Canvas, style Style, index int, points []core.Position) {
	if len(points) < 2 {
		return
	}

	width := l.Width
	if width <= 0 {
		width = defaultLineWidth
	}

	// One path rather than a segment per pair: drawn separately, each segment
	// would be capped independently and the corners would notch.
	canvas.DrawPath(core.Polyline(points...), core.PathStyle{
		Stroke: style.colorFor(index, l.Series[index]),
		Width:  width,
		Join:   core.JoinRound,
		Cap:    core.CapRound,
	})
}

func (l *Line) drawMarkers(canvas core.Canvas, style Style, index int, points []core.Position) {
	radius := l.MarkerRadius
	if radius <= 0 {
		radius = defaultMarkerRadius
	}

	colour := style.colorFor(index, l.Series[index])

	// A white centre keeps markers legible where a line passes behind them.
	for _, pt := range points {
		canvas.DrawPath(core.NewPath().Circle(pt, radius), core.PathStyle{
			Fill:   core.RGB(255, 255, 255),
			Stroke: colour,
			Width:  radius * 0.55,
		})
	}
}

var _ core.Element = (*Line)(nil)
