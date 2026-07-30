package chart_test

import (
	"math"
	"strings"
	"testing"

	"github.com/aaripurna/sanur-pdf/chart"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}

// --- ticks ------------------------------------------------------------------

func TestNiceStepUsesReadableMultipliers(t *testing.T) {
	// An axis stepping by 25 is harder to scan than one stepping by 20 or 50, so
	// only 1, 2, 5 and 10 times a power of ten are produced.
	for _, tc := range []struct {
		raw  float64
		want float64
	}{
		{0.4, 0.5},
		{0.9, 1},
		{1, 1},
		{1.5, 2},
		{3, 5},
		{6, 10},
		{9.9, 10},
		{12, 20},
		{37, 50},
		{240, 500},
		{1200, 2000},
	} {
		if got := chart.NiceStep(tc.raw); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("NiceStep(%g) = %g, want %g", tc.raw, got, tc.want)
		}
	}
}

func TestNiceStepHandlesDegenerateInput(t *testing.T) {
	for _, raw := range []float64{0, -5, math.NaN(), math.Inf(1)} {
		if got := chart.NiceStep(raw); got != 1 {
			t.Errorf("NiceStep(%g) = %g, want the fallback of 1", raw, got)
		}
	}
}

func TestTicksBracketTheDataOnRoundNumbers(t *testing.T) {
	ticks := chart.Ticks(3.1, 4.82, 5)

	if len(ticks) < 2 {
		t.Fatalf("got %d ticks, want at least 2", len(ticks))
	}
	// Ticks taken from the extremes would label the axis 3.1 to 4.82.
	if ticks[0] > 3.1 {
		t.Errorf("first tick %g does not reach below the data minimum 3.1", ticks[0])
	}
	if ticks[len(ticks)-1] < 4.82 {
		t.Errorf("last tick %g does not reach above the data maximum 4.82", ticks[len(ticks)-1])
	}
}

func TestTicksAreEvenlySpacedAndClean(t *testing.T) {
	ticks := chart.Ticks(0, 1940, 4)

	if len(ticks) < 3 {
		t.Fatalf("got %d ticks, want several", len(ticks))
	}

	step := ticks[1] - ticks[0]
	for i := 2; i < len(ticks); i++ {
		if math.Abs((ticks[i]-ticks[i-1])-step) > 1e-6 {
			t.Errorf("tick spacing is uneven at index %d: %v", i, ticks)
		}
	}

	// Repeated addition would drift, showing up as a label like "500.0000001".
	for _, v := range ticks {
		if math.Abs(v-math.Round(v)) > 1e-9 {
			t.Errorf("tick %v carries accumulated float error", v)
		}
	}
}

func TestTicksSpanZeroForNegativeData(t *testing.T) {
	ticks := chart.Ticks(-45, 150, 5)

	if ticks[0] > -45 || ticks[len(ticks)-1] < 150 {
		t.Errorf("ticks %v do not bracket -45..150", ticks)
	}

	// Zero has to be one of them, or the axis line would sit between gridlines.
	found := false
	for _, v := range ticks {
		if v == 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("ticks %v omit zero", ticks)
	}
}

func TestTicksToleratesFlatAndInvertedRanges(t *testing.T) {
	for _, tc := range []struct {
		name      string
		low, high float64
	}{
		{"flat", 5, 5},
		{"inverted", 10, 2},
		{"both zero", 0, 0},
	} {
		ticks := chart.Ticks(tc.low, tc.high, 5)
		if len(ticks) < 2 {
			t.Errorf("%s: got %d ticks, want at least 2", tc.name, len(ticks))
		}
		if ticks[len(ticks)-1] <= ticks[0] {
			t.Errorf("%s: ticks %v do not increase", tc.name, ticks)
		}
	}
}

func TestTicksClampsTheTarget(t *testing.T) {
	// A target below two would give an axis with no interval to read.
	if got := chart.Ticks(0, 10, 0); len(got) < 2 {
		t.Errorf("got %d ticks for a zero target, want at least 2", len(got))
	}
}

// --- scale ------------------------------------------------------------------

func TestScaleMapsDataOntoCoordinates(t *testing.T) {
	s := chart.Scale{Low: 0, High: 100, From: 0, To: 200}

	closeTo(t, "low", s.At(0), 0)
	closeTo(t, "mid", s.At(50), 100)
	closeTo(t, "high", s.At(100), 200)
	closeTo(t, "span", s.Span(), 200)
}

