package cli

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// completionTimeout limits how long Tab waits for the API, a shell must never hang.
const completionTimeout = 5 * time.Second

// completedRuns is how many recent runs Tab offers.
const completedRuns = 30

// freeTextFlags take values which can't be completed, they complete nothing (instead of file names).
var freeTextFlags = []string{"search", "limit", "page", "data"}

// placeholder finds the argument placeholders in a command's use line: run <action> → action.
var placeholder = regexp.MustCompile(`<([a-z]+)>`)

// completion computes the completions of an argument or flag value, mostly from the API.
type completion func(ctx context.Context, a *App, cmd *cobra.Command, args []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error)

// addCompletions makes Tab complete everything: commands and flags (by cobra), the arguments by
// their placeholder (<server>, <run>, …) and the flag values by the flag name, with the names of
// the user's resources. What can't be completed completes nothing instead of file names. A command
// whose argument or flag has no completion panics, so that no command is ever left out.
func addCompletions(a *App, root *cobra.Command) {
	forEachCommand(root, func(cmd *cobra.Command) {
		cmd.ValidArgsFunction = argCompletion(a, cmd)

		cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
			if flag.Value.Type() == "bool" {
				return
			}
			if err := cmd.RegisterFlagCompletionFunc(flag.Name, flagCompletion(a, cmd, flag.Name)); err != nil {
				panic(err)
			}
		})
	})
}

// argCompletion completes the positional arguments of a command, the nth by its nth placeholder.
func argCompletion(a *App, cmd *cobra.Command) cobra.CompletionFunc {
	var completions []completion
	for _, match := range placeholder.FindAllStringSubmatch(cmd.Use, -1) {
		completions = append(completions, argument(cmd, match[1]))
	}
	return func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) >= len(completions) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeWith(a, completions[len(args)], c, args, toComplete)
	}
}

// argument returns the completion of the argument with the placeholder.
func argument(cmd *cobra.Command, name string) completion {
	switch name {
	case "server":
		return completeNames(resolve.KindServer)
	case "action":
		return completeNames(resolve.KindAction)
	case "workflow":
		return completeNames(resolve.KindWorkflow)
	case "pipe":
		return completeNames(resolve.KindPipe)
	case "template":
		return completeNames(resolve.KindTemplate)
	case "team":
		return completeNames(resolve.KindTeam)
	case "run":
		return completeRuns(cmd.Name() == "cancel") // only unfinished runs can be canceled
	case "session":
		return completeSessions
	case "method":
		return completeFixed("GET", "POST", "PUT", "PATCH", "DELETE")
	case "path":
		return completeFixed(api.Servers, api.Actions, api.Workflows, api.Pipes, api.Templates, api.Teams, api.Runs, "account", "account/vars")
	case "shell":
		return completeFixed("bash", "zsh", "fish", "powershell")
	}
	panic(fmt.Sprintf("%s: no completion of <%s>", cmd.CommandPath(), name))
}

// flagCompletion completes the value of a flag by its name.
func flagCompletion(a *App, cmd *cobra.Command, name string) cobra.CompletionFunc {
	values := flagValues(cmd, name)
	return func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return completeWith(a, values, c, args, toComplete)
	}
}

// flagValues returns the completion of the flag's values.
func flagValues(cmd *cobra.Command, name string) completion {
	switch {
	case name == "server" && cmd.Flags().Lookup("var") != nil:
		return completeRunServers // pt action run -s …: servers and tags
	case name == "server" && cmd.CommandPath() == "pt run view":
		return completeServersOfRun
	case name == "server":
		return completeNames(resolve.KindServer)
	case name == "action":
		return completeNames(resolve.KindAction)
	case name == "workflow":
		return completeNames(resolve.KindWorkflow)
	case name == "pipe":
		return completeNames(resolve.KindPipe)
	case name == "team":
		return completeNames(resolve.KindTeam)
	case name == "tag":
		return completeTags
	case name == "var":
		return completeVariables
	case name == "status":
		return completeStatuses
	case name == "output":
		return completeFixed(FormatTable, FormatJSON, FormatYAML)
	case name == "file":
		return completeDefinitionFiles
	case slices.Contains(freeTextFlags, name):
		return completeFixed()
	}
	panic(fmt.Sprintf("%s: no completion of --%s", cmd.CommandPath(), name))
}

