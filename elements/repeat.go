package elements

import "github.com/aaripurna/sanur-pdf/core"

// Repeat draws its Header at the top of every sheet its Body occupies.
//
// This is what a long table needs. A table is a column of rows, and a column that
// splits across pages resumes at the row it reached — so the heading row, being
// row zero, appears once and never again. Wrapping the body in a Repeat puts it
// back at the top of each continuation.
//
// The header is measured against the space the parent offered and subtracted from
// what the body may use, so the two never overlap. A header taller than the
// available space, or one that would itself paginate, is reported as a wrap rather
// than silently truncated: a heading is only useful whole.
type Repeat struct {
	Header core.Element
	Body   core.Element
}

func (r *Repeat) Measure(available core.Size) core.SpacePlan {
	if r.Body == nil {
		return core.EmptyRender()
	}

	headerHeight, ok := r.headerHeight(available)
	if !ok {
		return core.Wrap("repeating header does not fit in %.1f points", available.Height)
	}

	body := r.Body.Measure(core.Size{
		Width:  available.Width,
		Height: available.Height - headerHeight,
	})
	if body.Wrapped() {
		return body
	}

	return core.SpacePlan{
		Type: body.Type,
		Size: core.Size{
			Width:  available.Width,
			Height: headerHeight + body.Size.Height,
		},
	}
}

// NaturalSize reports the header plus the body's own content height, so a Repeat
// inside a row cell sizes to what it holds.
func (r *Repeat) NaturalSize(available core.Size) core.SpacePlan {
	headerHeight, _ := r.headerHeight(available)

	body := core.NaturalSizeOf(r.Body, core.Size{
		Width:  available.Width,
		Height: available.Height - headerHeight,
	})
	if body.Wrapped() {
		return body
	}

	return core.FullRender(core.Size{
		Width:  available.Width,
		Height: headerHeight + body.Size.Height,
	})
}

// headerHeight measures the header, reporting false if it cannot be drawn whole.
func (r *Repeat) headerHeight(available core.Size) (float64, bool) {
	if r.Header == nil {
		return 0, true
	}

	plan := r.Header.Measure(available)
	if !plan.Full() {
		// A header that wrapped or split would appear differently on each sheet,
		// which defeats the purpose of repeating it.
		return 0, false
	}
	if plan.Size.Height > available.Height+core.Epsilon {
		return 0, false
	}
	return plan.Size.Height, true
}

func (r *Repeat) Draw(canvas core.Canvas, available core.Size) {
	if r.Body == nil {
		return
	}

	headerHeight, ok := r.headerHeight(available)
	if !ok {
		return
	}

	if r.Header != nil {
		canvas.Save()
		r.Header.Draw(canvas, core.Size{Width: available.Width, Height: headerHeight})
		canvas.Restore()
	}

	canvas.Save()
	canvas.Translate(core.Position{Y: headerHeight})
	r.Body.Draw(canvas, core.Size{
		Width:  available.Width,
		Height: available.Height - headerHeight,
	})
	canvas.Restore()

	// The header is rewound after drawing, not before, so that the Measure the
	// parent runs on the next sheet sees a header with nothing rendered. Resetting
	// on the way in would leave stale state behind for that measurement, which
	// would report a height of zero and let the body draw over the header.
	//
	// Rewinding here rather than in Measure also keeps Measure free of side
	// effects, which the engine relies on.
	core.ResetTree(r.Header, true)
}

func (r *Repeat) Children() []core.Element {
	var children []core.Element
	if r.Header != nil {
		children = append(children, r.Header)
	}
	if r.Body != nil {
		children = append(children, r.Body)
	}
	return children
}

// ResetState rewinds the body. The header is stateless between sheets by
// construction, since Draw rewinds it every time.
func (r *Repeat) ResetState(hard bool) {
	if hard {
		core.ResetTree(r.Header, true)
	}
}

var (
	_ core.Element         = (*Repeat)(nil)
	_ core.Composite       = (*Repeat)(nil)
	_ core.StateResettable = (*Repeat)(nil)
)
