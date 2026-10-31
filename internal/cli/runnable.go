package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// runnableKind describes the shared commands of actions and workflows.
type runnableKind struct {
	name     string
	resource string
	short    string
	resolve  func(context.Context, *resolve.Resolver, string) (resolve.Runnable, error)
}

var (
	actionKind = runnableKind{
		name:     "action",
		resource: api.Actions,
		short:    "List, inspect and run actions",
		resolve: func(ctx context.Context, r *resolve.Resolver, arg string) (resolve.Runnable, error) {
			action, err := r.Action(ctx, arg)
			return resolve.Runnable{Action: &action}, err
		},
	}
	workflowKind = runnableKind{
		name:     "workflow",
		resource: api.Workflows,
		short:    "List, inspect and run workflows",
		resolve: func(ctx context.Context, r *resolve.Resolver, arg string) (resolve.Runnable, error) {
			workflow, err := r.Workflow(ctx, arg)
			return resolve.Runnable{Workflow: &workflow}, err
		},
	}
)

// newActionCommand groups the action commands.
func newActionCommand(a *App) *cobra.Command {
	return newRunnableCommand(a, actionKind)
}

// newWorkflowCommand groups the workflow commands.
func newWorkflowCommand(a *App) *cobra.Command {
	return newRunnableCommand(a, workflowKind)
}

// newRunnableCommand builds list, show and run for actions or workflows.
func newRunnableCommand(a *App, kind runnableKind) *cobra.Command {
	cmd := &cobra.Command{Use: kind.name, Short: kind.short}
	cmd.AddCommand(newRunnableListCommand(a, kind), newRunnableShowCommand(a, kind), newRunnableRunCommand(a, kind))
	return cmd
}

// newRunnableListCommand lists actions or workflows.
func newRunnableListCommand(a *App, kind runnableKind) *cobra.Command {
	var page pageOptions
	var search string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List " + kind.resource,
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := page.options()
			opts.Search = search
			if kind.resource == api.Actions {
				return showList(cmd.Context(), a, actionListing(opts))
			}
			return showList(cmd.Context(), a, workflowListing(opts))
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&search, "search", "", "only "+kind.resource+" whose name contains this text")
	return cmd
}

// actionListing lists actions with their steps and variables.
func actionListing(opts api.ListOptions) listing[api.Action] {
	return listing[api.Action]{
		path:    api.Actions,
		opts:    opts,
		headers: []string{"ACTION", "STEPS", "VARIABLES", "TAGS", "ID"},
		row: func(x api.Action) []string {
			return []string{ui.Bold.Render(x.Name), fmt.Sprint(len(x.Steps)), variableNames(x.Variables), tags(x.Tags), ui.Faint.Render(x.ID)}
		},
		byTeam: true,
		empty:  "No actions yet.",
	}
}

// workflowListing lists workflows with their steps and variables.
func workflowListing(opts api.ListOptions) listing[api.Workflow] {
	return listing[api.Workflow]{
		path:    api.Workflows,
		opts:    opts,
		headers: []string{"WORKFLOW", "STEPS", "VARIABLES", "TAGS", "ID"},
		row: func(x api.Workflow) []string {
			return []string{ui.Bold.Render(x.Name), fmt.Sprint(len(x.Steps)), variableNames(x.Variables), tags(x.Tags), ui.Faint.Render(x.ID)}
		},
		byTeam: true,
		empty:  "No workflows yet.",
	}
}

// newRunnableShowCommand shows the steps and variables of an action or workflow.
func newRunnableShowCommand(a *App, kind runnableKind) *cobra.Command {
	return &cobra.Command{
		Use:   "show <" + kind.name + ">",
		Short: "Show the steps and variables of a " + kind.name,
		Args:  args(kind.name),
		RunE: func(cmd *cobra.Command, argv []string) error {
			runnable, err := resolveRunnable(cmd.Context(), a, kind, argv[0])
			if err != nil {
				return err
			}
			return showOne(cmd.Context(), a, runnable, api.Show(kind.resource, runnable.ID()), renderRunnable)
		},
	}
}

