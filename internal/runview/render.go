package runview

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

const (
	maxWidth       = 110
	runningTail    = 8
	failedTail     = 10
	pendingPreview = 3
	indentWidth    = 2
)

var gutter = ui.Faint.Render("│ ")

// mode decides how much of a run is rendered.
type mode int

const (
	modeLive     mode = iota // a running run: its steps and the live output
	modeFinal                // the last frame of a followed run: only what went wrong
	modeDetailed             // a finished run shown on request: all steps
	modeCompact              // a running run too tall for the terminal: one line per server
)

// view renders the state of a run at one moment.
type view struct {
	tracker *Tracker
	now     time.Time
	spinner string
	width   int
	mode    mode
	lines   []string
}

// renderRun renders the header and all executions of the run.
func renderRun(t *Tracker, now time.Time, spinner string, width int, mode mode) string {
	v := &view{tracker: t, now: now, spinner: spinner, width: min(width, maxWidth), mode: mode}
	v.header()
	for _, e := range v.executions() {
		v.execution(e)
	}
	return strings.Join(v.lines, "\n")
}

// renderLive renders a running run so that it fits the terminal height: the full tree if possible,
// otherwise one line per server (running and failing ones first), cut off at the height.
func renderLive(t *Tracker, now time.Time, spinner string, width, height int) string {
	tree := renderRun(t, now, spinner, width, modeLive)
	if height <= 0 || strings.Count(tree, "\n") < height {
		return tree
	}

	lines := strings.Split(renderRun(t, now, spinner, width, modeCompact), "\n")
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	hidden := len(lines) - height + 1
	return strings.Join(lines[:height-1], "\n") + "\n" + ui.Faint.Render(" … "+ui.Plural(hidden, "more server"))
}

// executions are the executions in the order of the view: as they are, or in the compact view
// the running ones first, then problems, pending and finished ones.
func (v *view) executions() []*api.Execution {
	executions := make([]*api.Execution, len(v.tracker.Run.Executions))
	for i := range v.tracker.Run.Executions {
		executions[i] = &v.tracker.Run.Executions[i]
	}
	if v.mode == modeCompact {
		slices.SortStableFunc(executions, func(a, b *api.Execution) int { return compactRank(a.Status) - compactRank(b.Status) })
	}
	return executions
}

// compactRank orders statuses by how much attention they need.
func compactRank(status string) int {
	switch {
	case status == api.StatusRunning:
		return 0
	case isProblem(status):
		return 1
	case status == api.StatusPending:
		return 2
	default:
		return 3
	}
}

// header renders the executable, the run id and the run clock.
func (v *view) header() {
	run := v.tracker.Run
	kind, name := "Run", "#"+run.ID
	if run.Executable != nil {
		kind, name = run.Executable.Type, run.Executable.Name
	}

	left := ui.Title.Render("▶ "+name) + "  " + ui.Muted.Render(fmt.Sprintf("%s · #%s · %s", kind, run.ID, ui.Plural(len(run.Executions), "server")))
	right := ""
	if run.StartedAt != nil {
		right = ui.Muted.Render(ui.Clock(ui.ElapsedTime(run.StartedAt, run.FinishedAt, v.now)))
	}
	v.row(0, left, right)
	v.lines = append(v.lines, "")
}

// execution renders a server line and, while relevant, its steps.
func (v *view) execution(e *api.Execution) {
	left := v.icon(e.Status) + " " + ui.Bold.Render(e.Server.Name) + "  " + ui.Faint.Render(e.Server.Host)
	if v.mode == modeCompact {
		left += ui.Muted.Render(currentStep(e))
	}
	v.row(0, left, v.statusTime(e.Status, e.StartedAt, e.FinishedAt))

	if v.mode == modeCompact {
		return
	}
	if v.mode != modeFinal || expands(e.Status) {
		v.steps(e.Steps, 1)
	}
}

