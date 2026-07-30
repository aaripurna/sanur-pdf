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
