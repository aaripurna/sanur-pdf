// Package theme loads document styling from JSON.
//
// # What belongs here, and what does not
//
// A theme carries the parts of a document's appearance that are genuinely static:
// page geometry, named colours, named text styles, chart styling. Structure stays
// in Go.
//
// That split is deliberate. JSON has no loops or conditionals, so describing
// document *structure* in it means inventing a template language inside string
// values — a DSL with no type checking, no editor support, and errors that point at
// a config file rather than at code. The first time a report needs one table row
// per invoice line, a Go `for` loop wins outright. What JSON is genuinely good at
// is the flat, static configuration a designer wants to change without a rebuild.
//
// # Resolution happens once, at load
//
// Text styles refer to colours and fonts by name, and every reference is resolved
// and checked while decoding. A misspelled colour fails at startup with the
// available names listed, rather than rendering as invisible text on page forty.
//
// # A worked example
//
//	{
//	  "page":   {"size": "A4 landscape", "margin": [24, 40]},
//	  "colors": {"ink": "#1A1D29", "accent": "#4F46E5"},
//	  "fonts":  {"body": "Helvetica", "bold": "Helvetica-Bold"},
//	  "text": {
//	    "body":    {"font": "body", "size": 9.5, "color": "ink"},
//	    "heading": {"font": "bold", "size": 19,  "color": "accent"}
//	  }
//	}
//
// Loaded and applied:
//
//	th, err := theme.Load("brand.json")
//
//	doc.EveryPage(func(p *sanur.Page) {
//		p.Size(th.PageSize()).MarginEach(th.Margins())
//		p.DefaultTextStyle(sanur.StyleFrom(th.Style("body")))
//	})
package theme

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/aaripurna/sanur-pdf/chart"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

// Theme is a resolved set of document styling.
//
// The exported fields hold what was written in the file; the resolved values
// behind the accessors are computed once at load. Reach for the accessors rather
// than the fields — Style returns a usable core.TextStyle, whereas Text holds
// unresolved names.
type Theme struct {
	Page   PageConfig            `json:"page"`
	Colors map[string]core.Color `json:"colors"`
	Fonts  map[string]string     `json:"fonts"`
	Text   map[string]TextConfig `json:"text"`
	Chart  ChartConfig           `json:"chart"`

	// resolved styles, keyed as Text is.
	styles map[string]core.TextStyle

	size       core.Size
	background core.Color
}

// PageConfig is a theme's page geometry.
type PageConfig struct {
	// Size names a standard size, optionally with an orientation: "A4",
	// "A4 landscape", "Letter". Width and Height override it when given.
	Size string `json:"size"`

	Width  float64 `json:"width"`
	Height float64 `json:"height"`

	Margin Margin `json:"margin"`

	// Background is a colour name from Colors, or a hex literal. Empty leaves the
	// page white.
	Background string `json:"background"`
}

// TextConfig is one named text style, before resolution.
//
// Font and Color hold names rather than values, which is the whole point: a name
// is something a JSON file can carry and a core.Font is not.
type TextConfig struct {
	// Font is an alias from Fonts, or a name registered with the fonts package.
	Font string `json:"font"`

	Size float64 `json:"size"`

	// Color is a name from Colors, or a hex literal.
	Color string `json:"color"`

	LineHeight    float64 `json:"lineHeight"`
	LetterSpacing float64 `json:"letterSpacing"`
	Underline     bool    `json:"underline"`
	Strikeout     bool    `json:"strikeout"`
}

