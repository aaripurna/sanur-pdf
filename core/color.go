package core

import (
	"fmt"
	"strconv"
	"strings"
)

// ColorSpace says how a colour's components are interpreted.
//
// Which space a colour is in decides the operators the canvas emits. Mixing them
// within a document is fine: PDF selects a colour space per drawing operation, so
// an RGB chart and a CMYK logo can share a page.
type ColorSpace uint8

const (
	// SpaceRGB is additive colour, for anything read on a screen. It is the zero
	// value, so a Color nobody has set is RGB.
	SpaceRGB ColorSpace = iota

	// SpaceCMYK is the subtractive colour a printing press uses.
	//
	// It matters because RGB cannot express the distinctions print cares about.
	// Black text wants 100% K on its own; a photographic black wants all four
	// plates. Both are #000000 in RGB, so converting has to pick one — which means
	// the choice belongs where the colour is specified, not in a conversion
	// afterwards.
	SpaceCMYK
)

func (s ColorSpace) String() string {
	if s == SpaceCMYK {
		return "CMYK"
	}
	return "RGB"
}

// Color is a straight (non-premultiplied) colour with an opacity, in one of the
// supported spaces.
//
// image/color is deliberately not used: its Color interface returns
// alpha-premultiplied 16-bit values, and PDF wants non-premultiplied components
// plus an alpha in a graphics state dictionary. Keeping our own type avoids
// converting back and forth on every draw call, and leaves room for CMYK, which
// image/color models but PDF treats quite differently.
//
// The components are unexported deliberately. A struct carrying both RGB and CMYK
// fields could be left saying one thing in its space and another in its values,
// and a caller reading .R off a CMYK colour would get cyan. Constructors and
// accessors make that unrepresentable.
//
// Color remains comparable with ==, which a great deal of calling code relies on.
type Color struct {
	space ColorSpace

	// components holds 0..255 values: the first three for RGB, all four for CMYK.
	components [4]uint8

	alpha uint8
}

// Transparent is the zero color; elements treat it as "draw nothing".
var Transparent = Color{}

// RGB builds an opaque additive colour.
func RGB(r, g, b uint8) Color { return RGBA(r, g, b, 255) }

// RGBA builds an additive colour with explicit alpha.
func RGBA(r, g, b, a uint8) Color {
	return Color{space: SpaceRGB, components: [4]uint8{r, g, b, 0}, alpha: a}
}

// CMYK builds an opaque subtractive colour for print.
//
// Components run 0..255 for consistency with the additive constructors, so a full
// plate is 255. CMYKPercent is usually the more natural way to write one.
func CMYK(c, m, y, k uint8) Color { return CMYKA(c, m, y, k, 255) }

// CMYKA builds a subtractive colour with explicit alpha.
func CMYKA(c, m, y, k, a uint8) Color {
	return Color{space: SpaceCMYK, components: [4]uint8{c, m, y, k}, alpha: a}
}

// CMYKPercent builds a subtractive colour from plate percentages, which is how a
// print specification is written: 100% K, not 255.
//
// Values outside 0..100 are clamped rather than refused, since a percentage is
// arithmetic output as often as a literal and a clamped plate is closer to the
// intent than an error.
func CMYKPercent(c, m, y, k float64) Color {
	return CMYKA(
		percentToByte(c), percentToByte(m), percentToByte(y), percentToByte(k), 255)
}

// PercentToByte converts a 0..100 percentage to the 0..255 byte the colour
// components and alpha are stored as, clamping rather than refusing out-of-range
// input for the reason given on CMYKPercent.
func PercentToByte(v float64) uint8 { return percentToByte(v) }

func percentToByte(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 100:
		return 255
	}
	return uint8(v*255/100 + 0.5)
}

func byteToPercent(v uint8) float64 { return float64(v) * 100 / 255 }

