// Package prompt asks the user interactively with inline prompts in the Pipetower style: the
// question with the input next to it, and after answering only the question and its answer.
package prompt

import (
	"errors"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// Prompter asks questions. The interface keeps callers testable without a terminal.
type Prompter interface {
	Pick(title string, options []Option) (int, error)
	PickMany(title string, options []Option) ([]int, error)
	Input(title string) (string, error)
	Secret(title string) (string, error)
	Confirm(title, affirmative, negative string) (bool, error)
}

// Terminal asks in the terminal. In and Out default to the terminal, Profile limits the colors,
// e.g. to none with --no-color.
type Terminal struct {
	In      io.Reader
	Out     io.Writer
	Profile colorprofile.Profile
}

// model is a prompt which ends answered or canceled.
type model interface {
	tea.Model
	answered() bool
}

// Pick lets the user choose one of many options by typing a part of it and returns its index.
func (t Terminal) Pick(title string, options []Option) (int, error) {
	p := newPicker(title, options, false)
	if err := t.run(p); err != nil {
		return 0, err
	}
	return p.selected[0], nil
}

// PickMany lets the user choose one or several options and returns their indexes in the order of the options.
func (t Terminal) PickMany(title string, options []Option) ([]int, error) {
	p := newPicker(title, options, true)
	if err := t.run(p); err != nil {
		return nil, err
	}
	return p.selected, nil
}

// Input asks for a non-empty text.
func (t Terminal) Input(title string) (string, error) {
	return t.input(title, false)
}

// Secret asks for a non-empty text which is not shown, like a token.
func (t Terminal) Secret(title string) (string, error) {
	return t.input(title, true)
}

// input asks for a non-empty text, shown or hidden.
func (t Terminal) input(title string, secret bool) (string, error) {
	i := newInput(title, secret)
	if err := t.run(i); err != nil {
		return "", err
	}
	return i.field.Value(), nil
}

// Confirm asks a yes/no question as a choice between the affirmative and the negative answer.
func (t Terminal) Confirm(title, affirmative, negative string) (bool, error) {
	index, err := t.Pick(title, []Option{{Columns: []string{affirmative}}, {Columns: []string{negative}}})
	return index == 0, err
}

// run shows the prompt until it is answered; canceling becomes a silent interrupt.
func (t Terminal) run(m model) error {
	var opts []tea.ProgramOption
	if t.In != nil {
		opts = append(opts, tea.WithInput(t.In))
	}
	if t.Out != nil {
		opts = append(opts, tea.WithOutput(t.Out))
	}
	if t.Profile != colorprofile.Unknown {
		opts = append(opts, tea.WithColorProfile(t.Profile))
	}

	_, err := tea.NewProgram(m, opts...).Run()
	if errors.Is(err, tea.ErrInterrupted) || (err == nil && !m.answered()) {
		return &fault.Silent{Code: fault.ExitInterrupted}
	}
	return err
}
