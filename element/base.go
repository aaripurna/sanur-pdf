package element

import "image/color"

type MeasurementType int

const (
	MeasurementFixed MeasurementType = iota
	MeasurementPercent
)

type Measurement struct {
	Value float64
	Type  MeasurementType
}

type Rec struct {
	Height Measurement
	Width  Measurement
}

func Percent(val float64) Measurement {
	return Measurement{Value: val, Type: MeasurementPercent}
}

func Fixed(val float64) Measurement {
	return Measurement{Value: val, Type: MeasurementFixed}
}

type Element interface {
	Dimension() Rec
	Draw(parent *Element) error
}

type LineStyle struct {
	Color color.Model
	Width float64
}

type BoxProps[T any] struct {
	Top    T
	Right  T
	Bottom T
	Left   T
}

type RecStyle struct {
	Border  *BoxProps[*LineStyle]
	Padding *BoxProps[float64]
	Rounded *BoxProps[float64]
}
