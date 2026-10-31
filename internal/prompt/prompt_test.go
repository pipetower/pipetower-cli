package prompt

import (
	"io"
	"testing"
)

// terminalInput delivers one line per read, like a terminal does.
type terminalInput struct{ lines []string }

func (t *terminalInput) Read(p []byte) (int, error) {
	if len(t.lines) == 0 {
		return 0, io.EOF
	}
	line := t.lines[0] + "\n"
	t.lines = t.lines[1:]
	return copy(p, line), nil
}

// answering is a prompter in accessible mode which reads the given answer lines.
func answering(lines ...string) Huh {
	return Huh{In: &terminalInput{lines: lines}, Out: io.Discard, Accessible: true}
}

func TestSelectReturnsTheIndexOfTheChosenOption(t *testing.T) {
	choice, err := answering("2").Select("Server", []string{"web-01", "web-02", "db-01"})

	if err != nil || choice != 1 {
		t.Errorf("choice = %d, err = %v, want 1", choice, err)
	}
}

func TestMultiSelectReturnsAllChosenIndexes(t *testing.T) {
	choices, err := answering("1", "3", "0").MultiSelect("Servers", []string{"web-01", "web-02", "db-01"})

	if err != nil || len(choices) != 2 || choices[0] != 0 || choices[1] != 2 {
		t.Errorf("choices = %v, err = %v, want [0 2]", choices, err)
	}
}

func TestInputRequiresAValue(t *testing.T) {
	value, err := answering("", "main").Input("branch", "")

	if err != nil || value != "main" {
		t.Errorf("value = %q, err = %v, want the second, non-empty answer", value, err)
	}
}

func TestConfirm(t *testing.T) {
	for answer, want := range map[string]bool{"y": true, "n": false} {
		confirmed, err := answering(answer).Confirm("Terminate run?", "Terminate", "Keep running")

		if err != nil || confirmed != want {
			t.Errorf("%q: confirmed = %v, err = %v", answer, confirmed, err)
		}
	}
}
