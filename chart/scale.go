package chart

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Ticks chooses axis values that land on round numbers spanning low..high.
//
// Ticks taken from the data extremes would label an axis 3.1 to 4.82, which is
// accurate and unreadable. Rounding outwards to a "nice" interval instead gives
// an axis a reader can interpolate against, at the cost of a little empty space
// above the largest value.
//
// The result always contains at least two values, and always brackets the input
// range, so the first and last entries are safe to use as the axis bounds.
func Ticks(low, high float64, target int) []float64 {
	if target < 2 {
		target = 2
	}

	// A degenerate range would divide by zero when scaled. Widening it by one
	// keeps a flat series renderable — it plots as a line along one gridline —
	// rather than failing.
	if high <= low {
		high = low + 1
	}

	step := NiceStep((high - low) / float64(target))

	start := math.Floor(low/step) * step
	end := math.Ceil(high/step) * step

	// Guarding with half a step absorbs the accumulated float error that would
	// otherwise drop or duplicate the final tick.
	out := make([]float64, 0, target+2)
	for v := start; v <= end+step/2; v += step {
		// Snapping to the step removes the drift that repeated addition
		// introduces, which would otherwise show up as a label of "0.30000000004".
		out = append(out, math.Round(v/step)*step)
	}
	return out
}

// NiceStep rounds an interval up to 1, 2, 5 or 10 times a power of ten.
//
// Those are the multipliers people read fluently: an axis stepping by 25 is
// harder to scan than one stepping by 20 or 50, even though 25 divides the range
// more evenly.
func NiceStep(raw float64) float64 {
	if raw <= 0 || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 1
	}

	magnitude := math.Pow(10, math.Floor(math.Log10(raw)))

	switch normalised := raw / magnitude; {
	case normalised <= 1:
		return magnitude
	case normalised <= 2:
		return 2 * magnitude
	case normalised <= 5:
		return 5 * magnitude
	default:
		return 10 * magnitude
	}
}

// Scale maps a value in a data range onto a coordinate range.
//
// The coordinate range is given as From and To rather than as an origin and a
// length so that an inverted axis needs no special case: a vertical scale simply
// runs From the bottom of the plot To the top, and because layout space grows
// downwards that means From is the larger number.
type Scale struct {
	// Low and High bound the data.
	Low, High float64

	// From and To bound the coordinates, and may run in either direction.
	From, To float64
}

// At maps a data value to a coordinate. Values outside Low..High extrapolate
// rather than clamping, which is what lets a series overshoot a fixed axis
// visibly instead of silently flattening against it.
func (s Scale) At(v float64) float64 {
	span := s.High - s.Low
	if span == 0 {
		// A zero-width domain has no gradient; everything maps to the midpoint.
		return (s.From + s.To) / 2
	}
	return s.From + (v-s.Low)/span*(s.To-s.From)
}

// Span returns the coordinate distance the scale covers, always positive.
func (s Scale) Span() float64 { return math.Abs(s.To - s.From) }

// FormatValue renders an axis or data value for display.
//
// Large numbers are abbreviated because an axis has no room for nine digits, and
// the thousands separator that would otherwise be needed reads badly in a tick
// label. Anything below a thousand keeps at most one decimal place, trimmed.
func FormatValue(v float64) string {
	abs := math.Abs(v)

	switch {
	case abs >= 1e9:
		return trimZeros(fmt.Sprintf("%.1f", v/1e9)) + "B"
	case abs >= 1e6:
		return trimZeros(fmt.Sprintf("%.1f", v/1e6)) + "M"
	case abs >= 1e4:
		// Four-digit values stay literal; only from ten thousand does an
		// abbreviation save enough room to be worth the loss of precision.
		return trimZeros(fmt.Sprintf("%.1f", v/1e3)) + "k"
	default:
		return trimZeros(strconv.FormatFloat(v, 'f', 1, 64))
	}
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// bounds returns the smallest and largest value across every series, always
// including zero so that a bar or area chart is measured from a real baseline
// rather than from its own minimum.
func bounds(series []Series, includeZero bool) (low, high float64) {
	if includeZero {
		low, high = 0, 0
	} else {
		low, high = math.Inf(1), math.Inf(-1)
	}

	for _, s := range series {
		for _, v := range s.Values {
			low = math.Min(low, v)
			high = math.Max(high, v)
		}
	}

	if math.IsInf(low, 1) || math.IsInf(high, -1) {
		return 0, 1
	}
	return low, high
}
