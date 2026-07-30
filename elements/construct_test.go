package elements_test

import (
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
)

// The constructors and incremental builders below exist for callers assembling
// elements directly rather than through the fluent API. They are thin, which is
// exactly why they are worth pinning: a swapped argument in a one-line helper
// produces no error, just a document that looks subtly wrong.

func TestFixedWidthConstrainsOnlyWidth(t *testing.T) {
	k := elements.FixedWidth(120, &fixedElement{w: 10, h: 30})

	plan := k.Measure(core.Size{Width: 500, Height: 500})

	closeTo(t, "width", plan.Size.Width, 120)
	// The height is left to the child, not forced to match the width.
	closeTo(t, "height", plan.Size.Height, 30)
}

func TestFixedHeightConstrainsOnlyHeight(t *testing.T) {
	k := elements.FixedHeight(75, &fixedElement{w: 40, h: 10})

	plan := k.Measure(core.Size{Width: 500, Height: 500})

	closeTo(t, "width", plan.Size.Width, 40)
	closeTo(t, "height", plan.Size.Height, 75)
}

func TestUniformPaddingAppliesEveryEdge(t *testing.T) {
	p := elements.UniformPadding(6, &fixedElement{w: 20, h: 10})

	plan := p.Measure(core.Size{Width: 500, Height: 500})

	closeTo(t, "width", plan.Size.Width, 32)   // 20 + 6 + 6
	closeTo(t, "height", plan.Size.Height, 22) // 10 + 6 + 6
}

func TestUniformBorderSetsEverySide(t *testing.T) {
	b := elements.UniformBorder(3, core.RGB(0, 0, 0), &fixedElement{w: 10, h: 10})

	for name, side := range map[string]elements.BorderSide{
		"top": b.Top, "right": b.Right, "bottom": b.Bottom, "left": b.Left,
	} {
		if side.Width != 3 {
			t.Errorf("%s width = %v, want 3", name, side.Width)
		}
		if !side.Visible() {
			t.Errorf("%s should be visible", name)
		}
	}
}

func TestBorderSideVisibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		side elements.BorderSide
		want bool
	}{
		{"solid", elements.BorderSide{Width: 1, Color: core.RGB(0, 0, 0)}, true},
		{"zero width", elements.BorderSide{Width: 0, Color: core.RGB(0, 0, 0)}, false},
		{"transparent", elements.BorderSide{Width: 1, Color: core.Transparent}, false},
		{"unset", elements.BorderSide{}, false},
	} {
		if got := tc.side.Visible(); got != tc.want {
			t.Errorf("%s: Visible = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestColumnAddAppendsInOrder(t *testing.T) {
	col := &elements.Column{Spacing: 5}
	col.Add(&fixedElement{w: 10, h: 20})
	col.Add(&fixedElement{w: 10, h: 30})

	plan := col.Measure(core.Size{Width: 100, Height: 500})

	// 20 + 5 spacing + 30
	closeTo(t, "height", plan.Size.Height, 55)
	if got := len(col.Children()); got != 2 {
		t.Errorf("children = %d, want 2", got)
	}
}

func TestRowAddAppendsInOrder(t *testing.T) {
	row := &elements.Row{Spacing: 4}
	row.Add(elements.Constant(60, &fixedElement{w: 1, h: 12}))
	row.Add(elements.Constant(40, &fixedElement{w: 1, h: 20}))

	plan := row.Measure(core.Size{Width: 300, Height: 100})

	closeTo(t, "width", plan.Size.Width, 300)
	// A row is as tall as its tallest cell.
	closeTo(t, "height", plan.Size.Height, 20)
	if got := len(row.Children()); got != 2 {
		t.Errorf("children = %d, want 2", got)
	}
}

func TestRowChildrenSkipsEmptySlots(t *testing.T) {
	row := elements.NewRow(0,
		elements.Constant(10, &fixedElement{w: 1, h: 1}),
		elements.Constant(10, nil), // declared but never filled
	)

	// A nil slot must not appear as a child, or tree walks would dereference it.
	if got := len(row.Children()); got != 1 {
		t.Errorf("children = %d, want 1", got)
	}
}

func TestContainerDelegatesThrough(t *testing.T) {
	c := elements.NewContainer()

	// An empty slot must measure as nothing rather than panicking, since the
	// fluent API hands out slots before they are filled.
	if plan := c.Measure(core.Size{Width: 10, Height: 10}); !plan.Full() || !plan.Size.IsEmpty() {
		t.Errorf("empty container plan = %v, want an empty full render", plan)
	}
	if got := c.Children(); got != nil {
		t.Errorf("empty container children = %v, want nil", got)
	}
	c.Draw(render_noop{}, core.Size{})

	child := &fixedElement{w: 30, h: 15}
	c.Set(child)

	plan := c.Measure(core.Size{Width: 100, Height: 100})
	closeTo(t, "width", plan.Size.Width, 30)
	closeTo(t, "height", plan.Size.Height, 15)

	c.Draw(render_noop{}, core.Size{Width: 30, Height: 15})
	if child.draws != 1 {
		t.Errorf("child drawn %d times, want 1", child.draws)
	}
	if got := len(c.Children()); got != 1 {
		t.Errorf("children = %d, want 1", got)
	}
}

func TestShowIfHidesWithoutReservingSpace(t *testing.T) {
	child := &fixedElement{w: 50, h: 50}
	s := &elements.ShowIf{Condition: false, Child: child}

	plan := s.Measure(core.Size{Width: 100, Height: 100})
	if !plan.Size.IsEmpty() {
		t.Errorf("hidden size = %v, want zero: a hidden child must reserve nothing", plan.Size)
	}
	if got := len(s.Children()); got != 1 {
		t.Errorf("children = %d, want 1 even while hidden", got)
	}

	s.Draw(render_noop{}, core.Size{Width: 100, Height: 100})
	if child.draws != 0 {
		t.Errorf("hidden child drawn %d times, want 0", child.draws)
	}

	s.Condition = true
	plan = s.Measure(core.Size{Width: 100, Height: 100})
	closeTo(t, "visible width", plan.Size.Width, 50)
	s.Draw(render_noop{}, core.Size{Width: 50, Height: 50})
	if child.draws != 1 {
		t.Errorf("visible child drawn %d times, want 1", child.draws)
	}
}

func TestEmptyElementTakesNoSpace(t *testing.T) {
	plan := elements.Empty{}.Measure(core.Size{Width: 100, Height: 100})

	if !plan.Full() || !plan.Size.IsEmpty() {
		t.Errorf("plan = %v, want an empty full render", plan)
	}
	elements.Empty{}.Draw(render_noop{}, core.Size{})
}

func TestSpacerWrapsWhenTooLarge(t *testing.T) {
	s := &elements.Spacer{Width: 200, Height: 10}

	if plan := s.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}
	if plan := s.Measure(core.Size{Width: 300, Height: 100}); !plan.Full() {
		t.Errorf("plan = %v, want FullRender", plan)
	}
	s.Draw(render_noop{}, core.Size{})
}

func TestLineReportsThicknessOnItsOwnAxis(t *testing.T) {
	horizontal := &elements.Line{Width: 2, Color: core.RGB(0, 0, 0)}
	plan := horizontal.Measure(core.Size{Width: 200, Height: 100})
	closeTo(t, "horizontal width", plan.Size.Width, 200)
	closeTo(t, "horizontal height", plan.Size.Height, 2)

	vertical := &elements.Line{Vertical: true, Width: 3, Color: core.RGB(0, 0, 0)}
	plan = vertical.Measure(core.Size{Width: 200, Height: 100})
	closeTo(t, "vertical width", plan.Size.Width, 3)
	closeTo(t, "vertical height", plan.Size.Height, 100)
}

func TestLineWrapsWhenThickerThanTheBox(t *testing.T) {
	horizontal := &elements.Line{Width: 50, Color: core.RGB(0, 0, 0)}
	if plan := horizontal.Measure(core.Size{Width: 200, Height: 10}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}

	vertical := &elements.Line{Vertical: true, Width: 50, Color: core.RGB(0, 0, 0)}
	if plan := vertical.Measure(core.Size{Width: 10, Height: 200}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}
}

func TestRotateMeasuresAgainstUnboundedSpace(t *testing.T) {
	// A rotated element's footprint bears no simple relation to its content, so
	// it reports the box it was offered and takes responsibility for staying in it.
	r := &elements.Rotate{Degrees: 90, Child: &fixedElement{w: 40, h: 10}}

	plan := r.Measure(core.Size{Width: 200, Height: 100})

	closeTo(t, "width", plan.Size.Width, 200)
	closeTo(t, "height", plan.Size.Height, 100)
	if got := len(r.Children()); got != 1 {
		t.Errorf("children = %d, want 1", got)
	}

	r.Draw(render_noop{}, core.Size{Width: 200, Height: 100})
}

func TestRotateAndClipToleratedWithoutChild(t *testing.T) {
	empty := &elements.Rotate{Degrees: 45}
	if plan := empty.Measure(core.Size{Width: 10, Height: 10}); !plan.Size.IsEmpty() {
		t.Errorf("plan = %v, want empty", plan)
	}
	empty.Draw(render_noop{}, core.Size{})
	if empty.Children() != nil {
		t.Error("a childless Rotate should report no children")
	}

	clip := &elements.Clip{}
	if plan := clip.Measure(core.Size{Width: 10, Height: 10}); !plan.Size.IsEmpty() {
		t.Errorf("plan = %v, want empty", plan)
	}
	clip.Draw(render_noop{}, core.Size{})
	if clip.Children() != nil {
		t.Error("a childless Clip should report no children")
	}
}

func TestChildlessDecoratorsReportNoChildren(t *testing.T) {
	for name, e := range map[string]core.Composite{
		"background":  &elements.Background{},
		"border":      &elements.Border{},
		"padding":     &elements.Padding{},
		"constrained": &elements.Constrained{},
		"extend":      &elements.Extend{},
		"aligned":     &elements.Aligned{},
		"showif":      &elements.ShowIf{},
		"container":   elements.NewContainer(),
	} {
		if got := e.Children(); got != nil {
			t.Errorf("%s: children = %v, want nil", name, got)
		}
	}
}

func TestChildlessDecoratorsDrawWithoutPanicking(t *testing.T) {
	size := core.Size{Width: 10, Height: 10}

	// Every decorator has to tolerate an unfilled slot, because the fluent API
	// can hand one out and never populate it.
	(&elements.Padding{Top: 1}).Draw(render_noop{}, size)
	(&elements.Background{Color: core.RGB(0, 0, 0)}).Draw(render_noop{}, size)
	(&elements.Border{}).Draw(render_noop{}, size)
	(&elements.Constrained{MinWidth: 5}).Draw(render_noop{}, size)
	(&elements.Extend{Horizontal: true}).Draw(render_noop{}, size)
	(&elements.Aligned{Horizontal: core.AlignCenter}).Draw(render_noop{}, size)
}

func TestImageMeasuresByFitMode(t *testing.T) {
	// A 2:1 image offered a 100x100 box.
	src := core.Image{
		Key: "sample", Format: "png", Data: []byte("x"),
		PixelWidth: 40, PixelHeight: 20,
	}
	available := core.Size{Width: 100, Height: 100}

	for _, tc := range []struct {
		name         string
		fit          elements.ImageFit
		wantW, wantH float64
	}{
		{"FitWidth", elements.FitWidth, 100, 50},
		{"FitArea", elements.FitArea, 100, 50},
		{"FitStretch", elements.FitStretch, 100, 100},
		{"FitUnscaled", elements.FitUnscaled, 40, 20},
	} {
		img := &elements.Image{Source: src, Fit: tc.fit}
		plan := img.Measure(available)
		closeTo(t, tc.name+" width", plan.Size.Width, tc.wantW)
		closeTo(t, tc.name+" height", plan.Size.Height, tc.wantH)
	}
}

func TestFitAreaConstrainsByTheTighterAxis(t *testing.T) {
	src := core.Image{
		Key: "tall", Format: "png", Data: []byte("x"),
		PixelWidth: 20, PixelHeight: 100,
	}
	img := &elements.Image{Source: src, Fit: elements.FitArea}

	// Height is the binding constraint here, so the width scales down with it
	// rather than filling the box.
	plan := img.Measure(core.Size{Width: 200, Height: 50})

	closeTo(t, "width", plan.Size.Width, 10)
	closeTo(t, "height", plan.Size.Height, 50)
}

func TestImageWrapsWhenItCannotFit(t *testing.T) {
	src := core.Image{
		Key: "big", Format: "png", Data: []byte("x"),
		PixelWidth: 400, PixelHeight: 400,
	}
	img := &elements.Image{Source: src, Fit: elements.FitUnscaled}

	// An image is atomic: there is no way to render half of it now and the rest
	// on the next page.
	if plan := img.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}
}

