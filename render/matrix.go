package render

import (
	"math"

	"github.com/aaripurna/sanur-pdf/core"
)

// matrix is a 2D affine transform in PDF's own layout: the six significant
// entries of
//
//	| a b 0 |
//	| c d 0 |
//	| e f 1 |
//
// with points treated as row vectors, so a point is transformed by p × M and
// transforms compose left to right in the order they are applied.
//
// The canvas keeps one of these alongside the operators it emits. Drawing alone
// needs no such bookkeeping — a cm operator is enough for the reader — but a link
// annotation's rectangle is stated in absolute page coordinates while the element
// that wants the link is several nested translations deep and knows only its own.
// Tracking the transform in Go is what bridges the two.
type matrix [6]float64

// identity is the transform that changes nothing.
func identity() matrix {
	return matrix{1, 0, 0, 1, 0, 0}
}

// translation moves the origin.
func translation(dx, dy float64) matrix {
	return matrix{1, 0, 0, 1, dx, dy}
}

// rotation turns the axes clockwise by the given degrees, which is what a
// positive angle means in a Y-down space.
func rotation(degrees float64) matrix {
	rad := degrees * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	return matrix{cos, sin, -sin, cos, 0, 0}
}

// mul returns m followed by n.
//
// The order matters and is easy to get backwards: applying a new transform on top
// of an existing one means the new one acts first on the local point, so the
// composition is new × existing, not the reverse.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

// apply transforms a point.
func (m matrix) apply(p core.Position) core.Position {
	return core.Position{
		X: m[0]*p.X + m[2]*p.Y + m[4],
		Y: m[1]*p.X + m[3]*p.Y + m[5],
	}
}

// bounds returns the axis-aligned box enclosing a local rectangle once
// transformed.
//
// All four corners are mapped rather than just two, because a rotation turns a
// rectangle into a quadrilateral whose extent is not derivable from opposite
// corners alone. PDF link rectangles are axis-aligned, so the enclosing box is
// the closest available answer for rotated content — a slightly generous
// clickable area rather than a misplaced one.
func (m matrix) bounds(pos core.Position, size core.Size) (x0, y0, x1, y1 float64) {
	corners := [4]core.Position{
		m.apply(pos),
		m.apply(core.Position{X: pos.X + size.Width, Y: pos.Y}),
		m.apply(core.Position{X: pos.X + size.Width, Y: pos.Y + size.Height}),
		m.apply(core.Position{X: pos.X, Y: pos.Y + size.Height}),
	}

	x0, y0 = corners[0].X, corners[0].Y
	x1, y1 = x0, y0

	for _, c := range corners[1:] {
		x0 = math.Min(x0, c.X)
		y0 = math.Min(y0, c.Y)
		x1 = math.Max(x1, c.X)
		y1 = math.Max(y1, c.Y)
	}
	return x0, y0, x1, y1
}
