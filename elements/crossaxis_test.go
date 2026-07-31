package elements_test

import (
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
)

// A row has to know its own height before it can align anything inside it, but a
// vertically centred child answers Measure with the full height on offer, because
// filling the offered space is what centring means. Resolving the row's height
// from natural sizes first is what breaks that circularity. These tests pin the
// behaviour, since getting it wrong is silent: the layout still succeeds, it just
// produces rows as tall as the page.

func TestRowHeightIgnoresVerticalGreed(t *testing.T) {
	row := elements.NewRow(0,
		elements.Constant(50, &elements.Aligned{
			Vertical: core.AlignMiddle,
			Child:    &fixedElement{w: 10, h: 20},
		}),
		elements.Constant(50, &fixedElement{w: 10, h: 30}),
	)

	// The page is 800 tall; the row must be 30, the tallest cell's content.
	plan := row.Measure(core.Size{Width: 200, Height: 800})

	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}
	closeTo(t, "height", plan.Size.Height, 30)
}

func TestRowHeightSeesThroughDecorators(t *testing.T) {
	// This is the shape that actually occurs: a table cell is a background
	// wrapping padding wrapping the alignment, not a bare aligned element. If any
	// link in the chain answered with Measure, the row would take the whole page.
	cell := &elements.Background{
		Color: core.RGB(240, 240, 240),
		Child: &elements.Padding{
			Top: 5, Bottom: 5, Left: 6, Right: 6,
			Child: &elements.Aligned{
				Vertical: core.AlignMiddle,
				Child:    &fixedElement{w: 12, h: 12},
			},
		},
	}

	row := elements.NewRow(0, elements.Constant(60, cell))

	plan := row.Measure(core.Size{Width: 200, Height: 800})

	// 12 of content plus 5 above and 5 below.
	closeTo(t, "height", plan.Size.Height, 22)
}

func TestRowHeightSeesThroughAColumnOfCells(t *testing.T) {
	cell := &elements.Column{
		Spacing: 4,
		Items: []core.Element{
			&fixedElement{w: 10, h: 10},
			&elements.Aligned{Vertical: core.AlignBottom, Child: &fixedElement{w: 10, h: 10}},
		},
	}

	row := elements.NewRow(0, elements.Constant(60, cell))

	plan := row.Measure(core.Size{Width: 200, Height: 800})

	// 10 + 4 spacing + 10, with the bottom-aligned child contributing only its
	// own height rather than the rest of the page.
	closeTo(t, "height", plan.Size.Height, 24)
}

func TestRowHeightSeesThroughConstraintsAndExtend(t *testing.T) {
	row := elements.NewRow(0,
		elements.Constant(50, &elements.Constrained{
			MinHeight: 40,
			Child:     &elements.Aligned{Vertical: core.AlignMiddle, Child: &fixedElement{w: 5, h: 5}},
		}),
		elements.Constant(50, &elements.Extend{
			Vertical: true,
			Child:    &fixedElement{w: 5, h: 15},
		}),
	)

	plan := row.Measure(core.Size{Width: 200, Height: 800})

	// The constraint's 40pt minimum is the tallest real contribution; the
	// extending cell reports only its child's 15.
	closeTo(t, "height", plan.Size.Height, 40)
}

func TestRowStillGrowsForGenuinelyTallContent(t *testing.T) {
	// The fix must not clamp rows that really do need the height.
	row := elements.NewRow(0,
		elements.Constant(50, &fixedElement{w: 10, h: 300}),
		elements.Constant(50, &fixedElement{w: 10, h: 20}),
	)

	plan := row.Measure(core.Size{Width: 200, Height: 800})

	closeTo(t, "height", plan.Size.Height, 300)
}

