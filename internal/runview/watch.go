package runview

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

// Options configures how a run is followed.
type Options struct {
	Mode  Mode
	Quiet bool
	// Live renders the interactive view, otherwise a line log is written.
	Live    bool
	Out     io.Writer
	Profile colorprofile.Profile
	Width   int
	// Notices receives connection problems in the line log (stderr), even when quiet.
	Notices io.Writer
}

// Deps are the services needed to follow a run.
type Deps struct {
	API     *api.Client
	Connect gateway.Connector
}

// Result is the end state of following a run.
type Result struct {
	Run     *api.Run
	Outcome Outcome
}

// ExitCode maps the result to the exit code of the CLI.
func (r Result) ExitCode() int {
	switch {
	case r.Outcome == OutcomeDetached:
		return fault.ExitInterrupted
	case r.Run == nil:
		return fault.ExitFailure
	}
	switch r.Run.Status {
	case api.StatusSuccessful, api.StatusSkipped:
		return fault.ExitOK
	case api.StatusTerminated:
		return fault.ExitInterrupted
	default:
		return fault.ExitFailure
	}
}

// Watch follows the run until it finished, the user detached or following failed.
func Watch(ctx context.Context, deps Deps, runID string, opts Options) (Result, error) {
	if opts.Live && !opts.Quiet {
		return watchLive(ctx, deps, runID, opts)
	}
	return watchLog(ctx, deps, runID, opts)
}

// watchLive runs the interactive Bubble Tea view.
func watchLive(ctx context.Context, deps Deps, runID string, opts Options) (Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tracker := NewTracker()
	terminate := func(ctx context.Context) error {
		_, err := deps.API.TerminateRun(ctx, runID)
		return err
	}
	m := newModel(runID, tracker, opts.Mode, opts.Width, terminate)
	program := tea.NewProgram(m, tea.WithContext(ctx), tea.WithOutput(opts.Out), tea.WithColorProfile(opts.Profile))

	follower := &Follower{API: deps.API, Connect: deps.Connect, RunID: runID, Emit: func(e any) { program.Send(e) }}
	go follower.Follow(ctx)

	_, err := program.Run()
	switch {
	case errors.Is(err, tea.ErrProgramPanic):
		return Result{Run: tracker.Run, Outcome: OutcomeFailed}, err
	case errors.Is(err, tea.ErrInterrupted) || errors.Is(err, tea.ErrProgramKilled):
		// a signal (SIGINT, SIGTERM) leaves the run running, like detaching
		m.outcome, err = OutcomeDetached, nil
	}
	if err != nil && m.outcome == OutcomeNone {
		return Result{}, err
	}
	_, _ = fmt.Fprint(&colorprofile.Writer{Forward: opts.Out, Profile: opts.Profile}, m.finalText())
	if m.outcome == OutcomeFailed {
		return Result{Run: tracker.Run, Outcome: m.outcome}, m.err
	}
	return Result{Run: tracker.Run, Outcome: m.outcome}, nil
}

// watchLog writes the run as a line log, ending the context (Ctrl+C) detaches.
func watchLog(ctx context.Context, deps Deps, runID string, opts Options) (Result, error) {
	events := make(chan any, 256)
	follower := &Follower{API: deps.API, Connect: deps.Connect, RunID: runID, Emit: func(e any) { events <- e }}
	go func() {
		follower.Follow(ctx)
		close(events)
	}()

	tracker := NewTracker()
	log := NewLog(tracker)
	degraded := false
	for event := range events {
		if err := apply(tracker, event); err != nil {
			return Result{Run: tracker.Run, Outcome: OutcomeFailed}, err
		}
		// connection problems and the recovery from them are announced, a healthy start is not
		if change, ok := event.(ConnectionChanged); ok && opts.Notices != nil && (degraded || change.State != ConnLive) {
			_, _ = fmt.Fprintln(opts.Notices, connectionNotice(change))
			degraded = change.State != ConnLive
		}
		if !opts.Quiet {
			for _, line := range log.Lines() {
				_, _ = fmt.Fprintln(opts.Out, line)
			}
		}
	}

	return logResult(ctx, tracker, opts)
}

// apply updates the tracker with a follower event.
func apply(t *Tracker, event any) error {
	switch e := event.(type) {
	case RunUpdated:
		t.SetRun(e.Run)
	case OutputReceived:
		t.Output(e.Stream, e.Data, e.Offset)
	case FollowFailed:
		return e.Err
	}
	return nil
}

// logResult prints the summary or the detach hint of the line log.
func logResult(ctx context.Context, t *Tracker, opts Options) (Result, error) {
	if t.Run == nil || !t.Run.Finished() {
		if ctx.Err() != nil && t.Run != nil {
			_, _ = fmt.Fprintln(opts.Out, detachedHint(t.Run.ID))
			return Result{Run: t.Run, Outcome: OutcomeDetached}, nil
		}
		return Result{Run: t.Run, Outcome: OutcomeFailed}, ctx.Err()
	}

	_, _ = fmt.Fprint(opts.Out, renderSummary(t.Run))
	return Result{Run: t.Run, Outcome: OutcomeFinished}, nil
}