func TestScaleInvertsWithoutSpecialCasing(t *testing.T) {
	// A vertical axis runs from the bottom of the plot to the top, and layout
	// space grows downwards, so From is the larger coordinate.
	s := chart.Scale{Low: 0, High: 100, From: 150, To: 50}

	closeTo(t, "low maps to the bottom", s.At(0), 150)
	closeTo(t, "high maps to the top", s.At(100), 50)
	closeTo(t, "span", s.Span(), 100)
}

func TestScaleExtrapolatesBeyondItsDomain(t *testing.T) {
	s := chart.Scale{Low: 0, High: 10, From: 0, To: 100}

	// Clamping would flatten an overshooting series against the axis and hide it.
	closeTo(t, "above", s.At(20), 200)
	closeTo(t, "below", s.At(-5), -50)
}

func TestScaleWithZeroWidthDomain(t *testing.T) {
	s := chart.Scale{Low: 5, High: 5, From: 0, To: 100}

	// No gradient exists, so everything lands on the midpoint rather than
	// dividing by zero.
	closeTo(t, "midpoint", s.At(5), 50)
	closeTo(t, "midpoint again", s.At(99), 50)
}

// --- formatting -------------------------------------------------------------

func TestFormatValueAbbreviatesLargeNumbers(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{7.5, "7.5"},
		{-42, "-42"},
		{999, "999"},
		{1500, "1500"},
		{9999, "9999"},
		// From ten thousand up, an abbreviation saves enough room to be worth the
		// lost precision.
		{10000, "10k"},
		{12500, "12.5k"},
		{1940000, "1.9M"},
		{4820000000, "4.8B"},
	} {
		if got := chart.FormatValue(tc.v); got != tc.want {
			t.Errorf("FormatValue(%g) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

// --- style ------------------------------------------------------------------

func TestDefaultStyleIsComplete(t *testing.T) {
	s := chart.DefaultStyle()

	if len(s.Palette) == 0 {
		t.Error("the default palette is empty")
	}
	for name, style := range map[string]core.TextStyle{
		"label": s.Label, "value": s.ValueLabel, "legend": s.LegendLabel,
	} {
		if style.Font == nil {
			t.Errorf("%s style has no font", name)
		}
		if style.Size <= 0 {
			t.Errorf("%s style has no size", name)
		}
	}
}

func TestSeriesAtReportsMissingValues(t *testing.T) {
	s := chart.Series{Values: []float64{1, 2, 3}}

	if v, ok := s.At(1); !ok || v != 2 {
		t.Errorf("At(1) = %v, %v; want 2, true", v, ok)
	}
	// A series shorter than the category list stops rather than reading past its
	// end, which is what lets an incomplete final period render as a short line.
	for _, i := range []int{-1, 3, 99} {
		if _, ok := s.At(i); ok {
			t.Errorf("At(%d) reported a value that does not exist", i)
		}
	}
}

// --- element semantics ------------------------------------------------------

var box = core.Size{Width: 400, Height: 200}

func TestChartsFillTheOfferedBox(t *testing.T) {
	// A plot has no natural size, so every chart takes what it is given and the
	// caller constrains it.
	for name, element := range map[string]core.Element{
		"line": &chart.Line{
			Categories: []string{"a", "b", "c"},
			Series:     []chart.Series{{Values: []float64{1, 2, 3}}},
		},
		"bar": &chart.Bar{
			Categories: []string{"a", "b"},
			Series:     []chart.Series{{Values: []float64{1, 2}}},
		},
		"pie": &chart.Pie{
			Slices: []chart.Slice{{Name: "a", Value: 1}, {Name: "b", Value: 2}},
		},
	} {
		plan := element.Measure(box)
		if !plan.Full() {
			t.Errorf("%s: plan = %v, want FullRender", name, plan)
		}
		if plan.Size != box {
			t.Errorf("%s: size = %v, want the offered %v", name, plan.Size, box)
		}
	}
}

func TestChartsWithNothingToPlotAreEmpty(t *testing.T) {
	for name, element := range map[string]core.Element{
		"no series":     &chart.Line{Categories: []string{"a", "b"}},
		"no categories": &chart.Line{Series: []chart.Series{{Values: []float64{1}}}},
		// One category cannot make a line, only a point.
		"single category": &chart.Line{
			Categories: []string{"a"},
			Series:     []chart.Series{{Values: []float64{1}}},
		},
		"empty bar": &chart.Bar{},
		"empty pie": &chart.Pie{},
		// A pie of zeros has no circle to divide.
		"zero pie": &chart.Pie{Slices: []chart.Slice{{Name: "a", Value: 0}}},
	} {
		plan := element.Measure(box)
		if !plan.Size.IsEmpty() {
			t.Errorf("%s: size = %v, want empty", name, plan.Size)
		}

		// Drawing an empty chart must be a no-op rather than a panic.
		canvas := &recorder{}
		element.Draw(canvas, box)
		if canvas.paths > 0 || canvas.texts > 0 {
			t.Errorf("%s: drew %d paths and %d labels, want none",
				name, canvas.paths, canvas.texts)
		}
	}
}

func TestChartsSurviveDegenerateBoxes(t *testing.T) {
	charts := []core.Element{
		&chart.Line{
			Categories: []string{"a", "b", "c"},
			Series:     []chart.Series{{Name: "s", Values: []float64{1, 2, 3}}},
			Area:       true,
		},
		&chart.Bar{
			Categories: []string{"a", "b"},
			Series:     []chart.Series{{Name: "s", Values: []float64{1, -2}}},
		},
		&chart.Pie{
			Slices:      []chart.Slice{{Name: "a", Value: 1}},
			InnerRadius: 40,
			CentreLabel: "x",
		},
	}

	// Boxes too small to hold the axis furniture must draw nothing rather than
	// emitting inverted geometry a reader would reject.
	for _, size := range []core.Size{
		{}, {Width: 400}, {Height: 200},
		{Width: 5, Height: 5}, {Width: 30, Height: 12},
	} {
		for i, c := range charts {
			canvas := &recorder{}
			c.Measure(size)
			c.Draw(canvas, size)
			if canvas.negative {
				t.Errorf("chart %d at %v produced negative geometry", i, size)
			}
		}
	}
}

// --- drawing behaviour ------------------------------------------------------

func TestLineDrawsSeriesGridAndLegend(t *testing.T) {
	canvas := &recorder{}

	c := &chart.Line{
		Categories: []string{"Jan", "Feb", "Mar"},
		Series: []chart.Series{
			{Name: "Revenue", Values: []float64{10, 20, 15}},
			{Name: "Costs", Values: []float64{5, 8, 7}},
		},
	}
	c.Draw(canvas, box)

	// Category names, tick values and legend entries all reach the page.
	for _, want := range []string{"Jan", "Feb", "Mar", "Revenue", "Costs"} {
		if !canvas.hasText(want) {
			t.Errorf("missing label %q; drew %v", want, canvas.strings)
		}
	}
	if canvas.paths == 0 {
		t.Error("no paths drawn")
	}
}

func TestLineAreaAddsFills(t *testing.T) {
	base := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b", "c"},
		Series:     []chart.Series{{Values: []float64{1, 2, 3}}},
	}).Draw(base, box)

	filled := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b", "c"},
		Series:     []chart.Series{{Values: []float64{1, 2, 3}}},
		Area:       true,
	}).Draw(filled, box)

	if filled.fills <= base.fills {
		t.Errorf("Area drew %d fills, want more than the %d without it",
			filled.fills, base.fills)
	}
}

