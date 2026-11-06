package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// runnableKind describes the shared commands of actions and workflows.
type runnableKind struct {
	kind    resolve.Kind
	short   string
	resolve func(*resolve.Resolver, context.Context, string) (resolve.Runnable, error)
}

var (
	actionKind = runnableKind{
		kind:  resolve.KindAction,
		short: "List, view, run, export, apply and delete actions",
		resolve: func(r *resolve.Resolver, ctx context.Context, arg string) (resolve.Runnable, error) {
			action, err := r.Action(ctx, arg)
			return resolve.Runnable{Action: &action}, err
		},
	}
	workflowKind = runnableKind{
		kind:  resolve.KindWorkflow,
		short: "List, view, run, export, apply and delete workflows",
		resolve: func(r *resolve.Resolver, ctx context.Context, arg string) (resolve.Runnable, error) {
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

// newRunnableCommand builds list, view, run, export, apply, delete and example for actions or workflows.
func newRunnableCommand(a *App, kind runnableKind) *cobra.Command {
	cmd := &cobra.Command{Use: kind.kind.Singular, Short: kind.short}
	cmd.AddCommand(newRunnableListCommand(a, kind), newRunnableViewCommand(a, kind), newRunnableRunCommand(a, kind))
	cmd.AddCommand(newDefinitionCommands(a, definitionKind[resolve.Runnable]{kind: kind.kind, find: kind.resolve, id: resolve.Runnable.ID, name: resolve.Runnable.Name})...)
	return cmd
}

// newRunnableListCommand lists actions or workflows.
func newRunnableListCommand(a *App, kind runnableKind) *cobra.Command {
	var page pageOptions
	var search string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List " + kind.kind.Resource,
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := searchOptions(page, search)
			if kind.kind.Resource == api.Actions {
				return showList(cmd.Context(), a, actionListing(opts))
			}
			return showList(cmd.Context(), a, workflowListing(opts))
		},
	}
	addPageFlags(cmd, &page)
	addSearchFlag(cmd, &search, kind.kind.Plural)
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

// newRunnableViewCommand views the steps and variables of an action or workflow. The steps come
// from its definition, which names what a step uses (its template, its action) instead of ids.
func newRunnableViewCommand(a *App, kind runnableKind) *cobra.Command {
	noun := kind.kind.Singular
	var web bool
	cmd := &cobra.Command{
		Use:        "view <" + noun + ">",
		Short:      "View the steps and variables of " + withArticle(noun),
		SuggestFor: []string{"show"},
		Args:       argOrPick(a, noun),
		RunE: func(cmd *cobra.Command, argv []string) error {
			runnable, err := resolveRunnable(cmd.Context(), a, kind, optionalArg(argv))
			if err != nil {
				return err
			}
			if web {
				return a.openWeb(kind.kind.Resource + "/" + runnable.ID())
			}
			if a.Structured() {
				return a.printEndpoint(cmd.Context(), api.Show(kind.kind.Resource, runnable.ID()))
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			definition, err := api.Get[definitionSteps](cmd.Context(), client, api.Definition(kind.kind.Resource, runnable.ID()))
			if err != nil {
				return err
			}
			a.IO.Println(renderRunnable(runnable, definition, a.IO.TableWidth(0)))
			return nil
		},
	}
	addWebFlag(cmd, &web)
	return cmd
}

// definitionSteps are the steps of a definition, as far as the view shows them.
type definitionSteps struct {
	Steps []definitionStep `json:"steps"`
}

// definitionStep is one step of an action (name, type and what it does) or of a workflow (its action).
type definitionStep struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Command    string            `json:"command"`
	Script     string            `json:"script"`
	Template   any               `json:"template"`
	DstPath    string            `json:"dst_path"`
	File       string            `json:"file"`
	Action     string            `json:"action"`
	Variables  map[string]string `json:"variables"`
	Conditions [][]any           `json:"conditions"`
}

// newRunnableRunCommand runs an action or workflow.
func newRunnableRunCommand(a *App, kind runnableKind) *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "run <" + kind.kind.Singular + ">",
		Short: "Run " + withArticle(kind.kind.Singular) + " on servers and follow it live",
		Long: "Run " + withArticle(kind.kind.Singular) + " on servers and follow every step live. Servers are given by\n" +
			"name, id or tag:<tag>, missing variables are asked for.\n\n" + followHelp,
		Example: "  pt " + kind.kind.Singular + " run \"Deploy App\" -s prod -s tag:web -v branch=main",
		Args:    argOrPick(a, kind.kind.Singular),
		RunE: func(cmd *cobra.Command, argv []string) error {
			runnable, err := resolveRunnable(cmd.Context(), a, kind, optionalArg(argv))
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
	return kind.resolve(resolver, ctx, arg)
}

// renderRunnable renders the details of an action or workflow.
func renderRunnable(r resolve.Runnable, definition definitionSteps, width int) string {
	lines := []string{"", " " + ui.Title.Render(r.Name()) + "  " + ui.Muted.Render(r.Type()+" · "+r.ID())}
	if r.Action != nil && len(r.Action.Tags) > 0 {
		lines = append(lines, " "+tags(r.Action.Tags))
	}
	lines = append(lines, " "+ui.Muted.Render("Runs on ")+resolve.AllowedServerNames(r))

	lines = append(lines, "", ui.Bold.Render(" Variables"))
	lines = append(lines, renderVariables(r.Variables(), width)...)
	lines = append(lines, "", ui.Bold.Render(" Steps"))
	lines = append(lines, renderSteps(definition.Steps, width)...)

	return strings.Join(lines, "\n") + "\n"
}

// renderVariables lists the variables needed on execution.
func renderVariables(variables []api.VariableDefinition, width int) []string {
	if len(variables) == 0 {
		return []string{ui.Muted.Render("   none")}
	}
	rows := make([][]string, len(variables))
	for i, v := range variables {
		rows[i] = []string{ui.Bold.Render(v.Name), ui.Muted.Render(v.Type), strings.Join(selectionValues(v), ", ")}
	}
	return []string{ui.Indent(ui.Table([]string{"NAME", "TYPE", "VALUES"}, rows, width-2), "  ")}
}

// renderSteps lists the steps of an action or the actions of a workflow, with what each does and
// whether it has conditions.
func renderSteps(steps []definitionStep, width int) []string {
	if len(steps) == 0 {
		return []string{ui.Muted.Render("   none")}
	}
	rows := make([][]string, len(steps))
	for i, step := range steps {
		name, kind := step.Name, stepTypeLabel(step.Type)
		if step.Action != "" {
			name, kind = step.Action, "action"
		}
		rows[i] = []string{ui.Faint.Render(fmt.Sprint(i + 1)), ui.Bold.Render(name), ui.Muted.Render(kind), ui.Code.Render(stepSummary(step)), conditionsMark(step)}
	}
	return []string{ui.Indent(ui.Table([]string{"#", "STEP", "TYPE", "", "IF"}, rows, width-2), "  ")}
}

// stepTypeLabel turns execute_shell_script into "shell script".
func stepTypeLabel(stepType string) string {
	return strings.ReplaceAll(strings.TrimPrefix(stepType, "execute_"), "_", " ")
}

// stepSummary shows the essential setting of a step on one line: the first line of its command
// or script, the template and where it goes, the modified file, or the fixed variables of an action.
func stepSummary(step definitionStep) string {
	switch {
	case step.Command != "":
		return firstLine(step.Command)
	case step.Script != "":
		return firstLine(step.Script)
	case step.Template != nil:
		return templateName(step.Template) + " → " + step.DstPath
	case step.File != "":
		return step.File
	case len(step.Variables) > 0:
		return fixedVariables(step.Variables)
	}
	return ""
}

// firstLine is the first line of a text which says something (not the shebang), shortened.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#!") {
			continue
		}
		if len([]rune(line)) > 60 {
			line = string([]rune(line)[:59]) + "…"
		}
		return line
	}
	return ""
}

// templateName is the name of a referenced or an embedded template.
func templateName(template any) string {
	switch t := template.(type) {
	case string:
		return t
	case map[string]any:
		name, _ := t["name"].(string)
		return name
	}
	return ""
}

// fixedVariables lists the fixed variable values of a workflow step, sorted by name.
func fixedVariables(variables map[string]string) string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + "=" + variables[name]
	}
	return strings.Join(parts, " ")
}

// conditionsMark tells how many conditions a step has, nothing without conditions.
func conditionsMark(step definitionStep) string {
	count := 0
	for _, group := range step.Conditions {
		count += len(group)
	}
	if count == 0 {
		return ""
	}
	return ui.Muted.Render(fmt.Sprint(count))
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

// withArticle puts the indefinite article before a noun: "an action", "a workflow".
func withArticle(noun string) string {
	if strings.ContainsRune("aeiou", rune(noun[0])) {
		return "an " + noun
	}
	return "a " + noun
}
