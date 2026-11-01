package prompt

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/pipetower/pipetower-cli/internal/ui"
)

// pickerHeight is the number of options the picker shows at once.
const pickerHeight = 10

// Option is an option of a picker: plain text columns, the first one is the name, and an
// optional styled mark in front, e.g. the connection state of a server. Typing searches all columns.
type Option struct {
	Mark    string
	Columns []string
}

// text is the searched text of the option: its columns separated by spaces.
func (o Option) text() string {
	return strings.Join(o.Columns, " ")
}

// picker is an inline list which the user narrows down by typing (fuzzy) and chooses from with the
// arrow keys: one option, or with multi several, selected with tab.
type picker struct {
	title    string
	options  []Option
	multi    bool
	widths   []int
	input    textinput.Model
	matches  fuzzy.Matches
	selected []int
	cursor   int
	offset   int
	width    int
	done     bool
	canceled bool
}

// newPicker creates a picker with all options matching.
func newPicker(title string, options []Option, multi bool) *picker {
	p := &picker{title: title, options: options, multi: multi, widths: columnWidths(options), input: newField()}
	p.filter()
	return p
}

// Init starts the blinking of the cursor.
func (p *picker) Init() tea.Cmd {
	return textinput.Blink
}

// Update moves the cursor, selects, chooses, cancels or passes typing to the input and filters again.
func (p *picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		return p, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			p.canceled = true
			return p, tea.Quit
		case "enter":
			return p, p.choose()
		case "tab":
			p.toggle()
			return p, nil
		case "up", "ctrl+p":
			p.move(-1)
			return p, nil
		case "down", "ctrl+n":
			p.move(1)
			return p, nil
		}
	}

	before := p.input.Value()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() != before {
		p.filter()
	}
	return p, cmd
}

// choose finishes with the selected options, or the one under the cursor if none is selected.
func (p *picker) choose() tea.Cmd {
	if len(p.selected) == 0 {
		if len(p.matches) == 0 {
			return nil
		}
		p.selected = []int{p.matches[p.cursor].Index}
	}
	slices.Sort(p.selected)
	p.done = true
	return tea.Quit
}

// toggle selects or deselects the option under the cursor and moves on, only when several can be chosen.
func (p *picker) toggle() {
	if !p.multi || len(p.matches) == 0 {
		return
	}
	index := p.matches[p.cursor].Index
	if i := slices.Index(p.selected, index); i >= 0 {
		p.selected = slices.Delete(p.selected, i, i+1)
	} else {
		p.selected = append(p.selected, index)
	}
	p.move(1)
}

// filter matches the options against the input (fuzzy). Options which contain the input in their
// name come first, then those which contain it in another column, then the other matches; each
// group keeps the order of the options. Without input all options match.
func (p *picker) filter() {
	p.cursor, p.offset = 0, 0
	input := p.input.Value()
	if input == "" {
		p.matches = make(fuzzy.Matches, len(p.options))
		for i := range p.options {
			p.matches[i] = fuzzy.Match{Index: i}
		}
		return
	}
	p.matches = fuzzy.FindNoSort(input, texts(p.options))
	slices.SortStableFunc(p.matches, func(a, b fuzzy.Match) int {
		return p.rank(a, input) - p.rank(b, input)
	})
}

// rank tells how well an option matches: 0 for the input in its name, 1 in another column, 2 only fuzzy.
func (p *picker) rank(match fuzzy.Match, input string) int {
	input = strings.ToLower(input)
	option := p.options[match.Index]
	switch {
	case strings.Contains(strings.ToLower(option.Columns[0]), input):
		return 0
	case strings.Contains(strings.ToLower(option.text()), input):
		return 1
	}
	return 2
}

// move moves the cursor, around the ends, and scrolls it into view.
func (p *picker) move(step int) {
	if len(p.matches) == 0 {
		return
	}
	p.cursor = (p.cursor + step + len(p.matches)) % len(p.matches)
	p.offset = min(p.offset, p.cursor)
	p.offset = max(p.offset, p.cursor-pickerHeight+1)
}

