package terminal

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

func TestNoSessionIsOpenedWithoutGateway(t *testing.T) {
	opened := false
	unreachable := errors.New("gateway unreachable")

	_, err := Attach(context.Background(), Options{
		Connect: func(context.Context) (*gateway.Client, error) { return nil, unreachable },
		Open: func(context.Context) (api.Session, error) {
			opened = true
			return api.Session{}, nil
		},
	})

	if !errors.Is(err, unreachable) || opened {
		t.Errorf("err = %v, opened = %v: the session must only be opened once the gateway is connected", err, opened)
	}
}

// reattaching is a client whose reattach answers with the error.
func reattaching(err error) *client {
	session := api.Session{ID: "s1"}
	return &client{
		session:  session,
		presence: newPresence(session),
		reattach: stoppedTimer(),
		opts: Options{Out: devNull(), Reattach: func(context.Context, string) (api.Session, error) {
			return api.Session{}, err
		}},
	}
}

func TestSessionWhichEndedMeanwhileIsReportedAsGone(t *testing.T) {
	for _, status := range []int{403, 404} {
		result, done, err := reattaching(&api.Error{Status: status}).tryReattach(context.Background())

		if !done || err != nil || result.Reason != "gone" {
			t.Errorf("%d: result = %+v, done = %v, err = %v", status, result, done, err)
		}
	}
}

func TestTemporaryReattachErrorsAreTriedAgain(t *testing.T) {
	c := reattaching(&api.Error{Status: 503})

	for attempt := 1; attempt <= 2; attempt++ {
		if _, done, err := c.tryReattach(context.Background()); done || err != nil {
			t.Fatalf("attempt %d: done = %v, err = %v, want a later retry", attempt, done, err)
		}
	}
	if c.attempts != 2 || !c.reattach.Stop() {
		t.Errorf("attempts = %d, a retry must be scheduled", c.attempts)
	}
}

func TestOtherReattachErrorsEndTheSession(t *testing.T) {
	_, done, err := reattaching(&api.Error{Status: 401}).tryReattach(context.Background())

	if !done || err == nil {
		t.Errorf("done = %v, err = %v", done, err)
	}
}

// devNull is a terminal output which discards everything.
func devNull() *os.File {
	file, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	return file
}

func TestReadyMessagesIdentifyTheUser(t *testing.T) {
	c := reattaching(nil)
	c.attachSent = true

	_, done, err := c.receive(context.Background(), gateway.Message{Type: gateway.TypeReady, User: &gateway.User{ID: "u1"}})

	if done || err != nil || c.me.ID != "u1" || !c.connected {
		t.Errorf("done = %v, err = %v, me = %q: the first ready message must identify the user", done, err, c.me.ID)
	}
}

func TestUnsentFirstAttachIsRepeatedOnTheNextReady(t *testing.T) {
	reattached := false
	c := reattaching(&api.Error{Status: 503})
	c.opts.Reattach = func(context.Context, string) (api.Session, error) {
		reattached = true
		return api.Session{}, &api.Error{Status: 503}
	}

	_, done, _ := c.receive(context.Background(), gateway.Message{Type: gateway.TypeReady})

	if done || !reattached {
		t.Errorf("done = %v, reattached = %v: a first attach which was not sent must be repeated", done, reattached)
	}
}