func TestAlignedCentresWithinTheResolvedRowHeight(t *testing.T) {
	tall := &fixedElement{w: 10, h: 40}
	short := &fixedElement{w: 10, h: 10}

	row := elements.NewRow(0,
		elements.Constant(50, tall),
		elements.Constant(50, &elements.Aligned{Vertical: core.AlignMiddle, Child: short}),
	)

	available := core.Size{Width: 200, Height: 800}
	plan := row.Measure(available)
	closeTo(t, "height", plan.Size.Height, 40)

	recorder := &translateRecorder{}
	row.Draw(recorder, core.Size{Width: 200, Height: plan.Size.Height})

	// Centring a 10pt child in a 40pt row offsets it by 15, not by half the page.
	if !recorder.sawTranslateY(15) {
		t.Errorf("expected a vertical offset of 15, got offsets %v", recorder.ys)
	}
}

// --- Clip ------------------------------------------------------------------

// oversized reports a size larger than whatever it is offered and never wraps,
// standing in for content that legitimately overflows.
type oversized struct {
	w, h      float64
	drawnWith core.Size
}

func (o *oversized) Measure(core.Size) core.SpacePlan {
	return core.FullRender(core.Size{Width: o.w, Height: o.h})
}

func (o *oversized) Draw(_ core.Canvas, available core.Size) {
	o.drawnWith = available
}

// filler accepts whatever it is offered, like a stretched image.
type filler struct {
	drawnWith core.Size
}

func (f *filler) Measure(available core.Size) core.SpacePlan {
	return core.FullRender(available)
}

func (f *filler) Draw(_ core.Canvas, available core.Size) {
	f.drawnWith = available
}

func TestClipReportsTheBoxAndDrawsNaturalSize(t *testing.T) {
	child := &oversized{w: 200, h: 400}
	clip := &elements.Clip{Child: child}

	box := core.Size{Width: 150, Height: 60}

	plan := clip.Measure(box)
	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}
	// The overflow is hidden, not laid out, so the clip claims only its box.
	closeTo(t, "reported width", plan.Size.Width, 150)
	closeTo(t, "reported height", plan.Size.Height, 60)

	clip.Draw(render_noop{}, box)

	// The child is drawn at full size so the excess is genuinely cropped rather
	// than scaled down to fit.
	closeTo(t, "drawn width", child.drawnWith.Width, 200)
	closeTo(t, "drawn height", child.drawnWith.Height, 400)
}

func TestClipNeverPassesInfinityToAFillingChild(t *testing.T) {
	child := &filler{}
	clip := &elements.Clip{Child: child}

	box := core.Size{Width: 120, Height: 80}

	plan := clip.Measure(box)
	closeTo(t, "reported width", plan.Size.Width, 120)
	closeTo(t, "reported height", plan.Size.Height, 80)

	clip.Draw(render_noop{}, box)

	// A child that merely fills has no natural size, so the probe value must not
	// reach it. Drawing at 1e9 would emit a transform scaled by a billion, which
	// readers reject outright.
	if child.drawnWith.Height >= core.Infinity || child.drawnWith.Width >= core.Infinity {
		t.Fatalf("child drawn with %v; the unbounded probe leaked through", child.drawnWith)
	}
	closeTo(t, "drawn width", child.drawnWith.Width, 120)
	closeTo(t, "drawn height", child.drawnWith.Height, 80)
}

func TestClipLetsAnImageBeCropped(t *testing.T) {
	// An image refuses a box it cannot fit, since rendering part of a photograph
	// makes no sense. Without Clip this measurement wraps and the layout fails.
	img := &elements.Image{
		Source: core.Image{
			Key: "wide", Format: "png", Data: []byte("x"),
			PixelWidth: 400, PixelHeight: 400,
		},
		Fit: elements.FitUnscaled,
	}

	box := core.Size{Width: 100, Height: 50}

	if plan := img.Measure(box); !plan.Wrapped() {
		t.Fatalf("bare image plan = %v, want Wrap", plan)
	}

	clipped := &elements.Clip{Child: img}
	plan := clipped.Measure(box)

	if !plan.Full() {
		t.Fatalf("clipped plan = %v, want FullRender", plan)
	}
	closeTo(t, "width", plan.Size.Width, 100)
	closeTo(t, "height", plan.Size.Height, 50)
}