// View renders the question with the input, the visible matches and a footer; when done only
// the answer, after canceling nothing.
func (p *picker) View() tea.View {
	switch {
	case p.canceled:
		return tea.NewView("")
	case p.done:
		return tea.NewView(ui.Bold.Render(p.title) + " " + ui.Accent.Render(p.answer()) + "\n")
	}

	lines := []string{ui.Bold.Render(p.title) + " " + p.input.View()}
	end := min(p.offset+pickerHeight, len(p.matches))
	for i := p.offset; i < end; i++ {
		lines = append(lines, p.truncate(p.row(p.matches[i], i == p.cursor)))
	}
	if len(p.matches) == 0 {
		lines = append(lines, ui.Faint.Render("  No matches"))
	}
	return tea.NewView(strings.Join(append(lines, ui.Faint.Render(p.footer())), "\n"))
}

// answer names the chosen options.
func (p *picker) answer() string {
	names := make([]string, len(p.selected))
	for i, index := range p.selected {
		names[i] = p.options[index].Columns[0]
	}
	return strings.Join(names, ", ")
}

// footer tells which keys work, how many options are selected and, while filtering or when not all
// options fit, how many match.
func (p *picker) footer() string {
	var parts []string
	if p.input.Value() != "" || len(p.options) > pickerHeight {
		parts = append(parts, fmt.Sprintf("%d of %d", len(p.matches), len(p.options)))
	}
	if p.multi {
		parts = append(parts, fmt.Sprintf("%d selected", len(p.selected)), "tab to select", "enter to confirm")
	} else {
		parts = append(parts, "↑↓ to move", "enter to select")
	}
	return "  " + strings.Join(append(parts, "esc to cancel"), " · ")
}

// row renders one option: cursor, selection, mark and its aligned columns with the matched characters highlighted.
func (p *picker) row(match fuzzy.Match, current bool) string {
	option := p.options[match.Index]
	prefix := "  "
	if current {
		prefix = ui.Accent.Render("› ")
	}
	if p.multi {
		prefix += p.checkbox(match.Index) + " "
	}

	var cells []string
	if option.Mark != "" {
		cells = append(cells, option.Mark)
	}
	start := 0
	for i, column := range option.Columns {
		style := ui.Muted
		if i == 0 {
			style = lipgloss.NewStyle().Bold(current)
		}
		cell := highlight(column, match.MatchedIndexes, start, style)
		cells = append(cells, cell+strings.Repeat(" ", p.widths[i]-lipgloss.Width(column)))
		start += len(column) + 1
	}
	return prefix + strings.TrimRight(strings.Join(cells, "  "), " ")
}

// checkbox shows whether an option is selected.
func (p *picker) checkbox(index int) string {
	if slices.Contains(p.selected, index) {
		return ui.Success.Render("✓")
	}
	return ui.Faint.Render("·")
}

// truncate cuts a line at the width of the terminal.
func (p *picker) truncate(line string) string {
	if p.width == 0 {
		return line
	}
	return lipgloss.NewStyle().MaxWidth(p.width).Render(line)
}

// highlight renders a column, its characters at the matched byte positions (counted from start) in the accent color.
func highlight(column string, matched []int, start int, style lipgloss.Style) string {
	var b strings.Builder
	for i, r := range column {
		if slices.Contains(matched, start+i) {
			b.WriteString(ui.Accent.Bold(true).Render(string(r)))
		} else {
			b.WriteString(style.Render(string(r)))
		}
	}
	return b.String()
}

// columnWidths returns the width of every column over all options, so that the layout stays while filtering.
func columnWidths(options []Option) []int {
	columns := make([][]string, len(options))
	for i, option := range options {
		columns[i] = option.Columns
	}
	return ui.ColumnWidths(columns)
}

// answered tells whether options were chosen.
func (p *picker) answered() bool {
	return p.done
}

// texts returns the searched texts of the options.
func texts(options []Option) []string {
	labels := make([]string, len(options))
	for i, option := range options {
		labels[i] = option.text()
	}
	return labels
}
