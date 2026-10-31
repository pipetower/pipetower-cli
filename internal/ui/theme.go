// Package ui holds the visual language of the CLI: colors, styles, icons,
// formatting and the rendering of tables and errors.
package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette: mid-tone colors which read well on dark and light terminals.
var (
	ColorBrand  = lipgloss.Color("#06B6D4")
	ColorGreen  = lipgloss.Color("#22C55E")
	ColorRed    = lipgloss.Color("#EF4444")
	ColorYellow = lipgloss.Color("#EAB308")
	ColorOrange = lipgloss.Color("#F97316")
	ColorBlue   = lipgloss.Color("#3B82F6")
	ColorMuted  = lipgloss.Color("#8A8F98")
	ColorFaint  = lipgloss.Color("#5F6670")
)

// Text styles.
var (
	Bold    = lipgloss.NewStyle().Bold(true)
	Title   = lipgloss.NewStyle().Bold(true).Foreground(ColorBrand)
	Accent  = lipgloss.NewStyle().Foreground(ColorBrand)
	Muted   = lipgloss.NewStyle().Foreground(ColorMuted)
	Faint   = lipgloss.NewStyle().Foreground(ColorFaint)
	Success = lipgloss.NewStyle().Foreground(ColorGreen)
	Danger  = lipgloss.NewStyle().Foreground(ColorRed)
	Warning = lipgloss.NewStyle().Foreground(ColorYellow)
	Label   = lipgloss.NewStyle().Foreground(ColorMuted).Width(14)
	Code    = lipgloss.NewStyle().Foreground(ColorBrand)
)

// Fg returns a style with the foreground color.
func Fg(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// Tag renders a tag name in its own color.
func Tag(name, hex string) string {
	style := Muted
	if hex != "" {
		style = Fg(lipgloss.Color(hex))
	}
	return style.Render("#" + name)
}
