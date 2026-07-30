package elements

import (
	"math"

	"codeberg.org/aaripurna/sanur/core"
)

// Column stacks children vertically and is the element that makes documents
// longer than one page work.
//
// It reports the full available width, so children inherit the column's width
// rather than the column shrinking to its widest child. Vertically it is the
// opposite: it grows to whatever its children need, and when they need more than
// one page it splits.
type Column struct {
	Items   []core.Element
	Spacing float64

	// rendered is the index of the first item not yet finished. It is the whole
	// of the column's pagination state: a value greater than zero means earlier
	// items are already on previous pages and must not be drawn again.
	rendered int
}

// NewColumn builds a column from items.
func NewColumn(spacing float64, items ...core.Element) *Column {
	return &Column{Items: items, Spacing: spacing}
}

// Add appends an item.
func (c *Column) Add(item core.Element) { c.Items = append(c.Items, item) }

func (c *Column) Measure(available core.Size) core.SpacePlan {
	if c.rendered >= len(c.Items) {
		return core.EmptyRender()
	}

	var used float64

	for i := c.rendered; i < len(c.Items); i++ {
		if i > c.rendered {
			used += c.Spacing
		}

		remaining := available.Height - used
		if remaining < -core.Epsilon {
			// The spacing alone overran the page. Whatever was placed before it
			// still stands, so this is a partial render, not a failure.
			return c.partial(available, used-c.Spacing)
		}

		plan := core.MeasureChild(c.Items[i], core.Size{
			Width:  available.Width,
			Height: remaining,
		})

		if plan.Wrapped() {
			// Nothing placed yet and the very first item will not fit: the whole
			// column defers to a fresh page. Reporting a zero-height partial
			// here instead would tell the parent that progress was made, and the
			// document loop would spin forever producing empty pages.
			if i == c.rendered {
				return core.Wrap("column item %d does not fit: %s", i, plan.WrapReason)
			}
			return c.partial(available, used-c.Spacing)
		}

		used += plan.Size.Height

		if plan.Partial() {
			return c.partial(available, used)
		}
	}

	return core.FullRender(core.Size{Width: available.Width, Height: used})
}

// NaturalSize reports the height the column's remaining items need, ignoring
// pagination.
//
// This is a pure "how tall is my content" query, so it has none of Measure's
// page-breaking logic: nothing stops early, and no partial render is produced. A
// row containing a column of cells needs the whole answer to size itself, not the
// part that happens to fit the current page.
func (c *Column) NaturalSize(available core.Size) core.SpacePlan {
	if c.rendered >= len(c.Items) {
		return core.EmptyRender()
	}

	var used float64
	for i := c.rendered; i < len(c.Items); i++ {
		if i > c.rendered {
			used += c.Spacing
		}

		plan := core.NaturalSizeOf(c.Items[i], core.Size{
			Width:  available.Width,
			Height: available.Height,
		})
		if plan.Wrapped() {
			return plan
		}
		used += plan.Size.Height
	}

	return core.FullRender(core.Size{Width: available.Width, Height: used})
}

// partial reports a column that filled height points of the page and has items
// left over.
func (c *Column) partial(available core.Size, height float64) core.SpacePlan {
	return core.PartialRender(core.Size{
		Width:  available.Width,
		Height: math.Max(0, height),
	})
}

func (c *Column) Draw(canvas core.Canvas, available core.Size) {
	var used float64
	i := c.rendered

	for ; i < len(c.Items); i++ {
		if i > c.rendered {
			used += c.Spacing
		}

		remaining := available.Height - used
		if remaining < -core.Epsilon {
			break
		}

		// Re-measuring recovers the size Measure promised this item. It is safe
		// because Measure is repeatable, and it is necessary because the plan
		// from the measuring pass belongs to the parent, not to us.
		plan := core.MeasureChild(c.Items[i], core.Size{
			Width:  available.Width,
			Height: remaining,
		})
		if plan.Wrapped() {
			break
		}

		canvas.Save()
		canvas.Translate(core.Position{Y: used})
		core.DrawChild(c.Items[i], canvas, core.Size{
			Width:  available.Width,
			Height: plan.Size.Height,
		})
		canvas.Restore()

		used += plan.Size.Height

		if plan.Partial() {
			// This item continues on the next page, so the cursor stays on it.
			break
		}
	}

	c.rendered = i
}

func (c *Column) Children() []core.Element { return c.Items }

func (c *Column) ResetState(hard bool) {
	if hard {
		c.rendered = 0
	}
}

var (
	_ core.Element         = (*Column)(nil)
	_ core.Composite       = (*Column)(nil)
	_ core.StateResettable = (*Column)(nil)
)
