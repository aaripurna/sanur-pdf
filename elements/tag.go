package elements

import "github.com/aaripurna/sanur-pdf/core"

// Tagged declares what its child means, for the document's logical structure.
//
// Roles that can be inferred are: text is a paragraph, an image is a figure, the content
// of a link is a link. This is for the ones that cannot be. A heading is text that
// happens to be large and bold, and no amount of looking at a font size will reveal
// whether it is a first- or a third-level heading — so a caller says, and a reader gets
// an outline that is right rather than one that is confidently wrong.
//
// It is inert unless the document asked to be tagged, so wrapping content costs nothing
// in an ordinary document.
type Tagged struct {
	// Mark is the structure element to open around the child.
	Mark core.Mark

	Child core.Element
}

func (t *Tagged) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(t.Child, available)
}

// NaturalSize forwards the query: a structure element adds nothing to its child's size.
func (t *Tagged) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(t.Child, available)
}

func (t *Tagged) Draw(canvas core.Canvas, available core.Size) {
	canvas.BeginMarked(t.Mark)
	defer canvas.EndMarked()

	core.DrawChild(t.Child, canvas, available)
}

func (t *Tagged) Children() []core.Element {
	if t.Child == nil {
		return nil
	}
	return []core.Element{t.Child}
}

var (
	_ core.Element   = (*Tagged)(nil)
	_ core.Composite = (*Tagged)(nil)
)
