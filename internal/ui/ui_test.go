package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

func TestDuration(t *testing.T) {
	cases := map[time.Duration]string{
		400 * time.Millisecond: "400ms",
		34 * time.Second:       "34s",
		125 * time.Second:      "2m 05s",
		62 * time.Minute:       "1h 02m",
	}
	for d, want := range cases {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestBytes(t *testing.T) {
	cases := map[float64]string{512: "512 B", 3.3 * 1024 * 1024 * 1024: "3.3 GB", 150 * 1024 * 1024: "150 MB"}
	for bytes, want := range cases {
		if got := Bytes(bytes); got != want {
			t.Errorf("Bytes(%v) = %q, want %q", bytes, got, want)
		}
	}
}

func TestBar(t *testing.T) {
	percent := 49.0
	if got := ansi.Strip(Bar(&percent, 10)); got != "▰▰▰▰▰▱▱▱▱▱" {
		t.Errorf("Bar = %q", got)
	}
	tiny := 0.4
	if got := ansi.Strip(Bar(&tiny, 10)); !strings.HasPrefix(got, "▰▱") {
		t.Errorf("tiny usage should show one segment, got %q", got)
	}
	if got := ansi.Strip(Bar(nil, 4)); got != "▱▱▱▱" {
		t.Errorf("unknown = %q", got)
	}
}

func TestAgo(t *testing.T) {
	now := time.Now()
	past := now.Add(-50 * time.Hour)
	if got := Ago(&past, now); got != "2 days ago" {
		t.Errorf("Ago = %q", got)
	}
}

func TestRenderProblemAlignsHints(t *testing.T) {
	out := ansi.Strip(RenderProblem(fault.Problem{
		Title:  "No API token found",
		Detail: "pt needs a token.",
		Hints:  []fault.Hint{{Label: "Create", Value: "a"}, {Label: "Save it", Value: "b"}, {Label: "Just text"}},
	}))
	for _, want := range []string{"✗  No API token found", "pt needs a token.", "Create    a", "Save it   b", "→ Just text"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestProblemOfPlainError(t *testing.T) {
	if p := ProblemOf(errors.New("boom")); p.Title != "Boom" || p.Code != fault.ExitFailure {
		t.Errorf("problem = %+v", p)
	}
}

func TestTableFitsIntoTheWidth(t *testing.T) {
	rows := [][]string{{"Jamil Soufan (owner), Jonas Gnioui (member), John Wagner (member)", "3meb4e"}}

	for _, line := range strings.Split(Table([]string{"TEAM", "ID"}, rows, 40), "\n") {
		if width := lipgloss.Width(line); width > 40 {
			t.Errorf("line is %d wide, want at most 40: %q", width, line)
		}
	}
	if out := Table([]string{"TEAM", "ID"}, rows, 0); !strings.Contains(out, "John Wagner (member)") {
		t.Errorf("without a width nothing is shortened:\n%s", out)
	}
}