// ChartConfig is a theme's chart styling.
//
// It deliberately omits chart.Style's Format field, which is a function and
// therefore cannot come from JSON. Charts fall back to their default number
// formatting; a caller wanting something else sets Format on the style returned by
// ChartStyle.
type ChartConfig struct {
	// Palette entries are colour names or hex literals.
	Palette []string `json:"palette"`

	// Grid and Axis are colour names or hex literals.
	Grid      string    `json:"grid"`
	GridWidth float64   `json:"gridWidth"`
	GridDash  []float64 `json:"gridDash"`
	HideGrid  bool      `json:"hideGrid"`

	Axis      string  `json:"axis"`
	AxisWidth float64 `json:"axisWidth"`
	HideAxis  bool    `json:"hideAxis"`

	// Legend is "top", "bottom", "right" or "none".
	Legend string `json:"legend"`

	TickCount int `json:"tickCount"`

	// Label, Value and Legend styles name entries in Text.
	LabelStyle  string `json:"labelStyle"`
	ValueStyle  string `json:"valueStyle"`
	LegendStyle string `json:"legendStyle"`

	HideValueLabels bool `json:"hideValueLabels"`

	// resolved
	palette                           []core.Color
	grid, axis                        core.Color
	legend                            chart.LegendPosition
	label, value, legendText          core.TextStyle
	hasLabel, hasValue, hasLegendText bool
}

// config holds decoding options.
type config struct {
	fonts *fonts.Registry
}

// Option adjusts how a theme is decoded.
type Option func(*config)

// WithFonts resolves font names against a specific registry.
//
// The shared registry is used by default, which is what a program with one set of
// fonts wants. An explicit registry matters for tests, and for anything rendering
// with per-tenant fonts where global state would leak between requests.
func WithFonts(r *fonts.Registry) Option {
	return func(c *config) { c.fonts = r }
}

// Load reads a theme from a file.
func Load(path string, opts ...Option) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sanur/theme: reading %s: %w", path, err)
	}

	th, err := Parse(data, opts...)
	if err != nil {
		// Naming the file matters when a program loads several themes.
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return th, nil
}

// Decode reads a theme from a stream.
func Decode(r io.Reader, opts ...Option) (*Theme, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("sanur/theme: reading theme: %w", err)
	}
	return Parse(data, opts...)
}

// Parse reads a theme from JSON, resolving and checking every reference.
func Parse(data []byte, opts ...Option) (*Theme, error) {
	cfg := config{fonts: fonts.Default()}
	for _, opt := range opts {
		opt(&cfg)
	}

	var th Theme
	if err := json.Unmarshal(data, &th); err != nil {
		return nil, fmt.Errorf("sanur/theme: %w", err)
	}

	if err := th.resolve(cfg); err != nil {
		return nil, err
	}
	return &th, nil
}

// resolve turns names into values and reports every problem it finds.
//
// Every problem, not just the first: a theme with three misspelled colours should
// take one edit to fix, not three rounds of load-and-retry.
func (t *Theme) resolve(cfg config) error {
	var problems []string
	note := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if err := t.resolvePage(); err != nil {
		note("%s", err)
	}

	t.styles = make(map[string]core.TextStyle, len(t.Text))
	for _, name := range sortedKeys(t.Text) {
		style, err := t.resolveText(cfg, t.Text[name])
		if err != nil {
			note("text style %q: %s", name, err)
			continue
		}
		t.styles[name] = style
	}

	if err := t.Chart.resolve(t, cfg); err != nil {
		note("chart: %s", err)
	}

	if len(problems) > 0 {
		return fmt.Errorf("sanur/theme: %s", strings.Join(problems, "; "))
	}
	return nil
}

func (t *Theme) resolvePage() error {
	switch {
	case t.Page.Width > 0 && t.Page.Height > 0:
		t.size = core.Size{Width: t.Page.Width, Height: t.Page.Height}

	case t.Page.Size != "":
		size, ok := core.ParseSize(t.Page.Size)
		if !ok {
			return fmt.Errorf(
				"page size %q is not recognised (available: %s, each optionally "+
					"followed by portrait or landscape)",
				t.Page.Size, strings.Join(core.SizeNames(), ", "))
		}
		t.size = size

	default:
		// A theme that says nothing about the page still has to produce something
		// usable, and A4 is the most widely applicable default.
		t.size = core.A4
	}

	t.background = core.RGB(255, 255, 255)
	if t.Page.Background != "" {
		colour, err := t.resolveColor(t.Page.Background)
		if err != nil {
			return fmt.Errorf("page background: %s", err)
		}
		t.background = colour
	}
	return nil
}