func TestLineHideMarkersReducesDrawing(t *testing.T) {
	with := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b", "c", "d"},
		Series:     []chart.Series{{Values: []float64{1, 2, 3, 4}}},
	}).Draw(with, box)

	without := &recorder{}
	(&chart.Line{
		Categories:  []string{"a", "b", "c", "d"},
		Series:      []chart.Series{{Values: []float64{1, 2, 3, 4}}},
		HideMarkers: true,
	}).Draw(without, box)

	if without.paths >= with.paths {
		t.Errorf("hiding markers drew %d paths, want fewer than %d",
			without.paths, with.paths)
	}
}

func TestLineToleratesShortSeries(t *testing.T) {
	canvas := &recorder{}

	// An incomplete final period should render as a line that stops, not one that
	// drops to zero.
	(&chart.Line{
		Categories: []string{"Jan", "Feb", "Mar", "Apr"},
		Series:     []chart.Series{{Name: "partial", Values: []float64{1, 2}}},
	}).Draw(canvas, box)

	if canvas.paths == 0 {
		t.Error("a short series drew nothing")
	}
	if !canvas.hasText("Apr") {
		t.Error("categories beyond the series should still be labelled")
	}
}

func TestBarValueLabelsAppear(t *testing.T) {
	canvas := &recorder{}

	(&chart.Bar{
		Categories: []string{"A", "B"},
		Series:     []chart.Series{{Values: []float64{120, 45}}},
	}).Draw(canvas, box)

	for _, want := range []string{"120", "45", "A", "B"} {
		if !canvas.hasText(want) {
			t.Errorf("missing %q; drew %v", want, canvas.strings)
		}
	}
}