// currentStep names the running action step of an execution for the compact view.
func currentStep(e *api.Execution) string {
	name := ""
	traverseSteps(e, nil, e.Steps, nil, func(v StepVisit) {
		if v.Step.Type == api.StepAction && v.Step.Status == api.StatusRunning {
			name = "  · " + v.Step.Name
		}
	}, nil)
	return name
}

// steps renders a list of steps, pending ones only as a short preview.
func (v *view) steps(steps []api.Step, depth int) {
	pending := 0
	for i := range steps {
		step := &steps[i]
		if step.Status == api.StatusPending {
			pending++
			if pending > pendingPreview {
				v.row(depth, ui.Faint.Render(fmt.Sprintf("… %d more", len(steps)-i)), "")
				return
			}
		}
		v.step(step, depth)
	}
}

// step renders one step with its children or output where it matters.
func (v *view) step(step *api.Step, depth int) {
	name := step.Name
	if step.Status == api.StatusPending {
		name = ui.Faint.Render(name)
	}
	icon := v.icon(step.Status)
	if step.Type == api.StepAction && !v.spinning(step.Status) {
		icon = present.StepIcon(step.Status)
	}
	v.row(depth, icon+" "+name+v.note(step), v.duration(step))

	if step.Type == api.StepWorkflow && (v.mode == modeDetailed || step.Status == api.StatusRunning || expands(step.Status)) {
		v.steps(step.Steps, depth+1)
	}
	if step.Type == api.StepAction {
		v.output(step, depth+1)
	}
}

// output renders the tail of a running or failed step.
func (v *view) output(step *api.Step, depth int) {
	tail := 0
	switch {
	case step.Status == api.StatusRunning && v.mode == modeLive:
		tail = runningTail
	case step.Status == api.StatusFailed:
		tail = failedTail
	}
	if tail == 0 {
		return
	}

	lines := v.tracker.Lines(step)
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	for _, line := range lines {
		v.outputLine(depth, line)
	}
}

// outputLine renders one output line behind the gutter, cut to the width.
func (v *view) outputLine(depth int, line string) {
	indent := strings.Repeat(" ", 1+depth*indentWidth)
	room := max(10, v.width-lipgloss.Width(indent)-2)
	v.lines = append(v.lines, indent+gutter+ansi.Truncate(line, room, "…")+"\x1b[0m")
}

// note explains skipped, refused and failed steps.
func (v *view) note(step *api.Step) string {
	switch {
	case step.SkippedByConditions:
		return ui.Faint.Render("  conditions not met")
	case step.Status == api.StatusNotAllowed && step.Reason != nil:
		return ui.Warning.Render("  " + *step.Reason)
	case step.Status == api.StatusFailed && step.Details != nil && step.Details.ExitCode != nil:
		return ui.Danger.Render(fmt.Sprintf("  exit %d", *step.Details.ExitCode))
	case step.Status == api.StatusFailed && step.Reason != nil:
		return ui.Danger.Render("  " + *step.Reason)
	}
	return ""
}

// duration renders how long a started step took or runs.
func (v *view) duration(step *api.Step) string {
	if step.StartedAt == nil {
		return ""
	}
	return ui.Faint.Render(ui.Duration(ui.ElapsedTime(step.StartedAt, step.FinishedAt, v.now)))
}

// statusTime renders "running · 0:14" or "successful · 34s".
func (v *view) statusTime(status string, start, end *time.Time) string {
	look := present.Status(status)
	text := strings.ToLower(look.Label)
	if start != nil {
		text += " · " + ui.Duration(ui.ElapsedTime(start, end, v.now))
	}
	return ui.Fg(look.Color).Render(text)
}

// icon renders the status icon, running statuses as the spinner (yellow, like in the web app).
func (v *view) icon(status string) string {
	if v.spinning(status) {
		return ui.Fg(present.Status(status).Color).Render(v.spinner)
	}
	return present.StatusIcon(status)
}

