package runview

import (
	"strconv"
	"strings"
	"time"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// Log turns the changing state of a run into log lines: each step's start, its
// output and its end, every line exactly once. Used for redirected output and
// for the full output mode.
type Log struct {
	tracker   *Tracker
	started   map[string]bool
	finished  map[string]bool
	announced bool
}

// NewLog creates a log of the tracked run.
func NewLog(t *Tracker) *Log {
	return &Log{tracker: t, started: map[string]bool{}, finished: map[string]bool{}}
}

// Lines returns the lines which are new since the last call.
func (l *Log) Lines() []string {
	run := l.tracker.Run
	if run == nil {
		return nil
	}

	var lines []string
	if !l.announced {
		l.announced = true
		lines = append(lines, l.headline(run))
	}

	multiple := len(run.Executions) > 1
	add := func(v StepVisit, stepLines []string) {
		for _, line := range stepLines {
			lines = append(lines, l.prefix(v, multiple)+line)
		}
	}
	Traverse(run,
		func(v StepVisit) { add(v, l.startLines(v)) },
		func(v StepVisit) { add(v, l.endLines(v)) },
	)
	for i := range run.Executions {
		lines = append(lines, l.executionEnd(&run.Executions[i])...)
	}

	return lines
}

// headline names the run.
func (l *Log) headline(run *api.Run) string {
	name := "Run #" + run.ID
	if run.Executable != nil {
		name = run.Executable.Type + " " + run.Executable.Name + " · run #" + run.ID
	}
	return ui.Title.Render("▶ "+name) + ui.Muted.Render(" · "+ui.Plural(len(run.Executions), "server"))
}

// startLines returns the start line and the new output of a step.
func (l *Log) startLines(v StepVisit) []string {
	key, indent := v.Key(), strings.Repeat("  ", v.Depth)
	var lines []string

	if v.Step.StartedAt != nil && !l.started[key] {
		l.started[key] = true
		if !l.endsSilently(v.Step) {
			lines = append(lines, indent+ui.Accent.Render("▶ ")+v.Step.Name)
		}
	}
	if v.Step.Type == api.StepAction && l.started[key] && !l.finished[key] {
		lines = append(lines, l.output(v, indent)...)
	}

	return lines
}

// endLines returns the end line of a finished step, after the lines of its children.
func (l *Log) endLines(v StepVisit) []string {
	key := v.Key()
	if v.Step.FinishedAt == nil || l.finished[key] {
		return nil
	}
	l.finished[key] = true
	if v.Parent != nil && v.Parent.StartedAt == nil {
		return nil
	}
	return []string{strings.Repeat("  ", v.Depth) + l.end(v.Step)}
}

// endsSilently tells whether an action step was already over without output when first seen
// (like a skipped step), then its end line says everything.
func (l *Log) endsSilently(step *api.Step) bool {
	return step.Type == api.StepAction && step.FinishedAt != nil && len(l.tracker.Lines(step)) == 0
}

// output returns the new output lines of an action step.
func (l *Log) output(v StepVisit, indent string) []string {
	finished := v.Step.FinishedAt != nil
	var raw []string

	if buffer := l.tracker.Buffer(v.Step); buffer != nil {
		raw = buffer.Flush()
		if finished {
			raw = append(raw, buffer.FlushAll()...)
		}
	} else if finished {
		raw = l.tracker.Lines(v.Step)
	}

	lines := make([]string, len(raw))
	for i, line := range raw {
		lines[i] = indent + gutter + line + "\x1b[0m"
	}
	return lines
}

// end renders the end of a step: status, duration and notes.
func (l *Log) end(step *api.Step) string {
	look := present.Status(step.Status)
	line := ui.Fg(look.Color).Render(look.Icon+" "+step.Name) + ui.Faint.Render("  "+strings.ToLower(look.Label))

	if step.StartedAt != nil && step.Status != api.StatusSkipped {
		line += ui.Faint.Render(" · " + ui.Duration(ui.ElapsedTime(step.StartedAt, step.FinishedAt, time.Now())))
	}
	if step.SkippedByConditions {
		line += ui.Faint.Render(" · conditions not met")
	} else if step.Reason != nil && *step.Reason != "" {
		line += ui.Faint.Render(" · " + *step.Reason)
	}
	if step.Details != nil && step.Details.ExitCode != nil && *step.Details.ExitCode != 0 {
		line += ui.Danger.Render(" · exit " + strconv.Itoa(*step.Details.ExitCode))
	}

	return line
}

// executionEnd announces a finished execution once.
func (l *Log) executionEnd(e *api.Execution) []string {
	key := e.ID + "/end"
	if e.FinishedAt == nil || l.finished[key] {
		return nil
	}
	l.finished[key] = true

	look := present.Status(e.Status)
	return []string{ui.Fg(look.Color).Bold(true).Render(look.Icon+" "+e.Server.Name) +
		ui.Muted.Render("  "+strings.ToLower(look.Label)+" in "+ui.Duration(ui.ElapsedTime(e.StartedAt, e.FinishedAt, time.Now())))}
}

// prefix marks the lines of a server when several servers run at once.
func (l *Log) prefix(v StepVisit, multiple bool) string {
	if !multiple {
		return ""
	}
	return ui.Muted.Render("[" + v.Execution.Server.Name + "] ")
}