func TestBarHideValueLabels(t *testing.T) {
	canvas := &recorder{}

	(&chart.Bar{
		Categories: []string{"A", "B"},
		Series:     []chart.Series{{Values: []float64{120, 45}}},
		Style:      chart.Style{HideValueLabels: true},
	}).Draw(canvas, box)

	if canvas.hasText("120") {
		t.Error("value labels should be suppressed")
	}
	if !canvas.hasText("A") {
		t.Error("category labels should still be drawn")
	}
}

func TestNegativeBarLabelsClearTheCategoryLabels(t *testing.T) {
	canvas := &recorder{}

	c := &chart.Bar{
		Categories: []string{"Jan", "Feb"},
		Series:     []chart.Series{{Values: []float64{120, -45}}},
	}
	c.Draw(canvas, box)

	// A negative bar's label hangs below it, where the category labels go. The two
	// must not land on the same line: the reservation for one is what keeps the
	// other clear, and getting it wrong is silent in the output.
	value, okValue := canvas.textAt("-45")
	category, okCategory := canvas.textAt("Feb")

	if !okValue || !okCategory {
		t.Fatalf("expected both labels; drew %v", canvas.strings)
	}
	if category.Y <= value.Y {
		t.Errorf("category label at y=%.2f is not below the value label at y=%.2f",
			category.Y, value.Y)
	}
}

func TestHorizontalBarLabelsStayInsideTheBox(t *testing.T) {
	canvas := &recorder{}

	c := &chart.Bar{
		Categories: []string{"South-East Asia", "Oceania"},
		Series:     []chart.Series{{Values: []float64{1940, 1120}}},
		Horizontal: true,
	}
	c.Draw(canvas, box)

	// The longest bar's label is drawn past its end, so the plot has to give it
	// room or it runs off the edge.
	pos, ok := canvas.textAt("1940")
	if !ok {
		t.Fatalf("expected a value label; drew %v", canvas.strings)
	}

	style := chart.DefaultStyle().ValueLabel
	if end := pos.X + style.MeasureText("1940"); end > box.Width {
		t.Errorf("label ends at x=%.2f, past the %v-wide box", end, box.Width)
	}
}

func TestBarGroupsMultipleSeries(t *testing.T) {
	canvas := &recorder{}

	(&chart.Bar{
		Categories: []string{"Q1", "Q2"},
		Series: []chart.Series{
			{Name: "Plan", Values: []float64{10, 20}},
			{Name: "Actual", Values: []float64{12, 18}},
		},
	}).Draw(canvas, box)

	for _, want := range []string{"Plan", "Actual", "10", "20", "12", "18"} {
		if !canvas.hasText(want) {
			t.Errorf("missing %q; drew %v", want, canvas.strings)
		}
	}
}

func TestPieLegendCarriesShares(t *testing.T) {
	canvas := &recorder{}

	(&chart.Pie{
		Slices: []chart.Slice{
			{Name: "Direct", Value: 50},
			{Name: "Partner", Value: 30},
			{Name: "Organic", Value: 20},
		},
	}).Draw(canvas, box)

	// A wedge's angle is far harder to read as a percentage than a number is.
	for _, want := range []string{"Direct  50%", "Partner  30%", "Organic  20%"} {
		if !canvas.hasText(want) {
			t.Errorf("missing legend row %q; drew %v", want, canvas.strings)
		}
	}
}

func TestPieHideShares(t *testing.T) {
	canvas := &recorder{}

	(&chart.Pie{
		Slices:     []chart.Slice{{Name: "Direct", Value: 50}, {Name: "Other", Value: 50}},
		HideShares: true,
	}).Draw(canvas, box)

	if !canvas.hasText("Direct") {
		t.Error("slice names should still appear")
	}
	for _, s := range canvas.strings {
		if strings.Contains(s, "%") {
			t.Errorf("shares should be suppressed, found %q", s)
		}
	}
}

