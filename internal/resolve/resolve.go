// Package resolve turns what users type (an id, a name or a tag) into API resources.
package resolve

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/prompt"
)

// maxCandidates limits how many search results are considered.
const maxCandidates = 300

var hashidPattern = regexp.MustCompile(`^[A-Za-z0-9]{4,}$`)

// Kind names a resource type in messages and commands.
type Kind struct {
	Singular string
	Plural   string
	Resource string
}

// Resource kinds.
var (
	KindServer         = Kind{"server", "servers", api.Servers}
	KindAction         = Kind{"action", "actions", api.Actions}
	KindWorkflow       = Kind{"workflow", "workflows", api.Workflows}
	KindPipe           = Kind{"pipe", "pipes", api.Pipes}
	KindTemplate       = Kind{"template", "templates", api.Templates}
	KindTeam           = Kind{"team", "teams", api.Teams}
	KindRunnable       = Kind{"action or workflow", "actions and workflows", ""}
	KindRun            = Kind{"run", "runs", api.Runs}
	KindRunOrExecution = Kind{"run or execution", "runs", api.Runs}
	KindSession        = Kind{"terminal session", "terminal sessions", "terminal-sessions"}
)

// Resolver resolves arguments, asking the user when a name is ambiguous.
type Resolver struct {
	Client *api.Client
	// Prompter is nil when the user must not be asked.
	Prompter prompt.Prompter
	// TeamID limits the resources (not the teams) to one team, if set.
	TeamID string
}

// entry is a resolvable item with its id, name and the label shown in pickers.
type entry[T any] struct {
	Item  T
	ID    string
	Name  string
	Label string
}

// Server resolves one server by id or name.
func (r *Resolver) Server(ctx context.Context, arg string) (api.Server, error) {
	return find(ctx, r, KindServer, arg, describeServer)
}

// Action resolves one action by id or name.
func (r *Resolver) Action(ctx context.Context, arg string) (api.Action, error) {
	return find(ctx, r, KindAction, arg, describeAction)
}

// Workflow resolves one workflow by id or name.
func (r *Resolver) Workflow(ctx context.Context, arg string) (api.Workflow, error) {
	return find(ctx, r, KindWorkflow, arg, describeWorkflow)
}

// Pipe resolves one pipe by id or name.
func (r *Resolver) Pipe(ctx context.Context, arg string) (api.Pipe, error) {
	return find(ctx, r, KindPipe, arg, func(p api.Pipe) entry[api.Pipe] { return entry[api.Pipe]{p, p.ID, p.Name, p.Name} })
}

// Template resolves one template by id or name.
func (r *Resolver) Template(ctx context.Context, arg string) (api.Template, error) {
	return find(ctx, r, KindTemplate, arg, func(t api.Template) entry[api.Template] {
		return entry[api.Template]{t, t.ID, t.Name, t.Name}
	})
}

// Team resolves one team by id or name.
func (r *Resolver) Team(ctx context.Context, arg string) (api.Team, error) {
	return find(ctx, r, KindTeam, arg, func(t api.Team) entry[api.Team] { return entry[api.Team]{t, t.ID, t.Name, t.Name} })
}

// find resolves an argument: exact name, then id, then a choice among partial matches.
func find[T any](ctx context.Context, r *Resolver, kind Kind, arg string, describe func(T) entry[T]) (T, error) {
	var zero T
	entries, err := search(ctx, r, kind.Resource, arg, describe)
	if err != nil {
		return zero, err
	}

	if exact := exactMatches(entries, arg); len(exact) > 0 {
		return choose(r, kind, arg, exact, true)
	}
	if item, found, err := lookupID[T](ctx, r, kind.Resource, arg); found || err != nil {
		return item, err
	}

	return choose(r, kind, arg, entries, false)
}

// search lists the items whose name contains the argument, described as entries of type E.
func search[T, E any](ctx context.Context, r *Resolver, resource, arg string, describe func(T) entry[E]) ([]entry[E], error) {
	items, err := api.ListAll[T](ctx, r.Client, resource, r.options(resource, api.ListOptions{Search: arg}), maxCandidates)
	if err != nil {
		return nil, err
	}

	entries := make([]entry[E], len(items))
	for i, item := range items {
		entries[i] = describe(item)
	}
	return entries, nil
}

// lookupID fetches the item directly if the argument looks like an id.
func lookupID[T any](ctx context.Context, r *Resolver, resource, arg string) (T, bool, error) {
	var zero T
	if resource == "" || !hashidPattern.MatchString(arg) {
		return zero, false, nil
	}

	raw, err := r.Client.JSON(ctx, api.Show(resource, arg))
	switch {
	case api.IsNotFound(err):
		return zero, false, nil
	case err != nil:
		return zero, false, err
	case !r.inTeam(resource, raw):
		return zero, false, nil
	}

	var item T
	err = json.Unmarshal(raw, &item)
	return item, err == nil, err
}

// inTeam tells whether a fetched item belongs to the team of the resolver (teams themselves always do).
func (r *Resolver) inTeam(resource string, raw json.RawMessage) bool {
	if r.TeamID == "" || resource == api.Teams {
		return true
	}
	var item struct {
		TeamID string `json:"team_id"`
	}
	return json.Unmarshal(raw, &item) == nil && item.TeamID == r.TeamID
}

// exactMatches returns the entries whose id or name equals the argument (names case-insensitive).
func exactMatches[T any](entries []entry[T], arg string) []entry[T] {
	var matches []entry[T]
	for _, e := range entries {
		if e.ID == arg || strings.EqualFold(e.Name, arg) {
			matches = append(matches, e)
		}
	}
	return matches
}

// choose returns an exact match, or lets the user pick among the candidates. Without a
// prompter only an unambiguous exact match is accepted, partial matches become suggestions.
func choose[T any](r *Resolver, kind Kind, arg string, entries []entry[T], exact bool) (T, error) {
	var zero T
	switch {
	case len(entries) == 0:
		return zero, &NotFoundError{Kind: kind, Arg: arg}
	case exact && len(entries) == 1:
		return entries[0].Item, nil
	case r.Prompter == nil && exact:
		return zero, &AmbiguousError{Kind: kind, Arg: arg, Candidates: labels(entries)}
	case r.Prompter == nil:
		return zero, &NotFoundError{Kind: kind, Arg: arg, Suggestions: labels(entries)}
	}

	index, err := r.Prompter.Select("Which "+kind.Singular+" do you mean by “"+arg+"”?", labels(entries))
	if err != nil {
		return zero, err
	}
	return entries[index].Item, nil
}

// labels returns the picker labels of the entries.
func labels[T any](entries []entry[T]) []string {
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.Label
	}
	return result
}

// options limits the list options to the team of the resolver, teams themselves are never limited.
func (r *Resolver) options(resource string, opts api.ListOptions) api.ListOptions {
	if resource != api.Teams {
		opts.Team = r.TeamID
	}
	return opts
}
