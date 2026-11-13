package charts

import (
	"fmt"
	"math"
)

// Scale is a linear value range mapped onto a chart's plot area.
type Scale struct {
	Min, Max float64
}

// IsZero reports whether s carries no range at all — the zero Scale, which
// every chart treats as "derive the range from the data".
func (s Scale) IsZero() bool { return s.Min == 0 && s.Max == 0 }

// niceSteps are the mantissas an axis maximum may round up to. 2.5 and 7.5 are
// included because a five-level axis then divides into readable quarters
// (7.5K -> 5K/2.5K reads better than 5.6K/3.8K/1.9K).
var niceSteps = [...]float64{1, 1.5, 2, 2.5, 3, 4, 5, 7.5, 10}

// AutoScale returns a 0-based scale whose maximum is max rounded up to the next
// "nice" number so axis labels are readable. A non-positive max yields 0..1,
// an empty plot with a sane axis rather than a divide by zero.
func AutoScale(max float64) Scale {
	if max <= 0 || math.IsNaN(max) || math.IsInf(max, 0) {
		return Scale{Min: 0, Max: 1}
	}
	mag := math.Pow(10, math.Floor(math.Log10(max)))
	norm := max / mag
	for _, step := range niceSteps {
		if norm <= step+1e-9 {
			return Scale{Min: 0, Max: step * mag}
		}
	}
	return Scale{Min: 0, Max: 10 * mag}
}

// Span is the scale's value range, never zero — a degenerate scale (Max at
// or below Min) reports 1 so callers can divide by it unconditionally.
func (s Scale) Span() float64 {
	span := s.Max - s.Min
	if span <= 0 {
		return 1
	}
	return span
}

// Cells maps a value to a length in cells within a plot of the given size,
// clamped to 0..cells. The result is fractional: the caller decides how
// much of the remainder to render with a partial block glyph.
func (s Scale) Cells(v float64, cells int) float64 {
	if cells <= 0 {
		return 0
	}
	frac := (v - s.Min) / s.Span()
	switch {
	case frac <= 0 || math.IsNaN(frac):
		return 0
	case frac >= 1:
		return float64(cells)
	}
	return frac * float64(cells)
}

// Ticks returns levels evenly spaced values from Min to Max inclusive,
// highest first — the order Y-axis labels are drawn in, top to bottom.
// Fewer than two levels yields just the maximum.
func (s Scale) Ticks(levels int) []float64 {
	if levels < 2 {
		return []float64{s.Max}
	}
	out := make([]float64, levels)
	for i := range out {
		out[i] = s.Max - (s.Max-s.Min)*float64(i)/float64(levels-1)
	}
	return out
}

// FormatValue renders an axis label or a numeric readout compactly: exact
// for small values, K/M/G suffixed above a thousand. Zero is "0.0" rather
// than "0" so a zero baseline lines up with the decimal labels above it.
//
// Each band rounds the value to its own decimals before testing its upper
// bound, and a value that rounds up to the bound moves to the next band.
// Choosing the band from the unrounded value printed 999960 as "1000K" and
// 99960 as "100.0K" — wider than the gutter sized for the usual labels.
func FormatValue(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	neg := ""
	if v < 0 {
		neg, v = "-", -v
	}
	if v == 0 {
		return "0.0"
	}
	for _, b := range valueBands {
		scaled := v / b.div
		p := math.Pow10(b.decimals)
		r := math.Round(scaled*p) / p
		if b.below == 0 || r < b.below {
			return neg + fmt.Sprintf("%.*f%s", b.decimals, r, b.unit)
		}
	}
	panic("unreachable: the last value band is unbounded")
}

// valueBands are FormatValue's formats, smallest first. below is the bound,
// in scaled units, the rounded value must stay under (0: unbounded). A
// suffixed magnitude keeps one decimal until the decimal adds width without
// adding information ("105K", not "105.0K").
var valueBands = [...]struct {
	div      float64
	below    float64
	decimals int
	unit     string
}{
	{1, 1, 2, ""},
	{1, 10, 1, ""},
	{1, 1000, 0, ""},
	{1e3, 100, 1, "K"},
	{1e3, 1000, 0, "K"},
	{1e6, 100, 1, "M"},
	{1e6, 1000, 0, "M"},
	{1e9, 100, 1, "G"},
	{1e9, 0, 0, "G"},
}
