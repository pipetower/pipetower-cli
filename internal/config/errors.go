package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// MissingError tells that a required key was found nowhere.
type MissingError struct {
	Key         string
	Searched    []string
	URL         string
	Home        string
	TokenSource Source
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
			Detail: fmt.Sprintf("The API token is taken from %s, but there is no %s next to it.\n"+
				"The URL must come from the same place as the token, so that no other file can redirect it.", e.TokenSource, EnvURL),
			Hints: []fault.Hint{e.addURLHint()},
			Code:  fault.ExitConfig,
		}
	}

	return fault.Problem{
		Title:  "No API token found",
		Detail: "pt authenticates with an API token from a .pipetower file.\n\n" + e.searchedText(),
		Hints: []fault.Hint{
			{Label: "Create a token", Value: e.accountURL() + "  →  API tokens"},
			{Label: "Save it", Value: fmt.Sprintf("printf '%s=<token>\\n' >> ~/%s", EnvToken, FileName)},
			{Label: "Protect it", Value: "chmod 600 ~/" + FileName},
		},
		Code: fault.ExitConfig,
	}
}

// addURLHint tells how to add the URL next to the token.
func (e *MissingError) addURLHint() fault.Hint {
	if e.TokenSource.Path == "" {
		return fault.Hint{Label: "Set the URL", Value: "export " + EnvURL + "=https://app.pipetower.com"}
	}
	return fault.Hint{Label: "Add the URL", Value: fmt.Sprintf("printf '%s=https://app.pipetower.com\\n' >> %s", EnvURL, e.TokenSource.Path)}
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
		Hints:  []fault.Hint{{Label: "Example", Value: EnvURL + "=https://app.pipetower.com"}},
		Code:   fault.ExitConfig,
	}
}
