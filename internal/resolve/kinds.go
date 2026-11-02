package resolve

import (
	"context"
	"fmt"
	"slices"
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

// TeamID returns the team of the action or workflow, it may only run on servers of this team.
func (r Runnable) TeamID() string {
	if r.Action != nil {
		return r.Action.TeamID
	}
	return r.Workflow.TeamID
}

// AllowedServers returns the servers the action or workflow is restricted to, nil if it may run on
// all servers of its team.
func (r Runnable) AllowedServers() *[]api.Reference {
	if r.Action != nil {
		return r.Action.AllowedServers
	}
	return r.Workflow.AllowedServers
}

// Allows tells whether the action or workflow may run on the server: one of its team, and one it
// is restricted to, if it is restricted.
func (r Runnable) Allows(server api.Server) bool {
	if server.TeamID != "" && r.TeamID() != "" && server.TeamID != r.TeamID() {
		return false
	}
	allowed := r.AllowedServers()
	return allowed == nil || slices.ContainsFunc(*allowed, func(ref api.Reference) bool { return ref.ID == server.ID })
}

// SoleServer returns the only server the action or workflow may run on, if it is restricted to one.
func (r Runnable) SoleServer() (api.Server, bool) {
	allowed := r.AllowedServers()
	if allowed == nil || len(*allowed) != 1 {
		return api.Server{}, false
	}
	return api.Server{ID: (*allowed)[0].ID, Name: (*allowed)[0].Name, TeamID: r.TeamID()}, true
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

// ServersToRunOn resolves the servers given for a run of the action or workflow, like Servers. A
// server it may not run on is an error; a tag expands to its servers it may run on, the others are
// returned as skipped (an error if none is left).
func (r *Resolver) ServersToRunOn(ctx context.Context, runnable Runnable, args []string) (servers []api.Server, skipped []string, err error) {
	seen := map[string]bool{}
	for _, arg := range args {
		found, err := r.serversOf(ctx, arg)
		if err != nil {
			return nil, nil, err
		}
		_, isTag := strings.CutPrefix(arg, TagPrefix)
		allowed := 0
		for _, server := range found {
			switch {
			case !runnable.Allows(server) && !isTag:
				return nil, nil, &NotAllowedError{Runnable: runnable, Arg: server.Name, OtherTeam: server.TeamID != runnable.TeamID()}
			case !runnable.Allows(server):
				skipped = append(skipped, server.Name)
			case !seen[server.ID]:
				seen[server.ID] = true
				servers = append(servers, server)
				allowed++
			default:
				allowed++
			}
		}
		if isTag && allowed == 0 {
			return nil, nil, &NotAllowedError{Runnable: runnable, Arg: arg}
		}
	}
	return servers, skipped, nil
}

// PickServers lets the user choose one or several of the servers the action or workflow may run on.
func (r *Resolver) PickServers(ctx context.Context, runnable Runnable) ([]api.Server, error) {
	items, err := api.ListAll[api.Server](ctx, r.Client, api.Servers, api.ListOptions{Team: runnable.TeamID()}, maxCandidates)
	if err != nil {
		return nil, err
	}
	var entries []entry[api.Server]
	for _, server := range items {
		if runnable.Allows(server) {
			entries = append(entries, describeServer(server))
		}
	}
	if len(entries) == 0 {
		return nil, &NotAllowedError{Runnable: runnable}
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
