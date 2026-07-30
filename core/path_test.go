package core_test

import (
	"math"
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
)

func TestPathRecordsSegmentsInOrder(t *testing.T) {
	p := core.NewPath().
		MoveTo(core.Position{X: 1, Y: 2}).
		LineTo(core.Position{X: 3, Y: 4}).
		CurveTo(
			core.Position{X: 5, Y: 6},
			core.Position{X: 7, Y: 8},
			core.Position{X: 9, Y: 10}).
		Close()

	segments := p.Segments()
	if len(segments) != 4 {
		t.Fatalf("got %d segments, want 4", len(segments))
	}

	for i, want := range []core.PathOp{core.PathMoveTo, core.PathLineTo, core.PathCurveTo, core.PathClose} {
		if segments[i].Op != want {
			t.Errorf("segment %d op = %v, want %v", i, segments[i].Op, want)
		}
	}

	// A curve carries two controls then the endpoint, in that order.
	curve := segments[2]
	closeTo(t, "control1 x", curve.Points[0].X, 5)
	closeTo(t, "control2 y", curve.Points[1].Y, 8)
	closeTo(t, "endpoint x", curve.Points[2].X, 9)
}

func TestLineToWithoutMoveToStartsTheSubpath(t *testing.T) {
	// A path built by looping over points should need no special case for the
	// first one, so a leading LineTo is promoted.
	p := core.NewPath().LineTo(core.Position{X: 4, Y: 5})

	segments := p.Segments()
	if len(segments) != 1 {
		t.Fatalf("got %d segments, want 1", len(segments))
	}
	if segments[0].Op != core.PathMoveTo {
		t.Errorf("op = %v, want PathMoveTo", segments[0].Op)
	}
}

func TestEmptyPathBehaviour(t *testing.T) {
	var nilPath *core.Path
	if !nilPath.Empty() {
		t.Error("a nil path must report empty")
	}
	if nilPath.Segments() != nil {
		t.Error("a nil path must have no segments")
	}

	p := core.NewPath()
	if !p.Empty() {
		t.Error("a fresh path must report empty")
	}
	// Closing nothing is a no-op rather than a stray operator.
	if p.Close(); !p.Empty() {
		t.Error("Close on an empty path should add nothing")
	}
}

// --- arcs ------------------------------------------------------------------

// pointsOnPath walks a path and returns every endpoint it passes through.
func pointsOnPath(p *core.Path) []core.Position {
	var out []core.Position
	for _, s := range p.Segments() {
		switch s.Op {
		case core.PathMoveTo, core.PathLineTo:
			out = append(out, s.Points[0])
		case core.PathCurveTo:
			out = append(out, s.Points[2])
		}
	}
	return out
}

func TestArcEndpointsLieOnTheCircle(t *testing.T) {
	centre := core.Position{X: 100, Y: 50}
	const radius float64 = 40

	p := core.NewPath().Arc(centre, radius, 0, 270)

	for i, pt := range pointsOnPath(p) {
		dx, dy := pt.X-centre.X, pt.Y-centre.Y
		got := math.Hypot(dx, dy)
		if math.Abs(got-radius) > 0.001 {
			t.Errorf("endpoint %d is %.4f from the centre, want %.4f", i, got, radius)
		}
	}
}

func TestArcSplitsIntoQuarterTurns(t *testing.T) {
	// A single cubic cannot approximate more than a quarter circle accurately, so
	// larger sweeps have to be subdivided.
	for _, tc := range []struct {
		sweep      float64
		wantCurves int
	}{
		{45, 1},
		{90, 1},
		{91, 2},
		{180, 2},
		{270, 3},
		{360, 4},
		{-360, 4},
	} {
		p := core.NewPath().Arc(core.Position{}, 10, 0, tc.sweep)

		curves := 0
		for _, s := range p.Segments() {
			if s.Op == core.PathCurveTo {
				curves++
			}
		}
		if curves != tc.wantCurves {
			t.Errorf("sweep %.0f produced %d curves, want %d", tc.sweep, curves, tc.wantCurves)
		}
	}
}

// bezierAt evaluates a cubic Bézier at parameter t.
func bezierAt(p0, c1, c2, p3 core.Position, t float64) core.Position {
	u := 1 - t
	w0 := u * u * u
	w1 := 3 * u * u * t
	w2 := 3 * u * t * t
	w3 := t * t * t
	return core.Position{
		X: w0*p0.X + w1*c1.X + w2*c2.X + w3*p3.X,
		Y: w0*p0.Y + w1*c1.Y + w2*c2.Y + w3*p3.Y,
	}
}

