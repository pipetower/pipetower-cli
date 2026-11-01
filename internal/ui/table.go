package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
)

// columnGap is the space between two table columns.
const columnGap = 3

// minColumnWidth is the width below which a column is not shortened to fit a table into the width.
const minColumnWidth = 8

var (
	tableHeader = lipgloss.NewStyle().Foreground(ColorMuted).Bold(true)
	tableCell   = lipgloss.NewStyle()
)

// Table renders rows below muted headers, without borders. With a width (0 for none), cells are
// shortened with "…" until every row fits, so that no row wraps in the terminal: first the name
// (the first column with a header), then the widest columns.
func Table(headers []string, rows [][]string, width int) string {
	if width > 0 {
		headers, rows = fit(headers, rows, width)
	}
	last := len(headers) - 1

	return table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := tableCell
			if row == table.HeaderRow {
				style = tableHeader
			}
			if col < last {
				style = style.PaddingRight(columnGap)
			}
			return style
		}).
		String()
}

// fit shortens the cells of the name column, then of the widest columns, until the rows fit into the width.
func fit(headers []string, rows [][]string, width int) ([]string, [][]string) {
	widths := ColumnWidths(append([][]string{headers}, rows...))
	excess := sumOf(widths) + columnGap*(len(widths)-1) - width

	name := slices.IndexFunc(headers, func(header string) bool { return header != "" })
	if name >= 0 {
		cut := min(excess, widths[name]-minColumnWidth)
		widths[name], excess = widths[name]-max(0, cut), excess-max(0, cut)
	}
	for ; excess > 0; excess-- {
		widest := slices.Index(widths, slices.Max(widths))
		if widths[widest] <= minColumnWidth {
			break
		}
		widths[widest]--
	}

	shorten := func(cells []string) []string {
		fitted := make([]string, len(cells))
		for i, cell := range cells {
			fitted[i] = ansi.Truncate(cell, widths[i], "…")
		}
		return fitted
	}
	fittedRows := make([][]string, len(rows))
	for i, row := range rows {
		fittedRows[i] = shorten(row)
	}
	return shorten(headers), fittedRows
}

// ColumnWidths returns the width of the widest cell of every column.
func ColumnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], lipgloss.Width(cell))
		}
	}
	return widths
}

// sumOf adds the numbers.
func sumOf(numbers []int) int {
	sum := 0
	for _, n := range numbers {
		sum += n
	}
	return sum
}

// Indent puts the prefix in front of every line of the text.
func Indent(text, prefix string) string {
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}

// Empty renders the hint shown instead of an empty table.
func Empty(text string) string {
	return Muted.Render(text)
}
