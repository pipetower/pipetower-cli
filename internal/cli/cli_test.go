package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
)

func TestParseVariables(t *testing.T) {
	definitions := []api.VariableDefinition{
		{Name: "branch", Type: api.VariableSelection, Values: "main, develop"},
		{Name: "tag", Type: api.VariableText},
	}

	values, err := parseVariables(definitions, []string{"branch=main", "tag=v1=rc"})
	if err != nil || values["branch"] != "main" || values["tag"] != "v1=rc" {
		t.Fatalf("values = %v, err = %v", values, err)
	}

	for _, invalid := range []string{"branch=feature", "unknown=1", "novalue"} {
		if _, err := parseVariables(definitions, []string{invalid}); fault.ExitCode(err) != fault.ExitUsage {
			t.Errorf("%q: err = %v", invalid, err)
		}
	}
}

func TestMissingVariablesWithoutPrompt(t *testing.T) {
	a := NewApp()
	a.IO.CanPrompt = false

	_, err := collectVariables(a, []api.VariableDefinition{{Name: "branch", Type: api.VariableText}}, nil)

	if fault.ExitCode(err) != fault.ExitUsage {
		t.Errorf("err = %v", err)
	}
}

func TestRawEndpoint(t *testing.T) {
	e := rawEndpoint("POST", "/api/v1/runs?status=running", `{"a":1}`)

	if e.Path != "runs" || e.Query.Get("status") != "running" || string(e.Body.([]byte)) != `{"a":1}` {
		t.Errorf("endpoint = %+v", e)
	}
}

func TestExitCode(t *testing.T) {
	cases := map[error]int{
		nil:              fault.ExitOK,
		context.Canceled: fault.ExitInterrupted,
		errors.New(`unknown command "x" for "pt"`): fault.ExitUsage,
		&fault.Silent{Code: 1}:                     fault.ExitFailure,
		&api.Error{Status: 404}:                    fault.ExitAPI,
		errors.New("boom"):                         fault.ExitFailure,
	}
	for err, want := range cases {
		if got := ExitCode(err); got != want {
			t.Errorf("ExitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestStepSummary(t *testing.T) {
	if got := stepSummary(map[string]any{"script": "#!/bin/bash\nset -e\n"}); got != "set -e" {
		t.Errorf("summary = %q", got)
	}
	if got := stepSummary(map[string]any{"command": "git pull"}); got != "git pull" {
		t.Errorf("summary = %q", got)
	}
}