func TestClipAbsorbsPartialRenders(t *testing.T) {
	// Content hidden by a crop is discarded, not continued on the next page, so a
	// clipped region must never report itself as partially rendered.
	clip := &elements.Clip{Child: &splittingElement{rows: 20, rowHeight: 10}}

	plan := clip.Measure(core.Size{Width: 100, Height: 45})

	if !plan.Full() {
		t.Errorf("plan = %v, want FullRender: a crop does not paginate", plan)
	}
}

func TestClipReportsLessThanTheBoxForSmallContent(t *testing.T) {
	clip := &elements.Clip{Child: &fixedElement{w: 20, h: 15}}

	plan := clip.Measure(core.Size{Width: 200, Height: 100})

	// Nothing to crop, so the clip is transparent to layout.
	closeTo(t, "width", plan.Size.Width, 20)
	closeTo(t, "height", plan.Size.Height, 15)
}

// --- helpers ---------------------------------------------------------------

// translateRecorder captures the vertical offsets applied while drawing.
type translateRecorder struct {
	render_noop
	xs []float64
	ys []float64
}

func (r *translateRecorder) Translate(p core.Position) {
	r.xs = append(r.xs, p.X)
	r.ys = append(r.ys, p.Y)
}

func (r *translateRecorder) sawTranslateX(want float64) bool {
	for _, x := range r.xs {
		if x > want-0.01 && x < want+0.01 {
			return true
		}
	}
	return false
}

func (r *translateRecorder) sawTranslateY(want float64) bool {
	for _, y := range r.ys {
		if y > want-0.01 && y < want+0.01 {
			return true
		}
	}
	return false
}

// Every pass-through decorator must forward NaturalSize. These cover the
// forwards individually, because a single one answering with Measure instead
// reintroduces the page-height row and nothing else would catch it.

func TestNaturalSizeForwardsThroughContainer(t *testing.T) {
	slot := elements.NewContainer()
	slot.Set(&elements.Aligned{
		Vertical: core.AlignMiddle,
		Child:    &fixedElement{w: 10, h: 16},
	})

	row := elements.NewRow(0, elements.Constant(50, slot))

	plan := row.Measure(core.Size{Width: 200, Height: 900})

	closeTo(t, "height", plan.Size.Height, 16)
}

func TestNaturalSizeForwardsThroughShowIf(t *testing.T) {
	visible := &elements.ShowIf{
		Condition: true,
		Child: &elements.Aligned{
			Vertical: core.AlignMiddle,
			Child:    &fixedElement{w: 10, h: 18},
		},
	}

	row := elements.NewRow(0, elements.Constant(50, visible))
	plan := row.Measure(core.Size{Width: 200, Height: 900})
	closeTo(t, "visible height", plan.Size.Height, 18)

	// Hidden, it must contribute nothing at all rather than its child's size.
	visible.Condition = false
	plan = row.Measure(core.Size{Width: 200, Height: 900})
	closeTo(t, "hidden height", plan.Size.Height, 0)
}

func TestNaturalSizeForwardsThroughBorder(t *testing.T) {
	bordered := elements.UniformBorder(1, core.RGB(0, 0, 0), &elements.Aligned{
		Vertical: core.AlignMiddle,
		Child:    &fixedElement{w: 10, h: 14},
	})

	row := elements.NewRow(0, elements.Constant(50, bordered))

	plan := row.Measure(core.Size{Width: 200, Height: 900})

	// A border is drawn on top of its child, so it adds nothing to the height.
	closeTo(t, "height", plan.Size.Height, 14)
}

func TestNaturalSizeForwardsThroughNestedRow(t *testing.T) {
	inner := elements.NewRow(0,
		elements.Constant(20, &elements.Aligned{
			Vertical: core.AlignMiddle,
			Child:    &fixedElement{w: 5, h: 12},
		}),
		elements.Constant(20, &fixedElement{w: 5, h: 26}),
	)

	outer := elements.NewRow(0, elements.Constant(60, inner))

	plan := outer.Measure(core.Size{Width: 200, Height: 900})

	// The nested row reports its own tallest content, not the page.
	closeTo(t, "height", plan.Size.Height, 26)
}

