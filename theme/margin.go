package theme

import (
	"encoding/json"
	"fmt"
)

// Margin is a set of page insets in points.
type Margin struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// UnmarshalJSON accepts several shorthand forms.
//
// Margins are the field a person editing a theme touches most, and insisting on
// the long form for the common case of "40 all round" would be tiresome:
//
//	"margin": 40                                  all four edges
//	"margin": [24, 40]                            vertical, horizontal
//	"margin": [10, 20, 30, 40]                    top, right, bottom, left
//	"margin": {"top": 10, "left": 20}             named, others zero
//
// The four-element order matches CSS, since that is the ordering anybody who has
// written a stylesheet already knows.
func (m *Margin) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	switch data[0] {
	case '{':
		return m.unmarshalObject(data)
	case '[':
		return m.unmarshalArray(data)
	default:
		return m.unmarshalScalar(data)
	}
}

func (m *Margin) unmarshalObject(data []byte) error {
	// An alias avoids recursing back into this method.
	type plain Margin

	var parsed plain
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("sanur/theme: reading margin: %w", err)
	}
	*m = Margin(parsed)
	return nil
}

func (m *Margin) unmarshalArray(data []byte) error {
	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("sanur/theme: reading margin: %w", err)
	}

	switch len(values) {
	case 1:
		*m = Margin{values[0], values[0], values[0], values[0]}
	case 2:
		*m = Margin{Top: values[0], Right: values[1], Bottom: values[0], Left: values[1]}
	case 4:
		*m = Margin{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}
	default:
		return fmt.Errorf(
			"sanur/theme: a margin array needs 1, 2 or 4 values, got %d", len(values))
	}
	return nil
}

func (m *Margin) unmarshalScalar(data []byte) error {
	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf(
			"sanur/theme: a margin must be a number, an array or an object, got %s", data)
	}
	*m = Margin{value, value, value, value}
	return nil
}

// MarshalJSON always writes the long form, so a round trip is unambiguous.
func (m Margin) MarshalJSON() ([]byte, error) {
	type plain Margin
	return json.Marshal(plain(m))
}
