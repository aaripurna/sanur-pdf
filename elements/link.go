package elements

import "github.com/aaripurna/sanur-pdf/core"

// Link makes its child clickable.
//
// The clickable area is the box the parent allocated, so a link wrapping a whole
// row is clickable across the row while one wrapping just the text is clickable
// only on the words. Nothing is drawn: underlining or colouring a link is left to
// the text style, since a document may reasonably want neither.
type Link struct {
	Target core.LinkTarget
	Child  core.Element

	// Description is what a reader announces in place of the link's own words, for a
	// tagged document. It is worth setting when the words alone do not say where the
	// link goes: a list of "read more" links is unusable to somebody hearing them out
	// of context.
	Description string
}

func (l *Link) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(l.Child, available)
}

// NaturalSize forwards the query, since a link adds nothing to its child's size.
func (l *Link) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(l.Child, available)
}

func (l *Link) Draw(canvas core.Canvas, available core.Size) {
	// The structure element opens before the annotation is recorded, so the annotation
	// knows which element owns it: a reader announcing a link needs to know which words
	// it is on, and an annotation floating free on the page does not say.
	canvas.BeginMarked(core.Mark{Role: core.RoleLink, Alt: l.Description})
	defer canvas.EndMarked()

	// Registered while the structure element is open, so the annotation can be reached
	// from the tree: a reader announcing a link needs to know which words it sits on, and
	// an annotation floating free on the page does not say. The rectangle is the box the
	// layout gave, not whatever ink landed in it, so this does not have to wait for the
	// child to draw.
	canvas.Link(core.Position{}, available, l.Target)

	core.DrawChild(l.Child, canvas, available)
}

func (l *Link) Children() []core.Element {
	if l.Child == nil {
		return nil
	}
	return []core.Element{l.Child}
}

// Anchor registers a named destination at its own top-left corner.
//
// Names are document-wide, and each must be unique: generation fails on a
// duplicate rather than quietly sending every link to whichever came first.
type Anchor struct {
	Name  string
	Child core.Element
}

func (a *Anchor) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(a.Child, available)
}

func (a *Anchor) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(a.Child, available)
}

func (a *Anchor) Draw(canvas core.Canvas, available core.Size) {
	// The destination is registered before the child draws so that it lands on the
	// page the anchor started on. A child that splits across a page break would
	// otherwise leave the anchor pointing at wherever it finished.
	canvas.Destination(a.Name, core.Position{})
	core.DrawChild(a.Child, canvas, available)
}

func (a *Anchor) Children() []core.Element {
	if a.Child == nil {
		return nil
	}
	return []core.Element{a.Child}
}

// Bookmark registers a destination and an outline entry pointing at it.
//
// Level nests the entry: an item becomes a child of the nearest preceding item
// with a lower level, the way document headings do. Entries appear in the outline
// in the order they were drawn, which is document order.
type Bookmark struct {
	Title string

	// Level is the nesting depth, zero being top level.
	Level int

	// Name is the destination name. Leave it empty to derive one from the title,
	// which is enough when nothing needs to link to the bookmark directly.
	Name string

	Child core.Element
}

// destination returns the name this bookmark registers.
func (b *Bookmark) destination() string {
	if b.Name != "" {
		return b.Name
	}
	// Deriving from the title keeps the common case free of invented identifiers.
	// Two bookmarks sharing a title will collide, which generation reports — the
	// fix being to name one of them explicitly.
	return "bookmark:" + b.Title
}

func (b *Bookmark) Measure(available core.Size) core.SpacePlan {
	return core.MeasureChild(b.Child, available)
}

func (b *Bookmark) NaturalSize(available core.Size) core.SpacePlan {
	return core.NaturalSizeOf(b.Child, available)
}

func (b *Bookmark) Draw(canvas core.Canvas, available core.Size) {
	name := b.destination()

	canvas.Destination(name, core.Position{})
	canvas.Bookmark(b.Title, b.Level, name)

	core.DrawChild(b.Child, canvas, available)
}

func (b *Bookmark) Children() []core.Element {
	if b.Child == nil {
		return nil
	}
	return []core.Element{b.Child}
}

var (
	_ core.Element   = (*Link)(nil)
	_ core.Composite = (*Link)(nil)
	_ core.Element   = (*Anchor)(nil)
	_ core.Element   = (*Bookmark)(nil)
)
