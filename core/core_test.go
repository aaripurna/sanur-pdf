package core_test

import (
	"encoding/json"
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
	if c.Opacity() != 255 {
		t.Errorf("alpha = %d, want 255", c.Opacity())
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
	r, g, b := core.RGB(255, 128, 0).RGBComponents()

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

// --- CMYK colour ------------------------------------------------------------

func TestCMYKKeepsItsSpace(t *testing.T) {
	// The space has to survive, because the whole point of specifying CMYK is that
	// the plates reach the printer rather than a conversion of them.
	c := core.CMYKPercent(0, 0, 0, 100)

	if c.Space() != core.SpaceCMYK {
		t.Errorf("space = %v, want CMYK", c.Space())
	}

	cy, m, y, k := c.CMYKComponents()
	closeTo(t, "cyan", cy, 0)
	closeTo(t, "magenta", m, 0)
	closeTo(t, "yellow", y, 0)
	closeTo(t, "black", k, 1)
}

func TestRGBIsTheZeroSpace(t *testing.T) {
	// A Color nobody has set must behave as RGB, since that is what every existing
	// caller means.
	if core.Transparent.Space() != core.SpaceRGB {
		t.Errorf("zero colour space = %v, want RGB", core.Transparent.Space())
	}
	if core.RGB(1, 2, 3).Space() != core.SpaceRGB {
		t.Error("RGB constructor did not produce an RGB colour")
	}
}

func TestCMYKPercentClampsOutOfRangeValues(t *testing.T) {
	// Percentages are arithmetic output as often as literals, and a clamped plate is
	// closer to the intent than an error.
	cy, m, y, k := core.CMYKPercent(-20, 150, 0, 100).CMYKComponents()

	closeTo(t, "cyan", cy, 0)
	closeTo(t, "magenta", m, 1)
	closeTo(t, "yellow", y, 0)
	closeTo(t, "black", k, 1)
}

func TestSpaceString(t *testing.T) {
	if got := core.SpaceRGB.String(); got != "RGB" {
		t.Errorf("SpaceRGB = %q", got)
	}
	if got := core.SpaceCMYK.String(); got != "CMYK" {
		t.Errorf("SpaceCMYK = %q", got)
	}
}

func TestWithAlphaKeepsTheSpaceAndPlates(t *testing.T) {
	faded := core.CMYKPercent(10, 20, 30, 40).WithAlpha(0x80)

	if faded.Space() != core.SpaceCMYK {
		t.Errorf("space = %v, want CMYK", faded.Space())
	}
	if faded.Opacity() != 0x80 {
		t.Errorf("alpha = %d, want 0x80", faded.Opacity())
	}

	// The plates are stored as bytes, so a percentage lands on the nearest 1/255.
	cy, _, _, k := faded.CMYKComponents()
	closeTo(t, "cyan", cy, 26.0/255)
	closeTo(t, "black", k, 102.0/255)
}

func TestCMYKConvertsToRGBForPreview(t *testing.T) {
	// Not colour management — a faithful conversion needs ICC profiles — but a CMYK
	// colour still has to answer an RGB query for anything that only speaks RGB.
	r, g, b := core.CMYKPercent(0, 100, 100, 0).RGBComponents()

	closeTo(t, "r", r, 1)
	closeTo(t, "g", g, 0)
	closeTo(t, "b", b, 0)
}

func TestRGBConvertsToCMYKWithBlackSeparated(t *testing.T) {
	for _, tc := range []struct {
		name        string
		in          core.Color
		cy, m, y, k float64
	}{
		// Pure black becomes 100% K alone rather than four plates, which is the safer
		// default for text: four-plate black on a press needs registration to be
		// perfect or the letters fringe.
		{"black", core.RGB(0, 0, 0), 0, 0, 0, 1},
		{"white", core.RGB(255, 255, 255), 0, 0, 0, 0},
		{"red", core.RGB(255, 0, 0), 0, 1, 1, 0},
		{"mid grey", core.RGB(128, 128, 128), 0, 0, 0, 1 - 128.0/255},
	} {
		cy, m, y, k := tc.in.CMYKComponents()

		closeTo(t, tc.name+" cyan", cy, tc.cy)
		closeTo(t, tc.name+" magenta", m, tc.m)
		closeTo(t, tc.name+" yellow", y, tc.y)
		closeTo(t, tc.name+" black", k, tc.k)
	}
}

func TestParseColorAcceptsBothNotations(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want core.Color
	}{
		{"#1E88E5", core.RGB(0x1E, 0x88, 0xE5)},
		{"cmyk(0, 0, 0, 100)", core.CMYKPercent(0, 0, 0, 100)},
		{"CMYK(100,0,0,0)", core.CMYKPercent(100, 0, 0, 0)},
		{"  cmyk( 10% , 20% , 30% , 40% )  ", core.CMYKPercent(10, 20, 30, 40)},
		// A fifth value is opacity, also a percentage.
		{"cmyk(0, 0, 0, 100, 50)", core.CMYKPercent(0, 0, 0, 100).WithAlpha(128)},
	} {
		got, err := core.ParseColor(tc.in)
		if err != nil {
			t.Errorf("ParseColor(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseColor(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseColorRejectsMalformedCMYK(t *testing.T) {
	for _, in := range []string{
		"cmyk(0, 0, 0)",
		"cmyk(0, 0, 0, 0, 0, 0)",
		"cmyk(0, 0, 0, black)",
		"cmyk 0 0 0 0",
		"cmyk(0, 0, 0, 0",
	} {
		if _, err := core.ParseColor(in); err == nil {
			t.Errorf("ParseColor(%q) succeeded, want an error", in)
		}
	}
}

func TestColorStringRoundTrips(t *testing.T) {
	// String and ParseColor have to be exact inverses, or a theme written back out
	// would not reload as the same document.
	for _, want := range []core.Color{
		core.RGB(0x1E, 0x88, 0xE5),
		core.RGBA(0x1E, 0x88, 0xE5, 0x80),
		core.CMYKPercent(0, 0, 0, 100),
		core.CMYKPercent(10.2, 20.4, 30.6, 40.8),
		core.CMYK(1, 2, 3, 4),
		core.CMYKA(200, 150, 100, 50, 77),
	} {
		text := want.String()

		got, err := core.ParseColor(text)
		if err != nil {
			t.Errorf("ParseColor(%q): %v", text, err)
			continue
		}
		if got != want {
			t.Errorf("%q parsed back as %q", text, got.String())
		}
	}
}

func TestCMYKStringUsesPlatePercentages(t *testing.T) {
	if got := core.CMYKPercent(0, 0, 0, 100).String(); got != "cmyk(0, 0, 0, 100)" {
		t.Errorf("String = %q, want cmyk(0, 0, 0, 100)", got)
	}
	if got := core.CMYKPercent(0, 0, 0, 100).WithAlpha(128).String(); got != "cmyk(0, 0, 0, 100, 50.2)" {
		t.Errorf("translucent String = %q", got)
	}
}

func TestColorJSONRoundTrip(t *testing.T) {
	type holder struct {
		Ink   core.Color `json:"ink"`
		Plate core.Color `json:"plate"`
	}

	in := holder{Ink: core.RGBA(0x1E, 0x88, 0xE5, 0x80), Plate: core.CMYKPercent(0, 0, 0, 100)}

	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	var out holder
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatalf("unmarshalling %s: %v", encoded, err)
	}
	if out != in {
		t.Errorf("round trip gave %+v, want %+v (via %s)", out, in, encoded)
	}
}

func TestColorJSONNullMeansInherit(t *testing.T) {
	// Zeroing on null would make an explicit null mean "invisible", which reads as a
	// missing colour rather than as deference to the default.
	colour := core.RGB(1, 2, 3)

	if err := colour.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatalf("null: %v", err)
	}
	if colour != core.RGB(1, 2, 3) {
		t.Errorf("null overwrote the colour with %v", colour)
	}
}

func TestColorJSONRejectsNonStrings(t *testing.T) {
	for _, in := range []string{`42`, `{"r": 1}`, `"#nope"`, `[]`} {
		var colour core.Color
		if err := colour.UnmarshalJSON([]byte(in)); err == nil {
			t.Errorf("UnmarshalJSON(%s) succeeded, want an error", in)
		}
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

// --- colour JSON ------------------------------------------------------------

func TestColorRoundTripsThroughJSON(t *testing.T) {
	for _, original := range []core.Color{
		core.RGB(0x1A, 0x1D, 0x29),
		core.RGBA(0x4F, 0x46, 0xE5, 0x80),
		core.RGB(0, 0, 0),
		core.RGB(255, 255, 255),
	} {
		encoded, err := json.Marshal(original)
		if err != nil {
			t.Errorf("marshalling %v: %v", original, err)
			continue
		}

		var decoded core.Color
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Errorf("unmarshalling %s: %v", encoded, err)
			continue
		}
		if decoded != original {
			t.Errorf("round trip gave %v, want %v (via %s)", decoded, original, encoded)
		}
	}
}

func TestColorMarshalsAsHex(t *testing.T) {
	// Hex rather than an object of channels, because that is how a colour is
	// written wherever a person types one.
	encoded, err := json.Marshal(core.RGB(0x4F, 0x46, 0xE5))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"#4F46E5"` {
		t.Errorf("encoded = %s, want a hex string", encoded)
	}

	// Alpha only appears when it matters.
	encoded, _ = json.Marshal(core.RGBA(0, 0, 0, 0x80))
	if string(encoded) != `"#00000080"` {
		t.Errorf("translucent encoded = %s", encoded)
	}
}

func TestColorUnmarshalAcceptsEveryHexForm(t *testing.T) {
	for _, tc := range []struct {
		json string
		want core.Color
	}{
		{`"#abc"`, core.RGB(0xAA, 0xBB, 0xCC)},
		{`"#1E88E5"`, core.RGB(0x1E, 0x88, 0xE5)},
		{`"1E88E5"`, core.RGB(0x1E, 0x88, 0xE5)},
		{`"#1E88E580"`, core.RGBA(0x1E, 0x88, 0xE5, 0x80)},
	} {
		var got core.Color
		if err := json.Unmarshal([]byte(tc.json), &got); err != nil {
			t.Errorf("unmarshalling %s: %v", tc.json, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s decoded to %v, want %v", tc.json, got, tc.want)
		}
	}
}

func TestColorUnmarshalNullLeavesTheValueAlone(t *testing.T) {
	// An explicit null means "inherit" rather than "invisible", matching the
	// resolve-against-defaults convention used elsewhere.
	existing := core.RGB(1, 2, 3)

	if err := json.Unmarshal([]byte("null"), &existing); err != nil {
		t.Fatal(err)
	}
	if existing != core.RGB(1, 2, 3) {
		t.Errorf("null overwrote the colour, giving %v", existing)
	}
}

func TestColorUnmarshalRejectsNonStrings(t *testing.T) {
	for _, bad := range []string{`42`, `true`, `{"r": 1}`, `[1,2,3]`, `"not a colour"`} {
		var got core.Color
		if err := json.Unmarshal([]byte(bad), &got); err == nil {
			t.Errorf("%s was accepted as a colour", bad)
		}
	}
}

// --- page sizes -------------------------------------------------------------

func TestParseSizeNamesAndOrientation(t *testing.T) {
	for _, tc := range []struct {
		name string
		want core.Size
	}{
		{"A4", core.A4},
		{"a4", core.A4},
		{"  A4  ", core.A4},
		{"Letter", core.Letter},
		{"A4 landscape", core.Landscape(core.A4)},
		{"a4 LANDSCAPE", core.Landscape(core.A4)},
		{"A4 portrait", core.A4},
		{"Executive", core.Executive},
	} {
		got, ok := core.ParseSize(tc.name)
		if !ok {
			t.Errorf("ParseSize(%q) failed", tc.name)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSize(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseSizeRejectsUnknownInput(t *testing.T) {
	for _, bad := range []string{"", "   ", "Foolscap", "A4 sideways", "A99"} {
		if _, ok := core.ParseSize(bad); ok {
			t.Errorf("ParseSize(%q) succeeded", bad)
		}
	}
}

func TestEverySizeNameParses(t *testing.T) {
	// The advertised names have to resolve, or an error message listing them would
	// be lying.
	for _, name := range core.SizeNames() {
		if _, ok := core.ParseSize(name); !ok {
			t.Errorf("advertised size %q does not parse", name)
		}
	}
}

func TestUnitConversionsInCore(t *testing.T) {
	closeTo(t, "one inch", core.In(1), 72)
	closeTo(t, "25.4 mm", core.Mm(25.4), 72)
	closeTo(t, "one cm", core.Cm(1), 28.3464566929)
}

func TestLandscapeSwapsDimensionsInCore(t *testing.T) {
	got := core.Landscape(core.A4)
	if got.Width != core.A4.Height || got.Height != core.A4.Width {
		t.Errorf("Landscape(A4) = %v, want the dimensions swapped", got)
	}
}
