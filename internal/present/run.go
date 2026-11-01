package present

import (
	"time"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// RunHeaders are the columns of run tables.
var RunHeaders = []string{"", "RUN", "", "BY", "WHEN", "DURATION"}

// RunRow renders a run as a table row: status, name, id, trigger, time and duration.
func RunRow(run api.Run, now time.Time) []string {
	return []string{
		StatusIcon(run.Status),
		RunName(run),
		ui.Faint.Render("#" + run.ID),
		ui.Muted.Render(RunBy(run)),
		ui.Muted.Render(ui.Ago(ui.FirstTime(run.StartedAt, run.CreatedAt), now)),
		ui.Muted.Render(ui.Elapsed(run.StartedAt, run.FinishedAt, now)),
	}
}

// RunName names a run by what it runs, e.g. "Workflow Deploy App".
func RunName(run api.Run) string {
	if run.Executable == nil {
		return "Run"
	}
	return run.Executable.Type + " " + run.Executable.Name
}

// RunBy names who started the run, empty for runs without a person (e.g. of pipes).
func RunBy(run api.Run) string {
	if run.TriggeredBy == nil {
		return ""
	}
	return run.TriggeredBy.Name
}
