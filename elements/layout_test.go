package elements_test

import (
	"math"
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
	"github.com/aaripurna/sanur-pdf/fonts"
)

func testStyle(size float64) core.TextStyle {
	return core.TextStyle{
		Font:  fonts.MustStandard(fonts.Helvetica),
		Size:  size,
		Color: core.RGB(0, 0, 0),
	}
}

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}

// fixedElement is a stub of known size, so container arithmetic can be checked
// without depending on font metrics.
type fixedElement struct {
	w, h  float64
	draws int
}

func (f *fixedElement) Measure(available core.Size) core.SpacePlan {
	size := core.Size{Width: f.w, Height: f.h}
	if !size.FitsWithin(available) {
		return core.Wrap("fixed %.1fx%.1f does not fit", f.w, f.h)
	}
	return core.FullRender(size)
}

func (f *fixedElement) Draw(core.Canvas, core.Size) { f.draws++ }

// splittingElement renders a fixed number of unit-height rows, one page at a
// time, standing in for text or a long table.
type splittingElement struct {
	rows      int
	rowHeight float64
	rendered  int
}

func (s *splittingElement) Measure(available core.Size) core.SpacePlan {
	remaining := s.rows - s.rendered
	if remaining <= 0 {
		return core.EmptyRender()
	}

	fit := int(available.Height / s.rowHeight)
	if fit <= 0 {
		return core.Wrap("no room for a row")
	}
	if fit > remaining {
		fit = remaining
	}

	size := core.Size{Width: available.Width, Height: float64(fit) * s.rowHeight}
	if fit < remaining {
		return core.PartialRender(size)
	}
	return core.FullRender(size)
}

func (s *splittingElement) Draw(_ core.Canvas, available core.Size) {
	fit := int(available.Height / s.rowHeight)
	if fit > s.rows-s.rendered {
		fit = s.rows - s.rendered
	}
	s.rendered += fit
}

func (s *splittingElement) ResetState(hard bool) {
	if hard {
		s.rendered = 0
	}
}

func TestPaddingReportsChildPlusInsets(t *testing.T) {
	p := &elements.Padding{
		Top: 5, Right: 10, Bottom: 15, Left: 20,
		Child: &fixedElement{w: 100, h: 50},
	}

	plan := p.Measure(core.Size{Width: 500, Height: 500})

	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}
	closeTo(t, "width", plan.Size.Width, 130)  // 100 + 20 + 10
	closeTo(t, "height", plan.Size.Height, 70) // 50 + 5 + 15
}

func TestPaddingWrapsWhenInsetsExceedSpace(t *testing.T) {
	p := elements.UniformPadding(30, &fixedElement{w: 10, h: 10})

	plan := p.Measure(core.Size{Width: 40, Height: 500})

	if !plan.Wrapped() {
		t.Fatalf("plan = %v, want Wrap: 60 points of horizontal padding cannot fit in 40", plan)
	}
}

func TestColumnTakesFullWidthAndSumsHeights(t *testing.T) {
	col := elements.NewColumn(4,
		&fixedElement{w: 30, h: 10},
		&fixedElement{w: 80, h: 20},
	)

	plan := col.Measure(core.Size{Width: 200, Height: 500})

	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}
	// A column is width-greedy: it reports the space offered, not its widest
	// child, so children inherit the column's width.
	closeTo(t, "width", plan.Size.Width, 200)
	closeTo(t, "height", plan.Size.Height, 34) // 10 + 4 spacing + 20
}

func TestColumnPartiallyRendersAndResumes(t *testing.T) {
	first := &fixedElement{w: 10, h: 60}
	second := &fixedElement{w: 10, h: 60}
	col := elements.NewColumn(0, first, second)

	available := core.Size{Width: 100, Height: 100}

	plan := col.Measure(available)
	if !plan.Partial() {
		t.Fatalf("first plan = %v, want PartialRender: only one 60pt item fits in 100", plan)
	}
	closeTo(t, "first height", plan.Size.Height, 60)

	col.Draw(render_noop{}, core.Size{Width: 100, Height: 60})

	if first.draws != 1 {
		t.Errorf("first item drawn %d times, want 1", first.draws)
	}
	if second.draws != 0 {
		t.Errorf("second item drawn %d times on the first page, want 0", second.draws)
	}

	// The second page must resume at the item that did not fit, not restart.
	plan = col.Measure(available)
	if !plan.Full() {
		t.Fatalf("second plan = %v, want FullRender", plan)
	}
	closeTo(t, "second height", plan.Size.Height, 60)

	col.Draw(render_noop{}, core.Size{Width: 100, Height: 60})
	if second.draws != 1 {
		t.Errorf("second item drawn %d times, want 1", second.draws)
	}
	if first.draws != 1 {
		t.Errorf("first item drawn %d times overall, want 1 (it must not repeat)", first.draws)
	}
}

