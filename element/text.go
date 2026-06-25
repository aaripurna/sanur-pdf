package element

type FontWeight int

const (
	FontThin       FontWeight = 100
	FontExtraLight FontWeight = 200
	FontLight      FontWeight = 300
	FontNormal     FontWeight = 400
	FontMedium     FontWeight = 500
	FontSemiBold   FontWeight = 600
	FontBold       FontWeight = 700
	FontExtraBold  FontWeight = 800
	FontBlack      FontWeight = 900
	FontExtraBlack FontWeight = 950
)

type TextStyle struct {
	FontFamily string
	FontSize   float64
	Bold       bool
	FontWeight FontWeight
}

type Text struct {
	Content string
}

// Dimension implements [Element].
func (t *Text) Dimension() Rec {
	return Rec{
		Height: Percent(100),
	}
}

// Draw implements [Element].
func (t *Text) Draw(parent *Element) error {
	panic("unimplemented")
}

// Fox explicitly implement [Element].
func _text() Element {
	return &Text{}
}