func TestNaturalSizeForwardsThroughStackedAligns(t *testing.T) {
	// Two Aligned elements nest whenever the alignments are written as a chain —
	// AlignRight().AlignMiddle() is the ordinary way to place a right-hand label —
	// so the outer one must ask the inner for its natural size rather than measuring
	// it. Measuring lets the inner expand again and the row goes page-tall.
	stacked := &elements.Aligned{
		Horizontal: core.AlignRight,
		Child: &elements.Aligned{
			Vertical: core.AlignMiddle,
			Child:    &fixedElement{w: 10, h: 20},
		},
	}

	row := elements.NewRow(0, elements.Constant(50, stacked))

	plan := row.Measure(core.Size{Width: 200, Height: 900})

	closeTo(t, "height", plan.Size.Height, 20)
}

func TestNaturalSizeForwardsThroughExtend(t *testing.T) {
	// Extend fills the axis it is told to, so it has the same problem: a row sizing
	// itself around a horizontally extending cell must still see the child's height.
	stacked := &elements.Extend{
		Horizontal: true,
		Child: &elements.Aligned{
			Vertical: core.AlignMiddle,
			Child:    &fixedElement{w: 10, h: 22},
		},
	}

	row := elements.NewRow(0, elements.Constant(50, stacked))

	plan := row.Measure(core.Size{Width: 200, Height: 900})

	closeTo(t, "height", plan.Size.Height, 22)
}

func TestAutoWidthComesFromTheNaturalSize(t *testing.T) {
	// An auto item asks "how much do you need". An element that fills whatever it is
	// offered answers Measure with the whole budget, so measuring it would let one auto
	// item take the entire row.
	//
	// The shape that found this is the total line of an invoice: a label on the left and
	// a right-aligned figure beside it. The figure claimed the whole row and the label
	// was squeezed to one character per line — a layout that succeeds and looks broken.
	aligned := &elements.Aligned{
		Horizontal: core.AlignRight,
		Child:      &fixedElement{w: 30, h: 10},
	}

	row := elements.NewRow(0,
		elements.Relative(1, &fixedElement{w: 20, h: 10}),
		elements.Auto(aligned),
	)

	plan := row.Measure(core.Size{Width: 200, Height: 100})
	if plan.Wrapped() {
		t.Fatalf("row wrapped: %s", plan.WrapReason)
	}

	// The auto item takes what its content needs, and the relative item gets the rest.
	// Drawing is what reveals the widths, since Measure reports only the total.
	// Drawing is what reveals the widths, since Measure reports only the total. The
	// auto item takes what its content needs, so it starts at 200 - 30.
	rec := &translateRecorder{}
	row.Draw(rec, core.Size{Width: 200, Height: 100})

	if !rec.sawTranslateX(170) {
		t.Errorf("expected the auto item at x=170, got offsets %v", rec.xs)
	}
}

func TestNaturalSizeOfPlainElementFallsBackToMeasure(t *testing.T) {
	// An element with no special filling behaviour needs no NaturalSize method;
	// the helper falls through to Measure.
	plan := core.NaturalSizeOf(&fixedElement{w: 12, h: 34}, core.Size{Width: 100, Height: 100})

	closeTo(t, "width", plan.Size.Width, 12)
	closeTo(t, "height", plan.Size.Height, 34)

	if got := core.NaturalSizeOf(nil, core.Size{Width: 10, Height: 10}); !got.Size.IsEmpty() {
		t.Errorf("nil element natural size = %v, want empty", got.Size)
	}
}

func TestNaturalSizePropagatesWraps(t *testing.T) {
	// A cell that cannot fit must still wrap through the natural-size path, or
	// the row would size itself around a zero and silently drop the content.
	row := elements.NewRow(0, elements.Constant(30, &elements.Padding{
		Left: 40, Right: 40,
		Child: &fixedElement{w: 5, h: 5},
	}))

	if plan := row.Measure(core.Size{Width: 200, Height: 200}); !plan.Wrapped() {
		t.Errorf("plan = %v, want Wrap", plan)
	}
}
