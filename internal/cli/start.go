package cli

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/runview"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// followHelp explains how following a run ends, for the help of the commands which follow runs.
const followHelp = "Ctrl+C asks whether to terminate the run or to detach from it.\n" +
	"The exit code tells how the run ended: 0 successful, 1 failed, 130 terminated or detached."

// runFlags are the flags of commands which start runs.
type runFlags struct {
	servers []string
	vars    []string
	detach  bool
	follow  followFlags
}

// followFlags are the flags of commands which follow runs.
type followFlags struct {
	quiet bool
	full  bool
}

// addRunFlags adds the flags of commands which start runs.
func addRunFlags(cmd *cobra.Command, flags *runFlags) {
	cmd.Flags().StringArrayVarP(&flags.servers, "server", "s", nil, "server to run on: name, id or tag:<tag> (repeatable)")
	cmd.Flags().StringArrayVarP(&flags.vars, "var", "v", nil, "variable as name=value (repeatable)")
	cmd.Flags().BoolVarP(&flags.detach, "detach", "d", false, "start the run and return immediately")
	addFollowFlags(cmd, &flags.follow)
}

// addFollowFlags adds the flags of commands which follow runs.
func addFollowFlags(cmd *cobra.Command, flags *followFlags) {
	cmd.Flags().BoolVarP(&flags.quiet, "quiet", "q", false, "only print the result")
	cmd.Flags().BoolVar(&flags.full, "full", false, "show the complete output instead of the step tree")
}

// startRun asks for what is missing, starts the run and follows it.
func startRun(ctx context.Context, a *App, runnable resolve.Runnable, flags runFlags) error {
	servers, err := serversToRunOn(ctx, a, runnable, flags)
	if err != nil {
		return err
	}
	variables, err := collectVariables(a, runnable.Variables(), flags.vars)
	if err != nil {
		return err
	}

	client, err := a.Client()
	if err != nil {
		return err
	}
	// the start is not canceled halfway: once sent, the run may exist, so its id must be shown
	run, err := ui.Spin(a.IO, "Starting "+runnable.Name()+"…", func() (api.Run, error) {
		return client.StartRun(context.WithoutCancel(ctx), runnable.Request(serverIDs(servers), variables))
	})
	if err != nil {
		return err
	}

	if flags.detach {
		return printStarted(a, run)
	}
	if ctx.Err() != nil {
		// interrupted while starting: the run runs, tell how to follow it
		if err := printStarted(a, run); err != nil {
			return err
		}
		return &fault.Silent{Code: fault.ExitInterrupted}
	}
	return followRun(ctx, a, run.ID, flags.follow)
}

// printStarted announces a detached run.
func printStarted(a *App, run api.Run) error {
	if a.Structured() {
		return a.PrintValue(run)
	}
	a.IO.Println(" " + ui.Success.Render("✓") + " Started run " + ui.Bold.Render("#"+run.ID) +
		ui.Muted.Render("  ·  follow it with ") + ui.Code.Render("pt run watch "+run.ID))
	return nil
}

// followRun follows a run until it finished and turns its status into the exit code.
func followRun(ctx context.Context, a *App, runID string, flags followFlags) error {
	mode := runview.ModeTree
	if flags.full {
		mode = runview.ModeFull
	}
	client, err := a.Client()
	if err != nil {
		return err
	}

	opts := runview.Options{Mode: mode, Quiet: flags.quiet || a.Structured(), Live: a.IO.Terminal, Out: a.IO.Out, Notices: a.IO.Err, Profile: a.IO.Profile, Width: a.IO.Width()}
	switch {
	case a.Structured():
		opts.Live, opts.Out = false, a.IO.Err // stdout carries only the structured run
	case opts.Live && !opts.Quiet:
		opts.Out = a.IO.RawOut
	}

	result, err := runview.Watch(ctx, runview.Deps{API: client, Connect: a.Connector()}, runID, opts)
	if err != nil {
		return orNotFound(err, resolve.KindRun, runID)
	}
	if a.Structured() && result.Run != nil {
		// also after Ctrl+C (detached), scripts get the run with its id
		printCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := a.printEndpoint(printCtx, api.RunEndpoint(runID, false)); err != nil {
			return err
		}
	}
	if code := result.ExitCode(); code != fault.ExitOK {
		return &fault.Silent{Code: code}
	}
	return nil
}

