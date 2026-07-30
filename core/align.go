package core

// HorizontalAlign positions content along the x axis of its box.
type HorizontalAlign int

const (
	AlignLeft HorizontalAlign = iota
	AlignCenter
	AlignRight

	// AlignJustify stretches the gaps between words so every line but the last
	// reaches both margins. It applies to text only; other elements treat it as
	// AlignLeft.
	AlignJustify
)

// VerticalAlign positions content along the y axis of its box.
type VerticalAlign int

const (
	AlignTop VerticalAlign = iota
	AlignMiddle
	AlignBottom
)

// OffsetX returns the x offset that places content of width inner inside a box
// of width outer.
func (a HorizontalAlign) OffsetX(outer, inner float64) float64 {
	switch a {
	case AlignCenter:
		return (outer - inner) / 2
	case AlignRight:
		return outer - inner
	}
	return 0
}

// OffsetY returns the y offset that places content of height inner inside a box
// of height outer.
func (a VerticalAlign) OffsetY(outer, inner float64) float64 {
	switch a {
	case AlignMiddle:
		return (outer - inner) / 2
	case AlignBottom:
		return outer - inner
	}
	return 0
}