// resolveColor accepts a name from Colors or a colour literal in either
// notation: "#4F46E5" for screen, "cmyk(0, 0, 0, 100)" for print.
//
// Allowing both a name and a literal means a theme can pull shared values out
// into Colors without having to, and a one-off shade needs no invented name.
func (t *Theme) resolveColor(ref string) (core.Color, error) {
	if colour, ok := t.Colors[ref]; ok {
		return colour, nil
	}

	if isColorLiteral(ref) {
		return core.ParseColor(ref)
	}

	available := sortedKeys(t.Colors)
	if len(available) == 0 {
		return core.Color{}, fmt.Errorf(
			"colour %q is not defined, and no colours are declared "+
				"(use a literal such as \"#4F46E5\" or \"cmyk(0, 0, 0, 100)\")", ref)
	}
	return core.Color{}, fmt.Errorf(
		"colour %q is not defined (available: %s, or use a literal such as "+
			"\"#4F46E5\" or \"cmyk(0, 0, 0, 100)\")",
		ref, strings.Join(available, ", "))
}

// isColorLiteral reports whether a reference looks like a colour rather than a
// name, so that a plain typo is reported against the declared colours instead of
// being handed to the parser and coming back as a syntax complaint.
func isColorLiteral(ref string) bool {
	trimmed := strings.TrimSpace(ref)
	return strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(strings.ToLower(trimmed), "cmyk(")
}

// resolveFont accepts an alias from Fonts or a registered font name.
func (t *Theme) resolveFont(cfg config, ref string) (core.Font, error) {
	name := ref
	if alias, ok := t.Fonts[ref]; ok {
		name = alias
	}
	return cfg.fonts.Resolve(name)
}

func (t *Theme) resolveText(cfg config, in TextConfig) (core.TextStyle, error) {
	style := core.TextStyle{
		Size:             in.Size,
		LineHeightFactor: in.LineHeight,
		LetterSpacing:    in.LetterSpacing,
		Underline:        in.Underline,
		Strikeout:        in.Strikeout,
	}

	if in.Font == "" {
		return style, fmt.Errorf("no font named")
	}
	font, err := t.resolveFont(cfg, in.Font)
	if err != nil {
		return style, err
	}
	style.Font = font

	// A size of zero would render nothing at all, so it falls back rather than
	// producing an invisible style.
	if style.Size <= 0 {
		style.Size = defaultTextSize
	}

	style.Color = core.RGB(0, 0, 0)
	if in.Color != "" {
		colour, err := t.resolveColor(in.Color)
		if err != nil {
			return style, err
		}
		style.Color = colour
	}

	return style, nil
}

// defaultTextSize is used by a style that names no size.
const defaultTextSize = 11

func (c *ChartConfig) resolve(t *Theme, cfg config) error {
	var problems []string

	for _, ref := range c.Palette {
		colour, err := t.resolveColor(ref)
		if err != nil {
			problems = append(problems, fmt.Sprintf("palette: %s", err))
			continue
		}
		c.palette = append(c.palette, colour)
	}

	for _, entry := range []struct {
		ref  string
		into *core.Color
		what string
	}{
		{c.Grid, &c.grid, "grid"},
		{c.Axis, &c.axis, "axis"},
	} {
		if entry.ref == "" {
			continue
		}
		colour, err := t.resolveColor(entry.ref)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s", entry.what, err))
			continue
		}
		*entry.into = colour
	}

	legend, err := parseLegend(c.Legend)
	if err != nil {
		problems = append(problems, err.Error())
	}
	c.legend = legend

	for _, entry := range []struct {
		ref   string
		into  *core.TextStyle
		found *bool
		what  string
	}{
		{c.LabelStyle, &c.label, &c.hasLabel, "labelStyle"},
		{c.ValueStyle, &c.value, &c.hasValue, "valueStyle"},
		{c.LegendStyle, &c.legendText, &c.hasLegendText, "legendStyle"},
	} {
		if entry.ref == "" {
			continue
		}
		style, ok := t.styles[entry.ref]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s: no text style named %q (available: %s)",
				entry.what, entry.ref, strings.Join(sortedKeys(t.Text), ", ")))
			continue
		}
		*entry.into = style
		*entry.found = true
	}

	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// parseLegend maps a legend position name onto the chart constant.
