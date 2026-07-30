package core

import "math"

// Epsilon is the tolerance used for all layout comparisons. Sizes are measured
// in PDF points, so a thousandth of a point is far below anything visible and
// safely absorbs float64 accumulation error from nested containers.
const Epsilon = 0.001

// Infinity is the size handed to an element when a dimension is unconstrained,
// such as the cross axis of an auto-sized row item.
const Infinity = 1e9

// Size is a width/height pair in PDF points (1/72 inch).
type Size struct {
	Width  float64
	Height float64
}

// Position is an offset in PDF points from the current canvas origin. Sanur
// works in a top-left origin space with Y growing downwards, matching how
// layout is naturally expressed; the PDF canvas flips it on the way out.
type Position struct {
	X float64
	Y float64
}

// Add returns the position translated by dx, dy.
func (p Position) Add(dx, dy float64) Position {
	return Position{X: p.X + dx, Y: p.Y + dy}
}

// Shrink returns the size reduced by dw, dh, clamped at zero so an
// over-large padding can never produce a negative available space.
func (s Size) Shrink(dw, dh float64) Size {
	return Size{
		Width:  math.Max(0, s.Width-dw),
		Height: math.Max(0, s.Height-dh),
	}
}

// Grow returns the size expanded by dw, dh.
func (s Size) Grow(dw, dh float64) Size {
	return Size{Width: s.Width + dw, Height: s.Height + dh}
}

// FitsWithin reports whether s fits inside available, within Epsilon. Elements
// use this to decide between FullRender and Wrap.
func (s Size) FitsWithin(available Size) bool {
	return s.Width <= available.Width+Epsilon &&
		s.Height <= available.Height+Epsilon
}

// IsEmpty reports whether both dimensions are negligible.
func (s Size) IsEmpty() bool {
	return s.Width < Epsilon && s.Height < Epsilon
}