// newRunnableRunCommand runs an action or workflow.
func newRunnableRunCommand(a *App, kind runnableKind) *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:     "run <" + kind.name + ">",
		Short:   "Run a " + kind.name + " on servers and follow it live",
		Example: "  pt " + kind.name + " run \"Deploy App\" -s prod -s tag:web -v branch=main",
		Args:    args(kind.name),
		RunE: func(cmd *cobra.Command, argv []string) error {
			runnable, err := resolveRunnable(cmd.Context(), a, kind, argv[0])
			if err != nil {
				return err
			}
			return startRun(cmd.Context(), a, runnable, flags)
		},
	}
	addRunFlags(cmd, &flags)
	return cmd
}

// resolveRunnable resolves an action or workflow argument.
func resolveRunnable(ctx context.Context, a *App, kind runnableKind, arg string) (resolve.Runnable, error) {
	resolver, err := a.Resolver(ctx)
	if err != nil {
		return resolve.Runnable{}, err
	}
	return kind.resolve(ctx, resolver, arg)
}

// renderRunnable renders the details of an action or workflow.
func renderRunnable(r resolve.Runnable) string {
	lines := []string{"", " " + ui.Title.Render(r.Name()) + "  " + ui.Muted.Render(r.Type()+" · "+r.ID())}
	if r.Action != nil && len(r.Action.Tags) > 0 {
		lines = append(lines, " "+tags(r.Action.Tags))
	}

	lines = append(lines, "", ui.Bold.Render(" Variables"))
	lines = append(lines, renderVariables(r.Variables())...)
	lines = append(lines, "", ui.Bold.Render(" Steps"))
	lines = append(lines, renderSteps(r)...)

	return strings.Join(lines, "\n") + "\n"
}

// renderVariables lists the variables needed on execution.
func renderVariables(variables []api.VariableDefinition) []string {
	if len(variables) == 0 {
		return []string{ui.Muted.Render("   none")}
	}
	rows := make([][]string, len(variables))
	for i, v := range variables {
		rows[i] = []string{ui.Bold.Render(v.Name), ui.Muted.Render(v.Type), v.Values}
	}
	return []string{indentBlock(ui.Table([]string{"NAME", "TYPE", "VALUES"}, rows), "  ")}
}

// renderSteps lists the steps of an action or the actions of a workflow.
func renderSteps(r resolve.Runnable) []string {
	var rows [][]string
	if r.Action != nil {
		for _, step := range r.Action.Steps {
			rows = append(rows, []string{ui.Faint.Render(fmt.Sprint(step.Order + 1)), ui.Bold.Render(step.Name), ui.Muted.Render(stepTypeLabel(step.Type)), ui.Code.Render(stepSummary(step.Execution))})
		}
	} else {
		for _, step := range r.Workflow.Steps {
			name := "?"
			if step.Action != nil {
				name = step.Action.Name
			}
			rows = append(rows, []string{ui.Faint.Render(fmt.Sprint(step.Order + 1)), ui.Bold.Render(name), ui.Muted.Render("action")})
		}
	}
	if len(rows) == 0 {
		return []string{ui.Muted.Render("   none")}
	}
	return []string{indentBlock(ui.Table([]string{"#", "STEP", "TYPE", ""}[:len(rows[0])], rows), "  ")}
}

// stepTypeLabel turns execute_shell_script into "shell script".
func stepTypeLabel(stepType string) string {
	return strings.ReplaceAll(strings.TrimPrefix(stepType, "execute_"), "_", " ")
}

// stepSummary shows the essential setting of a step, like its command, on one line.
func stepSummary(execution map[string]any) string {
	for _, key := range []string{"command", "script", "path", "destination", "file", "template"} {
		if value, ok := execution[key].(string); ok && value != "" {
			line := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(value, "#!/bin/bash\n"), "\n", 2)[0])
			if len([]rune(line)) > 60 {
				line = string([]rune(line)[:59]) + "…"
			}
			return line
		}
	}
	return ""
}

// variableNames lists the variable names of a runnable.
func variableNames(variables []api.VariableDefinition) string {
	names := make([]string, len(variables))
	for i, v := range variables {
		names[i] = v.Name
	}
	return ui.Muted.Render(strings.Join(names, ", "))
}

// tags renders tags in their colors.
func tags(list []api.Tag) string {
	rendered := make([]string, len(list))
	for i, tag := range list {
		rendered[i] = ui.Tag(tag.Name, tag.Color)
	}
	return strings.Join(rendered, " ")
}

// indentBlock indents every line of a block.
func indentBlock(text, prefix string) string {
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}
