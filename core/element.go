package core

// Element is anything that can be laid out and drawn.
//
// Rendering a document is a two-phase process per page. Measure asks the
// element what it would do with a given amount of space, without producing any
// output; Draw then commits to that answer. The document loop only calls Draw
// after a Measure it is happy with, so an element may assume Draw always
// receives space it has already approved.
//
// Measure must be free of side effects that alter the element's own answer:
// calling it twice with the same available space must return the same plan.
// Elements that carry progress across pages (text, columns) advance that
// progress in Draw, never in Measure.
type Element interface {
	Measure(available Size) SpacePlan
	Draw(canvas Canvas, available Size)
}

// Composite is implemented by elements that own children. The document loop
// walks the tree through this interface to reset state and inject context, so a
// custom container only has to expose its children to take part in pagination
// and page numbering.
type Composite interface {
	Children() []Element
}

// StateResettable is implemented by elements that remember how far they have
// rendered.
//
// The generation loop measures speculatively, and a page may be laid out more
// than once — a two-pass run for total page counts repeats every page. Without
// a reset hook, an element that consumed half its text on the discarded pass
// would resume from the wrong place on the real one.
type StateResettable interface {
	// ResetState rewinds rendering progress. A hard reset returns the element to
	// its initial state as if never drawn; a soft reset only discards progress
	// made since the last page boundary.
	ResetState(hard bool)
}

// ContextAware is implemented by elements whose content depends on where they
// land in the finished document, such as a page-number label.
type ContextAware interface {
	SetPageContext(ctx PageContext)
}

// PageContext describes the page an element is being rendered onto.
//
// TotalPages is only known once the whole document has been laid out, which is
// why generation runs twice: the first pass counts pages with TotalPages unset,
// the second renders with the real figure. Elements must therefore tolerate a
// zero TotalPages during the counting pass.
type PageContext struct {
	// PageNumber is 1-based.
	PageNumber int

	// TotalPages is zero during the counting pass.
	TotalPages int
}

// ResetTree hard- or soft-resets an element and everything beneath it.
func ResetTree(e Element, hard bool) {
	if e == nil {
		return
	}
	if r, ok := e.(StateResettable); ok {
		r.ResetState(hard)
	}
	if c, ok := e.(Composite); ok {
		for _, child := range c.Children() {
			ResetTree(child, hard)
		}
	}
}

// ApplyPageContext pushes ctx into every ContextAware element in the tree.
func ApplyPageContext(e Element, ctx PageContext) {
	if e == nil {
		return
	}
	if a, ok := e.(ContextAware); ok {
		a.SetPageContext(ctx)
	}
	if c, ok := e.(Composite); ok {
		for _, child := range c.Children() {
			ApplyPageContext(child, ctx)
		}
	}
}

// MeasureChild measures child, treating a nil child as empty. Containers use it
// so an unpopulated slot degrades to zero size instead of panicking.
func MeasureChild(child Element, available Size) SpacePlan {
	if child == nil {
		return EmptyRender()
	}
	return child.Measure(available)
}

// DrawChild draws child unless it is nil.
func DrawChild(child Element, canvas Canvas, available Size) {
	if child == nil {
		return
	}
	child.Draw(canvas, available)
}
