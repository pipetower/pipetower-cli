package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newTeamCommand groups the team commands.
func newTeamCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "team", Short: "List your teams and their members"}
	cmd.AddCommand(newTeamListCommand(a))
	return cmd
}

// newTeamListCommand lists the teams of the user.
func newTeamListCommand(a *App) *cobra.Command {
	var page pageOptions
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your teams",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showList(cmd.Context(), a, listing[api.Team]{
				path:    api.Teams,
				opts:    page.options(),
				headers: []string{"TEAM", "MEMBERS", "ID"},
				row: func(t api.Team) []string {
					return []string{ui.Bold.Render(t.Name), memberNames(t.Members), ui.Faint.Render(t.ID)}
				},
				empty: "You are in no team.",
			})
		},
	}
	addPageFlags(cmd, &page)
	return cmd
}

// newPipeCommand groups the pipe commands.
func newPipeCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "pipe", Short: "List and inspect pipes (automations)"}

	var page pageOptions
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List pipes",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showList(cmd.Context(), a, listing[api.Pipe]{
				path:    api.Pipes,
				opts:    page.options(),
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

	show := &cobra.Command{
		Use:   "show <pipe>",
		Short: "Show the trigger and steps of a pipe",
		Args:  args("pipe"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			resolver, err := a.Resolver(cmd.Context())
			if err != nil {
				return err
			}
			pipe, err := resolver.Pipe(cmd.Context(), argv[0])
			if err != nil {
				return err
			}
			return showOne(cmd.Context(), a, pipe, api.Show(api.Pipes, pipe.ID), renderPipe)
		},
	}

	cmd.AddCommand(list, show)
	return cmd
}

// newTemplateCommand groups the template commands.
func newTemplateCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "template", Short: "List and print file templates"}

	var page pageOptions
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List templates",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showList(cmd.Context(), a, listing[api.Template]{
				path:    api.Templates,
				opts:    page.options(),
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

	show := &cobra.Command{
		Use:   "show <template>",
		Short: "Print the content of a template",
		Args:  args("template"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			resolver, err := a.Resolver(cmd.Context())
			if err != nil {
				return err
			}
			template, err := resolver.Template(cmd.Context(), argv[0])
			if err != nil {
				return err
			}
			return showOne(cmd.Context(), a, template, api.Show(api.Templates, template.ID), renderTemplate)
		},
	}

	cmd.AddCommand(list, show)
	return cmd
}

// renderPipe renders the trigger and steps of a pipe.
func renderPipe(p api.Pipe) string {
	lines := []string{"", " " + ui.Title.Render(p.Name) + "  " + ui.Muted.Render("Pipe · "+p.ID) + "  " + active(p.Active), ""}
	lines = append(lines, " "+ui.Bold.Render("Trigger")+"  "+trigger(p.Trigger), "", " "+ui.Bold.Render("Steps"))

	rows := make([][]string, len(p.Steps))
	for i, step := range p.Steps {
		rows[i] = []string{ui.Faint.Render(fmt.Sprint(step.Order + 1)), ui.Bold.Render(step.Type), ui.Muted.Render(strings.ReplaceAll(step.Event, "_", " "))}
	}
	if len(rows) == 0 {
		lines = append(lines, ui.Muted.Render("   none"))
	} else {
		lines = append(lines, indentBlock(ui.Table([]string{"#", "TYPE", "EVENT"}, rows), "  "))
	}
	return strings.Join(lines, "\n") + "\n"
}

// renderTemplate renders a template: a header and its raw content.
func renderTemplate(t api.Template) string {
	return ui.Title.Render(t.Name) + "  " + ui.Code.Render(t.FileName) + "\n" + ui.Faint.Render(strings.Repeat("─", 40)) + "\n" + t.Content
}

// trigger describes a pipe trigger.
func trigger(t api.PipeTrigger) string {
	return t.Type + ui.Muted.Render(" · "+strings.ReplaceAll(t.Event, "_", " "))
}

// active renders whether a pipe is active.
func active(isActive bool) string {
	if isActive {
		return ui.Success.Render("● active")
	}
	return ui.Faint.Render("○ inactive")
}

// memberNames lists the members of a team with their roles.
func memberNames(members []api.Member) string {
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.Name + ui.Faint.Render(" ("+m.Role+")")
	}
	return strings.Join(names, ", ")
}
