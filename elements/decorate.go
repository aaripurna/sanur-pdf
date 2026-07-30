package elements

import "codeberg.org/aaripurna/sanur/core"

// Background paints a filled rectangle behind its child.
//
// The rectangle covers the box the parent allocated, not the child's measured
// size. In a column that means the full column width, which is what makes
// banded rows and full-bleed section headers work without any extra stretching.
type Background struct {
	Color  core.Color
	Radius float64

	Child core.Element
}

func (b *Background) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(b.Child, available)
}

func (b *Background) Draw(canvas core.Canvas, available core.Size) {
	if b.Radius > 0 {
		canvas.DrawRoundedRect(core.Position{}, available, b.Radius, b.Color)
	} else {
		canvas.DrawRect(core.Position{}, available, b.Color)
	}
	core.DrawChild(b.Child, canvas, available)
}

func (b *Background) Children() []core.Element {
	if b.Child == nil {
		return nil
	}
	return []core.Element{b.Child}
}

// BorderSide describes one edge of a border.
type BorderSide struct {
	Width float64
	Color core.Color
}

// Visible reports whether the side would draw anything.
func (s BorderSide) Visible() bool { return s.Width > 0 && s.Color.Visible() }

// Border strokes lines along the edges of its child's box.
//
// Borders are drawn on top of the child rather than beneath it, and inset by half
// their width so the stroke sits fully inside the box. A line stroked exactly on
// the boundary would straddle it, leaving half its thickness outside and
// visually overflowing the layout by a hairline on every edge.
type Border struct {
	Top    BorderSide
	Right  BorderSide
	Bottom BorderSide
	Left   BorderSide

	Child core.Element
}

// UniformBorder builds an equal border on all four edges.
func UniformBorder(width float64, color core.Color, child core.Element) *Border {
	side := BorderSide{Width: width, Color: color}
	return &Border{Top: side, Right: side, Bottom: side, Left: side, Child: child}
}

func (b *Border) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(b.Child, available)
}

func (b *Border) Draw(canvas core.Canvas, available core.Size) {
	core.DrawChild(b.Child, canvas, available)

	w, h := available.Width, available.Height

	if b.Top.Visible() {
		y := b.Top.Width / 2
		canvas.DrawLine(
			core.Position{X: 0, Y: y}, core.Position{X: w, Y: y},
			b.Top.Color, b.Top.Width)
	}
	if b.Bottom.Visible() {
		y := h - b.Bottom.Width/2
		canvas.DrawLine(
			core.Position{X: 0, Y: y}, core.Position{X: w, Y: y},
			b.Bottom.Color, b.Bottom.Width)
	}
	if b.Left.Visible() {
		x := b.Left.Width / 2
		canvas.DrawLine(
			core.Position{X: x, Y: 0}, core.Position{X: x, Y: h},
			b.Left.Color, b.Left.Width)
	}
	if b.Right.Visible() {
		x := w - b.Right.Width/2
		canvas.DrawLine(
			core.Position{X: x, Y: 0}, core.Position{X: x, Y: h},
			b.Right.Color, b.Right.Width)
	}
}

func (b *Border) Children() []core.Element {
	if b.Child == nil {
		return nil
	}
	return []core.Element{b.Child}
}

// Clip confines its child's drawing to the allocated box.
//
// Measurement is unaffected: the child still reports whatever size it wants, and
// clipping only stops the overflow from being painted. That makes it the tool for
// content of unknown length in a fixed-size box, where the alternative would be
// letting it spill over whatever follows.
type Clip struct {
	Child core.Element
}

func (c *Clip) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(c.Child, available)
}

func (c *Clip) Draw(canvas core.Canvas, available core.Size) {
	if c.Child == nil {
		return
	}
	canvas.Save()
	canvas.ClipRect(core.Position{}, available)
	c.Child.Draw(canvas, available)
	canvas.Restore()
}

func (c *Clip) Children() []core.Element {
	if c.Child == nil {
		return nil
	}
	return []core.Element{c.Child}
}

// Rotate turns its child about the top-left corner of its box.
//
// The child is measured against unconstrained space rather than the box it will
// be drawn into, because a rotated element's footprint bears no simple relation
// to its content size. The element reports the box the parent offered and takes
// responsibility for staying inside it.
type Rotate struct {
	Degrees float64
	Child   core.Element
}

func (r *Rotate) Measure(available core.Size) core.SpacePlan {
	if r.Child == nil {
		return core.EmptyRender()
	}
	plan := r.Child.Measure(core.Size{Width: core.Infinity, Height: core.Infinity})
	if plan.Wrapped() {
		return plan
	}
	return core.FullRender(available)
}

func (r *Rotate) Draw(canvas core.Canvas, available core.Size) {
	if r.Child == nil {
		return
	}
	plan := r.Child.Measure(core.Size{Width: core.Infinity, Height: core.Infinity})

	canvas.Save()
	canvas.Rotate(r.Degrees)
	r.Child.Draw(canvas, plan.Size)
	canvas.Restore()
}

func (r *Rotate) Children() []core.Element {
	if r.Child == nil {
		return nil
	}
	return []core.Element{r.Child}
}

var (
	_ core.Element   = (*Background)(nil)
	_ core.Composite = (*Background)(nil)
	_ core.Element   = (*Border)(nil)
	_ core.Element   = (*Clip)(nil)
	_ core.Element   = (*Rotate)(nil)
)
