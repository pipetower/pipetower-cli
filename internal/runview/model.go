package runview

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// Mode is how a run is shown in a terminal.
type Mode int

// Display modes.
const (
	// ModeTree shows the live step tree with the tail of running steps.
	ModeTree Mode = iota
	// ModeFull streams the complete output, with a live status line below.
	ModeFull
)

// Outcome is how following ended.
type Outcome int

// Outcomes.
const (
	OutcomeNone Outcome = iota
	OutcomeFinished
	OutcomeDetached
	OutcomeFailed
)

// terminated reports the result of the terminate request.
type terminated struct{ err error }

// model is the Bubble Tea model of the live run view.
type model struct {
	runID     string
	tracker   *Tracker
	log       *Log
	mode      Mode
	terminate func(context.Context) error

	spinner     spinner.Model
	now         time.Time
	width       int
	height      int
	conn        string
	confirming  bool
	terminating bool
	notice      string
	outcome     Outcome
	err         error
}

// newModel creates the live view.
func newModel(runID string, t *Tracker, mode Mode, width int, terminate func(context.Context) error) *model {
	return &model{
		runID:     runID,
		tracker:   t,
		log:       NewLog(t),
		mode:      mode,
		terminate: terminate,
		spinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(ui.Accent)),
		now:       time.Now(),
		width:     width,
		conn:      ConnLive,
	}
}

// Init starts the spinner.
func (m *model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles follower events, keys and timers.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case RunUpdated:
		m.tracker.SetRun(msg.Run)
		return m, m.afterChange(msg.Run.Finished())
	case OutputReceived:
		m.tracker.Output(msg.Stream, msg.Data, msg.Offset)
		return m, m.afterChange(false)
	case ConnectionChanged:
		m.conn = msg.State
	case FollowFailed:
		m.outcome, m.err = OutcomeFailed, msg.Err
		return m, tea.Quit
	case terminated:
		m.terminating = false
		m.notice = terminateNotice(msg.err)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m, m.key(msg.String())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.now = time.Now()
		return m, cmd
	}
	return m, nil
}

// afterChange prints new log lines in full mode and quits when the run finished.
func (m *model) afterChange(finished bool) tea.Cmd {
	var cmds []tea.Cmd
	if m.mode == ModeFull {
		for _, line := range m.log.Lines() {
			cmds = append(cmds, tea.Println(line))
		}
	}
	if finished {
		m.outcome = OutcomeFinished
		cmds = append(cmds, tea.Quit)
	}
	return tea.Sequence(cmds...)
}

// key handles Ctrl+C (ask to terminate or detach), and the answers of that question.
func (m *model) key(key string) tea.Cmd {
	if m.confirming {
		return m.answer(key)
	}
	switch key {
	case "ctrl+c":
		m.confirming = true
	case "q":
		m.outcome = OutcomeDetached
		return tea.Quit
	}
	return nil
}

// answer reacts to the terminate question.
func (m *model) answer(key string) tea.Cmd {
	m.confirming = false
	switch key {
	case "y", "Y":
		m.terminating = true
		return m.requestTermination
	case "esc":
		return nil
	default:
		m.outcome = OutcomeDetached
		return tea.Quit
	}
}

// requestTermination asks the API to terminate the run.
func (m *model) requestTermination() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return terminated{err: m.terminate(ctx)}
}

// View renders the live tree (or status line) and the footer. After quitting the view is empty,
// the final result is printed as normal output, so that it is never cut to the terminal height.
func (m *model) View() tea.View {
	if m.outcome != OutcomeNone {
		return tea.NewView("")
	}
	if m.tracker.Run == nil {
		return tea.NewView(" " + m.spinner.View() + ui.Muted.Render(" Connecting…") + "\n\n" + m.footer())
	}

	body := m.statusLine()
	if m.mode != ModeFull {
		// the blank line and the footer below take two lines of the terminal height
		body = renderLive(m.tracker, m.now, m.spinner.View(), m.width, m.height-2)
	}
	return tea.NewView(body + "\n\n" + m.footer())
}

// finalText is the result which is printed after the live view ended.
func (m *model) finalText() string {
	switch {
	case m.outcome == OutcomeFailed:
		return ""
	case m.tracker.Run == nil:
		return detachedHint(m.runID)
	case m.outcome == OutcomeDetached:
		return renderRun(m.tracker, m.now, "●", m.width, modeLive) + "\n\n" + detachedHint(m.runID)
	case m.mode == ModeFull:
		return renderSummary(m.tracker.Run)
	default:
		return renderRun(m.tracker, m.now, "", m.width, modeFinal) + "\n" + renderSummary(m.tracker.Run)
	}
}

// statusLine shows the running steps and their current progress line for the full output mode.
func (m *model) statusLine() string {
	var running []string
	progress := ""
	Walk(m.tracker.Run, func(v StepVisit) {
		if v.Step.Status == api.StatusRunning && v.Step.Type == api.StepAction {
			running = append(running, v.Step.Name)
			if buffer := m.tracker.Buffer(v.Step); buffer != nil {
				progress = ansi.Strip(buffer.Partial())
			}
		}
	})

	run := m.tracker.Run
	text := " " + m.spinner.View() + " " + ui.Bold.Render(strings.Join(running, ", ")) +
		ui.Muted.Render("  "+ui.Clock(ui.ElapsedTime(run.StartedAt, run.FinishedAt, m.now)))
	if progress != "" {
		text += "  " + ui.Faint.Render(ansi.Truncate(progress, max(10, m.width-lipgloss.Width(text)-4), "…"))
	}
	return text
}

// footer shows the connection state, the terminate question or the key help.
func (m *model) footer() string {
	switch {
	case m.confirming:
		return ui.Warning.Render(" Terminate this run on all servers?") + ui.Muted.Render("  y terminate · n detach · esc continue")
	case m.terminating:
		return ui.Warning.Render(" Terminating…")
	}

	parts := []string{connLabel(m.conn)}
	if m.notice != "" {
		parts = append(parts, m.notice)
	}
	parts = append(parts, ui.Faint.Render("ctrl+c terminate/detach · q detach"))
	return " " + strings.Join(parts, ui.Faint.Render("  ·  "))
}

// connLabel renders how updates arrive.
func connLabel(state string) string {
	switch state {
	case ConnReconnecting:
		return ui.Warning.Render("◌ reconnecting…")
	case ConnPolling:
		return ui.Muted.Render("◌ polling (gateway unavailable)")
	case ConnRetrying:
		return ui.Warning.Render("◌ API unavailable, retrying…")
	default:
		return ui.Success.Render("● live")
	}
}

// terminateNotice describes the result of the terminate request.
func terminateNotice(err error) string {
	if err != nil {
		return ui.Danger.Render("termination failed: " + err.Error())
	}
	return ui.Warning.Render("termination requested")
}

// detachedHint tells how to follow the run again.
func detachedHint(runID string) string {
	return ui.Muted.Render(" Detached, the run continues on the servers. Follow it again with ") + ui.Code.Render("pt run watch "+runID) + "\n"
}

// connectionNotice describes a change of the connection for the line log.
func connectionNotice(change ConnectionChanged) string {
	notice := " " + connLabel(change.State)
	if change.Err != nil {
		notice += ui.Faint.Render("  " + change.Err.Error())
	}
	return notice
}