func TestColumnWrapsWhenFirstItemDoesNotFit(t *testing.T) {
	col := elements.NewColumn(0, &fixedElement{w: 10, h: 200})

	plan := col.Measure(core.Size{Width: 100, Height: 100})

	// Reporting a zero-height partial here instead would tell the document loop
	// that progress had been made and spin out blank pages forever.
	if !plan.Wrapped() {
		t.Fatalf("plan = %v, want Wrap", plan)
	}
}

func TestColumnPropagatesChildPartialRender(t *testing.T) {
	col := elements.NewColumn(0, &splittingElement{rows: 10, rowHeight: 10})

	plan := col.Measure(core.Size{Width: 100, Height: 45})

	if !plan.Partial() {
		t.Fatalf("plan = %v, want PartialRender", plan)
	}
	closeTo(t, "height", plan.Size.Height, 40) // four whole rows
}

func TestRowResolvesConstantAutoAndRelativeWidths(t *testing.T) {
	auto := &fixedElement{w: 50, h: 10}
	relative := &fixedElement{w: 1, h: 10}

	row := elements.NewRow(0,
		elements.Constant(100, &fixedElement{w: 1, h: 10}),
		elements.Auto(auto),
		elements.Relative(1, relative),
	)

	plan := row.Measure(core.Size{Width: 400, Height: 100})

	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}
	closeTo(t, "width", plan.Size.Width, 400)
	closeTo(t, "height", plan.Size.Height, 10)

	// The relative item should have received 400 - 100 constant - 50 auto = 250.
	row.Draw(render_noop{}, core.Size{Width: 400, Height: 10})
	if relative.draws != 1 {
		t.Fatalf("relative item drawn %d times, want 1", relative.draws)
	}
}

func TestRowWrapsWhenConstantsOverflow(t *testing.T) {
	row := elements.NewRow(10,
		elements.Constant(100, &fixedElement{w: 1, h: 1}),
		elements.Constant(100, &fixedElement{w: 1, h: 1}),
	)

	plan := row.Measure(core.Size{Width: 150, Height: 100})

	if !plan.Wrapped() {
		t.Fatalf("plan = %v, want Wrap: 210 points of items cannot fit in 150", plan)
	}
}

func TestConstrainedFixesSizeRegardlessOfChild(t *testing.T) {
	k := elements.FixedSize(80, 40, &fixedElement{w: 10, h: 10})

	plan := k.Measure(core.Size{Width: 500, Height: 500})

	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}
	closeTo(t, "width", plan.Size.Width, 80)
	closeTo(t, "height", plan.Size.Height, 40)
}

func TestConstrainedWrapsWhenMinimumExceedsSpace(t *testing.T) {
	k := &elements.Constrained{MinWidth: 300, Child: &fixedElement{w: 10, h: 10}}

	plan := k.Measure(core.Size{Width: 200, Height: 200})

	if !plan.Wrapped() {
		t.Fatalf("plan = %v, want Wrap", plan)
	}
}

func TestExtendClaimsAvailableSpace(t *testing.T) {
	e := &elements.Extend{Horizontal: true, Child: &fixedElement{w: 10, h: 25}}

	plan := e.Measure(core.Size{Width: 300, Height: 500})

	closeTo(t, "width", plan.Size.Width, 300)
	// The vertical axis was not extended, so the child's own height stands.
	closeTo(t, "height", plan.Size.Height, 25)
}

func TestTextWrapsAtAvailableWidth(t *testing.T) {
	style := testStyle(10)
	text := elements.NewText("aaa bbb ccc ddd eee fff", style)

	plan := text.Measure(core.Size{Width: 40, Height: 1000})

	if !plan.Full() {
		t.Fatalf("plan = %v, want FullRender", plan)
	}

	// Greedy breaking fits two words per line here and no more: at 10pt
	// Helvetica "aaa bbb" advances 36.14pt, while adding "ccc" would reach
	// 53.92pt and overflow the 40pt column. Six words therefore give three lines.
	lineHeight := style.LineSpacing()
	lines := int(math.Round(plan.Size.Height / lineHeight))
	if lines != 3 {
		t.Errorf("text broke into %d lines in a 40pt column, want 3", lines)
	}
	if plan.Size.Width > 40+core.Epsilon {
		t.Errorf("reported width %.2f exceeds the 40pt column", plan.Size.Width)
	}
}

