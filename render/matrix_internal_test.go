package render

import (
	"math"
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
)

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}

func TestIdentityChangesNothing(t *testing.T) {
	p := identity().apply(core.Position{X: 12, Y: 34})

	closeTo(t, "x", p.X, 12)
	closeTo(t, "y", p.Y, 34)
}

func TestTranslationMovesTheOrigin(t *testing.T) {
	p := translation(10, 20).apply(core.Position{})

	closeTo(t, "x", p.X, 10)
	closeTo(t, "y", p.Y, 20)
}

func TestNestedTranslationsAccumulate(t *testing.T) {
	// This is the order that is easy to get backwards: a transform applied on top
	// of an existing one acts first on the local point, so the composition is
	// new × existing.
	m := identity()
	m = translation(10, 20).mul(m)
	m = translation(5, 3).mul(m)

	origin := m.apply(core.Position{})
	closeTo(t, "origin x", origin.X, 15)
	closeTo(t, "origin y", origin.Y, 23)

	// A point inside the innermost space is offset by the whole chain.
	inner := m.apply(core.Position{X: 2, Y: 1})
	closeTo(t, "inner x", inner.X, 17)
	closeTo(t, "inner y", inner.Y, 24)
}

func TestRotationTurnsClockwiseInLayoutSpace(t *testing.T) {
	// Layout space grows downwards, so a positive angle turns clockwise on the
	// page, matching Canvas.Rotate.
	p := rotation(90).apply(core.Position{X: 10, Y: 0})

	closeTo(t, "x", p.X, 0)
	closeTo(t, "y", p.Y, 10)
}

func TestRotationComposesWithTranslation(t *testing.T) {
	// Translate then rotate: the rotation happens in the translated frame, so the
	// rotated point is measured from the new origin.
	m := identity()
	m = translation(100, 50).mul(m)
	m = rotation(90).mul(m)

	p := m.apply(core.Position{X: 10, Y: 0})
	closeTo(t, "x", p.X, 100)
	closeTo(t, "y", p.Y, 60)
}

func TestBoundsMapsEveryCorner(t *testing.T) {
	// Two opposite corners are not enough once a rotation is involved: the extent
	// of the rotated quadrilateral depends on all four.
	m := rotation(45)

	x0, y0, x1, y1 := m.bounds(core.Position{}, core.Size{Width: 10, Height: 10})

	// A square turned 45 degrees spans its diagonal on both axes.
	diagonal := 10 * math.Sqrt2
	closeTo(t, "width", x1-x0, diagonal)
	closeTo(t, "height", y1-y0, diagonal)
}

func TestBoundsOfAnUnrotatedRect(t *testing.T) {
	m := translation(30, 40)

	x0, y0, x1, y1 := m.bounds(core.Position{X: 5, Y: 5}, core.Size{Width: 20, Height: 10})

	closeTo(t, "x0", x0, 35)
	closeTo(t, "y0", y0, 45)
	closeTo(t, "x1", x1, 55)
	closeTo(t, "y1", y1, 55)
}

// --- the coordinate flip ----------------------------------------------------

func TestPageRectFlipsToPDFSpace(t *testing.T) {
	// Layout space measures down from the top of the page; a PDF annotation
	// rectangle measures up from the bottom. The top and bottom edges therefore
	// swap as well as move, which is the step most easily got wrong.
	b := NewBuilder(Metadata{}, false)
	c := b.NewPage(core.Size{Width: 600, Height: 800})

	rect := c.pageRect(core.Position{X: 10, Y: 100}, core.Size{Width: 50, Height: 20})

	closeTo(t, "x0", rect[0], 10)
	// The layout box spans y 100..120 from the top, which is 680..700 from the
	// bottom of an 800-point page.
	closeTo(t, "y0", rect[1], 680)
	closeTo(t, "x1", rect[2], 60)
	closeTo(t, "y1", rect[3], 700)
}

