package elements

import "github.com/aaripurna/sanur-pdf/core"

// Layers draws several elements into the same box, back to front.
//
// PDF has no z-index: things appear in the order they are painted, and everything
// else in this library lays elements out side by side or one after another. Layers
// is the exception that lets them overlap — a watermark behind a page, a "DRAFT"
// stamp over it, a badge in the corner of a panel.
//
// Content alone decides the size. Below and Above are measured against the same
// box and drawn at whatever Content settled on, which is what keeps a deliberately
// oversized watermark from stretching the layout around it. Without that rule, the
// largest layer would win and a decoration would dictate the geometry.
type Layers struct {
	// Below is painted first, so it sits behind everything else.
	Below []core.Element

	// Content determines the size of the whole stack.
	Content core.Element

	// Above is painted last, over the content.
	Above []core.Element
}

// Behind builds a stack with one element behind the content.
func Behind(background, content core.Element) *Layers {
	return &Layers{Below: []core.Element{background}, Content: content}
}

// Over builds a stack with one element over the content.
func Over(content, foreground core.Element) *Layers {
	return &Layers{Content: content, Above: []core.Element{foreground}}
}

func (l *Layers) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(l.Content, available)
}

// NaturalSize forwards to the content, since the decorative layers do not
// contribute to the size.
func (l *Layers) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(l.Content, available)
}

func (l *Layers) Draw(canvas core.Canvas, available core.Size) {
	// Every layer fills the box the parent allocated, which is the same rule the
	// rest of the library follows: Measure reports what an element needs, Draw
	// fills what it was given.
	//
	// Re-measuring Content here to size the layers would be wrong twice over. It
	// would collapse a decoration around content that reports no size of its own,
	// and it would disagree with the box the parent already decided on from this
	// element's own Measure — which is where an oversized decoration is prevented
	// from stretching the layout.
	for _, layer := range l.Below {
		l.drawLayer(canvas, layer, available)
	}

	core.DrawChild(l.Content, canvas, available)

	for _, layer := range l.Above {
		l.drawLayer(canvas, layer, available)
	}
}

// drawLayer paints one decorative layer, isolated so that a layer which clips or
// transforms cannot disturb the ones after it.
func (l *Layers) drawLayer(canvas core.Canvas, layer core.Element, box core.Size) {
	if layer == nil {
		return
	}
	canvas.Save()
	layer.Draw(canvas, box)
	canvas.Restore()
}

func (l *Layers) Children() []core.Element {
	children := make([]core.Element, 0, len(l.Below)+len(l.Above)+1)

	children = append(children, l.Below...)
	if l.Content != nil {
		children = append(children, l.Content)
	}
	children = append(children, l.Above...)

	return children
}

var (
	_ core.Element   = (*Layers)(nil)
	_ core.Composite = (*Layers)(nil)
)
