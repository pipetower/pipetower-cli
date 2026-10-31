package ui

import (
	"image/color"
	"strings"
)

// Bar renders a segmented usage bar like ▰▰▰▰▱▱▱▱, colored by how full it is.
func Bar(percent *float64, width int) string {
	if percent == nil {
		return Faint.Render(strings.Repeat("▱", width))
	}

	filled := int(*percent/100*float64(width) + 0.5)
	filled = max(0, min(width, filled))
	if filled == 0 && *percent > 0 {
		filled = 1
	}

	return Fg(UsageColor(*percent)).Render(strings.Repeat("▰", filled)) +
		Faint.Render(strings.Repeat("▱", width-filled))
}

// UsageColor is green for normal, yellow for high and red for critical usage.
func UsageColor(percent float64) color.Color {
	switch {
	case percent >= 90:
		return ColorRed
	case percent >= 70:
		return ColorYellow
	default:
		return ColorGreen
	}
}

// LoadColor colors a load average relative to the number of cores.
func LoadColor(load float64, cores *int) color.Color {
	capacity := 1.0
	if cores != nil && *cores > 0 {
		capacity = float64(*cores)
	}
	return UsageColor(load / capacity * 100)
}
