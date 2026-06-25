package element

type Div struct {
	Children []Element
	Height   Measurement
	Width    Measurement
	Style    *RecStyle
}

// Dimension implements [Element].
func (d *Div) Dimension() Rec {
	return Rec{
		Height: d.Height,
		Width:  d.Width,
	}
}

// Draw implements [Element].
func (d *Div) Draw(parent *Element) error {
	panic("unimplemented")
}

// Fox explicitly implement [Element].
func _newDiv(height, width Measurement) Element {
	return &Div{
		Height: height,
		Width:  width,
	}
}
