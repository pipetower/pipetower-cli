package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/runview"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newRunsCommand groups the commands about existing runs.
func newRunsCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List, follow, cancel and export runs",
	}
	cmd.AddCommand(newRunsListCommand(a), newRunsShowCommand(a), newRunsCancelCommand(a), newRunsLogCommand(a))
	return cmd
}

// newRunsListCommand lists the latest runs.
func newRunsListCommand(a *App) *cobra.Command {
	var page pageOptions
	var status string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the latest runs",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := page.options()
			opts.Params = map[string]string{"status": status}
			return showList(cmd.Context(), a, runListing(api.Runs, opts))
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&status, "status", "", "only runs with this status, e.g. running or failed")
	return cmd
}

// newRunsShowCommand shows a run or follows it live.
func newRunsShowCommand(a *App) *cobra.Command {
	var follow bool
	var flags followFlags
	cmd := &cobra.Command{
		Use:     "show <run>",
		Short:   "Show a run, or follow it live with --follow",
		Example: "  pt runs show 30denr\n  pt runs show 30denr -f",
		Args:    args("run"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			if follow {
				return followRun(cmd.Context(), a, argv[0], flags)
			}
			return showRun(cmd.Context(), a, argv[0])
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the run live until it finished")
	addFollowFlags(cmd, &flags)
	return cmd
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

// newRunsCancelCommand terminates a run.
func newRunsCancelCommand(a *App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "cancel <run>",
		Aliases: []string{"terminate"},
		Short:   "Terminate a run on all servers",
		Args:    args("run"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			if err := confirm(a, yes, "Terminate run #"+argv[0]+" on all servers?"); err != nil {
				return err
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			run, err := client.TerminateRun(cmd.Context(), argv[0])
			if err != nil {
				return orNotFound(err, resolve.KindRun, argv[0])
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

// newRunsLogCommand prints the full text log of a run execution.
func newRunsLogCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "log <run-or-execution>",
		Short:   "Print the full log of a run execution (one server of a run)",
		Example: "  pt runs log 0r76v1 > deploy.log\n  pt runs log 30denr",
		Args:    args("run-or-execution"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			id, err := executionID(cmd.Context(), a, argv[0])
			if err != nil {
				return err
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			log, err := client.Text(cmd.Context(), api.ExecutionLogEndpoint(id))
			if err != nil {
				return err
			}
			_, err = io.WriteString(a.IO.RawOut, log)
			return err
		},
	}
}

// executionID accepts an execution id, or a run id whose execution is picked.
func executionID(ctx context.Context, a *App, id string) (string, error) {
	client, err := a.Client()
	if err != nil {
		return "", err
	}
	if _, err := api.Get[api.Execution](ctx, client, api.ExecutionEndpoint(id, false)); !api.IsNotFound(err) {
		return id, err
	}

	run, err := client.Run(ctx, id, false)
	if err != nil {
		return "", orNotFound(err, resolve.KindRunOrExecution, id)
	}
	return pickExecution(a, run)
}

// pickExecution chooses the execution of a run, asking when there are several.
func pickExecution(a *App, run api.Run) (string, error) {
	switch {
	case len(run.Executions) == 0:
		return "", fault.New(fault.Problem{Title: "Run #" + run.ID + " has no executions", Code: fault.ExitAPI})
	case len(run.Executions) == 1:
		return run.Executions[0].ID, nil
	}

	labels := make([]string, len(run.Executions))
	for i, e := range run.Executions {
		labels[i] = fmt.Sprintf("%s  %s  %s", e.Server.Name, ui.Faint.Render(e.Server.Host), present.StatusBadge(e.Status))
	}
	prompter := a.Prompter()
	if prompter == nil {
		return "", usageError("Run #"+run.ID+" ran on several servers", "Give the execution id instead:\n  "+executionList(run))
	}
	index, err := prompter.Select("Log of which server?", labels)
	if err != nil {
		return "", err
	}
	return run.Executions[index].ID, nil
}

// executionList lists the executions of a run with their ids.
func executionList(run api.Run) string {
	text := ""
	for i, e := range run.Executions {
		if i > 0 {
			text += "\n  "
		}
		text += e.ID + "  " + e.Server.Name
	}
	return text
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