func TestPageRectFollowsNestedTranslations(t *testing.T) {
	b := NewBuilder(Metadata{}, false)
	c := b.NewPage(core.Size{Width: 600, Height: 800})

	c.Save()
	c.Translate(core.Position{X: 40, Y: 60})
	c.Save()
	c.Translate(core.Position{X: 10, Y: 5})

	rect := c.pageRect(core.Position{}, core.Size{Width: 30, Height: 10})

	// The element sits 50 across and 65 down from the page's top-left corner.
	closeTo(t, "x0", rect[0], 50)
	closeTo(t, "y0", rect[1], 800-75)
	closeTo(t, "x1", rect[2], 80)
	closeTo(t, "y1", rect[3], 800-65)

	c.Restore()
	c.Restore()

	// After restoring, a rectangle is back in page coordinates.
	rect = c.pageRect(core.Position{}, core.Size{Width: 30, Height: 10})
	closeTo(t, "restored x0", rect[0], 0)
	closeTo(t, "restored y1", rect[3], 800)
}

func TestRestoreRewindsTheTransform(t *testing.T) {
	b := NewBuilder(Metadata{}, false)
	c := b.NewPage(core.Size{Width: 600, Height: 800})

	c.Save()
	c.Translate(core.Position{X: 100, Y: 100})
	c.Restore()

	// A leaked transform would put every later annotation in the wrong place, and
	// nothing in the drawn output would show it.
	rect := c.pageRect(core.Position{}, core.Size{Width: 10, Height: 10})
	closeTo(t, "x0", rect[0], 0)
	closeTo(t, "y1", rect[3], 800)
}

// --- outline tree -----------------------------------------------------------

// tree renders a node tree as an indented string, so nesting can be asserted
// compactly rather than by walking pointers.
func tree(nodes []*outlineNode, depth int) string {
	out := ""
	for _, n := range nodes {
		for i := 0; i < depth; i++ {
			out += "  "
		}
		out += n.entry.title + "\n"
		out += tree(n.children, depth+1)
	}
	return out
}

func TestOutlineTreeNestsByLevel(t *testing.T) {
	got := tree(buildOutlineTree([]bookmark{
		{title: "One", level: 0},
		{title: "One.A", level: 1},
		{title: "One.B", level: 1},
		{title: "One.B.i", level: 2},
		{title: "Two", level: 0},
		{title: "Two.A", level: 1},
	}), 0)

	want := "One\n  One.A\n  One.B\n    One.B.i\nTwo\n  Two.A\n"
	if got != want {
		t.Errorf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestOutlineTreeAttachesSkippedLevels(t *testing.T) {
	// A level that jumps by more than one is a formatting slip. Attaching the entry
	// to the nearest open ancestor keeps the bookmark; discarding it would lose
	// navigation over a typo.
	got := tree(buildOutlineTree([]bookmark{
		{title: "One", level: 0},
		{title: "Deep", level: 3},
	}), 0)

	if want := "One\n  Deep\n"; got != want {
		t.Errorf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestOutlineTreeHandlesLeadingIndent(t *testing.T) {
	// A document whose first bookmark is nested has no parent to nest under, so it
	// becomes a root rather than being dropped.
	got := tree(buildOutlineTree([]bookmark{
		{title: "Indented", level: 2},
		{title: "Root", level: 0},
	}), 0)

	if want := "Indented\nRoot\n"; got != want {
		t.Errorf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestOutlineTreeReturnsToShallowerLevels(t *testing.T) {
	got := tree(buildOutlineTree([]bookmark{
		{title: "A", level: 0},
		{title: "A.1", level: 1},
		{title: "A.1.a", level: 2},
		{title: "A.2", level: 1},
		{title: "B", level: 0},
	}), 0)

	want := "A\n  A.1\n    A.1.a\n  A.2\nB\n"
	if got != want {
		t.Errorf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestEmptyOutlineTree(t *testing.T) {
	if got := buildOutlineTree(nil); len(got) != 0 {
		t.Errorf("got %d roots for no bookmarks", len(got))
	}
}
