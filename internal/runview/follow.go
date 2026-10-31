package runview

import (
	"context"
	"time"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/debounce"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

const (
	refetchDelay   = 150 * time.Millisecond
	refetchMaxWait = time.Second
)

// The intervals of polling and of retrying temporary API errors, and how long temporary errors
// are retried before following gives up (variables for the tests).
var (
	pollInterval = 2 * time.Second
	retryDelay   = 3 * time.Second
	retryLimit   = 2 * time.Minute
)

// Connection states of a follower.
const (
	ConnLive         = "live"
	ConnReconnecting = "reconnecting"
	ConnPolling      = "polling"
	ConnRetrying     = "retrying"
)

// RunUpdated delivers a fresh REST view of the run.
type RunUpdated struct{ Run api.Run }

// OutputReceived delivers live output of a stream at its offset in the whole output.
type OutputReceived struct {
	Stream string
	Data   string
	Offset int
}

// ConnectionChanged tells how the follower receives updates.
type ConnectionChanged struct {
	State string
	Err   error
}

// FollowFailed tells that the run could not be followed anymore.
type FollowFailed struct{ Err error }

// RunAPI is the part of the API client the follower needs.
type RunAPI interface {
	Run(ctx context.Context, id string, withOutput bool) (api.Run, error)
}

// Follower keeps a run up to date: it subscribes to the run on the gateway, refetches
// the run whenever it changed and forwards live output. Without a gateway it polls.
type Follower struct {
	API     RunAPI
	Connect gateway.Connector
	RunID   string
	Emit    func(any)

	streamsWithOutput map[string]bool
	outputFetched     map[string]bool
	mode              string
	retrying          bool
	fetched           bool
	failingSince      time.Time
}

// Follow runs until the run finished, the context ended or following failed for good.
// It fetches the run right away, then follows it over the gateway. While the gateway is down it
// polls the API, and temporary API errors (unreachable, 5xx, 429) are retried instead of giving up.
func (f *Follower) Follow(ctx context.Context) {
	f.streamsWithOutput, f.outputFetched = map[string]bool{}, map[string]bool{}
	f.mode, f.retrying, f.fetched, f.failingSince = ConnLive, false, false, time.Time{}

	l := &loop{Follower: f, topic: gateway.RunTopic(f.RunID), refetch: debounce.New(refetchDelay, refetchMaxWait), fallback: time.NewTicker(pollInterval)}
	defer l.fallback.Stop()

	if l.update(ctx, true) {
		return
	}

	client, err := f.Connect(ctx)
	if err != nil {
		l.setMode(ConnPolling, err)
		l.poll(ctx)
		return
	}
	defer client.Close()

	if err := client.Subscribe(l.topic); err != nil {
		l.setMode(ConnPolling, err)
		l.poll(ctx)
		return
	}
	l.fallback.Stop()
	l.listen(ctx, client)
}

// loop is the state of following one run.
type loop struct {
	*Follower
	topic    string
	refetch  *debounce.Debounce
	fallback *time.Ticker
}

// listen handles gateway messages until the run finished, polling while the gateway is down.
func (l *loop) listen(ctx context.Context, client *gateway.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.refetch.C():
			l.refetch.Fired()
			if l.update(ctx, false) {
				return
			}
		case <-l.fallback.C:
			if l.update(ctx, true) {
				return
			}
		case message, ok := <-client.Messages():
			if !ok || l.handle(ctx, message) {
				return
			}
		}
	}
}

