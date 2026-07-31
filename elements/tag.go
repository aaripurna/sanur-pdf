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

// NewTagged wraps a child in a structure element.
func NewTagged(role core.Role, child core.Element) *Tagged {
	return &Tagged{Mark: core.Mark{Role: role}, Child: child}
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

// Artifact marks its child as carrying no meaning: a rule, a background, a running
// header, a decorative flourish.
//
// Marking these matters as much as tagging the content. A conforming tagged document has
// no third category — anything not marked as an artifact is content a reader must
// announce, and a horizontal rule announced between every paragraph makes a document
// worse than an untagged one.
type Artifact struct {
	Child core.Element
}

// NewArtifact marks a child as decoration.
func NewArtifact(child core.Element) *Artifact {
	return &Artifact{Child: child}
}

func (a *Artifact) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(a.Child, available)
}

func (a *Artifact) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(a.Child, available)
}

func (a *Artifact) Draw(canvas core.Canvas, available core.Size) {
	canvas.BeginMarked(core.Mark{Role: core.RoleArtifact})
	defer canvas.EndMarked()

	core.DrawChild(a.Child, canvas, available)
}

func (a *Artifact) Children() []core.Element {
	if a.Child == nil {
		return nil
	}
	return []core.Element{a.Child}
}

var (
	_ core.Element   = (*Tagged)(nil)
	_ core.Composite = (*Tagged)(nil)
	_ core.Element   = (*Artifact)(nil)
	_ core.Composite = (*Artifact)(nil)
)
