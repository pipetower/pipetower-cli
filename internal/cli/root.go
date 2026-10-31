package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// Command groups shown in the help.
const (
	groupRun       = "run"
	groupResources = "resources"
	groupMore      = "more"
)

// NewRootCommand builds the command tree of pt.
func NewRootCommand(a *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "pt",
		Short: "Run actions and workflows on your servers, in real time",
		Long: "pt is the command line of Pipetower: run actions and workflows on your servers and follow\n" +
			"them live, check server health, and open shared terminal sessions.\n\n" +
			"pt reads PIPETOWER_URL and PIPETOWER_TOKEN from a .pipetower file in this directory,\n" +
			"a parent directory or your home directory (or from the environment).",
		Example: "  pt server list\n" +
			"  pt run \"Deploy App\" --server prod -v branch=main\n" +
			"  pt ssh prod",
		PersistentPreRunE: func(*cobra.Command, []string) error { return a.setup() },
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&a.flags.output, "output", "o", FormatTable, "output format: table, json or yaml")
	pf.BoolVar(&a.flags.noInput, "no-input", false, "never prompt, fail instead")
	pf.BoolVar(&a.flags.noColor, "no-color", false, "disable colors (also via NO_COLOR)")
	pf.StringVar(&a.flags.team, "team", "", "only show resources of this team (id or name)")

	root.AddGroup(
		&cobra.Group{ID: groupRun, Title: "Run and follow"},
		&cobra.Group{ID: groupResources, Title: "Resources"},
		&cobra.Group{ID: groupMore, Title: "More"},
	)
	root.AddCommand(
		inGroup(groupRun, newRunCommand(a), newRunsCommand(a), newSSHCommand(a), newTerminalCommand(a))...,
	)
	root.AddCommand(
		inGroup(groupResources, newServerCommand(a), newActionCommand(a), newWorkflowCommand(a), newPipeCommand(a), newTemplateCommand(a), newTeamCommand(a))...,
	)
	root.AddCommand(
		inGroup(groupMore, newStatusCommand(a), newAPICommand(a), newVersionCommand(a))...,
	)
	root.AddCommand(listAliases(a)...)
	root.SetHelpCommandGroupID(groupMore)
	root.SetCompletionCommandGroupID(groupMore)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(ui.Capitalize(err.Error()), "See: "+cmd.CommandPath()+" --help")
	})

	return root
}

// inGroup assigns commands to a help group.
func inGroup(group string, commands ...*cobra.Command) []*cobra.Command {
	for _, cmd := range commands {
		cmd.GroupID = group
	}
	return commands
}

// listAliases are hidden plural shortcuts: pt servers = pt server list.
func listAliases(a *App) []*cobra.Command {
	aliases := map[string]func(*App) *cobra.Command{
		"servers":   newServerListCommand,
		"actions":   func(a *App) *cobra.Command { return newRunnableListCommand(a, actionKind) },
		"workflows": func(a *App) *cobra.Command { return newRunnableListCommand(a, workflowKind) },
		"teams":     newTeamListCommand,
	}

	commands := make([]*cobra.Command, 0, len(aliases))
	for name, build := range aliases {
		cmd := build(a)
		cmd.Use, cmd.Hidden = name, true
		commands = append(commands, cmd)
	}
	return commands
}

// HandleError renders an error on stderr, already shown or canceled errors stay quiet.
func (a *App) HandleError(w io.Writer, err error) {
	if isQuiet(err) {
		return
	}
	_, _ = fmt.Fprint(w, ui.RenderProblem(a.problemOf(err)))
}

// problemOf explains an error: usage errors of cobra get a help hint, errors caused by the
// configuration tell where the URL and the token came from.
func (a *App) problemOf(err error) fault.Problem {
	if isUsage(err) {
		return fault.Problem{Title: ui.Capitalize(err.Error()), Hints: []fault.Hint{{Label: "See", Value: "pt --help"}}, Code: fault.ExitUsage}
	}

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
	case isUsage(err):
		return fault.ExitUsage
	}
	return fault.ExitCode(err)
}

// isQuiet tells whether the error needs no message.
func isQuiet(err error) bool {
	var silent *fault.Silent
	return errors.As(err, &silent) || errors.Is(err, context.Canceled)
}

// isUsage recognizes the only usage error cobra reports as plain error: an unknown command. Flag
// errors are typed by the flag error function, arguments by args (TestUnknownCommandIsAUsageError
// notices if cobra changes the message).
func isUsage(err error) bool {
	var explainer fault.Explainer
	return !errors.As(err, &explainer) && strings.HasPrefix(err.Error(), "unknown command")
}

// args requires exactly the named positional arguments and explains missing ones.
func args(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, given []string) error {
		if len(given) == len(names) {
			return nil
		}
		usage := cmd.CommandPath() + " <" + strings.Join(names, "> <") + ">"
		if len(given) < len(names) {
			return usageError("Missing "+names[len(given)], "Usage: "+usage)
		}
		return usageError(fmt.Sprintf("Too many arguments: %s", strings.Join(given[len(names):], " ")), "Usage: "+usage)
	}
}