func TestDonutDrawsCentreText(t *testing.T) {
	canvas := &recorder{}

	(&chart.Pie{
		Slices:      []chart.Slice{{Name: "a", Value: 1}, {Name: "b", Value: 1}},
		InnerRadius: 30,
		CentreLabel: "4.82M",
		CentreNote:  "total",
	}).Draw(canvas, box)

	for _, want := range []string{"4.82M", "total"} {
		if !canvas.hasText(want) {
			t.Errorf("missing centre text %q; drew %v", want, canvas.strings)
		}
	}
}

func TestPieWithoutHoleSkipsCentreText(t *testing.T) {
	canvas := &recorder{}

	(&chart.Pie{
		Slices:      []chart.Slice{{Name: "a", Value: 1}},
		CentreLabel: "hidden",
	}).Draw(canvas, box)

	// With no hole there is nowhere for centre text to go; it would land on the
	// slices.
	if canvas.hasText("hidden") {
		t.Error("centre text should be skipped when there is no hole")
	}
}

func TestPieForcesAColumnLegend(t *testing.T) {
	canvas := &recorder{}

	// A pie is round, so a strip legend above or below wastes the width the circle
	// cannot use. Only a column reads well beside one.
	(&chart.Pie{
		Slices: []chart.Slice{{Name: "alpha", Value: 1}, {Name: "beta", Value: 1}},
		Style:  chart.Style{Legend: chart.LegendTop},
	}).Draw(canvas, box)

	alpha, okA := canvas.textAt("alpha  50%")
	beta, okB := canvas.textAt("beta  50%")
	if !okA || !okB {
		t.Fatalf("expected both legend rows; drew %v", canvas.strings)
	}
	if alpha.X != beta.X {
		t.Errorf("legend rows are not stacked: x=%.2f and %.2f", alpha.X, beta.X)
	}
	if beta.Y <= alpha.Y {
		t.Error("legend rows should stack downwards")
	}
}

func TestLegendNoneOmitsTheKey(t *testing.T) {
	canvas := &recorder{}

	(&chart.Line{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Name: "hidden", Values: []float64{1, 2}}},
		Style:      chart.Style{Legend: chart.LegendNone},
	}).Draw(canvas, box)

	if canvas.hasText("hidden") {
		t.Error("the legend should be omitted")
	}
}

func TestStyleOverridesApplyIndividually(t *testing.T) {
	canvas := &recorder{}

	// A zero Style resolves to the defaults, so setting one field must not blank
	// the rest.
	(&chart.Line{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Name: "s", Values: []float64{1000, 2000}}},
		Style: chart.Style{
			TickCount: 3,
			Format:    func(v float64) string { return "@" },
		},
	}).Draw(canvas, box)

	if !canvas.hasText("@") {
		t.Errorf("the custom formatter was not used; drew %v", canvas.strings)
	}
	if !canvas.hasText("s") {
		t.Error("the legend should still render with a partial style")
	}
}

func TestHideGridAndAxis(t *testing.T) {
	full := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Values: []float64{1, 2}}},
	}).Draw(full, box)

	bare := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Values: []float64{1, 2}}},
		Style:      chart.Style{HideGrid: true, HideAxis: true},
	}).Draw(bare, box)

	if bare.paths >= full.paths {
		t.Errorf("hiding the grid and axis drew %d paths, want fewer than %d",
			bare.paths, full.paths)
	}
}

func TestSeriesColorOverridesThePalette(t *testing.T) {
	canvas := &recorder{}
	custom := core.Hex("#FF00FF")

	(&chart.Bar{
		Categories: []string{"a"},
		Series:     []chart.Series{{Name: "s", Values: []float64{1}, Color: custom}},
	}).Draw(canvas, box)

	if !canvas.usedColour(custom) {
		t.Errorf("the series colour was not used; saw %v", canvas.colours)
	}
}