// Hex parses "#rgb", "#rrggbb" or "#rrggbbaa" (the leading # is optional) and
// panics on malformed input.
//
// Panicking is the right call here: colours are written as literals in layout
// code, so a bad string is a compile-time-shaped mistake that will surface on
// the very first run rather than in production data.
func Hex(s string) Color {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// ParseHex is the error-returning form of Hex, for colours from config or
// user input.
//
// Hex notation is inherently additive, so this always yields an RGB colour. Use
// ParseColor where either notation should be accepted.
func ParseHex(s string) (Color, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(s), "#")

	switch len(raw) {
	case 3, 4, 6, 8:
	default:
		return Color{}, fmt.Errorf("sanur: invalid hex color %q: want 3, 4, 6 or 8 digits", s)
	}

	digits := make([]uint8, len(raw))
	for i := 0; i < len(raw); i++ {
		v, err := strconv.ParseUint(raw[i:i+1], 16, 8)
		if err != nil {
			return Color{}, fmt.Errorf("sanur: invalid hex color %q", s)
		}
		digits[i] = uint8(v)
	}

	// Shorthand forms repeat each digit, so #abc means #aabbcc.
	if len(raw) <= 4 {
		expanded := make([]uint8, 0, len(digits)*2)
		for _, d := range digits {
			expanded = append(expanded, d, d)
		}
		digits = expanded
	}

	alpha := uint8(255)
	if len(digits) == 8 {
		alpha = digits[6]<<4 | digits[7]
	}

	return RGBA(
		digits[0]<<4|digits[1],
		digits[2]<<4|digits[3],
		digits[4]<<4|digits[5],
		alpha,
	), nil
}

// ParseColor parses either notation: "#4F46E5" for screen, or
// "cmyk(0, 0, 0, 100)" for print, where the values are plate percentages.
//
// A fifth value in the cmyk form is opacity, also a percentage, so
// "cmyk(0, 0, 0, 100, 50)" is a half-opaque black.
//
// This is what configuration should use. Accepting hex alone would quietly confine
// a theme to screen colour, and the whole point of naming colours in a file is that
// a print build and a screen build can differ by one line.
func ParseColor(s string) (Color, error) {
	trimmed := strings.TrimSpace(s)

	if strings.HasPrefix(strings.ToLower(trimmed), "cmyk(") {
		return parseCMYK(trimmed)
	}
	return ParseHex(trimmed)
}

func parseCMYK(s string) (Color, error) {
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.HasSuffix(s, ")") {
		return Color{}, fmt.Errorf(
			"sanur: invalid cmyk color %q: expected cmyk(c, m, y, k)", s)
	}

	fields := strings.Split(s[open+1:len(s)-1], ",")
	if len(fields) != 4 && len(fields) != 5 {
		return Color{}, fmt.Errorf(
			"sanur: invalid cmyk color %q: want 4 plates, optionally followed by an opacity", s)
	}

	values := make([]float64, len(fields))
	for i, field := range fields {
		// A trailing percent sign is accepted because the values are percentages
		// and somebody will write one.
		text := strings.TrimSuffix(strings.TrimSpace(field), "%")

		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Color{}, fmt.Errorf(
				"sanur: invalid cmyk color %q: %q is not a number", s, strings.TrimSpace(field))
		}
		values[i] = v
	}

	alpha := uint8(255)
	if len(values) == 5 {
		alpha = percentToByte(values[4])
	}

	return CMYKA(
		percentToByte(values[0]),
		percentToByte(values[1]),
		percentToByte(values[2]),
		percentToByte(values[3]),
		alpha,
	), nil
}

// Space reports which colour space the components are in.
func (c Color) Space() ColorSpace { return c.space }

// Visible reports whether the color would put any ink on the page.
func (c Color) Visible() bool { return c.alpha > 0 }

// Opaque reports whether the color needs no alpha graphics state.
func (c Color) Opaque() bool { return c.alpha == 255 }

// Opacity returns the raw alpha byte, for pooling graphics states by value.
func (c Color) Opacity() uint8 { return c.alpha }