func TestImageWithoutDataIsEmpty(t *testing.T) {
	img := &elements.Image{Source: core.Image{PixelWidth: 10, PixelHeight: 10}}

	if plan := img.Measure(core.Size{Width: 100, Height: 100}); !plan.Size.IsEmpty() {
		t.Errorf("plan = %v, want empty", plan)
	}
	img.Draw(render_noop{}, core.Size{Width: 100, Height: 100})

	// A degenerate source with no pixel height has no resolvable aspect ratio.
	degenerate := &elements.Image{Source: core.Image{Key: "d", Data: []byte("x"), PixelWidth: 10}}
	if plan := degenerate.Measure(core.Size{Width: 100, Height: 100}); !plan.Size.IsEmpty() {
		t.Errorf("degenerate plan = %v, want empty", plan)
	}
	degenerate.Draw(render_noop{}, core.Size{Width: 100, Height: 100})
}

func TestPageNumberExpandsPlaceholders(t *testing.T) {
	style := testStyle(10)
	pn := elements.NewPageNumber("Page {page} of {total}", style)

	pn.SetPageContext(core.PageContext{PageNumber: 2, TotalPages: 9})
	withTotal := pn.Measure(core.Size{Width: 500, Height: 100})
	if withTotal.Size.Width <= 0 {
		t.Fatal("expected the label to measure to a real width")
	}

	// During the counting pass the total is unknown and expands to "?", which
	// still has to measure to something believable.
	pn.SetPageContext(core.PageContext{PageNumber: 2})
	unknown := pn.Measure(core.Size{Width: 500, Height: 100})
	if unknown.Size.Width <= 0 {
		t.Error("the placeholder label measured to nothing")
	}

	pn.Draw(render_noop{}, core.Size{Width: 500, Height: unknown.Size.Height})
}

