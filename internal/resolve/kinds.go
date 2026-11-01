package resolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/ui"
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

// PickServers lets the user choose one or several of all servers.
func (r *Resolver) PickServers(ctx context.Context) ([]api.Server, error) {
	entries, err := all(ctx, r, KindServer, describeServer)
	if err != nil {
		return nil, err
	}
	chosen, err := r.Prompter.PickMany("Run on which servers?", options(entries))
	if err != nil {
		return nil, err
	}
	servers := make([]api.Server, len(chosen))
	for i, index := range chosen {
		servers[i] = entries[index].Item
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

// describeServer shows a server with its host, and in pickers also with its connection and tags.
func describeServer(s api.Server) entry[api.Server] {
	option := prompt.Option{Mark: ui.ConnectionMark(s.Connected), Columns: []string{s.Name, s.Host, tagList(s.Tags)}}
	return entry[api.Server]{Item: s, ID: s.ID, Name: s.Name, Label: s.Name + "  " + s.Host, Option: option}
}

// describeAction shows an action by its name, in pickers also with its tags.
func describeAction(a api.Action) entry[api.Action] {
	return withTags(named(a, a.ID, a.Name), a.Tags)
}

// describeWorkflow shows a workflow by its name, in pickers also with its tags.
func describeWorkflow(w api.Workflow) entry[api.Workflow] {
	return withTags(named(w, w.ID, w.Name), w.Tags)
}

// withTags adds the tags of an item as a column of its picker option.
func withTags[T any](e entry[T], tags []api.Tag) entry[T] {
	e.Option.Columns = append(e.Option.Columns, tagList(tags))
	return e
}

// tagList lists tags as #name.
func tagList(tags []api.Tag) string {
	names := make([]string, len(tags))
	for i, tag := range tags {
		names[i] = "#" + tag.Name
	}
	return strings.Join(names, " ")
}
