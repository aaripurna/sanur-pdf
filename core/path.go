package core

import "math"

// PathOp identifies one step in a path.
type PathOp int

const (
	// PathMoveTo starts a new subpath at Points[0].
	PathMoveTo PathOp = iota

	// PathLineTo draws a straight line to Points[0].
	PathLineTo

	// PathCurveTo draws a cubic Bézier: Points[0] and Points[1] are the control
	// points, Points[2] the endpoint.
	PathCurveTo

	// PathClose closes the current subpath back to its start.
	PathClose
)

// PathSegment is one step of a path.
type PathSegment struct {
	Op PathOp

	// Points carries the segment's coordinates: one for MoveTo and LineTo, three
	// for CurveTo, none for Close.
	Points [3]Position
}

// Path is a sequence of lines and curves in layout coordinates.
//
// It is a value built independently of any canvas, rather than a series of calls
// into one. That keeps canvases free of path state that could be left
// unbalanced, lets a shape be built once and drawn repeatedly, and makes path
// geometry testable without rendering anything — arcs in particular, where the
// Bézier approximation is worth checking numerically.
//
// Paths carry no styling. How a path is painted is decided by the PathStyle
// handed to Canvas.DrawPath, so the same outline can be filled, stroked, or both.
type Path struct {
	segments []PathSegment
}

// NewPath creates an empty path.
func NewPath() *Path { return &Path{} }

// Segments returns the path's steps, for a canvas to emit.
func (p *Path) Segments() []PathSegment {
	if p == nil {
		return nil
	}
	return p.segments
}

// Empty reports whether the path would draw nothing.
func (p *Path) Empty() bool { return p == nil || len(p.segments) == 0 }

// MoveTo starts a new subpath.
func (p *Path) MoveTo(to Position) *Path {
	p.segments = append(p.segments, PathSegment{Op: PathMoveTo, Points: [3]Position{to}})
	return p
}

// LineTo extends the current subpath with a straight line.
//
// A LineTo with no preceding MoveTo is promoted to a MoveTo, so a path built from
// a loop over points needs no special case for the first one.
func (p *Path) LineTo(to Position) *Path {
	if len(p.segments) == 0 {
		return p.MoveTo(to)
	}
	p.segments = append(p.segments, PathSegment{Op: PathLineTo, Points: [3]Position{to}})
	return p
}

// CurveTo extends the current subpath with a cubic Bézier.
func (p *Path) CurveTo(control1, control2, to Position) *Path {
	if len(p.segments) == 0 {
		p.MoveTo(control1)
	}
	p.segments = append(p.segments, PathSegment{
		Op:     PathCurveTo,
		Points: [3]Position{control1, control2, to},
	})
	return p
}

// Close closes the current subpath.
func (p *Path) Close() *Path {
	if len(p.segments) == 0 {
		return p
	}
	p.segments = append(p.segments, PathSegment{Op: PathClose})
	return p
}

// maxArcSweep is the largest sweep approximated by a single cubic Bézier.
//
// A quarter turn is the conventional limit: the error of a cubic fit to a
// circular arc grows steeply beyond it, and splitting at 90 degrees keeps the
// deviation well under a thousandth of the radius — far below anything visible at
// print resolution.
const maxArcSweep = 90.0

// Arc appends a circular arc centred on centre.
//
// Angles are in degrees, measured from the positive X axis. Because layout space
// has Y growing downwards, a positive sweep turns clockwise on the page, matching
// Canvas.Rotate.
//
// If the path is already started, a straight line joins the current point to the
// arc's start. That is what makes a pie slice expressible as one closed path:
// move to the centre, line out to the rim, sweep, and close.
func (p *Path) Arc(centre Position, radius, startDegrees, sweepDegrees float64) *Path {
	if radius <= 0 || sweepDegrees == 0 {
		return p
	}

	steps := int(math.Ceil(math.Abs(sweepDegrees) / maxArcSweep))
	step := sweepDegrees / float64(steps)

	at := func(degrees float64) Position {
		r := degrees * math.Pi / 180
		return Position{
			X: centre.X + radius*math.Cos(r),
			Y: centre.Y + radius*math.Sin(r),
		}
	}

	if len(p.segments) == 0 {
		p.MoveTo(at(startDegrees))
	} else {
		p.LineTo(at(startDegrees))
	}

	// The control points of a Bézier fitting a sweep of theta sit this far along
	// the tangents at each end. The factor is exact for the endpoints and
	// tangents to coincide with the circle; a negative sweep yields a negative
	// distance, which flips the controls correctly.
	theta := step * math.Pi / 180
	reach := 4.0 / 3.0 * math.Tan(theta/4) * radius

	for i := 0; i < steps; i++ {
		from := startDegrees + step*float64(i)
		to := from + step

		fromRad := from * math.Pi / 180
		toRad := to * math.Pi / 180

		start, end := at(from), at(to)

		// The tangent at angle a is (-sin a, cos a): the derivative of the point
		// on the circle with respect to the angle.
		p.CurveTo(
			Position{X: start.X - reach*math.Sin(fromRad), Y: start.Y + reach*math.Cos(fromRad)},
			Position{X: end.X + reach*math.Sin(toRad), Y: end.Y - reach*math.Cos(toRad)},
			end,
		)
	}
	return p
}

// Circle appends a full closed circle.
func (p *Path) Circle(centre Position, radius float64) *Path {
	return p.Arc(centre, radius, 0, 360).Close()
}

// Polyline builds an open path through the points.
func Polyline(points ...Position) *Path {
	path := NewPath()
	for _, pt := range points {
		path.LineTo(pt)
	}
	return path
}

