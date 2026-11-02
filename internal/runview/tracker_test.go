package runview

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
)

func ptr[T any](v T) *T { return &v }

// sampleRun is a workflow run on one server: a finished step and a running one.
func sampleRun(status string, finished bool) api.Run {
	start := time.Now().Add(-10 * time.Second)
	end := start.Add(5 * time.Second)

	clone := api.Step{Type: api.StepAction, Name: "Clone git repo", Status: api.StatusRunning, StreamID: ptr("s2"), StartedAt: &end}
	if finished {
		clone.Status, clone.FinishedAt = status, ptr(end.Add(3*time.Second))
	}
	run := api.Run{
		ID: "r1", Status: api.StatusRunning, StartedAt: &start,
		Executable: &api.Reference{ID: "w1", Type: "Workflow", Name: "Deploy App"},
		Executions: []api.Execution{{
			ID: "e1", Server: api.ServerRef{Name: "prod", Host: "5.75.144.227"}, Status: api.StatusRunning, StartedAt: &start,
			Steps: []api.Step{{
				Type: api.StepWorkflow, Name: "Deploy app", Status: api.StatusRunning, StartedAt: &start,
				Steps: []api.Step{
					{Type: api.StepAction, Name: "Create release dir", Status: api.StatusSuccessful, StreamID: ptr("s1"), StartedAt: &start, FinishedAt: &end,
						Details: &api.StepDetails{Output: "export.release_dir=/var/www\n", ExitCode: ptr(0)}},
					clone,
					{Type: api.StepAction, Name: "Install", Status: api.StatusPending},
				},
			}},
		}},
	}
	if finished {
		run.Status, run.FinishedAt = status, clone.FinishedAt
		run.Executions[0].Status, run.Executions[0].FinishedAt = status, clone.FinishedAt
		run.Executions[0].Steps[0].Status, run.Executions[0].Steps[0].FinishedAt = status, clone.FinishedAt
	}
	return run
}

func TestReplayAppendsOnlyNewOutput(t *testing.T) {
	cases := map[string]struct {
		live   string
		replay string
		offset int
		want   []string
	}{
		"replay of everything":         {"one\ntw", "one\ntwo\nthree\n", 0, []string{"one", "two", "three"}},
		"short repeating output":       {"ok\n", "ok\nbuilding\nok\n", 0, []string{"ok", "building", "ok"}},
		"single character":             {".", ".\nstep 2 done.", 0, []string{".", "step 2 done."}},
		"sliding window of the server": {"line 1\nline 2\n", "line 2\nline 3\n", 7, []string{"line 1", "line 2", "line 3"}},
		"nothing new":                  {"a\nb\n", "b\n", 2, []string{"a", "b"}},
		"emoji counted in UTF-16":      {"✓ 🚀\n", "✓ 🚀\ndone\n", 0, []string{"✓ 🚀", "done"}},
	}
	for name, c := range cases {
		tracker := NewTracker()
		tracker.Output("s1", c.live, 0)
		tracker.Output("s1", c.replay, c.offset)

		if got := tracker.Lines(&api.Step{StreamID: ptr("s1")}); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Lines = %q, want %q", name, got, c.want)
		}
	}
}

func TestEveryLineIsEmittedOnceAcrossReplays(t *testing.T) {
	tracker := NewTracker()
	tracker.Output("s1", "line 1\nline 2\n", 0)
	first := tracker.streams["s1"].buffer.Flush()
	tracker.Output("s1", "line 1\nline 2\nline 3\n", 0)

	if second := tracker.streams["s1"].buffer.Flush(); len(first) != 2 || !reflect.DeepEqual(second, []string{"line 3"}) {
		t.Errorf("first = %q, second = %q", first, second)
	}
}

func TestMissingOutputIsMarkedAsGap(t *testing.T) {
	tracker := NewTracker()
	tracker.Output("s1", "old\n", 0)
	tracker.Output("s1", "new\n", 100)

	want := []string{"old", "", "… output skipped …", "new"}
	if got := tracker.Lines(&api.Step{StreamID: ptr("s1")}); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines = %q, want %q", got, want)
	}
}

