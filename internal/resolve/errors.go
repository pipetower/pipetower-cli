package resolve

import (
	"fmt"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
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
	} else if list := listCommand(e.Kind); list != "" {
		p.Hints = []fault.Hint{{Label: "See all " + e.Kind.Plural, Value: list}}
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

// listCommand is the command which lists the kind.
func listCommand(kind Kind) string {
	switch kind.Resource {
	case "":
		return "pt action list  ·  pt workflow list"
	case api.Runs:
		return "pt runs list"
	case "terminal-sessions":
		return "pt terminal ls"
	}
	return "pt " + kind.Singular + " list"
}

// bullets renders a list of items.
func bullets(items []string) string {
	return "  • " + strings.Join(items, "\n  • ")
}
