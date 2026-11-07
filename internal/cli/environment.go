package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/config"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newEnvCommand groups the environment commands.
func newEnvCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "List the configured environments (instances) and switch between them"}
	cmd.AddCommand(newEnvListCommand(a), newEnvUseCommand(a))
	return cmd
}

// newEnvListCommand lists the environments of the configuration file with the active one marked.
func newEnvListCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the environments, the active one marked",
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.environments()
			if err != nil {
				return err
			}
			if a.Structured() {
				return a.PrintValue(environmentList(cfg))
			}
			rows := make([][]string, 0, len(cfg.Environments))
			for _, name := range cfg.EnvironmentNames() {
				env := cfg.Environments[name]
				mark, label := " ", name
				if name == cfg.Environment {
					mark, label = ui.Accent.Render("●"), ui.Bold.Render(name)
				}
				rows = append(rows, []string{mark, label, env.URL, ui.Muted.Render(ui.Or(env.Team, "all"))})
			}
			a.IO.Println(table(a, []string{"", "ENVIRONMENT", "URL", "TEAM"}, rows))
			a.IO.Errorln(ui.Faint.Render(" Configured in " + cfg.EnvironmentsPath + " · switch with pt env use <environment>"))
			return nil
		},
	}
}

// environmentList is the structured output of the environment list.
func environmentList(cfg *config.Config) []map[string]any {
	var list []map[string]any
	for _, name := range cfg.EnvironmentNames() {
		env := cfg.Environments[name]
		list = append(list, map[string]any{"name": name, "url": env.URL, "team": env.Team, "active": name == cfg.Environment})
	}
	return list
}

// newEnvUseCommand makes an environment the active one, in the file which holds the environments.
func newEnvUseCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "use <environment>",
		Short: "Make an environment the active one for all following commands",
		Long: "Make an environment the active one: all following commands talk to its instance with its\n" +
			"token, until another one is activated. It is written to the file which holds the environments.",
		Example: "  pt env use local",
		Args:    argOrPick(a, "environment"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			cfg, err := a.environments()
			if err != nil {
				return err
			}
			name, err := pickEnvironment(a, cfg, optionalArg(argv))
			if err != nil {
				return err
			}
			if err := config.SetEnvironment(cfg.EnvironmentsPath, name); err != nil {
				return err
			}
			env := cfg.Environments[name]
			a.IO.Println(" " + ui.Success.Render("✓") + " Using " + ui.Bold.Render(name) + ui.Muted.Render("  "+env.URL+" · team "+ui.Or(env.Team, "all")))
			if cfg.EnvironmentSource.Env != "" {
				a.IO.Errorln(ui.RenderWarning("$" + cfg.EnvironmentSource.Env + " is set and overrides the file, unset it to use " + name))
			}
			return nil
		},
	}
}

// pickEnvironment returns the given environment, or lets the user pick one of the configured ones.
func pickEnvironment(a *App, cfg *config.Config, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	names := cfg.EnvironmentNames()
	options := make([]prompt.Option, len(names))
	for i, name := range names {
		mark := " "
		if name == cfg.Environment {
			mark = "●"
		}
		options[i] = prompt.Option{Mark: mark, Columns: []string{name, cfg.Environments[name].URL}}
	}
	index, err := a.Prompter().Pick("Which environment?", options)
	if err != nil {
		return "", err
	}
	return names[index], nil
}

// newTeamUseCommand limits all following commands to a team, or lifts the limit with --all.
func newTeamUseCommand(a *App) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "use <team>",
		Short: "Limit all following commands to a team, or --all for all teams",
		Long: "Limit all following commands to one team: lists show only its resources, names are looked up\n" +
			"in it and new objects are created in it. --all lifts the limit. The team is written to the\n" +
			"active environment in the configuration file, --team and --all-teams override it per command.",
		Example: "  pt team use Ops\n  pt team use --all",
		Args: func(cmd *cobra.Command, argv []string) error {
			if all && len(argv) == 0 {
				return nil
			}
			return argOrPick(a, "team")(cmd, argv)
		},
		RunE: func(cmd *cobra.Command, argv []string) error {
			cfg, err := a.environments()
			if err != nil {
				return err
			}
			if cfg.Environment == "" {
				return fault.New(fault.Problem{Title: "No environment to limit", Detail: "URL and token come from $" + config.EnvToken + ", use $" + config.EnvTeam + " to limit the team.", Code: fault.ExitConfig})
			}

			team := ""
			if !all {
				client, err := a.Client()
				if err != nil {
					return err
				}
				// any of the user's teams, not only the one currently set; without a name the user picks one
				found, err := (&resolve.Resolver{Client: client, Prompter: a.Prompter()}).Team(cmd.Context(), optionalArg(argv))
				if err != nil {
					return err
				}
				team = found.Name
			}
			if err := config.SetTeam(cfg.EnvironmentsPath, cfg.Environment, team); err != nil {
				return err
			}

			if all {
				a.IO.Println(" " + ui.Success.Render("✓") + " Not limited to a team in " + ui.Bold.Render(cfg.Environment) + ui.Muted.Render(", resources of all your teams"))
			} else {
				a.IO.Println(" " + ui.Success.Render("✓") + " Limited to " + ui.Bold.Render(team) + ui.Muted.Render(" in "+cfg.Environment))
			}
			if cfg.TeamSource.Env != "" || (cfg.TeamSource.Path != "" && cfg.TeamSource.Path != cfg.EnvironmentsPath) {
				a.IO.Errorln(ui.RenderWarning(cfg.TeamSource.String() + " sets the team " + cfg.Team + " and overrides it here"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "lift the limit: resources of all your teams")
	return cmd
}

// environments loads the configuration for the environment commands: the environments are needed,
// the active environment may be missing or invalid (that is what the user is about to fix).
func (a *App) environments() (*config.Config, error) {
	cfg, err := a.Config()
	if err == nil {
		return cfg, nil
	}
	var none *config.NoEnvironmentError
	var unknown *config.UnknownEnvironmentError
	var missing *config.MissingError
	if errors.As(err, &none) || errors.As(err, &unknown) || (errors.As(err, &missing) && missing.Environment != "") {
		partial, _ := config.Load(a.env)
		if partial != nil && len(partial.Environments) > 0 {
			return partial, nil
		}
	}
	return nil, err
}

// completeEnvironments completes the names of the configured environments, without the API.
func completeEnvironments(_ context.Context, a *App, _ *cobra.Command, _ []string, toComplete string) ([]resolve.Candidate, cobra.ShellCompDirective, error) {
	cfg, err := a.environments()
	if err != nil {
		return nil, 0, err
	}
	var candidates []resolve.Candidate
	for _, name := range cfg.EnvironmentNames() {
		if strings.HasPrefix(name, toComplete) {
			candidates = append(candidates, resolve.Candidate{Value: name, Description: cfg.Environments[name].URL})
		}
	}
	return candidates, 0, nil
}
