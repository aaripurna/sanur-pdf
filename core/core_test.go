package core_test

import (
	"math"
	"strings"
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
)

// stubFont supplies predictable metrics so style arithmetic can be checked
// without depending on a real typeface.
//
// Every glyph advances a tenth of the size, which makes expected widths trivial
// to state: a five-character string at 20pt is exactly 10 points wide.
type stubFont struct{ name string }

func (f stubFont) Name() string { return f.name }

func (f stubFont) AdvanceOf(_ rune, size float64) float64 { return size / 10 }

func (f stubFont) Measure(text string, size float64) float64 {
	return float64(len([]rune(text))) * size / 10
}

func (f stubFont) Ascent(size float64) float64     { return size * 0.8 }
func (f stubFont) Descent(size float64) float64    { return size * 0.2 }
func (f stubFont) LineHeight(size float64) float64 { return size * 1.2 }

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("%s = %.6f, want %.6f", label, got, want)
	}
}

// --- geometry --------------------------------------------------------------

func TestSizeShrinkClampsAtZero(t *testing.T) {
	s := core.Size{Width: 10, Height: 10}

	got := s.Shrink(40, 3)

	// Over-large padding must not produce a negative available space, or every
	// element downstream would have to defend against it.
	closeTo(t, "width", got.Width, 0)
	closeTo(t, "height", got.Height, 7)
}

func TestSizeGrow(t *testing.T) {
	got := core.Size{Width: 10, Height: 20}.Grow(5, -5)

	closeTo(t, "width", got.Width, 15)
	closeTo(t, "height", got.Height, 15)
}