// run computes completions within the time limit. Failures (no configuration, no network) complete
// nothing: a completion has no place to explain them, the command itself will.
func completeWith(a *App, complete completion, cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	a.IO.Err = io.Discard // warnings of the configuration would garble the command line
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, completionTimeout)
	defer cancel()

	candidates, directive, err := complete(ctx, a, cmd, args, toComplete)
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	completions := make([]cobra.Completion, len(candidates))
	for i, c := range candidates {
		completions[i] = cobra.CompletionWithDesc(c.Value, c.Description)
	}
	if directive&cobra.ShellCompDirectiveFilterFileExt != 0 {
		return completions, directive // the shell completes the files itself
	}
	return completions, directive | cobra.ShellCompDirectiveNoFileComp
}

// completeDefinitionFiles lets the shell complete the definition files: YAML and JSON.
func completeDefinitionFiles(context.Context, *App, *cobra.Command, []string, string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	return []resolve.Candidate{{Value: "yml"}, {Value: "yaml"}, {Value: "json"}}, cobra.ShellCompDirectiveFilterFileExt, nil
}

// completionResolver returns the resolver of the app, limited to --team, which never asks.
func completionResolver(ctx context.Context, a *App) (*resolve.Resolver, error) {
	r, err := a.Resolver(ctx)
	if err != nil {
		return nil, err
	}
	r.Prompter = nil
	return r, nil
}

// completeNames completes the names of a kind.
func completeNames(kind resolve.Kind) completion {
	return func(ctx context.Context, a *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
		r, err := completionResolver(ctx, a)
		if err != nil {
			return nil, 0, err
		}
		candidates, err := r.Complete(ctx, kind, toComplete)
		return candidates, 0, err
	}
}

// completeFixed completes fixed values.
func completeFixed(values ...string) completion {
	return func(_ context.Context, _ *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
		var candidates []resolve.Candidate
		for _, value := range values {
			if strings.HasPrefix(value, toComplete) {
				candidates = append(candidates, resolve.Candidate{Value: value})
			}
		}
		return candidates, 0, nil
	}
}

// completeStatuses completes the run statuses with their labels.
func completeStatuses(_ context.Context, _ *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	var candidates []resolve.Candidate
	for _, status := range []string{api.StatusPending, api.StatusRunning, api.StatusSuccessful, api.StatusFailed,
		api.StatusPartiallyFailed, api.StatusCanceled, api.StatusTerminated, api.StatusSkipped, api.StatusNotAllowed} {
		if strings.HasPrefix(status, toComplete) {
			candidates = append(candidates, resolve.Candidate{Value: status, Description: present.Status(status).Label})
		}
	}
	return candidates, cobra.ShellCompDirectiveKeepOrder, nil
}

// completeTags completes the tags of the servers, for pt server list --tag.
func completeTags(ctx context.Context, a *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	r, err := completionResolver(ctx, a)
	if err != nil {
		return nil, 0, err
	}
	candidates, err := r.CompleteTags(ctx, toComplete)
	return candidates, 0, err
}

// completeRunServers completes the servers to run on and tag:<tag>, without those already given.
func completeRunServers(ctx context.Context, a *App, cmd *cobra.Command, args []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	r, err := completionResolver(ctx, a)
	if err != nil {
		return nil, 0, err
	}
	// only the servers the action or workflow may run on, all while it is not given (yet)
	runnable, _ := completionRunnable(ctx, r, cmd, args)
	candidates, err := r.CompleteServersToRunOn(ctx, runnable, toComplete)
	if err != nil {
		return nil, 0, err
	}

	given, _ := cmd.Flags().GetStringArray("server")
	return slices.DeleteFunc(candidates, func(c resolve.Candidate) bool { return slices.Contains(given, c.Value) }), 0, nil
}

// completeServersOfRun completes the servers a run ran on, for pt run view <run> --log --server.
func completeServersOfRun(ctx context.Context, a *App, cmd *cobra.Command, args []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	if len(args) == 0 {
		return completeNames(resolve.KindServer)(ctx, a, cmd, args, toComplete)
	}
	client, err := a.Client()
	if err != nil {
		return nil, 0, err
	}
	run, err := client.Run(ctx, args[0], false)
	if err != nil {
		return nil, 0, err
	}
	var candidates []resolve.Candidate
	for _, execution := range run.Executions {
		if strings.HasPrefix(execution.Server.Name, toComplete) {
			candidates = append(candidates, resolve.Candidate{Value: execution.Server.Name, Description: present.Status(execution.Status).Label})
		}
	}
	return candidates, 0, nil
}

