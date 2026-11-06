package runview

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
)

// press is the message of a pressed key, with ctrl as modifier if wanted.
func press(code rune, ctrl bool) tea.KeyPressMsg {
	if ctrl {
		return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}

// liveModel is the live view of the running deploy run, recording terminate requests.
func liveModel(terminations *int) *model {
	m := newModel("30denr", NewTracker(), ModeTree, 100, func(context.Context) error {
		*terminations++
		return nil
	})
	m.Update(RunUpdated{Run: deployRun()})
	return m
}

// quits tells whether the command ends the program.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestCtrlCAsksBeforeAnything(t *testing.T) {
	terminations := 0
	m := liveModel(&terminations)

	_, cmd := m.Update(press('c', true))

	if !m.confirming || cmd != nil || terminations != 0 {
		t.Errorf("ctrl+c must only ask: confirming=%v cmd=%v terminations=%d", m.confirming, cmd != nil, terminations)
	}
}

func TestConfirmedTerminationRequestsItAndKeepsFollowing(t *testing.T) {
	terminations := 0
	m := liveModel(&terminations)
	m.Update(press('c', true))

	_, cmd := m.Update(press('y', false))
	msg := cmd()
	m.Update(msg)

	if terminations != 1 || m.outcome != OutcomeNone {
		t.Errorf("terminations=%d outcome=%v, want 1 and still following", terminations, m.outcome)
	}
}

func TestDeclinedTerminationDetaches(t *testing.T) {
	terminations := 0
	m := liveModel(&terminations)
	m.Update(press('c', true))

	_, cmd := m.Update(press('n', false))

	if !quits(cmd) || m.outcome != OutcomeDetached || terminations != 0 {
		t.Errorf("quits=%v outcome=%v terminations=%d, want detached without termination", quits(cmd), m.outcome, terminations)
	}
}

func TestEscapeContinuesFollowing(t *testing.T) {
	m := liveModel(new(int))
	m.Update(press('c', true))

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if m.confirming || cmd != nil || m.outcome != OutcomeNone {
		t.Errorf("esc must close the question and continue: confirming=%v outcome=%v", m.confirming, m.outcome)
	}
}

func TestQDetachesImmediately(t *testing.T) {
	m := liveModel(new(int))

	_, cmd := m.Update(press('q', false))

	if !quits(cmd) || m.outcome != OutcomeDetached {
		t.Errorf("q must detach: quits=%v outcome=%v", quits(cmd), m.outcome)
	}
}

func TestFinishedRunEndsTheView(t *testing.T) {
	m := liveModel(new(int))

	_, cmd := m.Update(RunUpdated{Run: finishedRun()})

	if !quits(cmd) || m.outcome != OutcomeFinished || m.tracker.Run.Status != api.StatusFailed {
		t.Errorf("finished run must end the view: quits=%v outcome=%v", quits(cmd), m.outcome)
	}
}

func TestCtrlCWhileConnectingIsVisibleAndDetachesSafely(t *testing.T) {
	m := newModel("30denr", NewTracker(), ModeTree, 100, func(context.Context) error { return nil })

	m.Update(press('c', true))
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Terminate this run") {
		t.Errorf("the question must be visible while connecting:\n%s", view)
	}

	_, cmd := m.Update(press('n', false))
	if !quits(cmd) || !strings.Contains(ansi.Strip(m.finalText()), "pt run watch 30denr") {
		t.Errorf("detaching before the first update must quit with the follow hint: %q", ansi.Strip(m.finalText()))
	}
}

func TestLiveFrameFitsTheTerminalHeight(t *testing.T) {
	for _, height := range []int{8, 12, 24} {
		m := newModel("4k1x9z", NewTracker(), ModeTree, 100, func(context.Context) error { return nil })
		m.Update(tea.WindowSizeMsg{Width: 100, Height: height})
		m.Update(RunUpdated{Run: fleetRun()})

		frame := ansi.Strip(m.View().Content)
		if lines := strings.Count(frame, "\n") + 1; lines > height || !strings.Contains(frame, "▶ Restart PHP") {
			t.Errorf("height %d: frame has %d lines or lost its header:\n%s", height, lines, frame)
		}
	}
}

func TestFinalResultIsNotLimitedByTheTerminal(t *testing.T) {
	m := newModel("4k1x9z", NewTracker(), ModeTree, 100, func(context.Context) error { return nil })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m.Update(press('q', false))
	m.Update(RunUpdated{Run: fleetRun()})

	if view, final := m.View().Content, ansi.Strip(m.finalText()); view != "" || !strings.Contains(final, "web-12") {
		t.Errorf("after quitting the view must be empty and the final text complete:\n%s", final)
	}
}

// printed returns the text a command prints above the live view, "" if it prints nothing.
func printed(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	return fmt.Sprint(cmd())
}

func TestFKeyShowsTheMissedOutputOnceAndSwitchesBack(t *testing.T) {
	m := liveModel(new(int))
	m.Update(OutputReceived{Stream: "s2", Data: "Cloning into '.'...\n", Offset: 0})

	_, cmd := m.Update(press('f', false))
	if out := printed(cmd); m.mode != ModeFull || !strings.Contains(out, "Cloning into") {
		t.Fatalf("mode=%v printed=%q, want the full output with the output so far", m.mode, out)
	}

	_, cmd = m.Update(press('f', false))
	_, more := m.Update(OutputReceived{Stream: "s2", Data: "Receiving objects: 100%\n", Offset: 20})
	if m.mode != ModeTree || printed(cmd) != "" || printed(more) != "" {
		t.Fatalf("mode=%v, want the step tree which prints nothing", m.mode)
	}

	_, cmd = m.Update(press('f', false))
	if out := printed(cmd); !strings.Contains(out, "Receiving objects") || strings.Contains(out, "Cloning into") {
		t.Errorf("printed %q, want only the output since the full output was shown last", out)
	}
}

func TestFooterNamesTheViewTheKeySwitchesTo(t *testing.T) {
	m := liveModel(new(int))
	if footer := ansi.Strip(m.footer()); !strings.Contains(footer, "f full output") {
		t.Errorf("footer in the tree: %q", footer)
	}
	m.Update(press('f', false))
	if footer := ansi.Strip(m.footer()); !strings.Contains(footer, "f step tree") {
		t.Errorf("footer in the full output: %q", footer)
	}
}
