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
	name := "Run"
	if run.Executable != nil {
		name = run.Executable.Type + " " + run.Executable.Name
	}
	by := ""
	if run.TriggeredBy != nil {
		by = run.TriggeredBy.Name
	}
	return []string{
		StatusIcon(run.Status),
		name,
		ui.Faint.Render("#" + run.ID),
		ui.Muted.Render(by),
		ui.Muted.Render(ui.Ago(ui.FirstTime(run.StartedAt, run.CreatedAt), now)),
		ui.Muted.Render(ui.Elapsed(run.StartedAt, run.FinishedAt, now)),
	}
}
