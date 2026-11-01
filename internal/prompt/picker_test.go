package prompt

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var servers = []Option{
	{Columns: []string{"allaoui.com", "91.99.208.20"}},
	{Columns: []string{"localhost", "localhost"}},
	{Columns: []string{"pipetower.com", "5.75.144.227"}},
}

// typing sends the text to the picker key by key.
func typing(p *picker, text string) {
	for _, r := range text {
		p.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// press sends keys to the picker.
func press(p *picker, keys ...rune) {
	for _, key := range keys {
		p.Update(tea.KeyPressMsg{Code: key})
	}
}

func TestPickerMatchesFuzzyAcrossColumns(t *testing.T) {
	p := newPicker("Which server?", servers, false)

	typing(p, "ptw")
	if len(p.matches) != 1 || p.matches[0].Index != 2 {
		t.Errorf("ptw matches %v, want only pipetower.com", p.matches)
	}

	p.input.SetValue("")
	typing(p, "5.75")
	if len(p.matches) != 1 || p.matches[0].Index != 2 {
		t.Errorf("5.75 matches %v, want the server with that ip", p.matches)
	}
}

func TestPickerRanksMatchesInTheNameFirstAndKeepsTheOrder(t *testing.T) {
	options := []Option{
		{Columns: []string{"allaoui.com", "91.99.208.20"}},
		{Columns: []string{"db", "10.0.0.1", "#local"}},
		{Columns: []string{"localhost", "127.0.0.1"}},
		{Columns: []string{"local-dev", "127.0.0.2"}},
	}
	p := newPicker("Which server?", options, false)

	typing(p, "loc")
	var order []int
	for _, match := range p.matches {
		order = append(order, match.Index)
	}
	if !slices.Equal(order, []int{2, 3, 1, 0}) {
		t.Errorf("order = %v, want names first in their order, then other columns, then fuzzy matches", order)
	}
}

func TestPickerChoosesTheOptionUnderTheCursor(t *testing.T) {
	p := newPicker("Which server?", servers, false)

	press(p, tea.KeyDown, tea.KeyEnter)
	if !p.done || !slices.Equal(p.selected, []int{1}) {
		t.Errorf("selected = %v, want localhost", p.selected)
	}
}

func TestPickerCursorWrapsAround(t *testing.T) {
	p := newPicker("Which server?", servers, false)

	press(p, tea.KeyUp, tea.KeyEnter)
	if !slices.Equal(p.selected, []int{2}) {
		t.Errorf("selected = %v, want the last option", p.selected)
	}
}

func TestPickerWithoutMatchesChoosesNothing(t *testing.T) {
	p := newPicker("Which server?", servers, false)

	typing(p, "zzz")
	press(p, tea.KeyEnter)
	if p.done {
		t.Errorf("selected = %v, want none", p.selected)
	}
}

func TestPickerSelectsSeveralWithTab(t *testing.T) {
	p := newPicker("Which servers?", servers, true)

	press(p, tea.KeyTab, tea.KeyDown, tea.KeyTab, tea.KeyEnter)
	if !p.done || !slices.Equal(p.selected, []int{0, 2}) {
		t.Errorf("selected = %v, want the first and the last", p.selected)
	}
}

func TestPickerWithoutSelectionTakesTheOptionUnderTheCursor(t *testing.T) {
	p := newPicker("Which servers?", servers, true)

	press(p, tea.KeyDown, tea.KeyEnter)
	if !slices.Equal(p.selected, []int{1}) {
		t.Errorf("selected = %v, want the option under the cursor", p.selected)
	}
}

func TestTabSelectsOnlyWhenSeveralCanBeChosen(t *testing.T) {
	p := newPicker("Which server?", servers, false)

	press(p, tea.KeyTab, tea.KeyEnter)
	if !slices.Equal(p.selected, []int{0}) {
		t.Errorf("selected = %v, want the first option only", p.selected)
	}
}