// handle reacts to one gateway message, it returns true when following is over.
func (l *loop) handle(ctx context.Context, message gateway.Message) bool {
	switch {
	case message.Type == gateway.TypeReady:
		l.fallback.Stop()
		l.setMode(ConnLive, nil)
	case message.Type == gateway.TypeDisconnected:
		l.fallback.Reset(pollInterval)
		l.setMode(ConnReconnecting, nil)
	case message.Type == gateway.TypeFailed:
		l.setMode(ConnPolling, nil)
		l.poll(ctx)
		return true
	case message.Type == gateway.TypeError && message.SessionID() == "":
		// e.g. a rejected subscription: the run is polled, so following never depends on it
		l.fallback.Reset(pollInterval)
	case message.Topic != l.topic:
	case message.Type == gateway.TypeSubscribed, message.Type == gateway.TypeChanged:
		l.refetch.Trigger()
	case message.Type == gateway.TypeRunOutput:
		l.streamsWithOutput[message.Stream] = true
		l.Emit(OutputReceived{Stream: message.Stream, Data: message.Data, Offset: message.OutputOffset()})
	}
	return false
}

// poll refetches the run with its output in intervals until it finished.
func (l *loop) poll(ctx context.Context) {
	l.fallback.Reset(pollInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.fallback.C:
			if l.update(ctx, true) {
				return
			}
		}
	}
}

// update refreshes the run, it returns true when following is over. Once the run was fetched,
// temporary errors are retried for a while (polling retries by itself); any other error, an error
// on the first fetch, or errors for longer than the retry limit end following.
func (l *loop) update(ctx context.Context, withOutput bool) bool {
	finished, err := l.refresh(ctx, withOutput)
	switch {
	case err == nil:
		l.fetched, l.failingSince = true, time.Time{}
		l.setRetrying(nil)
		return finished
	case ctx.Err() != nil:
		return true
	case l.retryable(err):
		l.setRetrying(err)
		l.refetch.After(retryDelay)
		return false
	default:
		l.Emit(FollowFailed{Err: err})
		return true
	}
}

// retryable tells whether a failed refresh is worth retrying.
func (l *loop) retryable(err error) bool {
	if !l.fetched || !api.IsTransient(err) {
		return false
	}
	if l.failingSince.IsZero() {
		l.failingSince = time.Now()
	}
	return time.Since(l.failingSince) < retryLimit
}

// setMode announces how updates arrive now.
func (l *loop) setMode(mode string, err error) {
	l.mode = mode
	if !l.retrying {
		l.Emit(ConnectionChanged{State: mode, Err: err})
	}
}

// setRetrying announces a temporary API error, or the recovery from it.
func (l *loop) setRetrying(err error) {
	switch {
	case err != nil && !l.retrying:
		l.retrying = true
		l.Emit(ConnectionChanged{State: ConnRetrying, Err: err})
	case err == nil && l.retrying:
		l.retrying = false
		l.Emit(ConnectionChanged{State: l.mode})
	}
}

// refresh fetches the run and emits it, it tells whether the run finished.
func (f *Follower) refresh(ctx context.Context, withOutput bool) (bool, error) {
	run, err := f.API.Run(ctx, f.RunID, withOutput)
	if err == nil && !withOutput && f.missesOutput(&run) {
		run, err = f.API.Run(ctx, f.RunID, true)
	}
	if err != nil {
		return false, err
	}

	f.markOutputFetched(&run, withOutput)
	f.Emit(RunUpdated{Run: run})
	return run.Finished(), nil
}

// missesOutput tells whether a finished step produced no live output, its output must be fetched.
func (f *Follower) missesOutput(run *api.Run) bool {
	missing := false
	Walk(run, func(v StepVisit) {
		id := v.Step.Stream()
		if id != "" && v.Step.FinishedAt != nil && !f.streamsWithOutput[id] && !f.outputFetched[id] {
			missing = true
		}
	})
	return missing
}

// markOutputFetched remembers the finished streams whose output is known now.
func (f *Follower) markOutputFetched(run *api.Run, withOutput bool) {
	Walk(run, func(v StepVisit) {
		if id := v.Step.Stream(); id != "" && v.Step.FinishedAt != nil && (withOutput || v.Step.Details != nil) {
			f.outputFetched[id] = true
		}
	})
}