// Alpha returns the opacity in the 0..1 range.
func (c Color) Alpha() float64 { return float64(c.alpha) / 255 }

// WithAlpha returns the same colour at a different opacity, keeping its space.
func (c Color) WithAlpha(a uint8) Color {
	c.alpha = a
	return c
}

// RGBComponents returns the colour as additive components scaled to the 0..1
// range PDF operators expect.
//
// A CMYK colour is converted with the naive formula. That is adequate for a
// preview but it is not colour management: a faithful conversion needs the source
// and destination ICC profiles and depends on the press. Nothing in the output
// path converts a CMYK colour — the canvas emits CMYK operators directly, so the
// specified plates reach the printer untouched.
func (c Color) RGBComponents() (r, g, b float64) {
	if c.space == SpaceCMYK {
		cy, m, y, k := c.rawComponents()
		return (1 - cy) * (1 - k), (1 - m) * (1 - k), (1 - y) * (1 - k)
	}

	r, g, b, _ = c.rawComponents()
	return r, g, b
}

// CMYKComponents returns the colour as subtractive components scaled to 0..1.
//
// An RGB colour is converted naively, with the same caveat as RGBComponents. Pure
// black arrives as 100% K rather than as four plates, which is the safer default
// for text.
func (c Color) CMYKComponents() (cy, m, y, k float64) {
	if c.space == SpaceCMYK {
		return c.rawComponents()
	}

	r, g, b, _ := c.rawComponents()

	black := 1 - max3(r, g, b)
	if black >= 1 {
		// Pure black has no chromatic component to divide out, and the expression
		// below would divide by zero.
		return 0, 0, 0, 1
	}

	return (1 - r - black) / (1 - black),
		(1 - g - black) / (1 - black),
		(1 - b - black) / (1 - black),
		black
}

func (c Color) rawComponents() (float64, float64, float64, float64) {
	return float64(c.components[0]) / 255,
		float64(c.components[1]) / 255,
		float64(c.components[2]) / 255,
		float64(c.components[3]) / 255
}

func max3(a, b, c float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

// String renders the colour in the notation ParseColor accepts.
func (c Color) String() string {
	if c.space == SpaceCMYK {
		plates := fmt.Sprintf("cmyk(%s, %s, %s, %s",
			trimNumber(byteToPercent(c.components[0])),
			trimNumber(byteToPercent(c.components[1])),
			trimNumber(byteToPercent(c.components[2])),
			trimNumber(byteToPercent(c.components[3])))

		if c.Opaque() {
			return plates + ")"
		}
		return plates + ", " + trimNumber(byteToPercent(c.alpha)) + ")"
	}

	if c.Opaque() {
		return fmt.Sprintf("#%02X%02X%02X",
			c.components[0], c.components[1], c.components[2])
	}
	return fmt.Sprintf("#%02X%02X%02X%02X",
		c.components[0], c.components[1], c.components[2], c.alpha)
}

// trimNumber formats a percentage with at most one decimal place.
//
// One decimal is enough to carry an arbitrary byte through a percentage and back,
// which keeps String and ParseColor exact inverses, while whole numbers stay whole
// so an authored "40" comes back as "40".
func trimNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

// MarshalJSON writes the colour in the notation ParseColor accepts.
//
// A string rather than an object of channels, because that is how a colour is
// written wherever a person types one — a stylesheet, a design tool, a print
// specification — and configuration is meant to be edited by hand.
func (c Color) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

// UnmarshalJSON reads either notation.
//
// A JSON null leaves the colour untouched rather than zeroing it, so an explicit
// null means "inherit" rather than "invisible" — matching the resolve-against-
// defaults convention the rest of the library uses.
func (c *Color) UnmarshalJSON(data []byte) error {
	text := string(data)
	if text == "null" {
		return nil
	}

	if len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return fmt.Errorf("sanur: a colour must be a string, got %s", text)
	}

	parsed, err := ParseColor(text[1 : len(text)-1])
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}
