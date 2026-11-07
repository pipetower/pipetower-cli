package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// NewRootCommand builds the command tree of pt.
func NewRootCommand(a *App) *cobra.Command {
	root := &cobra.Command{
		Use: "pt",
		Example: "  pt server list\n" +
			"  pt workflow run \"Deploy App\" -s prod -v branch=main\n" +
			"  pt terminal open prod",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.setup(); err != nil {
				return err
			}
			return a.offerUpdate(cmd)
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&a.flags.output, "output", "o", FormatTable, "output format: table, json or yaml")
	pf.BoolVar(&a.flags.noInput, "no-input", false, "never prompt, fail instead")
	pf.BoolVar(&a.flags.noColor, "no-color", false, "disable colors (also via NO_COLOR)")
	pf.StringVar(&a.flags.team, "team", "", "only resources of this team (name or id) for this command, see pt team use")
	pf.BoolVar(&a.flags.allTeams, "all-teams", false, "resources of all your teams for this command, also where a team is set")
	pf.BoolP("help", "h", false, "show the help")

	root.AddCommand(
		newActionCommand(a), newWorkflowCommand(a), newRunCommand(a), newTerminalCommand(a),
		newServerCommand(a), newVariableCommand(a), newPipeCommand(a), newTemplateCommand(a), newTeamCommand(a),
		newEnvCommand(a), newStatusCommand(a), newAPICommand(a), newUpdateCommand(a), newVersionCommand(a), newCompletionCommand(),
	)
	// one way for each task: --help instead of a help command, our completion command instead of cobra's
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	root.SetHelpFunc(helpFunc(a))
	root.SilenceErrors, root.SilenceUsage = true, true
	root.CompletionOptions.DisableDefaultCmd = true
	forEachCommand(root, rejectUnknownSubcommand)
	addCompletions(a, root)
	showLogo(a, root)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageProblem(cmd, ui.Capitalize(err.Error()))
	})

	return root
}

// HandleError renders an error on stderr, already shown or canceled errors stay quiet.
func (a *App) HandleError(w io.Writer, err error) {
	if isQuiet(err) {
		return
	}
	_, _ = fmt.Fprint(w, ui.RenderProblem(a.problemOf(err)))
}

// problemOf explains an error, errors caused by the configuration tell where the URL and the token came from.
func (a *App) problemOf(err error) fault.Problem {
	problem := ui.ProblemOf(err)
	if a.cfg != nil && isConfigurationProblem(err) {
		problem.Hints = append(problem.Hints, fault.Hint{Label: "Configured in", Value: a.configSources()})
	}
	return problem
}

// isConfigurationProblem tells whether the URL or the token is a likely cause of the error.
func isConfigurationProblem(err error) bool {
	var apiErr *api.Error
	var connErr *api.ConnectionError
	var redirectErr *api.RedirectError
	return (errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized) || errors.As(err, &connErr) || errors.As(err, &redirectErr)
}

// configSources names where URL and token came from.
func (a *App) configSources() string {
	if a.cfg.URLSource == a.cfg.TokenSource {
		return a.cfg.URLSource.String()
	}
	return "URL from " + a.cfg.URLSource.String() + ", token from " + a.cfg.TokenSource.String()
}

// ExitCode returns the process exit code for the result of a command.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return fault.ExitOK
	case errors.Is(err, context.Canceled):
		return fault.ExitInterrupted
	}
	return fault.ExitCode(err)
}

// isQuiet tells whether the error needs no message.
func isQuiet(err error) bool {
	var silent *fault.Silent
	return errors.As(err, &silent) || errors.Is(err, context.Canceled)
}

// forEachCommand calls the function for the command and all its subcommands.
func forEachCommand(cmd *cobra.Command, call func(*cobra.Command)) {
	call(cmd)
	for _, child := range cmd.Commands() {
		forEachCommand(child, call)
	}
}

// showLogo prints the logo of Pipetower above the help when pt runs without a command and the terminal is wide enough.
func showLogo(a *App, root *cobra.Command) {
	help := root.RunE
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if logo := ui.Logo(); len(args) == 0 && lipgloss.Width(logo) <= a.IO.Width() {
			a.IO.Println("\n" + logo) // only where it fits, a wrapped logo is broken
		}
		return help(cmd, args)
	}
}

// rejectUnknownSubcommand lets a command which only groups subcommands (pt itself too) print its
// help without arguments, and reject an unknown subcommand with suggestions.
func rejectUnknownSubcommand(cmd *cobra.Command) {
	if !cmd.HasSubCommands() || cmd.Runnable() {
		return
	}
	cmd.Args = cobra.ArbitraryArgs
	cmd.SuggestionsMinimumDistance = 2 // cobra's default, applied by cobra only on its own error path
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return unknownCommand(cmd, args[0])
	}
}

// unknownCommand explains an unknown subcommand and suggests the intended one, e.g. view for show.
func unknownCommand(cmd *cobra.Command, name string) error {
	problem := fault.Problem{Title: fmt.Sprintf("Unknown command “%s” for “%s”", name, cmd.CommandPath()), Code: fault.ExitUsage}
	for _, suggestion := range cmd.SuggestionsFor(name) {
		problem.Hints = append(problem.Hints, fault.Hint{Label: "Did you mean", Value: cmd.CommandPath() + " " + suggestion})
	}
	problem.Hints = append(problem.Hints, fault.Hint{Label: "See", Value: cmd.CommandPath() + " --help"})
	return fault.New(problem)
}
