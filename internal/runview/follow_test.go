package runview

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

// fakeRunAPI answers with the scripted runs and errors in order, the last answer repeats.
type fakeRunAPI struct {
	mu     sync.Mutex
	calls  []bool
	script []any
}

// answering creates a fake API with the scripted answers (api.Run or error).
func answering(script ...any) *fakeRunAPI {
	return &fakeRunAPI{script: script}
}

func (f *fakeRunAPI) Run(_ context.Context, _ string, withOutput bool) (api.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, withOutput)
	answer := f.script[0]
	if len(f.script) > 1 {
		f.script = f.script[1:]
	}
	if err, ok := answer.(error); ok {
		return api.Run{}, err
	}
	return answer.(api.Run), nil
}

// fastIntervals shortens polling and retrying for the test.
func fastIntervals(t *testing.T) {
	poll, retry, limit := pollInterval, retryDelay, retryLimit
	pollInterval, retryDelay, retryLimit = 10*time.Millisecond, 10*time.Millisecond, time.Second
	t.Cleanup(func() { pollInterval, retryDelay, retryLimit = poll, retry, limit })
}

// connectionStates records the announced connection states.
func connectionStates(states *[]string) func(any) {
	return func(e any) {
		if c, ok := e.(ConnectionChanged); ok {
			*states = append(*states, c.State)
		}
	}
}

// unreachableGateway fails every connect.
func unreachableGateway(context.Context) (*gateway.Client, error) {
	return nil, context.DeadlineExceeded
}

// runGateway replays output on subscribe, then streams a chunk and announces a change.
func runGateway(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ctx := r.Context()
	send := func(v any) {
		data, _ := json.Marshal(v)
		_ = conn.Write(ctx, websocket.MessageText, data)
	}

	_, _, _ = conn.Read(ctx)
	send(map[string]any{"type": "ready", "sessions": []any{}})
	_, _, _ = conn.Read(ctx)
	send(map[string]any{"type": "run.output", "topic": "run.r1", "stream": "s2", "data": "Cloning\n", "replay": true})
	send(map[string]any{"type": "subscribed", "topic": "run.r1"})
	time.Sleep(50 * time.Millisecond)
	send(map[string]any{"type": "run.output", "topic": "run.r1", "stream": "s2", "data": "done\n"})
	send(map[string]any{"type": "changed", "topic": "run.r1", "entity": "run", "id": "r1"})
	_, _, _ = conn.Read(ctx)
}

