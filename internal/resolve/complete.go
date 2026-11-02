package resolve

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// maxCompletions limits the names offered for one Tab.
const maxCompletions = 100

// Candidate is a value offered by the shell completion, with a short description.
type Candidate struct {
	Value       string
	Description string
}

// Complete lists the names of a kind which start with the prefix (ignoring case), for the shell
// completion of arguments and flags.
func (r *Resolver) Complete(ctx context.Context, kind Kind, prefix string) ([]Candidate, error) {
	switch kind {
	case KindServer:
		return complete(ctx, r, kind, prefix, func(s api.Server) Candidate {
			return Candidate{s.Name, strings.TrimSpace(s.Host + "  " + tagList(s.Tags))}
		})
	case KindAction:
		return complete(ctx, r, kind, prefix, func(a api.Action) Candidate { return Candidate{a.Name, tagList(a.Tags)} })
	case KindWorkflow:
		return complete(ctx, r, kind, prefix, func(w api.Workflow) Candidate { return Candidate{w.Name, tagList(w.Tags)} })
	case KindPipe:
		return complete(ctx, r, kind, prefix, func(p api.Pipe) Candidate { return Candidate{p.Name, tagList(p.Tags)} })
	case KindTemplate:
		return complete(ctx, r, kind, prefix, func(t api.Template) Candidate { return Candidate{t.Name, t.FileName} })
	case KindTeam:
		return complete(ctx, r, kind, prefix, func(t api.Team) Candidate {
			return Candidate{t.Name, fmt.Sprintf("%d members", len(t.Members))}
		})
	}
	return nil, fmt.Errorf("%s can't be completed", kind.Plural)
}

// CompleteTags lists the tags of the servers which start with the prefix, for --tag.
func (r *Resolver) CompleteTags(ctx context.Context, prefix string) ([]Candidate, error) {
	servers, err := api.ListAll[api.Server](ctx, r.Client, api.Servers, r.options(api.Servers, api.ListOptions{}), maxCandidates)
	if err != nil {
		return nil, err
	}
	return tagCandidates(servers, prefix, false), nil
}

// CompleteServersToRunOn lists the servers to run on which start with the prefix, and the tags as
// tag:<name>: of an action or workflow only the servers it may run on, and the tags with any of
// them; without one (nil) all.
func (r *Resolver) CompleteServersToRunOn(ctx context.Context, runnable *Runnable, prefix string) ([]Candidate, error) {
	opts := r.options(api.Servers, api.ListOptions{})
	if runnable != nil {
		opts.Team = runnable.TeamID()
	}
	servers, err := api.ListAll[api.Server](ctx, r.Client, api.Servers, opts, maxCandidates)
	if err != nil {
		return nil, err
	}
	if runnable != nil {
		servers = slices.DeleteFunc(servers, func(s api.Server) bool { return !runnable.Allows(s) })
	}

	var candidates []Candidate
	if !strings.HasPrefix(prefix, TagPrefix) {
		seen := map[string]bool{}
		for _, s := range servers {
			if !seen[s.Name] && hasPrefixFold(s.Name, prefix) {
				seen[s.Name] = true
				candidates = append(candidates, Candidate{s.Name, strings.TrimSpace(s.Host + "  " + tagList(s.Tags))})
			}
		}
	}
	return append(candidates, tagCandidates(servers, prefix, true)...), nil
}

// tagCandidates lists the tags of the servers which start with the prefix, as tag:<name> when
// prefixed, described by how many of the servers have them.
func tagCandidates(servers []api.Server, prefix string, prefixed bool) []Candidate {
	counts := map[string]int{}
	for _, server := range servers {
		for _, tag := range server.Tags {
			counts[tag.Name]++
		}
	}
	var candidates []Candidate
	for name, count := range counts {
		value := name
		if prefixed {
			value = TagPrefix + name
		}
		if hasPrefixFold(value, prefix) {
			candidates = append(candidates, Candidate{value, ui.Plural(count, "server") + " tagged #" + name})
		}
	}
	slices.SortFunc(candidates, func(a, b Candidate) int { return strings.Compare(a.Value, b.Value) })
	return candidates
}

// complete lists the items of a kind whose name starts with the prefix, each name once.
func complete[T any](ctx context.Context, r *Resolver, kind Kind, prefix string, describe func(T) Candidate) ([]Candidate, error) {
	// the API searches names containing the text, which includes those starting with it
	items, err := api.ListAll[T](ctx, r.Client, kind.Resource, r.options(kind.Resource, api.ListOptions{Search: prefix}), maxCandidates)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var candidates []Candidate
	for _, item := range items {
		candidate := describe(item)
		if seen[candidate.Value] || !hasPrefixFold(candidate.Value, prefix) {
			continue
		}
		seen[candidate.Value] = true
		candidates = append(candidates, candidate)
		if len(candidates) == maxCompletions {
			break
		}
	}
	return candidates, nil
}

// hasPrefixFold tells whether the text starts with the prefix, ignoring case.
func hasPrefixFold(text, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}