func TestSetRunKeepsDetailsOfEarlierFetches(t *testing.T) {
	tracker := NewTracker()
	tracker.SetRun(sampleRun("", false))

	refetched := sampleRun("", false)
	refetched.Executions[0].Steps[0].Steps[0].Details = nil
	tracker.SetRun(refetched)

	step := &tracker.Run.Executions[0].Steps[0].Steps[0]
	if got := tracker.Lines(step); !reflect.DeepEqual(got, []string{"export.release_dir=/var/www"}) {
		t.Errorf("Lines = %q", got)
	}
}

func TestLogWritesEveryLineOnce(t *testing.T) {
	tracker := NewTracker()
	log := NewLog(tracker)

	tracker.SetRun(sampleRun("", false))
	tracker.Output("s2", "Cloning into '.'...\n50%\r", UnknownOffset)
	first := plain(log.Lines())

	tracker.Output("s2", "100% done\n", UnknownOffset)
	tracker.SetRun(sampleRun(api.StatusFailed, true))
	second := plain(log.Lines())

	for _, want := range []string{"▶ Workflow Deploy App", "▶ Deploy app", "│ export.release_dir=/var/www", "✓ Create release dir", "│ Cloning into '.'..."} {
		if !strings.Contains(first, want) {
			t.Errorf("first batch misses %q:\n%s", want, first)
		}
	}
	for _, want := range []string{"│ 100% done", "✗ Clone git repo  failed", "✗ prod  failed"} {
		if !strings.Contains(second, want) {
			t.Errorf("second batch misses %q:\n%s", want, second)
		}
	}
	if strings.Contains(second, "Create release dir") || strings.Contains(second, "50%") {
		t.Errorf("second batch repeats or shows overwritten output:\n%s", second)
	}
	if len(log.Lines()) != 0 {
		t.Error("nothing new should produce no lines")
	}
}

func TestRenderRunShowsTailOfRunningStep(t *testing.T) {
	tracker := NewTracker()
	tracker.SetRun(sampleRun("", false))
	tracker.Output("s2", "Receiving objects: 54% (671/1241)", UnknownOffset)

	out := ansi.Strip(renderRun(tracker, time.Now(), "⠹", 100, modeLive))

	for _, want := range []string{"▶ Deploy App", "Workflow · #r1 · 1 server", "⠹ prod", "✓ Create release dir", "⠹ Clone git repo", "│ Receiving objects: 54% (671/1241)", "◷ Install"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFinalRenderShowsFailureOutput(t *testing.T) {
	tracker := NewTracker()
	run := sampleRun(api.StatusFailed, true)
	run.Executions[0].Steps[0].Steps[1].Details = &api.StepDetails{ExitCode: ptr(128), Output: "fatal: repository not found\n"}
	tracker.SetRun(run)

	out := ansi.Strip(renderRun(tracker, time.Now(), "", 100, modeFinal) + renderSummary(tracker.Run))

	for _, want := range []string{"✗ Clone git repo  exit 128", "│ fatal: repository not found", "✗ Failed  in", "0 of 1 server succeeded", "pt run view r1 --log --server prod"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestResultExitCodes(t *testing.T) {
	cases := map[string]int{api.StatusSuccessful: 0, api.StatusSkipped: 0, api.StatusFailed: 1, api.StatusPartiallyFailed: 1, api.StatusTerminated: 130}
	for status, want := range cases {
		if got := (Result{Run: &api.Run{Status: status}, Outcome: OutcomeFinished}).ExitCode(); got != want {
			t.Errorf("%s: exit %d, want %d", status, got, want)
		}
	}
	if got := (Result{Outcome: OutcomeDetached}).ExitCode(); got != 130 {
		t.Errorf("detached: exit %d", got)
	}
}

func plain(lines []string) string {
	return ansi.Strip(strings.Join(lines, "\n"))
}

func TestStoredHeadPrecedesLiveOutputBeyondTheGatewayWindow(t *testing.T) {
	tracker := NewTracker()
	tracker.SetRun(api.Run{Executions: []api.Execution{{ID: "e1", Steps: []api.Step{{
		Type: api.StepAction, StreamID: ptr("s1"), Status: api.StatusRunning,
		Details: &api.StepDetails{Output: "line 1\nline 2\n"},
	}}}}})

	tracker.Output("s1", "line 9\n", 500)
	tracker.Output("s1", "line 10\n", 507)

	want := []string{"line 1", "line 2", "", "… output skipped …", "line 9", "line 10"}
	if got := tracker.Lines(&api.Step{StreamID: ptr("s1")}); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines = %q, want %q", got, want)
	}
}
