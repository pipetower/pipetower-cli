package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// AddHint is the command which creates the configuration file with a first environment.
const AddHint = "pt env add prod --url https://app.pipetower.com --token <token>"

// MissingError tells that the token or the URL was found nowhere.
type MissingError struct {
	Key         string
	Searched    []string
	URL         string
	Home        string
	TokenSource Source
	// Environment and Path name the active environment and its file, when there is one.
	Environment string
	Path        string
}

// Error implements error.
func (e *MissingError) Error() string {
	return e.Key + " is not configured"
}

// Problem implements fault.Explainer.
func (e *MissingError) Problem() fault.Problem {
	if e.Key == EnvURL {
		return fault.Problem{
			Title: "No Pipetower instance configured",
			Detail: fmt.Sprintf("The API token is taken from %s, but there is no URL next to it.\n"+
				"The URL must come from the same place as the token, so that no other file can redirect it.", e.TokenSource),
			Hints: []fault.Hint{e.addURLHint()},
			Code:  fault.ExitConfig,
		}
	}
	if e.Environment != "" {
		return fault.Problem{
			Title:  fmt.Sprintf("The environment %q has no token", e.Environment),
			Detail: "Add the API token of its instance to the environment in " + e.Path + ".",
			Hints:  []fault.Hint{{Label: "Create a token", Value: e.accountURL() + "  →  API tokens"}},
			Code:   fault.ExitConfig,
		}
	}

	return fault.Problem{
		Title:  "No API token found",
		Detail: "pt authenticates with an API token from a .pipetower file (JSON) with one or more environments.\n\n" + e.searchedText(),
		Hints: []fault.Hint{
			{Label: "Create a token", Value: e.accountURL() + "  →  API tokens"},
			{Label: "Save it", Value: AddHint},
		},
		Code: fault.ExitConfig,
	}
}

// addURLHint tells how to add the URL next to the token.
func (e *MissingError) addURLHint() fault.Hint {
	if e.TokenSource.Path == "" {
		return fault.Hint{Label: "Set the URL", Value: "export " + EnvURL + "=https://app.pipetower.com"}
	}
	return fault.Hint{Label: "Add the URL", Value: fmt.Sprintf(`"url": "https://app.pipetower.com" to the environment %q in %s`, e.Environment, e.TokenSource.Path)}
}

// searchedText lists the places which were checked.
func (e *MissingError) searchedText() string {
	lines := []string{"Looked in $" + EnvURL + " / $" + EnvToken + " and:"}
	for _, path := range e.Searched {
		if e.Home != "" && strings.HasPrefix(path, e.Home+string(filepath.Separator)) {
			path = "~" + strings.TrimPrefix(path, e.Home)
		}
		lines = append(lines, "  "+path)
	}
	return strings.Join(lines, "\n")
}

// accountURL is the page where API tokens are created.
func (e *MissingError) accountURL() string {
	if e.URL == "" {
		return "<your Pipetower URL>/account"
	}
	return e.URL + "/account"
}

// ExistingEnvironmentError tells that an environment of the name is already configured.
type ExistingEnvironmentError struct {
	Name string
	Path string
}

// Error implements error.
func (e *ExistingEnvironmentError) Error() string {
	return "environment " + e.Name + " exists"
}

// Problem implements fault.Explainer.
func (e *ExistingEnvironmentError) Problem() fault.Problem {
	return fault.Problem{
		Title:  fmt.Sprintf("The environment “%s” exists already", e.Name),
		Detail: "It is configured in " + e.Path + ", choose another name or change it there.",
		Hints:  []fault.Hint{{Label: "See them", Value: "pt env list"}},
		Code:   fault.ExitUsage,
	}
}

// NoEnvironmentError tells that none of several environments is active.
type NoEnvironmentError struct {
	Names []string
	Path  string
}

// Error implements error.
func (e *NoEnvironmentError) Error() string {
	return "no active environment"
}

// Problem implements fault.Explainer.
func (e *NoEnvironmentError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "Which environment?",
		Detail: fmt.Sprintf("%s has the environments %s, none is active.", e.Path, strings.Join(e.Names, ", ")),
		Hints:  []fault.Hint{{Label: "Activate one", Value: "pt env use <environment>"}},
		Code:   fault.ExitConfig,
	}
}

// UnknownEnvironmentError tells that the named environment is not configured.
type UnknownEnvironmentError struct {
	Name   string
	Source Source
	Names  []string
	Path   string
}

// Error implements error.
func (e *UnknownEnvironmentError) Error() string {
	return "unknown environment " + e.Name
}

// Problem implements fault.Explainer.
func (e *UnknownEnvironmentError) Problem() fault.Problem {
	detail := fmt.Sprintf("%s has the environments: %s.", e.Path, strings.Join(e.Names, ", "))
	if e.Source != (Source{}) {
		detail = fmt.Sprintf("Set in %s. ", e.Source) + detail
	}
	return fault.Problem{
		Title:  fmt.Sprintf("Unknown environment “%s”", e.Name),
		Detail: detail,
		Hints:  []fault.Hint{{Label: "See them", Value: "pt env list"}},
		Code:   fault.ExitConfig,
	}
}

// InvalidURLError tells that the configured URL is not an http(s) URL.
type InvalidURLError struct {
	URL    string
	Source Source
	Reason string
}

// Error implements error.
func (e *InvalidURLError) Error() string {
	return "invalid URL " + e.URL
}

// Problem implements fault.Explainer.
func (e *InvalidURLError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "Invalid Pipetower URL",
		Detail: fmt.Sprintf("%q (from %s) %s", e.URL, e.Source, e.Reason),
		Hints:  []fault.Hint{{Label: "Example", Value: `"url": "https://app.pipetower.com"`}},
		Code:   fault.ExitConfig,
	}
}
