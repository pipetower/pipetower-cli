package resolve

import (
	"fmt"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// NotFoundError tells that nothing matched an argument.
type NotFoundError struct {
	Kind        Kind
	Arg         string
	Suggestions []string
}

// Error implements error.
func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Kind.Singular, e.Arg)
}

// Problem implements fault.Explainer.
func (e *NotFoundError) Problem() fault.Problem {
	p := fault.Problem{
		Title: fmt.Sprintf("%s “%s” not found", ui.Capitalize(e.Kind.Singular), e.Arg),
		Code:  fault.ExitAPI,
	}

	if len(e.Suggestions) > 0 {
		p.Detail = "Did you mean:\n" + bullets(e.Suggestions)
		p.Hints = []fault.Hint{{Label: "Use the exact name or id"}}
	} else {
		p.Hints = []fault.Hint{{Label: "See all " + e.Kind.Plural, Value: listCommand(e.Kind)}}
	}

	return p
}

// AmbiguousError tells that several items have the given name.
type AmbiguousError struct {
	Kind       Kind
	Arg        string
	Candidates []string
}

// Error implements error.
func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %d %s", e.Arg, len(e.Candidates), e.Kind.Plural)
}

// Problem implements fault.Explainer.
func (e *AmbiguousError) Problem() fault.Problem {
	return fault.Problem{
		Title:  fmt.Sprintf("“%s” matches %d %s", e.Arg, len(e.Candidates), e.Kind.Plural),
		Detail: bullets(e.Candidates),
		Hints:  []fault.Hint{{Label: "Use the id instead of the name, or run pt interactively to pick one"}},
		Code:   fault.ExitUsage,
	}
}

// NotAllowedError tells that an action or workflow may not run on a server, or a tag's servers. Without
// Arg, it may not run on any server at all (its servers were deleted after it was restricted to them).
type NotAllowedError struct {
	Runnable  Runnable
	Arg       string
	OtherTeam bool
}

// Error implements error.
func (e *NotAllowedError) Error() string {
	if e.Arg == "" {
		return e.Runnable.Name() + " may not run on any server"
	}
	return e.Runnable.Name() + " may not run on " + e.Arg
}

// Problem implements fault.Explainer, it names where the action or workflow may run.
func (e *NotAllowedError) Problem() fault.Problem {
	p := fault.Problem{Title: ui.Capitalize(e.Error()), Code: fault.ExitUsage}
	kind := strings.ToLower(e.Runnable.Type())
	switch allowed := e.Runnable.AllowedServers(); {
	case e.OtherTeam:
		p.Detail = e.Arg + " belongs to another team than the " + kind + "."
	case allowed != nil && len(*allowed) == 0:
		p.Detail = "The servers it was restricted to don't exist anymore."
		p.Hints = []fault.Hint{{Label: "Choose its servers in the settings of the " + kind + " in Pipetower"}}
		return p
	}
	p.Hints = []fault.Hint{{Label: "It may run on", Value: AllowedServerNames(e.Runnable)}}
	return p
}

// AllowedServerNames names the servers the action or workflow may run on.
func AllowedServerNames(runnable Runnable) string {
	allowed := runnable.AllowedServers()
	if allowed == nil {
		return "any server of its team"
	}
	names := make([]string, len(*allowed))
	for i, ref := range *allowed {
		names[i] = ref.Name
	}
	return strings.Join(names, ", ")
}

// listCommand is the command which lists the kind.
func listCommand(kind Kind) string {
	return "pt " + kind.Command + " list"
}

// bullets renders a list of items.
func bullets(items []string) string {
	return "  • " + strings.Join(items, "\n  • ")
}