func TestPageNumberResetDiscardsCachedText(t *testing.T) {
	pn := elements.NewPageNumber("{page}", testStyle(10))

	pn.SetPageContext(core.PageContext{PageNumber: 1, TotalPages: 1})
	first := pn.Measure(core.Size{Width: 100, Height: 100})
	pn.Draw(render_noop{}, core.Size{Width: 100, Height: first.Size.Height})

	// Without discarding the cached layout, a redrawn label would keep the
	// previous page's rendered-line progress and vanish.
	pn.ResetState(true)
	again := pn.Measure(core.Size{Width: 100, Height: 100})
	if again.Size.Height <= 0 {
		t.Error("the label measured to nothing after a reset")
	}
}

func TestTextAddSpanInvalidatesLayout(t *testing.T) {
	style := testStyle(10)
	text := elements.NewText("first", style)

	before := text.Measure(core.Size{Width: 500, Height: 100})

	text.AddSpan(" and second", style)
	after := text.Measure(core.Size{Width: 500, Height: 100})

	// A stale cached layout would report the original width.
	if after.Size.Width <= before.Size.Width {
		t.Errorf("width after adding a span = %.2f, want more than %.2f",
			after.Size.Width, before.Size.Width)
	}
}

func TestTextIgnoresSpansWithNoUsableStyle(t *testing.T) {
	valid := testStyle(10)

	text := &elements.Text{Spans: []elements.Span{
		{Text: "no font", Style: core.TextStyle{Size: 10}},
		{Text: "zero size", Style: core.TextStyle{Font: valid.Font}},
		{Text: "ok", Style: valid},
	}}

	plan := text.Measure(core.Size{Width: 500, Height: 100})

	// Unusable spans are skipped rather than crashing on a nil font.
	if plan.Size.Width <= 0 {
		t.Fatalf("plan = %v, want the usable span to be laid out", plan)
	}
	closeTo(t, "width", plan.Size.Width, valid.MeasureText("ok"))
}

func TestTextWithNoSpansIsEmpty(t *testing.T) {
	text := &elements.Text{}

	if plan := text.Measure(core.Size{Width: 100, Height: 100}); !plan.Size.IsEmpty() {
		t.Errorf("plan = %v, want empty", plan)
	}
	text.Draw(render_noop{}, core.Size{Width: 100, Height: 100})
}

func TestPaddingWrapsOnVerticalOverflow(t *testing.T) {
	// The horizontal case is covered elsewhere; the vertical branch is a separate
	// condition and would otherwise go unexercised.
	p := elements.UniformPadding(40, &fixedElement{w: 1, h: 1})

	if plan := p.Measure(core.Size{Width: 500, Height: 50}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap: 80 points of vertical padding cannot fit in 50", plan)
	}
}

func TestPaddingPropagatesChildWrap(t *testing.T) {
	p := elements.UniformPadding(5, &fixedElement{w: 1000, h: 1})

	if plan := p.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want the child's wrap to propagate", plan)
	}
}

func TestConstrainedWrapsWhenClampingExceedsSpace(t *testing.T) {
	// The minimum itself fits, but clamping the child up to it pushes the box past
	// what the parent offered.
	k := &elements.Constrained{
		MinWidth: 90,
		MaxWidth: 90,
		Child:    &fixedElement{w: 10, h: 200},
	}

	if plan := k.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}
}

