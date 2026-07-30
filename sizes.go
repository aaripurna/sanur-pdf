package sanur

import "github.com/aaripurna/sanur-pdf/core"

// PDF measures everything in points: 72 to the inch, regardless of the units the
// page size was originally defined in. The conversion helpers below exist so
// layout code can be written in whatever unit the design was specified in.
const (
	Point      = 1.0
	Inch       = 72.0
	Millimetre = Inch / 25.4
	Centimetre = Millimetre * 10
)

// Mm converts millimetres to points.
func Mm(v float64) float64 { return v * Millimetre }

// Cm converts centimetres to points.
func Cm(v float64) float64 { return v * Centimetre }

// In converts inches to points.
func In(v float64) float64 { return v * Inch }

// Standard page sizes in points, portrait.
//
// The ISO A series is defined in millimetres, so its point sizes are not round
// numbers; they are given to two decimal places, which is finer than any printer
// can place a sheet.
var (
	A0 = core.Size{Width: 2383.94, Height: 3370.39}
	A1 = core.Size{Width: 1683.78, Height: 2383.94}
	A2 = core.Size{Width: 1190.55, Height: 1683.78}
	A3 = core.Size{Width: 841.89, Height: 1190.55}
	A4 = core.Size{Width: 595.28, Height: 841.89}
	A5 = core.Size{Width: 419.53, Height: 595.28}
	A6 = core.Size{Width: 297.64, Height: 419.53}

	Letter    = core.Size{Width: 612, Height: 792}
	Legal     = core.Size{Width: 612, Height: 1008}
	Tabloid   = core.Size{Width: 792, Height: 1224}
	Executive = core.Size{Width: 521.86, Height: 756}
)

// Landscape swaps a size's dimensions.
func Landscape(s core.Size) core.Size {
	return core.Size{Width: s.Height, Height: s.Width}
}

// Common colours, so a first document needs no colour arithmetic.
var (
	Black       = core.RGB(0, 0, 0)
	White       = core.RGB(255, 255, 255)
	Transparent = core.Transparent

	Red    = core.Hex("#E53935")
	Orange = core.Hex("#FB8C00")
	Yellow = core.Hex("#FDD835")
	Green  = core.Hex("#43A047")
	Teal   = core.Hex("#00897B")
	Blue   = core.Hex("#1E88E5")
	Indigo = core.Hex("#3949AB")
	Purple = core.Hex("#8E24AA")
	Pink   = core.Hex("#D81B60")
	Brown  = core.Hex("#6D4C41")

	Grey100 = core.Hex("#F5F5F5")
	Grey200 = core.Hex("#EEEEEE")
	Grey300 = core.Hex("#E0E0E0")
	Grey400 = core.Hex("#BDBDBD")
	Grey500 = core.Hex("#9E9E9E")
	Grey600 = core.Hex("#757575")
	Grey700 = core.Hex("#616161")
	Grey800 = core.Hex("#424242")
	Grey900 = core.Hex("#212121")
)

// RGB builds an opaque colour.
func RGB(r, g, b uint8) core.Color { return core.RGB(r, g, b) }

// RGBA builds a colour with alpha.
func RGBA(r, g, b, a uint8) core.Color { return core.RGBA(r, g, b, a) }

// Hex parses a hex colour such as "#1E88E5", panicking on malformed input.
func Hex(s string) core.Color { return core.Hex(s) }

// Alignment values, re-exported so callers need not import core.
const (
	AlignLeft    = core.AlignLeft
	AlignCenter  = core.AlignCenter
	AlignRight   = core.AlignRight
	AlignJustify = core.AlignJustify

	AlignTop    = core.AlignTop
	AlignMiddle = core.AlignMiddle
	AlignBottom = core.AlignBottom
)
