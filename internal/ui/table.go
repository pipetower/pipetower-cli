package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

var (
	tableHeader = lipgloss.NewStyle().Foreground(ColorMuted).Bold(true).PaddingRight(3)
	tableCell   = lipgloss.NewStyle().PaddingRight(3)
)

// Table renders rows below headers, aligned and without outer borders.
func Table(headers []string, rows [][]string) string {
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(false).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return tableHeader
			}
			return tableCell
		})

	return t.String()
}

// Empty renders the hint shown instead of an empty table.
func Empty(text string) string {
	return Muted.Render(text)
}
