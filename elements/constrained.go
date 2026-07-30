package elements

import (
	"math"

	"github.com/aaripurna/sanur-pdf/core"
)

// Constrained bounds its child's size.
//
// A zero bound means "unset", which is why these are separate fields rather than
// one Size: a caller fixing only the height must be able to leave the width
// entirely to the content. Exact sizing is expressed by setting a minimum and
// maximum to the same value, which is what Width and Height below do.
type Constrained struct {
	MinWidth  float64
	MaxWidth  float64
	MinHeight float64
	MaxHeight float64

	Child core.Element
}

// FixedWidth constrains a child to an exact width.
func FixedWidth(w float64, child core.Element) *Constrained {
	return &Constrained{MinWidth: w, MaxWidth: w, Child: child}
}

// FixedHeight constrains a child to an exact height.
func FixedHeight(h float64, child core.Element) *Constrained {
	return &Constrained{MinHeight: h, MaxHeight: h, Child: child}
}

// FixedSize constrains a child to an exact width and height.
func FixedSize(w, h float64, child core.Element) *Constrained {
	return &Constrained{MinWidth: w, MaxWidth: w, MinHeight: h, MaxHeight: h, Child: child}
}

func (c *Constrained) Measure(available core.Size) core.SpacePlan {
	return c.resolve(available, core.MeasureChild)
}

// NaturalSize applies the same bounds, but asks the child for its natural size so
// the query composes through a constraint.
func (c *Constrained) NaturalSize(available core.Size) core.SpacePlan {
	return c.resolve(available, core.NaturalSizeOf)
}

// resolve holds the bounding logic shared by Measure and NaturalSize; they differ
// only in how they interrogate the child.
func (c *Constrained) resolve(
	available core.Size,
	measure func(core.Element, core.Size) core.SpacePlan,
) core.SpacePlan {
	// A minimum larger than what the parent has is unsatisfiable here, but may
	// be satisfiable on an emptier page, so it wraps rather than erroring.
	if c.MinWidth > available.Width+core.Epsilon {
		return core.Wrap("minimum width %.1f exceeds the available %.1f",
			c.MinWidth, available.Width)
	}
	if c.MinHeight > available.Height+core.Epsilon {
		return core.Wrap("minimum height %.1f exceeds the available %.1f",
			c.MinHeight, available.Height)
	}

	inner := c.innerSpace(available)

	plan := measure(c.Child, inner)
	if plan.Wrapped() {
		return plan
	}

	size := plan.Size
	if c.MinWidth > 0 {
		size.Width = math.Max(size.Width, c.MinWidth)
	}
	if c.MinHeight > 0 {
		size.Height = math.Max(size.Height, c.MinHeight)
	}
	if c.MaxWidth > 0 {
		size.Width = math.Min(size.Width, c.MaxWidth)
	}
	if c.MaxHeight > 0 {
		size.Height = math.Min(size.Height, c.MaxHeight)
	}

	// Clamping to the minimum can push the box past what the parent offered,
	// even though the minimum itself fit — a child that measured larger than its
	// own maximum, for instance.
	if !size.FitsWithin(available) {
		return core.Wrap("constrained size %.1fx%.1f exceeds the available %.1fx%.1f",
			size.Width, size.Height, available.Width, available.Height)
	}

	return core.SpacePlan{Type: plan.Type, Size: size}
}

// innerSpace is the space offered to the child: the parent's space narrowed by
// any maximum. Minimums do not restrict the child, they only pad the result.
func (c *Constrained) innerSpace(available core.Size) core.Size {
	inner := available
	if c.MaxWidth > 0 {
		inner.Width = math.Min(inner.Width, c.MaxWidth)
	}
	if c.MaxHeight > 0 {
		inner.Height = math.Min(inner.Height, c.MaxHeight)
	}
	return inner
}

