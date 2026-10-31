package resolve

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
)

// TagPrefix selects all servers with a tag, like tag:web.
const TagPrefix = "tag:"

// Runnable is an action or a workflow.
type Runnable struct {
	Action   *api.Action
	Workflow *api.Workflow
}

// ID returns the id of the action or workflow.
func (r Runnable) ID() string {
	if r.Action != nil {
		return r.Action.ID
	}
	return r.Workflow.ID
}

// Name returns the name of the action or workflow.
func (r Runnable) Name() string {
	if r.Action != nil {
		return r.Action.Name
	}
	return r.Workflow.Name
}

// Type returns "Action" or "Workflow".
func (r Runnable) Type() string {
	if r.Action != nil {
		return "Action"
	}
	return "Workflow"
}

// Variables returns the variables needed on execution.
func (r Runnable) Variables() []api.VariableDefinition {
	if r.Action != nil {
		return r.Action.Variables
	}
	return r.Workflow.Variables
}

// Request creates the request which starts the runnable.
func (r Runnable) Request(serverIDs []string, variables map[string]string) api.RunRequest {
	req := api.RunRequest{ServerIDs: serverIDs, Variables: variables}
	if r.Action != nil {
		req.ActionID = r.Action.ID
	} else {
		req.WorkflowID = r.Workflow.ID
	}
	return req
}

// Runnable resolves an action or a workflow with the given id or name.
func (r *Resolver) Runnable(ctx context.Context, arg string) (Runnable, error) {
	actions, err := search(ctx, r, api.Actions, arg, asRunnableAction)
	if err != nil {
		return Runnable{}, err
	}
	workflows, err := search(ctx, r, api.Workflows, arg, asRunnableWorkflow)
	if err != nil {
		return Runnable{}, err
	}

	entries := slices.Concat(actions, workflows)
	if exact := exactMatches(entries, arg); len(exact) > 0 {
		return choose(r, KindRunnable, arg, exact, true)
	}
	if runnable, found, err := r.runnableByID(ctx, arg); found || err != nil {
		return runnable, err
	}

	return choose(r, KindRunnable, arg, entries, false)
}

// runnableByID looks the id up as an action first, then as a workflow.
func (r *Resolver) runnableByID(ctx context.Context, arg string) (Runnable, bool, error) {
	if action, found, err := lookupID[api.Action](ctx, r, api.Actions, arg); found || err != nil {
		return Runnable{Action: &action}, found, err
	}
	workflow, found, err := lookupID[api.Workflow](ctx, r, api.Workflows, arg)
	return Runnable{Workflow: &workflow}, found, err
}

// Servers resolves server arguments; tag:<name> expands to all servers with the tag.
func (r *Resolver) Servers(ctx context.Context, args []string) ([]api.Server, error) {
	var servers []api.Server
	seen := map[string]bool{}

	for _, arg := range args {
		found, err := r.serversOf(ctx, arg)
		if err != nil {
			return nil, err
		}
		for _, server := range found {
			if !seen[server.ID] {
				seen[server.ID] = true
				servers = append(servers, server)
			}
		}
	}

	return servers, nil
}

// serversOf resolves one argument to one or (for tags) more servers.
func (r *Resolver) serversOf(ctx context.Context, arg string) ([]api.Server, error) {
	tag, isTag := strings.CutPrefix(arg, TagPrefix)
	if !isTag {
		server, err := r.Server(ctx, arg)
		return []api.Server{server}, err
	}

	servers, err := api.ListAll[api.Server](ctx, r.Client, api.Servers, r.options(api.Servers, api.ListOptions{Params: map[string]string{"tag": tag}}), maxCandidates+1)
	switch {
	case err != nil:
		return nil, err
	case len(servers) == 0:
		return nil, &NotFoundError{Kind: KindServer, Arg: arg}
	case len(servers) > maxCandidates:
		// never run on a part of the tagged servers
		return nil, fault.New(fault.Problem{Title: fmt.Sprintf("Tag “%s” matches more than %d servers", tag, maxCandidates), Detail: "Run on a narrower tag or on servers by name.", Code: fault.ExitUsage})
	}
	return servers, nil
}

func describeServer(s api.Server) entry[api.Server] {
	return entry[api.Server]{s, s.ID, s.Name, s.Name + "  " + s.Host}
}

func describeAction(a api.Action) entry[api.Action] {
	return entry[api.Action]{a, a.ID, a.Name, a.Name}
}

func describeWorkflow(w api.Workflow) entry[api.Workflow] {
	return entry[api.Workflow]{w, w.ID, w.Name, w.Name}
}

func asRunnableAction(a api.Action) entry[Runnable] {
	return entry[Runnable]{Runnable{Action: &a}, a.ID, a.Name, "Action    " + a.Name}
}

func asRunnableWorkflow(w api.Workflow) entry[Runnable] {
	return entry[Runnable]{Runnable{Workflow: &w}, w.ID, w.Name, "Workflow  " + w.Name}
}