func TestTextSplitsAcrossPages(t *testing.T) {
	style := testStyle(10)
	text := elements.NewText("aaa bbb ccc ddd eee fff ggg hhh iii jjj", style)

	lineHeight := style.LineSpacing()
	// Offer room for exactly two lines.
	available := core.Size{Width: 40, Height: lineHeight * 2.5}

	first := text.Measure(available)
	if !first.Partial() {
		t.Fatalf("first plan = %v, want PartialRender", first)
	}
	closeTo(t, "first height", first.Size.Height, lineHeight*2)

	text.Draw(render_noop{}, core.Size{Width: 40, Height: first.Size.Height})

	second := text.Measure(available)
	if second.Wrapped() {
		t.Fatalf("second plan = %v, want progress after the first page", second)
	}
	if second.Size.Height <= 0 {
		t.Errorf("second page rendered no lines")
	}
}

func TestTextPreservesBlankLines(t *testing.T) {
	style := testStyle(10)
	text := elements.NewText("first\n\nsecond", style)

	plan := text.Measure(core.Size{Width: 500, Height: 1000})

	lines := int(math.Round(plan.Size.Height / style.LineSpacing()))
	// A blank line that collapsed to zero height would make this 2.
	if lines != 3 {
		t.Errorf("got %d lines for \"first\\n\\nsecond\", want 3", lines)
	}
}

func TestTextBreaksOverlongWord(t *testing.T) {
	style := testStyle(10)
	text := elements.NewText("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", style)

	plan := text.Measure(core.Size{Width: 30, Height: 1000})

	if plan.Wrapped() {
		t.Fatalf("plan = %v, want the word to be broken rather than wrapped", plan)
	}
	if plan.Size.Width > 30+core.Epsilon {
		t.Errorf("reported width %.2f overflows the 30pt column", plan.Size.Width)
	}
}

func TestPageBreakDefersOnceThenClears(t *testing.T) {
	pb := &elements.PageBreak{}

	first := pb.Measure(core.Size{Width: 100, Height: 100})
	if !first.Partial() {
		t.Fatalf("first plan = %v, want PartialRender so the page ends", first)
	}

	pb.Draw(render_noop{}, core.Size{})

	second := pb.Measure(core.Size{Width: 100, Height: 100})
	if !second.Full() {
		t.Fatalf("second plan = %v, want FullRender so the column continues", second)
	}
}

func TestResetTreeRewindsNestedState(t *testing.T) {
	inner := &splittingElement{rows: 10, rowHeight: 10}
	col := elements.NewColumn(0, inner)

	col.Measure(core.Size{Width: 100, Height: 45})
	col.Draw(render_noop{}, core.Size{Width: 100, Height: 40})

	if inner.rendered == 0 {
		t.Fatal("expected the child to have recorded progress")
	}

	core.ResetTree(col, true)

	if inner.rendered != 0 {
		t.Errorf("child progress = %d after reset, want 0", inner.rendered)
	}

	plan := col.Measure(core.Size{Width: 100, Height: 1000})
	if !plan.Full() {
		t.Fatalf("plan after reset = %v, want FullRender of the whole content", plan)
	}
	closeTo(t, "height", plan.Size.Height, 100)
}

// render_noop is a canvas that ignores everything, letting layout logic be
// tested without producing a document.
type render_noop struct{}

func (render_noop) Save()                                                         {}
func (render_noop) Restore()                                                      {}
func (render_noop) Translate(core.Position)                                       {}
func (render_noop) Rotate(float64)                                                {}
func (render_noop) ClipRect(core.Position, core.Size)                             {}
func (render_noop) DrawRect(core.Position, core.Size, core.Color)                 {}
func (render_noop) DrawRoundedRect(core.Position, core.Size, float64, core.Color) {}
func (render_noop) DrawLine(core.Position, core.Position, core.Color, float64)    {}
func (render_noop) DrawText(string, core.Position, core.TextStyle)                {}
func (render_noop) DrawImage(core.Image, core.Position, core.Size)                {}
func (render_noop) Fail(error)                                                    {}
func (render_noop) Err() error                                                    { return nil }
