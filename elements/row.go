package elements

import (
	"math"

	"codeberg.org/aaripurna/sanur/core"
)

// RowSizing selects how a row item's width is decided.
type RowSizing int

const (
	// RowConstant is a fixed width in points.
	RowConstant RowSizing = iota

	// RowRelative shares out the leftover width in proportion to Size, the way
	// flex weights do.
	RowRelative

	// RowAuto takes the item's natural measured width.
	RowAuto
)

// RowItem is one cell of a row.
type RowItem struct {
	Sizing RowSizing

	// Size is the width in points for RowConstant, or the weight for
	// RowRelative. It is ignored for RowAuto.
	Size float64

	Element core.Element
}

// Constant builds a fixed-width row item.
func Constant(width float64, e core.Element) RowItem {
	return RowItem{Sizing: RowConstant, Size: width, Element: e}
}

// Relative builds a row item that takes a share of the leftover width.
func Relative(weight float64, e core.Element) RowItem {
	return RowItem{Sizing: RowRelative, Size: weight, Element: e}
}

// Auto builds a row item sized to its content.
func Auto(e core.Element) RowItem {
	return RowItem{Sizing: RowAuto, Element: e}
}

// Row places children side by side.
//
// Widths are resolved in a fixed order — constants first, then auto items
// measured against what constants left, then relative items sharing the
// remainder. Auto has to come before relative because an auto item's width is
// only knowable by measuring it, and measuring needs an upper bound; relative
// items have no natural size at all, so they are the only ones that can safely
// absorb whatever is left.
//
// Vertically a row is as tall as its tallest item, and every item is drawn at
// that full height so that backgrounds and borders line up across the row.
type Row struct {
	Items   []RowItem
	Spacing float64
}

// NewRow builds a row from items.
func NewRow(spacing float64, items ...RowItem) *Row {
	return &Row{Items: items, Spacing: spacing}
}

// Add appends an item.
func (r *Row) Add(item RowItem) { r.Items = append(r.Items, item) }

// resolveWidths computes the width allotted to each item.
func (r *Row) resolveWidths(available core.Size) ([]float64, bool) {
	widths := make([]float64, len(r.Items))

	totalSpacing := 0.0
	if len(r.Items) > 1 {
		totalSpacing = r.Spacing * float64(len(r.Items)-1)
	}

	fixed := totalSpacing
	var totalWeight float64

	for i, item := range r.Items {
		switch item.Sizing {
		case RowConstant:
			widths[i] = item.Size
			fixed += item.Size
		case RowRelative:
			totalWeight += item.Size
		}
	}

	// Auto items are measured against everything the constants and spacing left
	// behind. Offering each of them the same budget can overcommit when several
	// are present, which the final fit check below catches.
	autoBudget := math.Max(0, available.Width-fixed)
	for i, item := range r.Items {
		if item.Sizing != RowAuto {
			continue
		}
		plan := core.MeasureChild(item.Element, core.Size{
			Width:  autoBudget,
			Height: available.Height,
		})
		if plan.Wrapped() {
			return nil, false
		}
		widths[i] = plan.Size.Width
		fixed += plan.Size.Width
	}

	leftover := available.Width - fixed
	if leftover < -core.Epsilon {
		return nil, false
	}

	if totalWeight > 0 {
		for i, item := range r.Items {
			if item.Sizing == RowRelative {
				widths[i] = leftover * item.Size / totalWeight
			}
		}
	}

	return widths, true
}

func (r *Row) Measure(available core.Size) core.SpacePlan {
	if len(r.Items) == 0 {
		return core.EmptyRender()
	}

	widths, ok := r.resolveWidths(available)
	if !ok {
		return core.Wrap("row items need more than the available width %.1f", available.Width)
	}

	var height float64
	anyPartial := false

	for i, item := range r.Items {
		plan := core.MeasureChild(item.Element, core.Size{
			Width:  widths[i],
			Height: available.Height,
		})
		if plan.Wrapped() {
			// A row is atomic across its width: there is no way to place some
			// cells and defer others without leaving a hole, so one cell that
			// cannot fit sends the whole row to the next page.
			return core.Wrap("row item %d does not fit: %s", i, plan.WrapReason)
		}
		height = math.Max(height, plan.Size.Height)
		if plan.Partial() {
			anyPartial = true
		}
	}

	size := core.Size{Width: available.Width, Height: height}
	if anyPartial {
		return core.PartialRender(size)
	}
	return core.FullRender(size)
}

func (r *Row) Draw(canvas core.Canvas, available core.Size) {
	if len(r.Items) == 0 {
		return
	}

	widths, ok := r.resolveWidths(available)
	if !ok {
		return
	}

	var x float64
	for i, item := range r.Items {
		canvas.Save()
		canvas.Translate(core.Position{X: x})
		// Every cell is drawn at the row's full height, not its own measured
		// height, so that a cell's background or border spans the whole row.
		core.DrawChild(item.Element, canvas, core.Size{
			Width:  widths[i],
			Height: available.Height,
		})
		canvas.Restore()

		x += widths[i] + r.Spacing
	}
}

func (r *Row) Children() []core.Element {
	out := make([]core.Element, 0, len(r.Items))
	for _, item := range r.Items {
		if item.Element != nil {
			out = append(out, item.Element)
		}
	}
	return out
}

var (
	_ core.Element   = (*Row)(nil)
	_ core.Composite = (*Row)(nil)
)