func TestArcCurvesTrackTheCircleBetweenEndpoints(t *testing.T) {
	// The endpoints lying on the circle is necessary but not sufficient: a wrong
	// control-point distance would bow the curve inwards or outwards between them.
	// Sampling the Bézier is what actually pins the approximation.
	centre := core.Position{X: 0, Y: 0}
	const radius float64 = 100

	p := core.NewPath().Arc(centre, radius, 0, 360)

	current := core.Position{}
	worst := 0.0

	for _, s := range p.Segments() {
		switch s.Op {
		case core.PathMoveTo:
			current = s.Points[0]
		case core.PathCurveTo:
			for i := 0; i <= 20; i++ {
				at := bezierAt(current, s.Points[0], s.Points[1], s.Points[2], float64(i)/20)
				deviation := math.Abs(math.Hypot(at.X, at.Y) - radius)
				worst = math.Max(worst, deviation)
			}
			current = s.Points[2]
		}
	}

	// A quarter-circle cubic fit deviates by well under a thousandth of the
	// radius, which is far below anything visible at print resolution.
	if worst > radius*0.001 {
		t.Errorf("worst deviation from the circle = %.5f points, want under %.5f",
			worst, radius*0.001)
	}
}

func TestArcSweepsClockwiseInLayoutSpace(t *testing.T) {
	// Layout space has Y growing downwards, so a positive sweep should turn
	// clockwise on the page, matching Canvas.Rotate.
	p := core.NewPath().Arc(core.Position{}, 10, 0, 90)

	points := pointsOnPath(p)
	start, end := points[0], points[len(points)-1]

	// Zero degrees is the positive X axis; a clockwise quarter turn from there
	// lands on the positive Y axis, which is downwards.
	closeTo(t, "start x", start.X, 10)
	closeTo(t, "start y", start.Y, 0)
	closeTo(t, "end x", end.X, 0)
	closeTo(t, "end y", end.Y, 10)
}

func TestNegativeSweepTurnsTheOtherWay(t *testing.T) {
	p := core.NewPath().Arc(core.Position{}, 10, 0, -90)

	points := pointsOnPath(p)
	end := points[len(points)-1]

	closeTo(t, "end x", end.X, 0)
	closeTo(t, "end y", end.Y, -10)
}

func TestDegenerateArcsAddNothing(t *testing.T) {
	for _, tc := range []struct {
		name          string
		radius, sweep float64
	}{
		{"zero radius", 0, 90},
		{"negative radius", -5, 90},
		{"zero sweep", 10, 0},
	} {
		p := core.NewPath().Arc(core.Position{}, tc.radius, 0, tc.sweep)
		if !p.Empty() {
			t.Errorf("%s produced %d segments, want none", tc.name, len(p.Segments()))
		}
	}
}

func TestArcJoinsAnExistingSubpath(t *testing.T) {
	// This is what makes a pie slice one closed path: centre, out to the rim,
	// sweep, close.
	p := core.NewPath().
		MoveTo(core.Position{X: 0, Y: 0}).
		Arc(core.Position{}, 50, 0, 60).
		Close()

	segments := p.Segments()
	if segments[0].Op != core.PathMoveTo {
		t.Fatalf("first op = %v, want MoveTo", segments[0].Op)
	}
	// A line has to bridge the centre to the arc's starting point.
	if segments[1].Op != core.PathLineTo {
		t.Errorf("second op = %v, want LineTo joining the centre to the rim", segments[1].Op)
	}
	if segments[len(segments)-1].Op != core.PathClose {
		t.Error("the slice should be closed")
	}
}

func TestCircleIsClosed(t *testing.T) {
	p := core.NewPath().Circle(core.Position{X: 5, Y: 5}, 3)

	segments := p.Segments()
	if segments[len(segments)-1].Op != core.PathClose {
		t.Error("a circle should be closed")
	}
}

// --- helpers ---------------------------------------------------------------

func TestPolylineAndPolygon(t *testing.T) {
	points := []core.Position{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}

	open := core.Polyline(points...)
	if got := len(open.Segments()); got != 3 {
		t.Errorf("polyline segments = %d, want 3", got)
	}
	for _, s := range open.Segments() {
		if s.Op == core.PathClose {
			t.Error("a polyline must not be closed")
		}
	}

	closed := core.Polygon(points...)
	segments := closed.Segments()
	if segments[len(segments)-1].Op != core.PathClose {
		t.Error("a polygon must be closed")
	}

	// No points means no path, rather than a stray Close.
	if !core.Polygon().Empty() {
		t.Error("an empty polygon should have no segments")
	}
}