func parseLegend(name string) (chart.LegendPosition, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		// Unset means the chart package's own default, not a position.
		return chart.LegendTop, nil
	case "top":
		return chart.LegendTop, nil
	case "bottom":
		return chart.LegendBottom, nil
	case "right":
		return chart.LegendRight, nil
	case "none":
		return chart.LegendNone, nil
	default:
		return chart.LegendTop, fmt.Errorf(
			"legend %q is not recognised (want top, bottom, right or none)", name)
	}
}

// --- accessors --------------------------------------------------------------

// PageSize returns the resolved page size.
func (t *Theme) PageSize() core.Size { return t.size }

// Margins returns the page insets in the order MarginEach takes them.
func (t *Theme) Margins() (top, right, bottom, left float64) {
	m := t.Page.Margin
	return m.Top, m.Right, m.Bottom, m.Left
}

// Background returns the resolved page background.
func (t *Theme) Background() core.Color { return t.background }

// Style returns a named text style.
//
// It panics on an unknown name, listing the ones that exist. A style name is
// authoring-time input like a colour literal, and the alternative — returning a
// zero style — renders as invisible text somewhere in the middle of a document,
// which is far harder to trace back than a stack trace on the first run. Use
// LookupStyle where a missing style is a real possibility.
func (t *Theme) Style(name string) core.TextStyle {
	style, ok := t.styles[name]
	if !ok {
		panic(fmt.Sprintf(
			"sanur/theme: no text style named %q (available: %s)",
			name, strings.Join(t.StyleNames(), ", ")))
	}
	return style
}

// LookupStyle returns a named text style, reporting whether it exists.
func (t *Theme) LookupStyle(name string) (core.TextStyle, bool) {
	style, ok := t.styles[name]
	return style, ok
}

// Color returns a named colour, panicking on an unknown name for the same reason
// Style does.
func (t *Theme) Color(name string) core.Color {
	colour, ok := t.Colors[name]
	if !ok {
		panic(fmt.Sprintf(
			"sanur/theme: no colour named %q (available: %s)",
			name, strings.Join(t.ColorNames(), ", ")))
	}
	return colour
}

// LookupColor returns a named colour, reporting whether it exists.
func (t *Theme) LookupColor(name string) (core.Color, bool) {
	colour, ok := t.Colors[name]
	return colour, ok
}

// StyleNames lists the declared text styles in a stable order.
func (t *Theme) StyleNames() []string { return sortedKeys(t.styles) }

// ColorNames lists the declared colours in a stable order.
func (t *Theme) ColorNames() []string { return sortedKeys(t.Colors) }

// ChartStyle returns the chart styling, leaving anything the theme did not mention
// for the chart package to default.
//
// Format is never set: it is a function, so no JSON file can supply one. Assign it
// on the returned value if the default number formatting is not what you want.
func (t *Theme) ChartStyle() chart.Style {
	c := t.Chart

	style := chart.Style{
		Palette:         c.palette,
		Grid:            c.grid,
		GridWidth:       c.GridWidth,
		GridDash:        c.GridDash,
		HideGrid:        c.HideGrid,
		Axis:            c.axis,
		AxisWidth:       c.AxisWidth,
		HideAxis:        c.HideAxis,
		Legend:          c.legend,
		TickCount:       c.TickCount,
		HideValueLabels: c.HideValueLabels,
	}

	if c.hasLabel {
		style.Label = c.label
	}
	if c.hasValue {
		style.ValueLabel = c.value
	}
	if c.hasLegendText {
		style.LegendLabel = c.legendText
	}

	return style
}

// sortedKeys returns a map's keys in a stable order, so error messages and name
// listings do not change between runs.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