func (c *Constrained) Draw(canvas core.Canvas, available core.Size) {
	if c.Child == nil {
		return
	}
	// The child is drawn into the box this element settled on, so a fixed-size
	// container really does hand its child that size even when the child asked
	// for less.
	c.Child.Draw(canvas, c.innerSpace(available))
}

func (c *Constrained) Children() []core.Element {
	if c.Child == nil {
		return nil
	}
	return []core.Element{c.Child}
}

// Extend claims all the space offered on the chosen axes.
//
// This is the counterpart to the cross-axis pass-through rule: elements that hug
// their content stay tight, and one that needs to fill its parent says so here.
// A full-width divider or an item that should stretch to a row's height is an
// Extend around the content.
type Extend struct {
	Horizontal bool
	Vertical   bool

	Child core.Element
}

func (e *Extend) Measure(available core.Size) core.SpacePlan {
	plan := core.MeasureChild(e.Child, available)
	if plan.Wrapped() {
		return plan
	}

	size := plan.Size
	if e.Horizontal {
		size.Width = available.Width
	}
	if e.Vertical {
		size.Height = available.Height
	}
	return core.SpacePlan{Type: plan.Type, Size: size}
}

// NaturalSize reports the child's own size, ignoring this element's tendency to
// fill, so a row can size itself around an extending cell.
//
// As with Aligned, the query descends as a natural-size query so that a child which
// also expands does not undo the answer.
func (e *Extend) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(e.Child, available)
}

func (e *Extend) Draw(canvas core.Canvas, available core.Size) {
	if e.Child == nil {
		return
	}
	e.Child.Draw(canvas, available)
}

func (e *Extend) Children() []core.Element {
	if e.Child == nil {
		return nil
	}
	return []core.Element{e.Child}
}

// Aligned positions a smaller child inside the whole box the parent offered.
//
// It reports the full available size, since it needs the entire box to have
// anything to align against, and then draws the child tight at an offset. That
// makes it the one place where a child is deliberately given less space in Draw
// than its parent had — otherwise the child would expand and there would be
// nothing left to align.
type Aligned struct {
	Horizontal core.HorizontalAlign
	Vertical   core.VerticalAlign

	Child core.Element
}

func (a *Aligned) Measure(available core.Size) core.SpacePlan {
	plan := core.MeasureChild(a.Child, available)
	if plan.Wrapped() {
		return plan
	}

	// Only the axes actually being aligned are expanded. Claiming the full
	// height to align horizontally would make a centred label consume the rest
	// of the page.
	size := plan.Size
	if a.Horizontal != core.AlignLeft {
		size.Width = available.Width
	}
	if a.Vertical != core.AlignTop {
		size.Height = available.Height
	}
	return core.SpacePlan{Type: plan.Type, Size: size}
}

// NaturalSize reports the child's own size, before alignment expands the box.
//
// The query goes down as a natural-size query, not as a Measure. Asking the child
// to Measure would let it expand in turn, which is exactly what happens when the
// alignments are written as a chain: AlignRight().AlignMiddle() nests two of these,
// and the outer one asking the inner to Measure gets the full height back — so a
// row containing one silently became page-tall.
func (a *Aligned) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(a.Child, available)
}

func (a *Aligned) Draw(canvas core.Canvas, available core.Size) {
	if a.Child == nil {
		return
	}
	plan := a.Child.Measure(available)
	if plan.Wrapped() {
		return
	}

	offset := core.Position{
		X: a.Horizontal.OffsetX(available.Width, plan.Size.Width),
		Y: a.Vertical.OffsetY(available.Height, plan.Size.Height),
	}

	canvas.Save()
	canvas.Translate(offset)
	a.Child.Draw(canvas, plan.Size)
	canvas.Restore()
}

func (a *Aligned) Children() []core.Element {
	if a.Child == nil {
		return nil
	}
	return []core.Element{a.Child}
}

var (
	_ core.Element   = (*Constrained)(nil)
	_ core.Composite = (*Constrained)(nil)
	_ core.Element   = (*Extend)(nil)
	_ core.Element   = (*Aligned)(nil)
)