func TestConstrainedPropagatesChildWrap(t *testing.T) {
	k := &elements.Constrained{MaxWidth: 50, Child: &fixedElement{w: 500, h: 10}}

	if plan := k.Measure(core.Size{Width: 500, Height: 500}); !plan.Wrapped() {
		t.Errorf("plan = %v, want the child's wrap to propagate", plan)
	}
}

func TestExtendPropagatesChildWrap(t *testing.T) {
	e := &elements.Extend{Horizontal: true, Child: &fixedElement{w: 10, h: 500}}

	if plan := e.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want the child's wrap to propagate", plan)
	}
}

func TestAlignedPropagatesChildWrapAndSkipsDrawing(t *testing.T) {
	child := &fixedElement{w: 10, h: 500}
	a := &elements.Aligned{Horizontal: core.AlignCenter, Child: child}

	if plan := a.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want the child's wrap to propagate", plan)
	}

	// Drawing a wrapped child would place it at a meaningless offset.
	a.Draw(render_noop{}, core.Size{Width: 100, Height: 100})
	if child.draws != 0 {
		t.Errorf("wrapped child drawn %d times, want 0", child.draws)
	}
}

func TestRowDrawSkipsWhenWidthsCannotResolve(t *testing.T) {
	row := elements.NewRow(0,
		elements.Constant(200, &fixedElement{w: 1, h: 1}),
		elements.Constant(200, &fixedElement{w: 1, h: 1}),
	)

	// Measure would have wrapped, so Draw must decline rather than place cells
	// at negative widths.
	row.Draw(render_noop{}, core.Size{Width: 100, Height: 100})

	for i, child := range row.Children() {
		if fe, ok := child.(*fixedElement); ok && fe.draws != 0 {
			t.Errorf("item %d drawn %d times, want 0", i, fe.draws)
		}
	}
}

func TestRowWithAutoItemThatWrapsIsUnresolvable(t *testing.T) {
	row := elements.NewRow(0, elements.Auto(&fixedElement{w: 1, h: 500}))

	if plan := row.Measure(core.Size{Width: 100, Height: 100}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}
}

func TestEmptyRowAndColumnAreEmpty(t *testing.T) {
	if plan := (&elements.Row{}).Measure(core.Size{Width: 10, Height: 10}); !plan.Size.IsEmpty() {
		t.Errorf("empty row plan = %v, want empty", plan)
	}
	(&elements.Row{}).Draw(render_noop{}, core.Size{Width: 10, Height: 10})

	if plan := (&elements.Column{}).Measure(core.Size{Width: 10, Height: 10}); !plan.Size.IsEmpty() {
		t.Errorf("empty column plan = %v, want empty", plan)
	}
}

func TestColumnPropagatesPartialFromALaterItem(t *testing.T) {
	col := elements.NewColumn(0,
		&fixedElement{w: 10, h: 20},
		&splittingElement{rows: 10, rowHeight: 10},
	)

	plan := col.Measure(core.Size{Width: 100, Height: 55})

	if !plan.Partial() {
		t.Fatalf("plan = %v, want PartialRender", plan)
	}
	// 20 for the first item, then three whole rows of the second.
	closeTo(t, "height", plan.Size.Height, 50)
}

func TestColumnStopsWhenSpacingOverrunsThePage(t *testing.T) {
	col := elements.NewColumn(40,
		&fixedElement{w: 10, h: 30},
		&fixedElement{w: 10, h: 30},
	)

	// The first item fits; the spacing alone then exceeds what is left, which is
	// still progress rather than a failure.
	plan := col.Measure(core.Size{Width: 100, Height: 35})

	if !plan.Partial() {
		t.Fatalf("plan = %v, want PartialRender", plan)
	}
	closeTo(t, "height", plan.Size.Height, 30)
}

func TestSoftResetLeavesProgressIntact(t *testing.T) {
	inner := &splittingElement{rows: 10, rowHeight: 10}
	col := elements.NewColumn(0, inner)

	col.Measure(core.Size{Width: 100, Height: 45})
	col.Draw(render_noop{}, core.Size{Width: 100, Height: 40})
	before := inner.rendered

	// Only a hard reset rewinds pagination progress; a soft one must not, or a
	// partially rendered element would restart mid-document.
	core.ResetTree(col, false)

	if inner.rendered != before {
		t.Errorf("progress = %d after a soft reset, want %d", inner.rendered, before)
	}
}
