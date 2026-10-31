// Package prompt asks the user interactively, styled in the Pipetower theme.
package prompt

import (
	"errors"
	"io"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// Prompter asks questions. The interface keeps callers testable without a terminal.
type Prompter interface {
	Select(title string, options []string) (int, error)
	MultiSelect(title string, options []string) ([]int, error)
	Input(title, description string) (string, error)
	Confirm(title, affirmative, negative string) (bool, error)
}

// Huh asks with charm's huh forms. In and Out default to the terminal, Accessible asks
// line by line instead of with an interactive form (screen readers, dumb terminals, tests).
type Huh struct {
	In         io.Reader
	Out        io.Writer
	Accessible bool
}

// Select lets the user choose one option and returns its index.
func (h Huh) Select(title string, options []string) (int, error) {
	var choice int
	field := huh.NewSelect[int]().Title(title).Options(indexed(options)...).Value(&choice)
	if len(options) > 8 {
		field = field.Height(10).Filtering(false)
	}
	return choice, h.run(field)
}

// MultiSelect lets the user choose options and returns their indexes.
func (h Huh) MultiSelect(title string, options []string) ([]int, error) {
	var choices []int
	field := huh.NewMultiSelect[int]().
		Title(title).
		Description("space to select · enter to confirm · / to filter").
		Options(indexed(options)...).
		Height(min(len(options), 10) + 3).
		Value(&choices)
	return choices, h.run(field)
}

// Input asks for a non-empty text.
func (h Huh) Input(title, description string) (string, error) {
	var value string
	field := huh.NewInput().Title(title).Description(description).Value(&value).Validate(func(text string) error {
		if text == "" {
			return errors.New("a value is required")
		}
		return nil
	})
	return value, h.run(field)
}

// Confirm asks a yes/no question.
func (h Huh) Confirm(title, affirmative, negative string) (bool, error) {
	var confirmed bool
	field := huh.NewConfirm().Title(title).Affirmative(affirmative).Negative(negative).Value(&confirmed)
	return confirmed, h.run(field)
}

// run shows a single field, aborting with Ctrl+C becomes a silent interrupt.
func (h Huh) run(field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithTheme(theme).WithShowHelp(false).WithAccessible(h.Accessible)
	if h.In != nil {
		form = form.WithInput(h.In)
	}
	if h.Out != nil {
		form = form.WithOutput(h.Out)
	}

	err := form.Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return &fault.Silent{Code: fault.ExitInterrupted}
	}
	return err
}

// indexed creates options whose values are their positions.
func indexed(labels []string) []huh.Option[int] {
	options := make([]huh.Option[int], len(labels))
	for i, label := range labels {
		options[i] = huh.NewOption(label, i)
	}
	return options
}

// theme is the charm theme in Pipetower colors.
var theme = huh.ThemeFunc(func(isDark bool) *huh.Styles {
	t := huh.ThemeCharm(isDark)
	brand := lipgloss.NewStyle().Foreground(ui.ColorBrand)

	t.Focused.Title = t.Focused.Title.Foreground(ui.ColorBrand)
	t.Focused.SelectSelector = brand.SetString("› ")
	t.Focused.MultiSelectSelector = brand.SetString("› ")
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(ui.ColorBrand)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(ui.ColorBrand)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(ui.ColorGreen)
	t.Focused.TextInput.Prompt = brand
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(ui.ColorBrand)
	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Group.Title = t.Focused.Title

	return t
})
