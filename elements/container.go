package elements

import "codeberg.org/aaripurna/sanur/core"

// Container is a single-child pass-through.
//
// It exists so the fluent API has somewhere to put a child that has not been
// declared yet: `page.Content()` hands back a slot, and whatever the caller
// chains onto it is installed here later. Measurement and drawing delegate
// straight through, so an empty slot costs nothing but also breaks nothing.
type Container struct {
	Child core.Element
}

// NewContainer creates an empty container.
func NewContainer() *Container { return &Container{} }

// Set installs the child, replacing any previous one.
func (c *Container) Set(child core.Element) { c.Child = child }

func (c *Container) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(c.Child, available)
}

// NaturalSize forwards the query so a row can size itself around whatever ends
// up in this slot.
//
// Every pass-through decorator has to forward this. A row asks its cells for
// their natural size, and a cell is rarely a bare aligned element — it is far
// more often a background wrapping padding wrapping the alignment. If any link in
// that chain answered with Measure instead, the vertically greedy element beneath
// would claim the whole page and take the row with it.
func (c *Container) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(c.Child, available)
}

func (c *Container) Draw(canvas core.Canvas, available core.Size) {
	core.DrawChild(c.Child, canvas, available)
}

func (c *Container) Children() []core.Element {
	if c.Child == nil {
		return nil
	}
	return []core.Element{c.Child}
}

// Empty occupies no space and draws nothing.
type Empty struct{}

func (Empty) Measure(core.Size) core.SpacePlan { return core.EmptyRender() }
func (Empty) Draw(core.Canvas, core.Size)      {}

// ShowIf renders its child only when Condition holds.
//
// The condition is evaluated in Measure as well as Draw, so a hidden child
// contributes nothing to the layout rather than reserving invisible space.
type ShowIf struct {
	Condition bool
	Child     core.Element
}

func (s *ShowIf) Measure(available core.Size) core.SpacePlan {
	if !s.Condition {
		return core.EmptyRender()
	}
	return core.MeasureChild(s.Child, available)
}

// NaturalSize reports nothing while hidden, and the child's natural size
// otherwise.
func (s *ShowIf) NaturalSize(available core.Size) core.SpacePlan {
	if !s.Condition {
		return core.EmptyRender()
	}
	return core.NaturalSizeOf(s.Child, available)
}

func (s *ShowIf) Draw(canvas core.Canvas, available core.Size) {
	if !s.Condition {
		return
	}
	core.DrawChild(s.Child, canvas, available)
}

func (s *ShowIf) Children() []core.Element {
	if s.Child == nil {
		return nil
	}
	return []core.Element{s.Child}
}

var (
	_ core.Element   = (*Container)(nil)
	_ core.Composite = (*Container)(nil)
	_ core.Element   = Empty{}
	_ core.Element   = (*ShowIf)(nil)
)
