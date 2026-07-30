package elements

import "codeberg.org/aaripurna/sanur/core"

// Padding insets its child on each edge.
//
// Unlike most containers, padding is tight on both axes: it reports its child's
// size plus the insets rather than claiming the full width available. A caller
// who wants padding inside a full-width band puts the padding inside the band's
// background, not the other way round.
type Padding struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64

	Child core.Element
}

// UniformPadding builds equal padding on all four edges.
func UniformPadding(value float64, child core.Element) *Padding {
	return &Padding{Top: value, Right: value, Bottom: value, Left: value, Child: child}
}

func (p *Padding) horizontal() float64 { return p.Left + p.Right }
func (p *Padding) vertical() float64   { return p.Top + p.Bottom }

func (p *Padding) Measure(available core.Size) core.SpacePlan {
	inner := available.Shrink(p.horizontal(), p.vertical())

	// Padding that consumes the entire width leaves the child nothing to render
	// into. Reporting this as a wrap rather than silently measuring against zero
	// gives the caller a diagnosable failure instead of an invisible element.
	if available.Width-p.horizontal() < -core.Epsilon ||
		available.Height-p.vertical() < -core.Epsilon {
		return core.Wrap("padding of %.1fx%.1f exceeds the available %.1fx%.1f",
			p.horizontal(), p.vertical(), available.Width, available.Height)
	}

	plan := core.MeasureChild(p.Child, inner)
	if plan.Wrapped() {
		return plan
	}

	size := plan.Size.Grow(p.horizontal(), p.vertical())
	return core.SpacePlan{Type: plan.Type, Size: size}
}

func (p *Padding) Draw(canvas core.Canvas, available core.Size) {
	if p.Child == nil {
		return
	}
	inner := available.Shrink(p.horizontal(), p.vertical())

	canvas.Save()
	canvas.Translate(core.Position{X: p.Left, Y: p.Top})
	p.Child.Draw(canvas, inner)
	canvas.Restore()
}

func (p *Padding) Children() []core.Element {
	if p.Child == nil {
		return nil
	}
	return []core.Element{p.Child}
}

var (
	_ core.Element   = (*Padding)(nil)
	_ core.Composite = (*Padding)(nil)
)