func TestFollowerReconcilesReplayOutputAndChanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(runGateway))
	defer server.Close()

	runAPI := answering(sampleRun("", false), sampleRun(api.StatusSuccessful, true))
	var events []any
	follower := &Follower{
		API:   runAPI,
		RunID: "r1",
		Emit:  func(e any) { events = append(events, e) },
		Connect: func(ctx context.Context) (*gateway.Client, error) {
			return gateway.Connect(ctx, func(context.Context) (string, string, error) { return server.URL, "t", nil })
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	follower.Follow(ctx)

	tracker := NewTracker()
	for _, e := range events {
		if err := apply(tracker, e); err != nil {
			t.Fatal(err)
		}
	}

	if tracker.Run == nil || !tracker.Run.Finished() {
		t.Fatalf("run not finished, events = %#v", events)
	}
	if lines := tracker.Lines(&api.Step{StreamID: ptr("s2")}); len(lines) != 2 || lines[1] != "done" {
		t.Errorf("lines = %q", lines)
	}
	if len(runAPI.calls) != 2 || !runAPI.calls[0] || runAPI.calls[1] {
		t.Errorf("fetches with output = %v, want [true false]", runAPI.calls)
	}
}

func TestFollowerPollsWithoutGateway(t *testing.T) {
	fastIntervals(t)
	runAPI := answering(sampleRun("", false), sampleRun(api.StatusSuccessful, true))
	var states []string
	follower := &Follower{API: runAPI, RunID: "r1", Emit: connectionStates(&states), Connect: unreachableGateway}

	follower.Follow(context.Background())

	if len(states) != 1 || states[0] != ConnPolling || len(runAPI.calls) != 2 {
		t.Errorf("states = %v, calls = %v", states, runAPI.calls)
	}
}

func TestFinishedRunNeedsNoGateway(t *testing.T) {
	runAPI := answering(sampleRun(api.StatusSuccessful, true))
	connected := false
	follower := &Follower{API: runAPI, RunID: "r1", Emit: func(any) {}, Connect: func(context.Context) (*gateway.Client, error) {
		connected = true
		return nil, context.DeadlineExceeded
	}}

	follower.Follow(context.Background())

	if connected || len(runAPI.calls) != 1 {
		t.Errorf("connected = %v, calls = %v", connected, runAPI.calls)
	}
}

func TestFollowerRetriesTemporaryAPIErrors(t *testing.T) {
	fastIntervals(t)
	unreachable := &api.ConnectionError{URL: "http://x", Err: context.DeadlineExceeded}
	runAPI := answering(sampleRun("", false), &api.Error{Status: 502}, unreachable, &api.Error{Status: 429}, sampleRun(api.StatusSuccessful, true))
	var states []string
	follower := &Follower{API: runAPI, RunID: "r1", Emit: connectionStates(&states), Connect: unreachableGateway}

	follower.Follow(context.Background())

	if len(runAPI.calls) != 5 || len(states) != 3 || states[1] != ConnRetrying || states[2] != ConnPolling {
		t.Errorf("states = %v, calls = %d", states, len(runAPI.calls))
	}
}

func TestFollowerFailsWhenTheRunCannotBeFetchedAtAll(t *testing.T) {
	unreachable := &api.ConnectionError{URL: "http://127.0.0.1:9", Err: context.DeadlineExceeded}
	var failed error
	follower := &Follower{API: answering(unreachable), RunID: "r1", Connect: unreachableGateway, Emit: func(e any) {
		if f, ok := e.(FollowFailed); ok {
			failed = f.Err
		}
	}}

	follower.Follow(context.Background())

	if !errors.Is(failed, unreachable) {
		t.Errorf("failed = %v, following must end with the error of the first fetch", failed)
	}
}

func TestFollowerGivesUpAfterTheRetryLimit(t *testing.T) {
	fastIntervals(t)
	retryLimit = 50 * time.Millisecond
	var failed error
	follower := &Follower{API: answering(sampleRun("", false), &api.Error{Status: 503}), RunID: "r1", Connect: unreachableGateway, Emit: func(e any) {
		if f, ok := e.(FollowFailed); ok {
			failed = f.Err
		}
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	follower.Follow(ctx)

	if ctx.Err() != nil || !api.IsTransient(failed) {
		t.Errorf("failed = %v, following must give up after the retry limit", failed)
	}
}

func TestFollowerGivesUpOnPermanentAPIErrors(t *testing.T) {
	runAPI := answering(sampleRun("", false), &api.Error{Status: 404})
	var failed error
	follower := &Follower{API: runAPI, RunID: "r1", Connect: unreachableGateway, Emit: func(e any) {
		if f, ok := e.(FollowFailed); ok {
			failed = f.Err
		}
	}}
	fastIntervals(t)

	follower.Follow(context.Background())

	if !api.IsNotFound(failed) || len(runAPI.calls) != 2 {
		t.Errorf("failed = %v, calls = %d", failed, len(runAPI.calls))
	}
}

func TestFollowerPollsWhileTheGatewayIsDown(t *testing.T) {
	fastIntervals(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_, _, _ = conn.Read(r.Context())
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"ready","sessions":[]}`))
		_, _, _ = conn.Read(r.Context())
		_ = conn.CloseNow() // the gateway dies after the subscribe
	}))
	defer server.Close()

	runAPI := answering(sampleRun("", false), sampleRun("", false), sampleRun(api.StatusSuccessful, true))
	var states []string
	url := "ws" + server.URL[len("http"):]
	follower := &Follower{API: runAPI, RunID: "r1", Emit: connectionStates(&states), Connect: func(ctx context.Context) (*gateway.Client, error) {
		return gateway.Connect(ctx, func(context.Context) (string, string, error) { return url, "t", nil })
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	follower.Follow(ctx)

	if ctx.Err() != nil || len(states) == 0 || states[len(states)-1] != ConnReconnecting {
		t.Errorf("run not followed to the end while the gateway was down: states = %v, calls = %d", states, len(runAPI.calls))
	}
}
