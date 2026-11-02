package resolve

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
)

// restrictedTo is a workflow of team t1 which may only run on the servers with the ids.
func restrictedTo(ids ...string) Runnable {
	allowed := make([]api.Reference, len(ids))
	for i, id := range ids {
		allowed[i] = api.Reference{ID: id, Name: map[string]string{"srv1": "prod", "srv2": "prod-db", "srv3": "staging"}[id]}
	}
	return Runnable{Workflow: &api.Workflow{ID: "wf1", TeamID: "t1", Name: "Deploy All", AllowedServers: &allowed}}
}

func TestAllowsOnlyServersOfItsTeamAndRestriction(t *testing.T) {
	unrestricted := Runnable{Workflow: &api.Workflow{TeamID: "t1"}}
	restricted := restrictedTo("srv1")

	cases := []struct {
		runnable Runnable
		server   api.Server
		want     bool
	}{
		{unrestricted, api.Server{ID: "srv1", TeamID: "t1"}, true},
		{unrestricted, api.Server{ID: "srv9", TeamID: "t2"}, false}, // another team
		{restricted, api.Server{ID: "srv1", TeamID: "t1"}, true},
		{restricted, api.Server{ID: "srv3", TeamID: "t1"}, false},
	}
	for _, c := range cases {
		if got := c.runnable.Allows(c.server); got != c.want {
			t.Errorf("Allows(%s of %s) = %v, want %v", c.server.ID, c.server.TeamID, got, c.want)
		}
	}
}

func TestSoleServer(t *testing.T) {
	if server, ok := restrictedTo("srv1").SoleServer(); !ok || server.ID != "srv1" || server.Name != "prod" {
		t.Errorf("sole server = %+v, %v", server, ok)
	}
	if _, ok := restrictedTo("srv1", "srv3").SoleServer(); ok {
		t.Error("two servers are no sole server")
	}
	if _, ok := (Runnable{Workflow: &api.Workflow{}}).SoleServer(); ok {
		t.Error("an unrestricted workflow has no sole server")
	}
}

func TestAServerItMayNotRunOnIsNamedWithTheAllowedOnes(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	_, _, err := r.ServersToRunOn(context.Background(), restrictedTo("srv1"), []string{"staging"})

	var notAllowed *NotAllowedError
	if !errors.As(err, &notAllowed) || notAllowed.Arg != "staging" {
		t.Fatalf("err = %v", err)
	}
	p := notAllowed.Problem()
	if p.Title != "Deploy All may not run on staging" || len(p.Hints) != 1 || p.Hints[0].Value != "prod" {
		t.Errorf("problem = %+v", p)
	}
}

func TestTagsSkipTheServersItMayNotRunOn(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	servers, skipped, err := r.ServersToRunOn(context.Background(), restrictedTo("srv1"), []string{"tag:web"})

	if err != nil || len(servers) != 1 || servers[0].Name != "prod" || !slices.Equal(skipped, []string{"staging"}) {
		t.Errorf("servers = %v, skipped = %v, err = %v", servers, skipped, err)
	}
}

func TestATagWithoutServerItMayRunOnIsAnError(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	_, _, err := r.ServersToRunOn(context.Background(), restrictedTo("srv2"), []string{"tag:web"})

	var notAllowed *NotAllowedError
	if !errors.As(err, &notAllowed) || notAllowed.Arg != "tag:web" {
		t.Errorf("err = %v", err)
	}
}

func TestThePickerOffersOnlyServersItMayRunOn(t *testing.T) {
	prompter := &fakePrompter{}
	r := &Resolver{Client: fakeAPI(t), Prompter: prompter}

	servers, err := r.PickServers(context.Background(), restrictedTo("srv1", "srv3"))

	if err != nil || len(servers) != 1 || !slices.Equal(prompter.asked, []string{"prod", "staging"}) {
		t.Errorf("servers = %v, offered = %v, err = %v", servers, prompter.asked, err)
	}
}

func TestRestrictedToDeletedServersOnly(t *testing.T) {
	p := (&NotAllowedError{Runnable: restrictedTo()}).Problem()

	if p.Title != "Deploy All may not run on any server" || p.Detail == "" {
		t.Errorf("problem = %+v", p)
	}
}
