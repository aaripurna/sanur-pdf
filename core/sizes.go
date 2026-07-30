package core

import "strings"

// PDF measures everything in points: 72 to the inch, regardless of the units a
// page size was originally defined in. These conversions let layout be written in
// whatever unit the design was specified in.
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
// numbers; they are given to two decimal places, finer than any printer can place
// a sheet.
//
// These live here rather than in the fluent package because a page size is pure
// geometry, and anything reading a size from configuration needs to name one
// without the figures being written down in two places.
var (
	A0 = Size{Width: 2383.94, Height: 3370.39}
	A1 = Size{Width: 1683.78, Height: 2383.94}
	A2 = Size{Width: 1190.55, Height: 1683.78}
	A3 = Size{Width: 841.89, Height: 1190.55}
	A4 = Size{Width: 595.28, Height: 841.89}
	A5 = Size{Width: 419.53, Height: 595.28}
	A6 = Size{Width: 297.64, Height: 419.53}

	Letter    = Size{Width: 612, Height: 792}
	Legal     = Size{Width: 612, Height: 1008}
	Tabloid   = Size{Width: 792, Height: 1224}
	Executive = Size{Width: 521.86, Height: 756}
)

// Landscape swaps a size's dimensions.
func Landscape(s Size) Size {
	return Size{Width: s.Height, Height: s.Width}
}

// namedSizes indexes the standard sizes for lookup by name.
var namedSizes = map[string]Size{
	"a0": A0, "a1": A1, "a2": A2, "a3": A3, "a4": A4, "a5": A5, "a6": A6,
	"letter": Letter, "legal": Legal, "tabloid": Tabloid, "executive": Executive,
}

// ParseSize resolves a page size by name, optionally with an orientation.
//
// It accepts forms like "A4", "a4 landscape" and "Letter portrait", because a size
// read from configuration arrives as text and the orientation is part of what
// somebody means by a page size. Matching is case-insensitive so a config file need
// not know the library's capitalisation.
func ParseSize(name string) (Size, bool) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(name)))
	if len(fields) == 0 {
		return Size{}, false
	}

	size, ok := namedSizes[fields[0]]
	if !ok {
		return Size{}, false
	}

	for _, modifier := range fields[1:] {
		switch modifier {
		case "landscape":
			size = Landscape(size)
		case "portrait":
			// The named sizes are already portrait, so this only has to be accepted
			// rather than acted on.
		default:
			return Size{}, false
		}
	}
	return size, true
}

// SizeNames lists the recognised page size names in a stable order.
func SizeNames() []string {
	return []string{
		"A0", "A1", "A2", "A3", "A4", "A5", "A6",
		"Letter", "Legal", "Tabloid", "Executive",
	}
}
