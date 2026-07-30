package chart

import (
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

// Series is one run of values sharing a name and a colour.
//
// Values are positional: index three is the fourth category. A series shorter
// than the category list simply stops, which is what makes a partially reported
// period render as a line that ends early instead of failing.
type Series struct {
	Name   string
	Values []float64

	// Color overrides the palette entry for this series. Leave it unset to take
	// the next palette colour by position.
	Color core.Color
}

// At returns the value at index i, and whether the series has one.
func (s Series) At(i int) (float64, bool) {
	if i < 0 || i >= len(s.Values) {
		return 0, false
	}
	return s.Values[i], true
}

// LegendPosition places the key relative to the plot.
type LegendPosition int

const (
	// LegendTop puts the key in a strip above the plot.
	LegendTop LegendPosition = iota

	// LegendBottom puts it below the category labels.
	LegendBottom

	// LegendRight puts it in a column to the right, which suits pies and any
	// chart with long series names.
	LegendRight

	// LegendNone omits it.
	LegendNone
)

// Style is the theme shared by every chart type.
//
// A zero Style is valid: each chart resolves it against the defaults before
// drawing, so a chart with no configuration still looks finished. That is why
// suppression is expressed with explicit Hide flags rather than by zeroing a
// colour — an unset colour means "use the default", and there would otherwise be
// no way to say "no gridlines".
type Style struct {
	// Palette is cycled through by series index. Charts wrap around rather than
	// running out, so a long series list still renders.
	Palette []core.Color

	// Label styles tick values and category names.
	Label core.TextStyle

	// ValueLabel styles the figures drawn on or above data points.
	ValueLabel core.TextStyle

	// LegendLabel styles the key.
	LegendLabel core.TextStyle

	// Grid draws the lines behind the plot.
	Grid      core.Color
	GridWidth float64
	GridDash  []float64
	HideGrid  bool

	// Axis draws the baseline.
	Axis      core.Color
	AxisWidth float64
	HideAxis  bool

	// Legend placement and the gap between its entries.
	Legend        LegendPosition
	LegendSpacing float64

	// HideValueLabels suppresses the figures drawn beside data points, which get
	// crowded once a chart has many categories.
	HideValueLabels bool

	// TickCount is the number of value-axis divisions aimed for. The actual count
	// varies, because ticks are rounded to readable intervals.
	TickCount int

	// Format renders a value for display. Leave nil for FormatValue.
	Format func(float64) string
}

// DefaultPalette is a categorical sequence chosen to stay distinguishable in
// print and in the common forms of colour blindness.
var DefaultPalette = []core.Color{
	core.Hex("#4F46E5"),
	core.Hex("#0891B2"),
	core.Hex("#059669"),
	core.Hex("#D97706"),
	core.Hex("#DC2626"),
	core.Hex("#7C3AED"),
	core.Hex("#0369A1"),
	core.Hex("#65A30D"),
}

// DefaultStyle returns the theme charts use when none is given.
func DefaultStyle() Style {
	label := core.TextStyle{
		Font:  fonts.MustStandard(fonts.Helvetica),
		Size:  7.5,
		Color: core.Hex("#6B7280"),
	}

	value := label
	value.Size = 8
	value.Color = core.Hex("#1A1D29")
	value.Weight = core.FontBold
	value.Font = fonts.MustStandard(fonts.HelveticaBold)

	legend := label
	legend.Size = 8
	legend.Color = core.Hex("#1A1D29")

	return Style{
		Palette:       DefaultPalette,
		Label:         label,
		ValueLabel:    value,
		LegendLabel:   legend,
		Grid:          core.Hex("#E5E7EB"),
		GridWidth:     0.75,
		GridDash:      []float64{3, 3},
		Axis:          core.Hex("#9CA3AF"),
		AxisWidth:     0.75,
		Legend:        LegendTop,
		LegendSpacing: 16,
		TickCount:     5,
	}
}

// resolve fills in whatever the caller left unset.
//
// Charts call this once at the top of Draw rather than reading Style fields
// directly, so that every field has a usable value from then on and no drawing
// code has to carry a fallback.
func (s Style) resolve() Style {
	d := DefaultStyle()

	if len(s.Palette) == 0 {
		s.Palette = d.Palette
	}
	s.Label = resolveText(s.Label, d.Label)
	s.ValueLabel = resolveText(s.ValueLabel, d.ValueLabel)
	s.LegendLabel = resolveText(s.LegendLabel, d.LegendLabel)

	if !s.Grid.Visible() {
		s.Grid = d.Grid
	}
	if s.GridWidth <= 0 {
		s.GridWidth = d.GridWidth
	}
	if s.GridDash == nil {
		s.GridDash = d.GridDash
	}
	if !s.Axis.Visible() {
		s.Axis = d.Axis
	}
	if s.AxisWidth <= 0 {
		s.AxisWidth = d.AxisWidth
	}
	if s.LegendSpacing <= 0 {
		s.LegendSpacing = d.LegendSpacing
	}
	if s.TickCount <= 0 {
		s.TickCount = d.TickCount
	}
	if s.Format == nil {
		s.Format = FormatValue
	}
	return s
}

// resolveText fills a text style's gaps from a fallback, field by field, so a
// caller can override only the size and keep the default font and colour.
func resolveText(s, fallback core.TextStyle) core.TextStyle {
	if s.Font == nil {
		s.Font = fallback.Font
	}
	if s.Size <= 0 {
		s.Size = fallback.Size
	}
	if !s.Color.Visible() {
		s.Color = fallback.Color
	}
	return s
}

// colorFor returns the colour for a series, honouring an explicit override.
func (s Style) colorFor(index int, series Series) core.Color {
	if series.Color.Visible() {
		return series.Color
	}
	return s.Palette[index%len(s.Palette)]
}

// gridStyle is the path style for a gridline.
func (s Style) gridStyle() core.PathStyle {
	return core.PathStyle{Stroke: s.Grid, Width: s.GridWidth, Dash: s.GridDash}
}

// axisStyle is the path style for the baseline. It is solid so that it reads as
// an axis rather than as one more gridline.
func (s Style) axisStyle() core.PathStyle {
	return core.PathStyle{Stroke: s.Axis, Width: s.AxisWidth}
}

// fade returns a colour at reduced opacity, for area fills beneath a line.
func fade(c core.Color, alpha uint8) core.Color {
	c.A = alpha
	return c
}