func TestWideValuesDoNotOverlapThePlot(t *testing.T) {
	// The left gutter is measured from the widest tick label. Fixed at a guess, it
	// would be too narrow the moment values reached seven figures and the label
	// would run into the plot.
	narrow := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Values: []float64{1, 2}}},
	}).Draw(narrow, box)

	wide := &recorder{}
	(&chart.Line{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Values: []float64{1e6, 2e6}}},
		Style:      chart.Style{Format: func(v float64) string { return "1234567890" }},
	}).Draw(wide, box)

	narrowest := narrow.leftmostTextX()
	widest := wide.leftmostTextX()

	// Both gutters start at the box edge, but the wide one has to push the plot
	// further right, so its labels are no further left than the narrow one's.
	if widest < narrowest-0.01 {
		t.Errorf("wide labels start at x=%.2f, left of the narrow ones at %.2f",
			widest, narrowest)
	}
}

// --- recording canvas -------------------------------------------------------

// recorder captures what a chart drew, so behaviour can be asserted without
// pinning exact coordinates, which would break on any cosmetic change.
type recorder struct {
	paths  int
	fills  int
	texts  int
	rects  int
	labels map[string]core.Position

	strings  []string
	colours  []core.Color
	negative bool
	err      error
}

// checkSize flags geometry a reader would reject. A chart squeezed into a box too
// small for its axis furniture must decline to draw rather than emit a negative
// width.
func (r *recorder) checkSize(size core.Size) {
	if size.Width < -core.Epsilon || size.Height < -core.Epsilon {
		r.negative = true
	}
}

func (r *recorder) note(c core.Color) {
	r.colours = append(r.colours, c)
}

func (r *recorder) hasText(s string) bool {
	_, ok := r.labels[s]
	return ok
}

func (r *recorder) textAt(s string) (core.Position, bool) {
	pos, ok := r.labels[s]
	return pos, ok
}

func (r *recorder) usedColour(want core.Color) bool {
	for _, c := range r.colours {
		if c == want {
			return true
		}
	}
	return false
}

func (r *recorder) leftmostTextX() float64 {
	leftmost := math.Inf(1)
	for _, pos := range r.labels {
		leftmost = math.Min(leftmost, pos.X)
	}
	return leftmost
}

func (r *recorder) Save()                             {}
func (r *recorder) Restore()                          {}
func (r *recorder) Translate(core.Position)           {}
func (r *recorder) Rotate(float64)                    {}
func (r *recorder) ClipRect(core.Position, core.Size) {}

func (r *recorder) DrawRect(_ core.Position, size core.Size, fill core.Color) {
	r.checkSize(size)
	r.rects++
	r.fills++
	r.note(fill)
}

func (r *recorder) DrawRoundedRect(_ core.Position, size core.Size, _ float64, fill core.Color) {
	r.checkSize(size)
	r.rects++
	r.fills++
	r.note(fill)
}

func (r *recorder) DrawLine(_, _ core.Position, stroke core.Color, _ float64) {
	r.paths++
	r.note(stroke)
}

func (r *recorder) DrawPath(path *core.Path, style core.PathStyle) {
	if path.Empty() {
		return
	}
	r.paths++
	if style.Fills() {
		r.fills++
		r.note(style.Fill)
	}
	if style.Strokes() {
		r.note(style.Stroke)
	}
}

func (r *recorder) DrawText(text string, pos core.Position, _ core.TextStyle) {
	if text == "" {
		return
	}
	if r.labels == nil {
		r.labels = map[string]core.Position{}
	}
	r.texts++
	r.labels[text] = pos
	r.strings = append(r.strings, text)
}

func (r *recorder) DrawImage(core.Image, core.Position, core.Size) {}

func (r *recorder) Link(core.Position, core.Size, core.LinkTarget) {}
func (r *recorder) Destination(string, core.Position)              {}
func (r *recorder) Bookmark(string, int, string)                   {}

// Fail keeps the first failure, matching how the real canvases behave: Draw has
// no error return, so problems are collected and reported once.
func (r *recorder) Fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *recorder) Err() error { return r.err }

var _ core.Canvas = (*recorder)(nil)

// Keep the font package referenced: the default styles resolve through it, and a
// missing font would surface here first.
var _ = fonts.Helvetica

// --- negative values --------------------------------------------------------

