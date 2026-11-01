package cli

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/runview"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newRunCommand groups the commands about runs. Runs are started with pt action run and pt workflow run.
func newRunCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "run",
		Short:      "List, view, watch and cancel runs",
		SuggestFor: []string{"runs"},
	}
	cmd.AddCommand(newRunListCommand(a), newRunViewCommand(a), newRunWatchCommand(a), newRunCancelCommand(a))
	return cmd
}

// runFilters are the filters of the run list: servers, actions, workflows and pipes by name or id.
type runFilters struct {
	server, action, workflow, pipe, status string
}

// newRunListCommand lists the latest runs.
func newRunListCommand(a *App) *cobra.Command {
	var page pageOptions
	var filters runFilters
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List the latest runs",
		Example: "  pt run list --server prod --status failed\n  pt run list --workflow \"Deploy App\"",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			params, err := filters.params(cmd.Context(), a)
			if err != nil {
				return err
			}
			opts := page.options()
			opts.Params = params
			return showList(cmd.Context(), a, runListing(opts))
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&filters.server, "server", "", "only runs on this server (name or id)")
	cmd.Flags().StringVar(&filters.action, "action", "", "only runs of this action (name or id)")
	cmd.Flags().StringVar(&filters.workflow, "workflow", "", "only runs of this workflow (name or id)")
	cmd.Flags().StringVar(&filters.pipe, "pipe", "", "only runs of this pipe (name or id)")
	cmd.Flags().StringVar(&filters.status, "status", "", "only runs with this status, e.g. running or failed")
	return cmd
}

// params resolves the filters into the query parameters of the run list.
func (f runFilters) params(ctx context.Context, a *App) (map[string]string, error) {
	resolver, err := a.Resolver(ctx)
	if err != nil {
		return nil, err
	}
	lookups := []struct {
		param, arg string
		id         func() (string, error)
	}{
		{"server", f.server, func() (string, error) { x, err := resolver.Server(ctx, f.server); return x.ID, err }},
		{"action", f.action, func() (string, error) { x, err := resolver.Action(ctx, f.action); return x.ID, err }},
		{"workflow", f.workflow, func() (string, error) { x, err := resolver.Workflow(ctx, f.workflow); return x.ID, err }},
		{"pipe", f.pipe, func() (string, error) { x, err := resolver.Pipe(ctx, f.pipe); return x.ID, err }},
	}

	params := map[string]string{"status": f.status}
	for _, lookup := range lookups {
		if lookup.arg == "" {
			continue
		}
		id, err := lookup.id()
		if err != nil {
			return nil, err
		}
		params[lookup.param] = id
	}
	return params, nil
}

// runListing lists runs.
func runListing(opts api.ListOptions) listing[api.Run] {
	return listing[api.Run]{
		path:    api.Runs,
		opts:    opts,
		headers: present.RunHeaders,
		row:     func(r api.Run) []string { return present.RunRow(r, time.Now()) },
		byTeam:  true,
		empty:   "No runs found.",
	}
}

// newRunViewCommand views a run with its steps, its log or its page in the web.
func newRunViewCommand(a *App) *cobra.Command {
	var log, web bool
	var server string
	cmd := &cobra.Command{
		Use:        "view <run>",
		SuggestFor: []string{"show"},
		Short:      "View a run with its steps, or its full log with --log",
		Example: "  pt run view 30denr\n" +
			"  pt run view 30denr --log --server prod > deploy.log\n" +
			"  pt run view 30denr --web",
		Args: argOrPick(a, "run"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			if server != "" && !log {
				return usageError("--server only applies to --log", "Use: pt run view <run> --log --server "+ui.Arg(server))
			}
			id, err := runOrPick(cmd.Context(), a, argv, false)
			switch {
			case err != nil:
				return err
			case web:
				return a.openWeb("runs/" + id)
			case log:
				return printRunLog(cmd.Context(), a, id, server)
			}
			return showRun(cmd.Context(), a, id)
		},
	}
	cmd.Flags().BoolVar(&log, "log", false, "print the full log of the run on one server")
	cmd.Flags().StringVar(&server, "server", "", "the server of the log, if the run ran on several (name or id)")
	addWebFlag(cmd, &web)
	return cmd
}

// newRunWatchCommand follows a run live until it finished.
func newRunWatchCommand(a *App) *cobra.Command {
	var flags followFlags
	cmd := &cobra.Command{
		Use:   "watch <run>",
		Short: "Follow a run live until it finishes",
		Long:  "Follow a run live: every step, its output and its result.\n\n" + followHelp,
		Args:  argOrPick(a, "run"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			id, err := runOrPick(cmd.Context(), a, argv, true)
			if err != nil {
				return err
			}
			return followRun(cmd.Context(), a, id, flags)
		},
	}
	addFollowFlags(cmd, &flags)
	return cmd
}

// runOrPick returns the run argument, or the id of a run the user picks: one of the latest runs,
// or with unfinished one of the runs which are pending or running.
func runOrPick(ctx context.Context, a *App, argv []string, unfinished bool) (string, error) {
	if id := optionalArg(argv); id != "" {
		return id, nil
	}
	runs, err := pickableRuns(ctx, a, unfinished)
	if err != nil {
		return "", err
	}

	now := time.Now()
	options := make([]prompt.Option, len(runs))
	for i, run := range runs {
		started := ui.Ago(ui.FirstTime(run.StartedAt, run.CreatedAt), now)
		options[i] = prompt.Option{Mark: present.StatusIcon(run.Status), Columns: []string{present.RunName(run), "#" + run.ID, present.RunBy(run), started}}
	}
	index, err := a.Prompter().Pick("Which run?", options)
	if err != nil {
		return "", err
	}
	return runs[index].ID, nil
}

