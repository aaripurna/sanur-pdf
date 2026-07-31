package elements

import (
	"math"

	"github.com/aaripurna/sanur-pdf/core"
)

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

// NaturalSize forwards the query, since a background adds nothing to its child's
// size.
func (b *Background) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(b.Child, available)
}

func (b *Background) Draw(canvas core.Canvas, available core.Size) {
	// Only the fill is decoration, not what sits on it, so the artifact scope closes before
	// the child draws. Wrapping the child too would tell a reader the content is decoration
	// and it would skip a banded table row entirely.
	canvas.BeginMarked(core.Mark{Role: core.RoleArtifact})
	if b.Radius > 0 {
		canvas.DrawRoundedRect(core.Position{}, available, b.Radius, b.Color)
	} else {
		canvas.DrawRect(core.Position{}, available, b.Color)
	}
	canvas.EndMarked()

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

// NaturalSize forwards the query, since a border is drawn on top of its child
// rather than around it.
func (b *Border) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(b.Child, available)
}

func (b *Border) Draw(canvas core.Canvas, available core.Size) {
	core.DrawChild(b.Child, canvas, available)

	// The edges are decoration; the child above is not, which is why the scope opens only
	// now. A rule announced around every table row is worse than no tagging at all.
	canvas.BeginMarked(core.Mark{Role: core.RoleArtifact})
	defer canvas.EndMarked()

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

// Clip crops its child to the box, letting it overflow rather than wrap.
//
// This is the element that makes cropping possible. Most elements refuse space
// they cannot fit into — an image reports a wrap, because there is no sensible
// way to render two thirds of a photograph — which means simply putting one in a
// small box fails the layout instead of trimming it. Clip measures its child
// against unbounded space so the child answers with its natural size, then
// reports only what the parent offered and hides the remainder.
//
// A clipped region never paginates: partial renders from the child are absorbed
// into a full render, since content hidden by a crop is meant to be discarded
// rather than continued on the next page.
type Clip struct {
	Child core.Element
}

// measureChild asks the child for its natural size.
//
// The width is offered as given on the first attempt so that text still wraps to
// the column and width-fitted images still scale to it — unbounding both axes
// would turn every paragraph into one endless line. Only content that cannot
// even fit the width, such as an unscaled image wider than its box, is measured
// against unlimited space so it can be cropped horizontally too.
func (c *Clip) measureChild(available core.Size) core.SpacePlan {
	if c.Child == nil {
		return core.EmptyRender()
	}

	plan := c.Child.Measure(core.Size{Width: available.Width, Height: core.Infinity})
	if !plan.Wrapped() {
		return plan
	}
	return c.Child.Measure(core.Size{Width: core.Infinity, Height: core.Infinity})
}

// contentSize is the size the child is drawn at: its natural size, with any axis
// it merely filled replaced by the box's own extent.
//
// A child that stretches to whatever it is offered — a stretched image, an
// Extend — answers the unbounded probe with the probe value itself. Drawing at
// that size would emit a transform scaled by a billion, which is both meaningless
// and rejected by readers. Such an axis has no natural size and therefore nothing
// to crop, so the box's extent is used instead.
func (c *Clip) contentSize(available core.Size) (core.SpacePlan, core.Size) {
	plan := c.measureChild(available)
	if plan.Wrapped() {
		return plan, core.Size{}
	}

	size := plan.Size
	if size.Width >= core.Infinity {
		size.Width = available.Width
	}
	if size.Height >= core.Infinity {
		size.Height = available.Height
	}
	return plan, size
}

func (c *Clip) Measure(available core.Size) core.SpacePlan {
	plan, size := c.contentSize(available)
	if plan.Wrapped() {
		return plan
	}

	// Never claim more than was offered: the overflow is hidden, not laid out.
	return core.FullRender(core.Size{
		Width:  math.Min(size.Width, available.Width),
		Height: math.Min(size.Height, available.Height),
	})
}

func (c *Clip) Draw(canvas core.Canvas, available core.Size) {
	if c.Child == nil {
		return
	}
	// The child is drawn at its natural size so that what falls outside the box
	// is genuinely cropped rather than scaled to fit.
	plan, size := c.contentSize(available)
	if plan.Wrapped() {
		return
	}

	canvas.Save()
	canvas.ClipRect(core.Position{}, available)
	c.Child.Draw(canvas, size)
	canvas.Restore()
}

func (c *Clip) Children() []core.Element {
	if c.Child == nil {
		return nil
	}
	return []core.Element{c.Child}
}

// Rotate turns its child in place, about the centre of its box.
//
// Rotating about the centre rather than a corner is what "turn this" ordinarily
// means: a stamp across a page, a sideways label in a narrow column. About the
// top-left corner, anything turned more than a few degrees swings out of its own
// box and usually off the page, leaving the caller to work out a compensating
// offset by hand.
//
// The child is measured against unconstrained space rather than the box it will be
// drawn into, because a rotated element's footprint bears no simple relation to its
// content size — which also means text inside a Rotate does not wrap. The element
// reports the box the parent offered and takes responsibility for staying inside it.
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
	if plan.Wrapped() {
		return
	}

	canvas.Save()

	// Move the origin to the centre of the box, turn there, then step back by half
	// the child so it ends up centred on the point it was turned about.
	canvas.Translate(core.Position{X: available.Width / 2, Y: available.Height / 2})
	canvas.Rotate(r.Degrees)
	canvas.Translate(core.Position{
		X: -plan.Size.Width / 2,
		Y: -plan.Size.Height / 2,
	})

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