func TestLineAndBarHandleNegativeValues(t *testing.T) {
	crossing := []float64{80, -45, 30, -60, 95}
	categories := []string{"Jan", "Feb", "Mar", "Apr", "May"}

	for name, element := range map[string]core.Element{
		"line": &chart.Line{Categories: categories,
			Series: []chart.Series{{Values: crossing}}},
		"area": &chart.Line{Categories: categories,
			Series: []chart.Series{{Values: crossing}}, Area: true},
		"columns": &chart.Bar{Categories: categories,
			Series: []chart.Series{{Values: crossing}}},
		"horizontal": &chart.Bar{Categories: categories,
			Series: []chart.Series{{Values: crossing}}, Horizontal: true},
		"all negative": &chart.Bar{Categories: categories,
			Series: []chart.Series{{Values: []float64{-10, -25, -40, -15, -30}}}},
	} {
		canvas := &recorder{}
		element.Draw(canvas, box)

		if canvas.err != nil {
			t.Errorf("%s: unexpected failure: %v", name, canvas.err)
		}
		if canvas.paths == 0 && canvas.rects == 0 {
			t.Errorf("%s: drew nothing", name)
		}
		if canvas.negative {
			t.Errorf("%s: produced negative geometry", name)
		}
		// Every category and value label should still be placed.
		for _, c := range categories {
			if !canvas.hasText(c) {
				t.Errorf("%s: missing category %q", name, c)
			}
		}
	}
}

func TestNegativeDataPutsTheAxisAtZero(t *testing.T) {
	canvas := &recorder{}

	(&chart.Bar{
		Categories: []string{"a", "b"},
		Series:     []chart.Series{{Values: []float64{100, -100}}},
	}).Draw(canvas, box)

	// Zero has to be a labelled gridline, or the axis line would float between
	// two of them with nothing to anchor it.
	if !canvas.hasText("0") {
		t.Errorf("expected a zero tick label; drew %v", canvas.strings)
	}

	zero, _ := canvas.textAt("0")
	hundred, ok := canvas.textAt("100")
	if !ok {
		t.Fatalf("expected a 100 tick label; drew %v", canvas.strings)
	}
	// With a symmetric range, zero sits below the positive tick on the page.
	if zero.Y <= hundred.Y {
		t.Errorf("zero at y=%.2f is not below 100 at y=%.2f", zero.Y, hundred.Y)
	}
}

func TestPieReportsNegativeSlices(t *testing.T) {
	canvas := &recorder{}

	// Dropping the negative would rescale the rest to 100% and produce a chart
	// that looks right while a third of the data has gone missing.
	(&chart.Pie{
		Slices: []chart.Slice{
			{Name: "Gains", Value: 100},
			{Name: "Losses", Value: -30},
			{Name: "Other", Value: 50},
		},
	}).Draw(canvas, box)

	if canvas.err == nil {
		t.Fatal("expected a negative slice to be reported")
	}
	for _, want := range []string{"negative", "Losses", "-30"} {
		if !strings.Contains(canvas.err.Error(), want) {
			t.Errorf("error %q does not mention %q", canvas.err, want)
		}
	}
	// Nothing is drawn, so a half-built chart cannot reach the page.
	if canvas.paths > 0 || canvas.texts > 0 {
		t.Errorf("drew %d paths and %d labels alongside the failure",
			canvas.paths, canvas.texts)
	}
}

func TestPieNamesUnlabelledNegativeSlices(t *testing.T) {
	canvas := &recorder{}

	(&chart.Pie{Slices: []chart.Slice{{Value: 10}, {Value: -4}}}).Draw(canvas, box)

	if canvas.err == nil {
		t.Fatal("expected a failure")
	}
	// Without a name, the index is the only way to point at the offending slice.
	if !strings.Contains(canvas.err.Error(), "slice 1") {
		t.Errorf("error %q does not identify the slice by index", canvas.err)
	}
}

func TestPieAcceptsZeroSlices(t *testing.T) {
	canvas := &recorder{}

	// A zero slice is absent rather than invalid: it has no wedge, but it also
	// misrepresents nothing.
	(&chart.Pie{
		Slices: []chart.Slice{{Name: "a", Value: 60}, {Name: "b", Value: 0}, {Name: "c", Value: 40}},
	}).Draw(canvas, box)

	if canvas.err != nil {
		t.Errorf("unexpected failure: %v", canvas.err)
	}
	if !canvas.hasText("a  60%") || !canvas.hasText("c  40%") {
		t.Errorf("expected the non-zero slices in the legend; drew %v", canvas.strings)
	}
	for _, s := range canvas.strings {
		if strings.HasPrefix(s, "b ") {
			t.Errorf("a zero slice should not appear in the legend, found %q", s)
		}
	}
}