// serversToRunOn returns the servers to run on, only those the action or workflow may run on: the
// --server flags (tags without the servers it may not run on), its only server if it is restricted
// to one, or those the user picks.
func serversToRunOn(ctx context.Context, a *App, runnable resolve.Runnable, flags runFlags) ([]api.Server, error) {
	resolver, err := a.Resolver(ctx)
	if err != nil {
		return nil, err
	}
	notice := func(text string) {
		if !flags.follow.quiet {
			a.IO.Errorln(" " + text)
		}
	}

	if len(flags.servers) > 0 {
		servers, skipped, err := resolver.ServersToRunOn(ctx, runnable, flags.servers)
		if err == nil && len(skipped) > 0 {
			notice(ui.Muted.Render("Skipping " + strings.Join(skipped, ", ") + ": " + runnable.Name() + " may not run there"))
		}
		return servers, err
	}
	if server, ok := runnable.SoleServer(); ok {
		// like the run button of the web app, which runs it on its only server without asking
		notice(ui.Muted.Render("On ") + ui.Bold.Render(server.Name) + ui.Muted.Render(", the only server "+runnable.Name()+" may run on"))
		return []api.Server{server}, nil
	}
	if allowed := runnable.AllowedServers(); allowed != nil && len(*allowed) == 0 {
		return nil, &resolve.NotAllowedError{Runnable: runnable}
	}
	if resolver.Prompter == nil {
		return nil, usageError("No server given", "Choose servers with --server <name|id|tag:tag>, it can be repeated.")
	}
	return resolver.PickServers(ctx, runnable)
}

// collectVariables combines --var flags with answers for the missing variables.
func collectVariables(a *App, definitions []api.VariableDefinition, flags []string) (map[string]string, error) {
	values, err := parseVariables(definitions, flags)
	if err != nil {
		return nil, err
	}

	var missing []api.VariableDefinition
	for _, definition := range definitions {
		if values[definition.Name] == "" {
			missing = append(missing, definition)
		}
	}
	if len(missing) == 0 {
		return values, nil
	}

	prompter := a.Prompter()
	if prompter == nil {
		return nil, missingVariablesError(missing)
	}
	for _, definition := range missing {
		if values[definition.Name], err = askVariable(a, definition); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// parseVariables parses name=value flags and checks them against the definitions.
func parseVariables(definitions []api.VariableDefinition, flags []string) (map[string]string, error) {
	known := map[string]api.VariableDefinition{}
	for _, definition := range definitions {
		known[definition.Name] = definition
	}

	values := map[string]string{}
	for _, flag := range flags {
		name, value, ok := strings.Cut(flag, "=")
		definition, isKnown := known[name]
		switch {
		case !ok || name == "":
			return nil, usageError("Invalid variable “"+flag+"”", "Use --var name=value.")
		case !isKnown:
			return nil, usageError("Unknown variable “"+name+"”", "Known variables: "+ui.Or(definitionNames(definitions), "none"))
		case definition.Type == api.VariableSelection && !slices.Contains(selectionValues(definition), value):
			return nil, usageError("Invalid value for "+name, "Choose one of: "+strings.Join(selectionValues(definition), ", "))
		}
		values[name] = value
	}
	return values, nil
}

// askVariable asks for the value of one variable.
func askVariable(a *App, definition api.VariableDefinition) (string, error) {
	prompter := a.Prompter()
	if definition.Type == api.VariableSelection {
		values := selectionValues(definition)
		options := make([]prompt.Option, len(values))
		for i, value := range values {
			options[i] = prompt.Option{Columns: []string{value}}
		}
		index, err := prompter.Pick("Value of "+definition.Name+"?", options)
		if err != nil {
			return "", err
		}
		return values[index], nil
	}
	return prompter.Input("Value of " + definition.Name + "?")
}

// missingVariablesError explains which variables must be given.
func missingVariablesError(missing []api.VariableDefinition) error {
	var hints []string
	for _, definition := range missing {
		hint := "-v " + definition.Name + "=…"
		if definition.Type == api.VariableSelection {
			hint += "  (" + strings.Join(selectionValues(definition), " | ") + ")"
		}
		hints = append(hints, "  "+hint)
	}
	sort.Strings(hints)
	return usageError("Missing variables", "Give them with --var:\n"+strings.Join(hints, "\n"))
}

// selectionValues splits the comma separated values of a selection variable.
func selectionValues(definition api.VariableDefinition) []string {
	var values []string
	for _, value := range strings.Split(definition.Values, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// definitionNames lists the names of variable definitions.
func definitionNames(definitions []api.VariableDefinition) string {
	names := make([]string, len(definitions))
	for i, definition := range definitions {
		names[i] = definition.Name
	}
	return strings.Join(names, ", ")
}

// serverIDs returns the ids of servers.
func serverIDs(servers []api.Server) []string {
	ids := make([]string, len(servers))
	for i, server := range servers {
		ids[i] = server.ID
	}
	return ids
}