func TestFitsWithinToleratesEpsilon(t *testing.T) {
	available := core.Size{Width: 100, Height: 100}

	for _, tc := range []struct {
		name string
		size core.Size
		want bool
	}{
		{"exact", core.Size{Width: 100, Height: 100}, true},
		{"smaller", core.Size{Width: 99, Height: 99}, true},
		// Accumulated float error from nested containers must not be read as
		// an overflow, which is the entire reason Epsilon exists.
		{"within epsilon", core.Size{Width: 100.0000001, Height: 100}, true},
		{"wider", core.Size{Width: 101, Height: 100}, false},
		{"taller", core.Size{Width: 100, Height: 101}, false},
	} {
		if got := tc.size.FitsWithin(available); got != tc.want {
			t.Errorf("%s: FitsWithin = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSizeIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		size core.Size
		want bool
	}{
		{core.Size{}, true},
		{core.Size{Width: 0.0000001, Height: 0.0000001}, true},
		{core.Size{Width: 1}, false},
		{core.Size{Height: 1}, false},
	} {
		if got := tc.size.IsEmpty(); got != tc.want {
			t.Errorf("IsEmpty(%.7f x %.7f) = %v, want %v",
				tc.size.Width, tc.size.Height, got, tc.want)
		}
	}
}

func TestPositionAdd(t *testing.T) {
	got := core.Position{X: 3, Y: 4}.Add(10, -2)

	closeTo(t, "x", got.X, 13)
	closeTo(t, "y", got.Y, 2)
}

// --- space plans -----------------------------------------------------------

func TestSpacePlanPredicatesAreMutuallyExclusive(t *testing.T) {
	size := core.Size{Width: 10, Height: 20}

	for _, tc := range []struct {
		name                   string
		plan                   core.SpacePlan
		wrapped, partial, full bool
	}{
		{"wrap", core.Wrap("nope"), true, false, false},
		{"partial", core.PartialRender(size), false, true, false},
		{"full", core.FullRender(size), false, false, true},
		{"empty", core.EmptyRender(), false, false, true},
	} {
		if tc.plan.Wrapped() != tc.wrapped {
			t.Errorf("%s: Wrapped = %v, want %v", tc.name, tc.plan.Wrapped(), tc.wrapped)
		}
		if tc.plan.Partial() != tc.partial {
			t.Errorf("%s: Partial = %v, want %v", tc.name, tc.plan.Partial(), tc.partial)
		}
		if tc.plan.Full() != tc.full {
			t.Errorf("%s: Full = %v, want %v", tc.name, tc.plan.Full(), tc.full)
		}
	}
}

func TestEmptyRenderIsAZeroSizedFullRender(t *testing.T) {
	plan := core.EmptyRender()

	if !plan.Full() {
		t.Error("an empty render must count as fully rendered, or containers would never finish")
	}
	if !plan.Size.IsEmpty() {
		t.Errorf("size = %v, want zero", plan.Size)
	}
}

func TestWrapCarriesFormattedReason(t *testing.T) {
	plan := core.Wrap("needs %.1f but only %.1f available", 100.0, 50.0)

	want := "needs 100.0 but only 50.0 available"
	if plan.WrapReason != want {
		t.Errorf("WrapReason = %q, want %q", plan.WrapReason, want)
	}

	// Wrap reasons end up in user-facing layout errors, so they must survive
	// into the plan's own string form.
	if !strings.Contains(plan.String(), want) {
		t.Errorf("String = %q, want it to include the reason", plan.String())
	}
}

func TestWrapWithoutArgsLeavesReasonLiteral(t *testing.T) {
	// A reason containing a stray percent sign must not be mangled when no
	// formatting arguments were supplied.
	plan := core.Wrap("scaled to 50% and still too wide")

	if plan.WrapReason != "scaled to 50% and still too wide" {
		t.Errorf("WrapReason = %q", plan.WrapReason)
	}
}

func TestSpacePlanStringIncludesSize(t *testing.T) {
	got := core.FullRender(core.Size{Width: 12.5, Height: 30}).String()

	for _, want := range []string{"FullRender", "12.50", "30.00"} {
		if !strings.Contains(got, want) {
			t.Errorf("String = %q, want it to contain %q", got, want)
		}
	}
}

func TestSpacePlanTypeStrings(t *testing.T) {
	for _, tc := range []struct {
		typ  core.SpacePlanType
		want string
	}{
		{core.SpaceWrap, "Wrap"},
		{core.SpacePartialRender, "PartialRender"},
		{core.SpaceFullRender, "FullRender"},
		{core.SpacePlanType(99), "Unknown"},
	} {
		if got := tc.typ.String(); got != tc.want {
			t.Errorf("SpacePlanType(%d).String() = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

// --- colour ----------------------------------------------------------------

func TestRGBIsOpaque(t *testing.T) {
	c := core.RGB(10, 20, 30)

	if !c.Opaque() || !c.Visible() {
		t.Errorf("RGB colour = %v, want opaque and visible", c)
	}
	if c.A != 255 {
		t.Errorf("alpha = %d, want 255", c.A)
	}
}

func TestTransparentIsInvisible(t *testing.T) {
	if core.Transparent.Visible() {
		t.Error("the zero colour must be invisible so elements can skip drawing")
	}
}

func TestParseHexForms(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want core.Color
	}{
		{"#1E88E5", core.RGBA(0x1E, 0x88, 0xE5, 255)},
		{"1E88E5", core.RGBA(0x1E, 0x88, 0xE5, 255)},
		{"  #1e88e5  ", core.RGBA(0x1E, 0x88, 0xE5, 255)},
		// Shorthand repeats each digit, so #abc means #aabbcc.
		{"#abc", core.RGBA(0xAA, 0xBB, 0xCC, 255)},
		{"#abcd", core.RGBA(0xAA, 0xBB, 0xCC, 0xDD)},
		{"#1E88E580", core.RGBA(0x1E, 0x88, 0xE5, 0x80)},
	} {
		got, err := core.ParseHex(tc.in)
		if err != nil {
			t.Errorf("ParseHex(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseHex(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseHexRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{"", "#", "#12", "#12345", "#1234567", "#123456789", "#gggggg", "not a colour"} {
		if _, err := core.ParseHex(in); err == nil {
			t.Errorf("ParseHex(%q) succeeded, want an error", in)
		}
	}
}

func TestHexPanicsOnMalformedInput(t *testing.T) {
	// Colours are written as literals in layout code, so a bad one is a
	// programming error that should surface on the first run.
	defer func() {
		if recover() == nil {
			t.Error("Hex did not panic on malformed input")
		}
	}()
	core.Hex("#zzz")
}

func TestColorComponentsAreNormalised(t *testing.T) {
	r, g, b := core.RGB(255, 128, 0).Components()

	closeTo(t, "r", r, 1)
	closeTo(t, "g", g, 128.0/255)
	closeTo(t, "b", b, 0)
	closeTo(t, "alpha", core.RGBA(0, 0, 0, 51).Alpha(), 0.2)
}

func TestColorStringOmitsAlphaWhenOpaque(t *testing.T) {
	if got := core.RGB(0x1E, 0x88, 0xE5).String(); got != "#1E88E5" {
		t.Errorf("opaque String = %q, want #1E88E5", got)
	}
	if got := core.RGBA(0x1E, 0x88, 0xE5, 0x80).String(); got != "#1E88E580" {
		t.Errorf("translucent String = %q, want #1E88E580", got)
	}
}

// --- text styles -----------------------------------------------------------

func TestLineSpacingUsesFontDefaultUnlessOverridden(t *testing.T) {
	style := core.TextStyle{Font: stubFont{}, Size: 10}

	closeTo(t, "default", style.LineSpacing(), 12) // 10 * 1.2

	style.LineHeightFactor = 2
	// The factor multiplies the font's own line height, not the em size.
	closeTo(t, "doubled", style.LineSpacing(), 24)
}

func TestMeasureTextIncludesLetterAndWordSpacing(t *testing.T) {
	base := core.TextStyle{Font: stubFont{}, Size: 10}

	closeTo(t, "plain", base.MeasureText("ab cd"), 5) // 5 runes at 1pt each

	letter := base
	letter.LetterSpacing = 2
	closeTo(t, "letter spacing", letter.MeasureText("ab cd"), 5+5*2)

	word := base
	word.WordSpacing = 3
	// Only the single space is stretched.
	closeTo(t, "word spacing", word.MeasureText("ab cd"), 5+3)

	both := base
	both.LetterSpacing = 2
	both.WordSpacing = 3
	closeTo(t, "both", both.MeasureText("ab cd"), 5+5*2+3)
}

func TestMeasureTextHandlesEmptyString(t *testing.T) {
	style := core.TextStyle{Font: stubFont{}, Size: 10, LetterSpacing: 5, WordSpacing: 5}

	closeTo(t, "empty", style.MeasureText(""), 0)
}

// --- alignment -------------------------------------------------------------

func TestHorizontalAlignOffsets(t *testing.T) {
	for _, tc := range []struct {
		align core.HorizontalAlign
		want  float64
	}{
		{core.AlignLeft, 0},
		{core.AlignCenter, 25},
		{core.AlignRight, 50},
		// Justification is a text-level concern; as a box offset it behaves as
		// flush left.
		{core.AlignJustify, 0},
	} {
		if got := tc.align.OffsetX(100, 50); math.Abs(got-tc.want) > 0.0001 {
			t.Errorf("align %d: OffsetX(100, 50) = %.2f, want %.2f", tc.align, got, tc.want)
		}
	}
}

func TestVerticalAlignOffsets(t *testing.T) {
	for _, tc := range []struct {
		align core.VerticalAlign
		want  float64
	}{
		{core.AlignTop, 0},
		{core.AlignMiddle, 30},
		{core.AlignBottom, 60},
	} {
		if got := tc.align.OffsetY(100, 40); math.Abs(got-tc.want) > 0.0001 {
			t.Errorf("align %d: OffsetY(100, 40) = %.2f, want %.2f", tc.align, got, tc.want)
		}
	}
}

// --- tree helpers ----------------------------------------------------------

// recorder is an element that logs the tree operations applied to it.
type recorder struct {
	children []core.Element

	hardResets int
	softResets int
	ctx        core.PageContext
	measures   int
	draws      int
}

func (r *recorder) Measure(core.Size) core.SpacePlan {
	r.measures++
	return core.EmptyRender()
}

func (r *recorder) Draw(core.Canvas, core.Size) { r.draws++ }

func (r *recorder) Children() []core.Element { return r.children }

func (r *recorder) ResetState(hard bool) {
	if hard {
		r.hardResets++
		return
	}
	r.softResets++
}

func (r *recorder) SetPageContext(ctx core.PageContext) { r.ctx = ctx }

func TestResetTreeReachesEveryDescendant(t *testing.T) {
	leaf := &recorder{}
	middle := &recorder{children: []core.Element{leaf}}
	root := &recorder{children: []core.Element{middle}}

	core.ResetTree(root, true)

	for name, r := range map[string]*recorder{"root": root, "middle": middle, "leaf": leaf} {
		if r.hardResets != 1 {
			t.Errorf("%s: hard resets = %d, want 1", name, r.hardResets)
		}
	}

	core.ResetTree(root, false)
	if leaf.softResets != 1 {
		t.Errorf("leaf soft resets = %d, want 1", leaf.softResets)
	}
	if leaf.hardResets != 1 {
		t.Errorf("a soft reset must not count as a hard one; got %d hard", leaf.hardResets)
	}
}

func TestResetTreeToleratesNilAndPlainElements(t *testing.T) {
	// Neither a nil root nor a child that implements none of the optional
	// interfaces may panic, since most elements are stateless.
	core.ResetTree(nil, true)
	core.ResetTree(&recorder{children: []core.Element{plainElement{}}}, true)
}

func TestApplyPageContextReachesEveryDescendant(t *testing.T) {
	leaf := &recorder{}
	root := &recorder{children: []core.Element{leaf, plainElement{}}}

	ctx := core.PageContext{PageNumber: 3, TotalPages: 7}
	core.ApplyPageContext(root, ctx)

	if leaf.ctx != ctx {
		t.Errorf("leaf context = %+v, want %+v", leaf.ctx, ctx)
	}
	if root.ctx != ctx {
		t.Errorf("root context = %+v, want %+v", root.ctx, ctx)
	}

	core.ApplyPageContext(nil, ctx)
}

func TestMeasureChildTreatsNilAsEmpty(t *testing.T) {
	plan := core.MeasureChild(nil, core.Size{Width: 10, Height: 10})

	// An unpopulated container slot must degrade to zero size, not panic.
	if !plan.Full() || !plan.Size.IsEmpty() {
		t.Errorf("plan = %v, want an empty full render", plan)
	}

	child := &recorder{}
	if plan := core.MeasureChild(child, core.Size{}); !plan.Full() {
		t.Errorf("plan = %v, want the child's own plan", plan)
	}
	if child.measures != 1 {
		t.Errorf("child measured %d times, want 1", child.measures)
	}
}

func TestDrawChildSkipsNil(t *testing.T) {
	core.DrawChild(nil, nil, core.Size{})

	child := &recorder{}
	core.DrawChild(child, nil, core.Size{})
	if child.draws != 1 {
		t.Errorf("child drawn %d times, want 1", child.draws)
	}
}

func TestImageAspectRatio(t *testing.T) {
	img := core.Image{PixelWidth: 200, PixelHeight: 100}
	closeTo(t, "ratio", img.AspectRatio(), 2)

	// A zero height would divide by zero; the degenerate case reports no ratio
	// so callers can skip the image rather than producing NaN geometry.
	closeTo(t, "degenerate", core.Image{PixelWidth: 10}.AspectRatio(), 0)
}

// plainElement implements only core.Element, with none of the optional
// interfaces the tree helpers look for.
type plainElement struct{}

func (plainElement) Measure(core.Size) core.SpacePlan { return core.EmptyRender() }
func (plainElement) Draw(core.Canvas, core.Size)      {}
