package prompt

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/pipetower/pipetower-cli/internal/ui"
)

// input asks for a non-empty text on one line.
type input struct {
	title    string
	field    textinput.Model
	empty    bool
	done     bool
	canceled bool
	secret   bool
}

// newInput creates a focused input, a secret one hides what is typed.
func newInput(title string, secret bool) *input {
	i := &input{title: title, field: newField(), secret: secret}
	if secret {
		i.field.EchoMode = textinput.EchoPassword
		i.field.EchoCharacter = '•'
	}
	return i
}

// newField creates the focused text field of the prompts.
func newField() textinput.Model {
	field := textinput.New()
	field.Prompt = "› "
	styles := field.Styles()
	styles.Focused.Prompt = ui.Accent
	field.SetStyles(styles)
	field.Focus()
	return field
}

// Init starts the blinking of the cursor.
func (i *input) Init() tea.Cmd {
	return textinput.Blink
}

// Update accepts a non-empty text with enter, cancels or passes typing to the field.
func (i *input) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			i.canceled = true
			return i, tea.Quit
		case "enter":
			i.empty = strings.TrimSpace(i.field.Value()) == ""
			i.done = !i.empty
			if i.done {
				return i, tea.Quit
			}
			return i, nil
		}
	}
	var cmd tea.Cmd
	i.field, cmd = i.field.Update(msg)
	return i, cmd
}

// View renders the question with the field and a footer; when done only the answer, after canceling nothing.
func (i *input) View() tea.View {
	switch {
	case i.canceled:
		return tea.NewView("")
	case i.done && i.secret:
		return tea.NewView(ui.Bold.Render(i.title) + " " + ui.Accent.Render("••••••") + "\n")
	case i.done:
		return tea.NewView(ui.Bold.Render(i.title) + " " + ui.Accent.Render(i.field.Value()) + "\n")
	}
	footer := ui.Faint.Render("  enter to confirm · esc to cancel")
	if i.empty {
		footer = ui.Danger.Render("  A value is required")
	}
	return tea.NewView(ui.Bold.Render(i.title) + " " + i.field.View() + "\n" + footer)
}

// answered tells whether a text was entered.
func (i *input) answered() bool {
	return i.done
}
