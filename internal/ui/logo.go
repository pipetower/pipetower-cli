package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// logo is the name Pipetower in filled blocks (█) with a line shadow (figlet "ANSI Shadow").
const logo = `██████╗ ██╗██████╗ ███████╗████████╗ ██████╗ ██╗    ██╗███████╗██████╗
██╔══██╗██║██╔══██╗██╔════╝╚══██╔══╝██╔═══██╗██║    ██║██╔════╝██╔══██╗
██████╔╝██║██████╔╝█████╗     ██║   ██║   ██║██║ █╗ ██║█████╗  ██████╔╝
██╔═══╝ ██║██╔═══╝ ██╔══╝     ██║   ██║   ██║██║███╗██║██╔══╝  ██╔══██╗
██║     ██║██║     ███████╗   ██║   ╚██████╔╝╚███╔███╔╝███████╗██║  ██║
╚═╝     ╚═╝╚═╝     ╚══════╝   ╚═╝    ╚═════╝  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝`

// logoGradient are the colors of the gradient on pipetower.com: cyan-300, emerald-500, cyan-600.
var logoGradient = []color.Color{lipgloss.Color("#53EAFD"), lipgloss.Color("#00BC7D"), lipgloss.Color("#0092B8")}

// shadowDarkening is how much darker the shadow lines are than the blocks.
const shadowDarkening = 0.45

// Logo renders the logo with the gradient of the website from left to right: the blocks in the
// gradient, their shadow in a darker shade of it. It is indented like the help.
func Logo() string {
	lines := strings.Split(logo, "\n")
	width := 0
	for _, line := range lines {
		width = max(width, len([]rune(line)))
	}
	colors := lipgloss.Blend1D(width, logoGradient...)

	for i, line := range lines {
		var b strings.Builder
		for x, char := range []rune(line) {
			b.WriteString(logoChar(char, colors[x]))
		}
		lines[i] = "  " + b.String()
	}
	return strings.Join(lines, "\n")
}

// logoChar colors one character of the logo: blocks in the color, shadow lines darker, spaces not.
func logoChar(char rune, gradient color.Color) string {
	switch char {
	case ' ':
		return " "
	case '█':
		return lipgloss.NewStyle().Foreground(gradient).Render(string(char))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Darken(gradient, shadowDarkening)).Render(string(char))
	}
}