func TestRoundedRectGeometry(t *testing.T) {
	p := core.RoundedRect(core.Position{}, core.Size{Width: 100, Height: 60}, 8)

	curves := 0
	for _, s := range p.Segments() {
		if s.Op == core.PathCurveTo {
			curves++
		}
	}
	if curves != 4 {
		t.Errorf("got %d curves, want one per corner", curves)
	}

	segments := p.Segments()
	if segments[len(segments)-1].Op != core.PathClose {
		t.Error("a rounded rectangle should be closed")
	}
}

func TestRoundedRectClampsAndDegrades(t *testing.T) {
	// A radius beyond half the shorter side would self-intersect, so it is clamped
	// to produce a stadium rather than being rejected.
	clamped := core.RoundedRect(core.Position{}, core.Size{Width: 40, Height: 40}, 500)
	curves := 0
	for _, s := range clamped.Segments() {
		if s.Op == core.PathCurveTo {
			curves++
		}
	}
	if curves != 4 {
		t.Errorf("clamped corners = %d, want 4", curves)
	}

	// With no radius it is a plain quadrilateral.
	square := core.RoundedRect(core.Position{}, core.Size{Width: 20, Height: 20}, 0)
	for _, s := range square.Segments() {
		if s.Op == core.PathCurveTo {
			t.Error("a zero radius should produce no curves")
		}
	}

	if !core.RoundedRect(core.Position{}, core.Size{}, 4).Empty() {
		t.Error("a zero-sized rectangle should produce no path")
	}
}

// --- styles ----------------------------------------------------------------

func TestPathStylePredicates(t *testing.T) {
	opaque := core.RGB(0, 0, 0)

	for _, tc := range []struct {
		name                    string
		style                   core.PathStyle
		fills, strokes, visible bool
	}{
		{"fill only", core.Filled(opaque), true, false, true},
		{"stroke only", core.Stroked(opaque, 2), false, true, true},
		{"both", core.PathStyle{Fill: opaque, Stroke: opaque, Width: 1}, true, true, true},
		{"empty", core.PathStyle{}, false, false, false},
		// A stroke colour with no width puts no ink on the page.
		{"zero width stroke", core.PathStyle{Stroke: opaque}, false, false, false},
		{"transparent fill", core.PathStyle{Fill: core.Transparent}, false, false, false},
	} {
		if got := tc.style.Fills(); got != tc.fills {
			t.Errorf("%s: Fills = %v, want %v", tc.name, got, tc.fills)
		}
		if got := tc.style.Strokes(); got != tc.strokes {
			t.Errorf("%s: Strokes = %v, want %v", tc.name, got, tc.strokes)
		}
		if got := tc.style.Visible(); got != tc.visible {
			t.Errorf("%s: Visible = %v, want %v", tc.name, got, tc.visible)
		}
	}
}

func TestDashedIgnoresAllZeroPatterns(t *testing.T) {
	for _, tc := range []struct {
		name string
		dash []float64
		want bool
	}{
		{"nil", nil, false},
		{"empty", []float64{}, false},
		// A pattern of zeros draws nothing at all and is rejected by readers, so
		// it is treated as solid rather than emitted.
		{"all zero", []float64{0, 0}, false},
		{"single", []float64{3}, true},
		{"pair", []float64{4, 2}, true},
		{"leading zero", []float64{0, 3}, true},
	} {
		style := core.PathStyle{Dash: tc.dash}
		if got := style.Dashed(); got != tc.want {
			t.Errorf("%s: Dashed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLineCapAndJoinMatchPDFNumbers(t *testing.T) {
	// The constants are emitted directly as PDF operands, so their numeric values
	// are part of the contract rather than an implementation detail.
	if core.CapButt != 0 || core.CapRound != 1 || core.CapSquare != 2 {
		t.Error("line cap values must match the PDF line cap style numbers")
	}
	if core.JoinMiter != 0 || core.JoinRound != 1 || core.JoinBevel != 2 {
		t.Error("line join values must match the PDF line join style numbers")
	}
}

func TestEvenOddIsOptIn(t *testing.T) {
	// Nonzero winding is the PDF default and the right choice for simple shapes;
	// even-odd is needed only when subpaths have to punch holes in each other.
	if core.Filled(core.RGB(0, 0, 0)).EvenOdd {
		t.Error("even-odd fill must be opt-in")
	}

	ring := core.PathStyle{Fill: core.RGB(0, 0, 0), EvenOdd: true}
	if !ring.EvenOdd || !ring.Fills() {
		t.Error("an even-odd fill should still be a fill")
	}
}