// completeRuns completes the ids of the latest runs, newest first, described by what ran and how it went.
func completeRuns(unfinished bool) completion {
	return func(ctx context.Context, a *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
		client, err := a.Client()
		if err != nil {
			return nil, 0, err
		}
		teamID, err := a.TeamID(ctx)
		if err != nil {
			return nil, 0, err
		}
		page, err := api.Get[api.Page[api.Run]](ctx, client, api.List(api.Runs, api.ListOptions{Limit: completedRuns, Team: teamID}))
		if err != nil {
			return nil, 0, err
		}

		now := time.Now()
		var candidates []resolve.Candidate
		for _, run := range page.Data {
			if (unfinished && run.Finished()) || !strings.HasPrefix(run.ID, toComplete) {
				continue
			}
			what := "?"
			if run.Executable != nil {
				what = run.Executable.Name
			}
			description := what + " · " + present.Status(run.Status).Label + " · " + ui.Ago(ui.FirstTime(run.StartedAt, run.CreatedAt), now)
			candidates = append(candidates, resolve.Candidate{Value: run.ID, Description: description})
		}
		return candidates, cobra.ShellCompDirectiveKeepOrder, nil
	}
}

// completeSessions completes the ids of the live terminal sessions, which only the gateway knows.
func completeSessions(ctx context.Context, a *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	client, err := a.Connector()(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer client.Close()

	var candidates []resolve.Candidate
	for _, s := range client.Ready().Sessions {
		if strings.HasPrefix(s.ID, toComplete) {
			candidates = append(candidates, resolve.Candidate{Value: s.ID, Description: s.Server.Name + " · " + s.Owner.Name})
		}
	}
	return candidates, 0, nil
}

// completeVariables completes -v: the variable names of the action or workflow as name=, then the values
// of a selection variable as name=value.
func completeVariables(ctx context.Context, a *App, cmd *cobra.Command, args []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	if len(args) == 0 {
		return nil, 0, nil // the variables depend on the action or workflow
	}
	r, err := completionResolver(ctx, a)
	if err != nil {
		return nil, 0, err
	}
	runnable, err := completionRunnable(ctx, r, cmd, args)
	if err != nil {
		return nil, 0, err
	}

	name, value, hasValue := strings.Cut(toComplete, "=")
	given, _ := cmd.Flags().GetStringArray("var")
	var candidates []resolve.Candidate
	for _, definition := range runnable.Variables() {
		switch {
		case hasValue && definition.Name == name:
			for _, option := range selectionValues(definition) {
				if strings.HasPrefix(option, value) {
					candidates = append(candidates, resolve.Candidate{Value: name + "=" + option})
				}
			}
		case !hasValue && strings.HasPrefix(definition.Name, name) && !isGiven(given, definition.Name):
			candidates = append(candidates, resolve.Candidate{Value: definition.Name + "=", Description: variableDescription(definition)})
		}
	}
	if hasValue {
		return candidates, 0, nil
	}
	return candidates, cobra.ShellCompDirectiveNoSpace, nil // the value follows the =
}

// completionRunnable resolves the action or workflow of pt action|workflow run <name>, nil without one.
func completionRunnable(ctx context.Context, r *resolve.Resolver, cmd *cobra.Command, args []string) (*resolve.Runnable, error) {
	if len(args) == 0 {
		return nil, nil
	}
	kind := actionKind
	if cmd.Parent().Name() == workflowKind.kind.Singular {
		kind = workflowKind
	}
	runnable, err := kind.resolve(r, ctx, args[0])
	if err != nil {
		return nil, err
	}
	return &runnable, nil
}

// isGiven tells whether a variable was already given as name=value.
func isGiven(given []string, name string) bool {
	return slices.ContainsFunc(given, func(v string) bool { return strings.HasPrefix(v, name+"=") })
}

// variableDescription describes a variable: its type, a selection with its values.
func variableDescription(definition api.VariableDefinition) string {
	if values := selectionValues(definition); len(values) > 0 {
		return "one of " + strings.Join(values, ", ")
	}
	return definition.Type
}
