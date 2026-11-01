package prompt

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInputRequiresAValue(t *testing.T) {
	i := newInput("Value of BRANCH?")

	i.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if i.answered() || !i.empty {
		t.Fatal("an empty value must not be accepted")
	}

	for _, r := range "main" {
		i.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	i.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !i.answered() || i.field.Value() != "main" {
		t.Errorf("value = %q, answered = %v", i.field.Value(), i.answered())
	}
}

func TestInputCanBeCanceled(t *testing.T) {
	i := newInput("Value of BRANCH?")

	i.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if i.answered() || !i.canceled {
		t.Error("esc must cancel")
	}
}

// answering is a terminal which reads the given keys as input.
func answering(keys string) Terminal {
	return Terminal{In: strings.NewReader(keys), Out: io.Discard}
}

func TestInputReturnsTheEnteredText(t *testing.T) {
	value, err := answering("main\r").Input("Value of BRANCH?")
	if err != nil || value != "main" {
		t.Errorf("value = %q, err = %v", value, err)
	}
}

func TestPickManyReturnsTheSelectedOptions(t *testing.T) {
	chosen, err := answering("\t\t\r").PickMany("Which servers?", servers)
	if err != nil || len(chosen) != 2 || chosen[0] != 0 || chosen[1] != 1 {
		t.Errorf("chosen = %v, err = %v, want the first two", chosen, err)
	}
}

func TestConfirmTakesTheChosenAnswer(t *testing.T) {
	confirmed, err := answering("\x1b[B\r").Confirm("Terminate run?", "Yes", "No")
	if err != nil || confirmed {
		t.Errorf("confirmed = %v, err = %v, want no", confirmed, err)
	}
}
