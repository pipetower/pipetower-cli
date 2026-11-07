package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newTeamCommand groups the team commands.
func newTeamCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "team", Short: "List and view your teams, limit all commands to one of them"}
	cmd.AddCommand(newTeamListCommand(a), newViewCommand(a, resolve.KindTeam, "View the members of a team",
		(*resolve.Resolver).Team, func(t api.Team) string { return t.ID }, renderTeam), newTeamUseCommand(a))
	return cmd
}

// newTeamListCommand lists the teams of the user.
func newTeamListCommand(a *App) *cobra.Command {
	var page pageOptions
	var search string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your teams",
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showList(cmd.Context(), a, listing[api.Team]{
				path:    api.Teams,
				opts:    searchOptions(page, search),
				headers: []string{"TEAM", "MEMBERS", "ID"},
				row: func(t api.Team) []string {
					return []string{ui.Bold.Render(t.Name), fmt.Sprint(len(t.Members)), ui.Faint.Render(t.ID)}
				},
				empty: "You are in no team.",
			})
		},
	}
	addPageFlags(cmd, &page)
	addSearchFlag(cmd, &search, "teams")
	return cmd
}

// newPipeCommand groups the pipe commands.
func newPipeCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "pipe", Short: "List and view pipes (automations)"}

	var page pageOptions
	var search string
	list := &cobra.Command{
		Use:   "list",
		Short: "List pipes",
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showList(cmd.Context(), a, listing[api.Pipe]{
				path:    api.Pipes,
				opts:    searchOptions(page, search),
				headers: []string{"PIPE", "TRIGGER", "STEPS", "ACTIVE", "ID"},
				row: func(p api.Pipe) []string {
					return []string{ui.Bold.Render(p.Name), trigger(p.Trigger), fmt.Sprint(len(p.Steps)), active(p.Active), ui.Faint.Render(p.ID)}
				},
				byTeam: true,
				empty:  "No pipes yet.",
			})
		},
	}
	addPageFlags(list, &page)
	addSearchFlag(list, &search, "pipes")

	cmd.AddCommand(list, newViewCommand(a, resolve.KindPipe, "View the trigger and steps of a pipe",
		(*resolve.Resolver).Pipe, func(p api.Pipe) string { return p.ID }, renderPipe))
	return cmd
}

// newTemplateCommand groups the template commands.
func newTemplateCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "template", Short: "List, view, export, apply and delete file templates"}

	var page pageOptions
	var search string
	list := &cobra.Command{
		Use:   "list",
		Short: "List templates",
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showList(cmd.Context(), a, listing[api.Template]{
				path:    api.Templates,
				opts:    searchOptions(page, search),
				headers: []string{"TEMPLATE", "FILE", "UPDATED", "ID"},
				row: func(t api.Template) []string {
					return []string{ui.Bold.Render(t.Name), ui.Code.Render(t.FileName), ui.Muted.Render(ui.Ago(t.UpdatedAt, time.Now())), ui.Faint.Render(t.ID)}
				},
				byTeam: true,
				empty:  "No templates yet.",
			})
		},
	}
	addPageFlags(list, &page)
	addSearchFlag(list, &search, "templates")

	cmd.AddCommand(list, newViewCommand(a, resolve.KindTemplate, "View the content of a template",
		(*resolve.Resolver).Template, func(t api.Template) string { return t.ID }, renderTemplate))
	cmd.AddCommand(newDefinitionCommands(a, definitionKind[api.Template]{
		kind: resolve.KindTemplate, find: (*resolve.Resolver).Template,
		id: func(t api.Template) string { return t.ID }, name: func(t api.Template) string { return t.Name },
	})...)
	return cmd
}

// renderPipe renders the trigger and steps of a pipe.
func renderPipe(p api.Pipe, width int) string {
	lines := []string{"", " " + ui.Title.Render(p.Name) + "  " + ui.Muted.Render("Pipe · "+p.ID) + "  " + active(p.Active), ""}
	lines = append(lines, " "+ui.Bold.Render("Trigger")+"  "+trigger(p.Trigger), "", " "+ui.Bold.Render("Steps"))

	rows := make([][]string, len(p.Steps))
	for i, step := range p.Steps {
		rows[i] = []string{ui.Faint.Render(fmt.Sprint(step.Order + 1)), ui.Bold.Render(step.Type), ui.Muted.Render(strings.ReplaceAll(step.Event, "_", " "))}
	}
	if len(rows) == 0 {
		lines = append(lines, ui.Muted.Render("   none"))
	} else {
		lines = append(lines, ui.Indent(ui.Table([]string{"#", "TYPE", "EVENT"}, rows, width-2), "  "))
	}
	return strings.Join(lines, "\n") + "\n"
}

// renderTemplate renders a template: a header like the other views and its raw content, ready to copy.
func renderTemplate(t api.Template, _ int) string {
	return "\n " + ui.Title.Render(t.Name) + "  " + ui.Muted.Render("Template · "+t.ID+" · "+t.FileName) + "\n\n" + t.Content + "\n"
}

// trigger describes a pipe trigger.
func trigger(t api.PipeTrigger) string {
	return t.Type + ui.Muted.Render(" · "+strings.ReplaceAll(t.Event, "_", " "))
}

// active renders whether a pipe is active.
func active(isActive bool) string {
	if isActive {
		return ui.Accent.Render("● on") // cyan like the switch in the web app
	}
	return ui.Muted.Render("○ off")
}

// renderTeam renders the members of a team with their roles.
func renderTeam(t api.Team, width int) string {
	rows := make([][]string, len(t.Members))
	for i, m := range t.Members {
		twoFactor := ui.Danger.Render("✗") // like the web app: green when enabled, red when not
		if m.TwoFactor {
			twoFactor = ui.Success.Render("✓")
		}
		rows[i] = []string{ui.Bold.Render(m.Name), m.Email, m.Role, twoFactor}
	}
	return "\n " + ui.Title.Render(t.Name) + "  " + ui.Muted.Render("Team · "+t.ID) + "\n\n" +
		ui.Indent(ui.Table([]string{"MEMBER", "EMAIL", "ROLE", "2FA"}, rows, width-2), "  ") + "\n"
}
