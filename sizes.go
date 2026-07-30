package sanur

import "github.com/aaripurna/sanur-pdf/core"

// Units and page sizes are defined in core, since a size is pure geometry and
// anything reading one from configuration needs to name it without the figures
// being written down twice. They are re-exported here so layout code needs only
// the one import.
const (
	Point      = core.Point
	Inch       = core.Inch
	Millimetre = core.Millimetre
	Centimetre = core.Centimetre
)

// Mm converts millimetres to points.
func Mm(v float64) float64 { return core.Mm(v) }

// Cm converts centimetres to points.
func Cm(v float64) float64 { return core.Cm(v) }

// In converts inches to points.
func In(v float64) float64 { return core.In(v) }

// Standard page sizes in points, portrait.
var (
	A0 = core.A0
	A1 = core.A1
	A2 = core.A2
	A3 = core.A3
	A4 = core.A4
	A5 = core.A5
	A6 = core.A6

	Letter    = core.Letter
	Legal     = core.Legal
	Tabloid   = core.Tabloid
	Executive = core.Executive
)

// Landscape swaps a size's dimensions.
func Landscape(s core.Size) core.Size { return core.Landscape(s) }

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