// spinning tells whether the status is shown as the spinner.
func (v *view) spinning(status string) bool {
	return status == api.StatusRunning && (v.mode == modeLive || v.mode == modeCompact)
}

// row renders a line with indentation and a right-aligned part.
func (v *view) row(depth int, left, right string) {
	left = " " + strings.Repeat(" ", depth*indentWidth) + left
	gap := v.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	switch {
	case right == "":
		v.lines = append(v.lines, left)
		return
	case gap < 2:
		v.lines = append(v.lines, left+"  "+right)
		return
	}
	v.lines = append(v.lines, left+strings.Repeat(" ", gap)+right)
}

// expands tells whether the steps below a status are shown in the final view: where something went wrong.
func expands(status string) bool {
	return isProblem(status) && status != api.StatusCanceled
}

// isProblem tells whether a final status needs attention.
func isProblem(status string) bool {
	switch status {
	case api.StatusFailed, api.StatusPartiallyFailed, api.StatusTerminated, api.StatusNotAllowed, api.StatusCanceled:
		return true
	}
	return false
}

// renderSummary renders the outcome of a finished run.
func renderSummary(run *api.Run) string {
	look := present.Status(run.Status)
	succeeded := 0
	for _, e := range run.Executions {
		if !isProblem(e.Status) {
			succeeded++
		}
	}

	line := ui.Fg(look.Color).Bold(true).Render(look.Icon+" "+look.Label) +
		ui.Muted.Render(fmt.Sprintf("  in %s · %d of %s succeeded", ui.Duration(ui.ElapsedTime(run.StartedAt, run.FinishedAt, time.Now())), succeeded, ui.Plural(len(run.Executions), "server")))

	lines := []string{"", " " + line}
	for _, e := range run.Executions {
		if isProblem(e.Status) {
			lines = append(lines, "   "+ui.Muted.Render("Log of "+e.Server.Name+": ")+ui.Code.Render("pt run view "+run.ID+" --log --server "+ui.Arg(e.Server.Name)))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// Render renders a run once: a finished run with all steps and its summary, a running one as it is now.
func Render(run api.Run, width int) string {
	tracker := NewTracker()
	tracker.SetRun(run)

	if run.Finished() {
		return renderRun(tracker, time.Now(), "", width, modeDetailed) + "\n" + renderSummary(tracker.Run)
	}
	return renderRun(tracker, time.Now(), "●", width, modeLive) + "\n"
}

// renderUpcoming renders the pending steps below the status line of the full output, exactly as
// the step tree shows them: indented, at most a preview per list, and with the server when several run.
func renderUpcoming(t *Tracker, now time.Time, spinner string, width int) string {
	v := &view{tracker: t, now: now, spinner: spinner, width: min(width, maxWidth), mode: modeLive}
	executions := v.executions()
	for _, e := range executions {
		start := len(v.lines)
		if len(executions) > 1 {
			v.row(0, v.icon(e.Status)+" "+ui.Bold.Render(e.Server.Name)+"  "+ui.Faint.Render(e.Server.Host), "")
		}
		v.pendingSteps(e.Steps, 1)
		if len(executions) > 1 && len(v.lines) == start+1 {
			v.lines = v.lines[:start] // a server without upcoming steps
		}
	}
	return strings.Join(v.lines, "\n")
}

// pendingSteps renders the pending steps of a list and of its running workflow steps.
func (v *view) pendingSteps(steps []api.Step, depth int) {
	pending := 0
	for i := range steps {
		step := &steps[i]
		switch {
		case step.Status == api.StatusPending:
			pending++
			if pending > pendingPreview {
				v.row(depth, ui.Faint.Render(fmt.Sprintf("… %d more", len(steps)-i)), "")
				return
			}
			v.step(step, depth)
		case step.Type == api.StepWorkflow && step.Status == api.StatusRunning:
			v.pendingSteps(step.Steps, depth+1)
		}
	}
}
