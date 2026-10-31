package runview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/pipetower/pipetower-cli/internal/api"
)

// The golden tests pin the complete rendered views. After an intended change of a view,
// review the diff and update the files with: go test ./internal/runview -update

var goldenNow = time.Date(2026, 10, 31, 12, 0, 30, 0, time.UTC)

// at returns a fixed point in time, seconds after the start of the fixture runs.
func at(seconds int) *time.Time {
	t := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
	return &t
}

// actionStep is a step of an action with its stream.
func actionStep(name, status, stream string, start, end *time.Time) api.Step {
	return api.Step{Type: api.StepAction, Name: name, Status: status, StreamID: &stream, StartedAt: start, FinishedAt: end}
}

// deployRun is a workflow run on two servers: prod is cloning, staging waits.
func deployRun() api.Run {
	return api.Run{
		ID: "30denr", Status: api.StatusRunning, StartedAt: at(0),
		Executable: &api.Reference{ID: "78vr4v", Type: "Workflow", Name: "Deploy App"},
		Executions: []api.Execution{
			{
				ID: "0r76v1", Server: api.ServerRef{Name: "prod", Host: "5.75.144.227"}, Status: api.StatusRunning, StartedAt: at(0),
				Steps: []api.Step{
					{Type: api.StepWorkflow, Name: "Deploy app", Status: api.StatusRunning, StartedAt: at(0), Steps: []api.Step{
						actionStep("Create release dir", api.StatusSuccessful, "s1", at(0), at(1)),
						{Type: api.StepAction, Name: "Clear cache", Status: api.StatusSkipped, SkippedByConditions: true, StartedAt: at(1), FinishedAt: at(1)},
						actionStep("Clone git repo", api.StatusRunning, "s2", at(1), nil),
						{Type: api.StepAction, Name: "Install dependencies", Status: api.StatusPending},
					}},
					{Type: api.StepWorkflow, Name: "Restart PHP", Status: api.StatusPending},
				},
			},
			{
				ID: "0r76v2", Server: api.ServerRef{Name: "staging", Host: "10.0.0.4"}, Status: api.StatusPending,
				Steps: []api.Step{{Type: api.StepWorkflow, Name: "Deploy app", Status: api.StatusPending}},
			},
		},
	}
}

// finishedRun is the deploy run after prod failed and staging was not allowed to run.
func finishedRun() api.Run {
	run := deployRun()
	run.Status, run.FinishedAt = api.StatusFailed, at(9)

	prod := &run.Executions[0]
	prod.Status, prod.FinishedAt = api.StatusFailed, at(9)
	deploy := &prod.Steps[0]
	deploy.Status, deploy.FinishedAt = api.StatusFailed, at(9)
	deploy.Steps[2].Status, deploy.Steps[2].FinishedAt = api.StatusFailed, at(9)
	deploy.Steps[2].Details = &api.StepDetails{ExitCode: ptr(128), Output: "Cloning into '.'...\nfatal: repository not found\n"}
	deploy.Steps[3].Status, deploy.Steps[3].FinishedAt = api.StatusCanceled, at(9)
	prod.Steps[1].Status, prod.Steps[1].FinishedAt = api.StatusCanceled, at(9)

	staging := &run.Executions[1]
	staging.Status, staging.StartedAt, staging.FinishedAt = api.StatusNotAllowed, at(0), at(0)
	staging.Steps[0].Status, staging.Steps[0].Reason = api.StatusNotAllowed, ptr("Not allowed to run on this server")
	staging.Steps[0].StartedAt, staging.Steps[0].FinishedAt = at(0), at(0)

	return run
}

// trackedDeploy tracks the running deploy run with live output of the clone step.
func trackedDeploy() *Tracker {
	tracker := NewTracker()
	tracker.SetRun(deployRun())
	tracker.Output("s1", "export.release_dir=/var/www/releases/20261031\n", UnknownOffset)
	tracker.Output("s2", "Cloning into '.'...\nReceiving objects:  12% (149/1241)\rReceiving objects:  54% (671/1241)", UnknownOffset)
	return tracker
}

func TestGoldenLiveTree(t *testing.T) {
	golden.RequireEqual(t, ansi.Strip(renderRun(trackedDeploy(), goldenNow, "⠹", 100, modeLive)))
}

func TestGoldenFinalFrameShowsOnlyProblems(t *testing.T) {
	run := finishedRun()
	staging := &run.Executions[1]
	staging.Status, staging.Steps[0].Status, staging.Steps[0].Reason = api.StatusSuccessful, api.StatusSuccessful, nil
	tracker := NewTracker()
	tracker.SetRun(run)

	golden.RequireEqual(t, ansi.Strip(renderRun(tracker, goldenNow, "", 100, modeFinal)+"\n"+renderSummary(tracker.Run)))
}

func TestGoldenDetailedFinishedRun(t *testing.T) {
	golden.RequireEqual(t, ansi.Strip(Render(finishedRun(), 100)))
}

func TestGoldenLineLog(t *testing.T) {
	tracker := trackedDeploy()
	log := NewLog(tracker)
	first := log.Lines()

	tracker.Output("s2", "\nfatal: repository not found\n", UnknownOffset)
	tracker.SetRun(finishedRun())
	second := log.Lines()

	golden.RequireEqual(t, ansi.Strip(strings.Join(first, "\n")+"\n--- after the run finished ---\n"+strings.Join(second, "\n")+"\n"))
}

// fleetRun is a running action on twelve servers: some finished, two running, one failed, the rest pending.
func fleetRun() api.Run {
	run := api.Run{ID: "4k1x9z", Status: api.StatusRunning, StartedAt: at(0), Executable: &api.Reference{Type: "Action", Name: "Restart PHP"}}
	for i := range 12 {
		e := api.Execution{ID: fmt.Sprintf("e%d", i), Server: api.ServerRef{Name: fmt.Sprintf("web-%02d", i+1), Host: fmt.Sprintf("10.0.0.%d", i+1)}, Status: api.StatusPending}
		step := api.Step{Type: api.StepAction, Name: "Restart", Status: api.StatusPending}
		switch {
		case i < 4:
			e.Status, e.StartedAt, e.FinishedAt = api.StatusSuccessful, at(0), at(2)
			step.Status = api.StatusSuccessful
		case i == 6 || i == 9:
			e.Status, e.StartedAt = api.StatusRunning, at(3)
			step.Status, step.StartedAt = api.StatusRunning, at(3)
		case i == 8:
			e.Status, e.StartedAt, e.FinishedAt = api.StatusFailed, at(0), at(4)
			step.Status = api.StatusFailed
		}
		e.Steps = []api.Step{step}
		run.Executions = append(run.Executions, e)
	}
	return run
}

func TestGoldenCompactLiveViewFitsTheTerminal(t *testing.T) {
	tracker := NewTracker()
	tracker.SetRun(fleetRun())

	golden.RequireEqual(t, ansi.Strip(renderLive(tracker, goldenNow, "⠹", 100, 10)))
}

func TestLiveViewKeepsTheTreeWhenItFits(t *testing.T) {
	tracker := trackedDeploy()

	if live, tree := renderLive(tracker, goldenNow, "⠹", 100, 40), renderRun(tracker, goldenNow, "⠹", 100, modeLive); live != tree {
		t.Errorf("the full tree must be shown when it fits:\n%s", ansi.Strip(live))
	}
}