// Polygon builds a closed path through the points.
func Polygon(points ...Position) *Path {
	path := Polyline(points...)
	if !path.Empty() {
		path.Close()
	}
	return path
}

// kappa is the control-point ratio that makes a cubic Bézier approximate a
// quarter circle, which is the reach formula above evaluated at 90 degrees.
const kappa = 0.55228475

// RoundedRect builds a closed rounded rectangle.
//
// The radius is clamped to half the shorter side: anything larger would make
// opposite corners overlap and the outline self-intersect.
func RoundedRect(pos Position, size Size, radius float64) *Path {
	radius = math.Min(radius, math.Min(size.Width, size.Height)/2)

	path := NewPath()
	if size.Width <= 0 || size.Height <= 0 {
		return path
	}

	x, y := pos.X, pos.Y
	w, h := size.Width, size.Height

	if radius <= 0 {
		return Polygon(
			Position{X: x, Y: y},
			Position{X: x + w, Y: y},
			Position{X: x + w, Y: y + h},
			Position{X: x, Y: y + h},
		)
	}

	reach := radius * kappa

	path.MoveTo(Position{X: x + radius, Y: y})
	path.LineTo(Position{X: x + w - radius, Y: y})
	path.CurveTo(
		Position{X: x + w - radius + reach, Y: y},
		Position{X: x + w, Y: y + radius - reach},
		Position{X: x + w, Y: y + radius})
	path.LineTo(Position{X: x + w, Y: y + h - radius})
	path.CurveTo(
		Position{X: x + w, Y: y + h - radius + reach},
		Position{X: x + w - radius + reach, Y: y + h},
		Position{X: x + w - radius, Y: y + h})
	path.LineTo(Position{X: x + radius, Y: y + h})
	path.CurveTo(
		Position{X: x + radius - reach, Y: y + h},
		Position{X: x, Y: y + h - radius + reach},
		Position{X: x, Y: y + h - radius})
	path.LineTo(Position{X: x, Y: y + radius})
	path.CurveTo(
		Position{X: x, Y: y + radius - reach},
		Position{X: x + radius - reach, Y: y},
		Position{X: x + radius, Y: y})

	return path.Close()
}

// LineCap selects how the ends of a stroke are finished. The values match the
// PDF line cap style numbers so no translation is needed on the way out.
type LineCap int

const (
	// CapButt ends the stroke square at the endpoint.
	CapButt LineCap = 0

	// CapRound ends it with a semicircle centred on the endpoint.
	CapRound LineCap = 1

	// CapSquare extends it half a line width past the endpoint.
	CapSquare LineCap = 2
)

// LineJoin selects how corners of a stroke are finished. The values match the PDF
// line join style numbers.
type LineJoin int

const (
	// JoinMiter extends the outer edges until they meet, falling back to a bevel
	// once the spike would exceed MiterLimit.
	JoinMiter LineJoin = 0

	// JoinRound fills the corner with an arc.
	JoinRound LineJoin = 1

	// JoinBevel cuts the corner off with a straight edge.
	JoinBevel LineJoin = 2
)

// DefaultMiterLimit is the PDF default: corners sharper than about 11 degrees
// are bevelled rather than growing an unbounded spike.
const DefaultMiterLimit = 10

// PathStyle describes how a path is painted.
//
// A single style covering both fill and stroke keeps the Canvas interface to one
// path method and matches PDF, which has a single fill-and-stroke operator. An
// invisible Fill or Stroke simply omits that half, so the same struct expresses
// fill-only, stroke-only, and both.
type PathStyle struct {
	// Fill paints the interior. Leave it transparent for an outline only.
	Fill Color

	// Stroke paints the outline. Leave it transparent, or Width at zero, for a
	// fill only.
	Stroke Color

	// Width is the stroke width in points.
	Width float64

	Cap  LineCap
	Join LineJoin

	// MiterLimit bounds how far a mitred corner may spike. Zero means the PDF
	// default.
	MiterLimit float64

	// Dash alternates on and off lengths in points; nil or empty draws a solid
	// line. A single value means equal dashes and gaps.
	Dash []float64

	// DashPhase is how far into the pattern to start.
	DashPhase float64

	// EvenOdd fills using the even-odd rule instead of the default nonzero
	// winding rule.
	//
	// This is what makes a ring possible. Under nonzero winding, two circles
	// traced the same way both contribute +1 to the region between them, so the
	// middle counts as inside and fills solid. Even-odd counts crossings instead,
	// so the inner circle punches a hole regardless of which way either was
	// traced. The alternative — reversing the inner subpath with a negative sweep
	// so the windings cancel — works too, but requires the caller to reason about
	// direction, which is exactly the detail this flag removes.
	EvenOdd bool
}

// Fills reports whether the style paints an interior.
func (s PathStyle) Fills() bool { return s.Fill.Visible() }

// Strokes reports whether the style paints an outline.
func (s PathStyle) Strokes() bool { return s.Stroke.Visible() && s.Width > 0 }

// Dashed reports whether a dash pattern applies.
//
// A pattern whose lengths are all zero would produce no visible line at all and
// is rejected by readers, so it is treated as solid.
func (s PathStyle) Dashed() bool {
	for _, d := range s.Dash {
		if d > 0 {
			return true
		}
	}
	return false
}

// Visible reports whether the style would put any ink on the page.
func (s PathStyle) Visible() bool { return s.Fills() || s.Strokes() }

// Filled builds a fill-only style.
func Filled(fill Color) PathStyle { return PathStyle{Fill: fill} }

// Stroked builds a stroke-only style.
func Stroked(stroke Color, width float64) PathStyle {
	return PathStyle{Stroke: stroke, Width: width}
}
