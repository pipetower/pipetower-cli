package ui

import (
	"errors"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

var (
	problemTitle = lipgloss.NewStyle().Bold(true).Foreground(ColorRed)
	hintLabel    = lipgloss.NewStyle().Foreground(ColorMuted)
)

// ProblemOf explains any error: explaining errors describe themselves, others become a generic problem.
func ProblemOf(err error) fault.Problem {
	var explainer fault.Explainer
	if errors.As(err, &explainer) {
		return explainer.Problem()
	}
	return fault.Problem{Title: Capitalize(err.Error()), Code: fault.ExitFailure}
}

// RenderProblem renders a problem: a red title, the detail and aligned hints.
func RenderProblem(p fault.Problem) string {
	var b strings.Builder
	b.WriteString("\n " + problemTitle.Render("✗  "+p.Title) + "\n")

	if p.Detail != "" {
		b.WriteString("\n" + indentLines(p.Detail, "    ", Muted) + "\n")
	}
	if len(p.Hints) > 0 {
		b.WriteString("\n" + indentLines(renderHints(p.Hints), "    ", lipgloss.NewStyle()) + "\n")
	}

	return b.String() + "\n"
}

// renderHints aligns the hint labels and values in two columns.
func renderHints(hints []fault.Hint) string {
	width := 0
	for _, hint := range hints {
		if hint.Value != "" {
			width = max(width, lipgloss.Width(hint.Label))
		}
	}

	lines := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint.Value == "" {
			lines = append(lines, Accent.Render("→ ")+hint.Label)
			continue
		}
		label := hintLabel.Render(hint.Label) + strings.Repeat(" ", width+3-lipgloss.Width(hint.Label))
		lines = append(lines, label+Code.Render(hint.Value))
	}

	return strings.Join(lines, "\n")
}

// RenderWarning renders a non-fatal warning line.
func RenderWarning(text string) string {
	return Warning.Render(" !  ") + Muted.Render(text)
}

// indentLines styles and prefixes every line on its own, so lines are not padded to a common width.
func indentLines(text, prefix string, style lipgloss.Style) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + style.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
