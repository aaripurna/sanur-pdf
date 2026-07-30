package core

import (
	"fmt"
	"strconv"
	"strings"
)

// Color is a straight (non-premultiplied) 8-bit RGBA color.
//
// image/color is deliberately not used here: its Color interface returns
// alpha-premultiplied 16-bit values, and PDF wants separate non-premultiplied
// color operators plus an alpha in a graphics state dictionary. Keeping our own
// struct avoids converting back and forth on every draw call.
type Color struct {
	R, G, B, A uint8
}

// Transparent is the zero color; elements treat it as "draw nothing".
var Transparent = Color{}

// RGB builds an opaque color.
func RGB(r, g, b uint8) Color { return Color{R: r, G: g, B: b, A: 255} }

// RGBA builds a color with explicit alpha.
func RGBA(r, g, b, a uint8) Color { return Color{R: r, G: g, B: b, A: a} }

// Hex parses "#rgb", "#rrggbb" or "#rrggbbaa" (the leading # is optional) and
// panics on malformed input.
//
// Panicking is the right call here: colors are written as literals in layout
// code, so a bad string is a compile-time-shaped mistake that will surface on
// the very first run rather than in production data.
func Hex(s string) Color {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// ParseHex is the error-returning form of Hex, for colors from config or
// user input.
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

	c := Color{
		R: digits[0]<<4 | digits[1],
		G: digits[2]<<4 | digits[3],
		B: digits[4]<<4 | digits[5],
		A: 255,
	}
	if len(digits) == 8 {
		c.A = digits[6]<<4 | digits[7]
	}
	return c, nil
}

// Visible reports whether the color would put any ink on the page.
func (c Color) Visible() bool { return c.A > 0 }

// Opaque reports whether the color needs no alpha graphics state.
func (c Color) Opaque() bool { return c.A == 255 }

// Components returns the color scaled to the 0..1 range PDF operators expect.
func (c Color) Components() (r, g, b float64) {
	return float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255
}

// Alpha returns the opacity in the 0..1 range.
func (c Color) Alpha() float64 { return float64(c.A) / 255 }

func (c Color) String() string {
	if c.Opaque() {
		return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", c.R, c.G, c.B, c.A)
}

// MarshalJSON writes the colour as a hex string.
//
// Hex rather than an object of channels, because that is how a colour is written
// everywhere a person types one — a stylesheet, a design tool, a brand guide — and
// configuration is meant to be edited by hand.
func (c Color) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

// UnmarshalJSON reads a hex string in any of the forms ParseHex accepts.
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
		return fmt.Errorf("sanur: a colour must be a hex string, got %s", text)
	}

	parsed, err := ParseHex(text[1 : len(text)-1])
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}