// pickableRuns loads the latest runs, or the pending and running ones, of the team of --team.
func pickableRuns(ctx context.Context, a *App, unfinished bool) ([]api.Run, error) {
	client, err := a.Client()
	if err != nil {
		return nil, err
	}
	team, err := a.TeamID(ctx)
	if err != nil {
		return nil, err
	}

	statuses := []string{""}
	if unfinished {
		statuses = []string{api.StatusRunning, api.StatusPending}
	}
	var runs []api.Run
	for _, status := range statuses {
		found, err := api.ListAll[api.Run](ctx, client, api.Runs, api.ListOptions{Team: team, Params: map[string]string{"status": status}}, api.MaxLimit)
		if err != nil {
			return nil, err
		}
		runs = append(runs, found...)
	}

	if len(runs) == 0 && unfinished {
		return nil, fault.New(fault.Problem{Title: "No run is running", Hints: []fault.Hint{{Label: "See the latest runs", Value: "pt run list"}}, Code: fault.ExitAPI})
	}
	if len(runs) == 0 {
		return nil, fault.New(fault.Problem{Title: "No runs found", Code: fault.ExitAPI})
	}
	return runs, nil
}

// showRun prints the current state of a run once.
func showRun(ctx context.Context, a *App, id string) error {
	if a.Structured() {
		return orNotFound(a.printEndpoint(ctx, api.RunEndpoint(id, true)), resolve.KindRun, id)
	}
	client, err := a.Client()
	if err != nil {
		return err
	}
	run, err := client.Run(ctx, id, true)
	if err != nil {
		return orNotFound(err, resolve.KindRun, id)
	}
	a.IO.Println(runview.Render(run, a.IO.Width()))
	return nil
}

// newRunCancelCommand terminates a run.
func newRunCancelCommand(a *App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "cancel <run>",
		Short: "Cancel a run: terminate it on all servers",
		Args:  argOrPick(a, "run"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			id, err := runOrPick(cmd.Context(), a, argv, true)
			if err != nil {
				return err
			}
			if err := confirm(a, yes, "Terminate run #"+id+" on all servers?"); err != nil {
				return err
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			run, err := client.TerminateRun(cmd.Context(), id)
			if err != nil {
				return orNotFound(err, resolve.KindRun, id)
			}
			if a.Structured() {
				return a.PrintValue(run)
			}
			a.IO.Println(" " + ui.Warning.Render("■") + " Termination of run " + ui.Bold.Render("#"+run.ID) + " requested")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}

// printRunLog prints the full log of a run on one of its servers.
func printRunLog(ctx context.Context, a *App, runID, server string) error {
	client, err := a.Client()
	if err != nil {
		return err
	}
	run, err := client.Run(ctx, runID, false)
	if err != nil {
		return orNotFound(err, resolve.KindRun, runID)
	}
	execution, err := pickExecution(a, run, server)
	if err != nil {
		return err
	}
	log, err := client.Text(ctx, api.ExecutionLogEndpoint(execution))
	if err != nil {
		return err
	}
	_, err = io.WriteString(a.IO.RawOut, log)
	return err
}

// pickExecution chooses the execution of a run on the given server, the only one, or asks.
func pickExecution(a *App, run api.Run, server string) (string, error) {
	switch {
	case len(run.Executions) == 0:
		return "", fault.New(fault.Problem{Title: "Run #" + run.ID + " has no executions", Code: fault.ExitAPI})
	case server != "":
		return executionOn(run, server)
	case len(run.Executions) == 1:
		return run.Executions[0].ID, nil
	}

	prompter := a.Prompter()
	if prompter == nil {
		return "", usageError("Run #"+run.ID+" ran on several servers", "Choose one with --server: "+serverNames(run))
	}
	options := make([]prompt.Option, len(run.Executions))
	for i, e := range run.Executions {
		options[i] = prompt.Option{Mark: present.StatusIcon(e.Status), Columns: []string{e.Server.Name, e.Server.Host}}
	}
	index, err := prompter.Pick("Log of which server?", options)
	if err != nil {
		return "", err
	}
	return run.Executions[index].ID, nil
}

// executionOn finds the execution of the run on the server given by name or id.
func executionOn(run api.Run, server string) (string, error) {
	for _, e := range run.Executions {
		if strings.EqualFold(e.Server.Name, server) || e.Server.ID == server {
			return e.ID, nil
		}
	}
	return "", usageError("Run #"+run.ID+" did not run on “"+server+"”", "It ran on: "+serverNames(run))
}

// serverNames lists the servers a run ran on.
func serverNames(run api.Run) string {
	names := make([]string, len(run.Executions))
	for i, e := range run.Executions {
		names[i] = ui.Arg(e.Server.Name)
	}
	return strings.Join(names, ", ")
}

// confirm asks a yes/no question unless --yes was given; without a terminal --yes is required.
func confirm(a *App, yes bool, question string) error {
	if yes {
		return nil
	}
	prompter := a.Prompter()
	if prompter == nil {
		return usageError("Confirmation needed", "Add --yes to confirm without a prompt.")
	}
	ok, err := prompter.Confirm(question, "Yes", "No")
	if err != nil {
		return err
	}
	if !ok {
		return &fault.Silent{Code: fault.ExitInterrupted}
	}
	return nil
}
