package cli

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/config"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newEnvCommand groups the environment commands.
func newEnvCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Add, list and switch the configured environments (instances)"}
	cmd.AddCommand(newEnvAddCommand(a), newEnvListCommand(a), newEnvUseCommand(a))
	return cmd
}

// newEnvAddCommand adds an environment to the configuration file, creating the file at first.
func newEnvAddCommand(a *App) *cobra.Command {
	var env config.Environment
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add an environment: an instance with your API token for it",
		Long: "Add an environment to the configuration file, which is created as ~/.pipetower if there is\n" +
			"none yet, and the first one becomes the active one. Without --url it is " + config.DefaultURL + ",\n" +
			"without a name it is named after the host of the URL. The token is asked for, hidden, when\n" +
			"--token is left out in a terminal. One environment is all most users need.",
		Example: "  " + config.AddHint + "\n  pt env add staging --url https://staging.example.com",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, argv []string) error {
			if len(argv) > 1 {
				return usageProblem(cmd, "Too many arguments: "+strings.Join(argv[1:], " "))
			}
			if env.URL == "" {
				env.URL = config.DefaultURL
			}
			name := optionalArg(argv)
			if name == "" {
				if name = hostOf(env.URL); name == "" {
					return usageProblem(cmd, "Invalid --url "+env.URL)
				}
			}
			if env.Token == "" {
				prompter := a.Prompter()
				if prompter == nil {
					return usageProblem(cmd, "Missing --token")
				}
				token, err := prompter.Secret("Your API token (Account → API tokens):")
				if err != nil {
					return err
				}
				env.Token = token
			}

			path := a.configPath()
			activated, err := config.AddEnvironment(path, name, env)
			if err != nil {
				return err
			}
			a.cfg = nil // read again with the new environment
			state := ""
			if activated {
				state = ", active"
			}
			a.IO.Println(" " + ui.Success.Render("✓") + " Added " + ui.Bold.Render(name) + ui.Muted.Render("  "+env.URL+" · team "+ui.Or(env.Team, "all")+" · in "+path+state))
			if !activated {
				a.IO.Errorln(ui.Faint.Render(" Switch to it with pt env use " + name))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&env.URL, "url", "", "the URL of the Pipetower instance ("+config.DefaultURL+")")
	cmd.Flags().StringVar(&env.Token, "token", "", "your API token for the instance (Account → API tokens)")
	cmd.Flags().StringVar(&env.Team, "team", "", "limit all commands to this team while the environment is active")
	return cmd
}

// hostOf is the host of a URL, the default name of an environment ("app.pipetower.com").
func hostOf(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// configPath is the file environments are added to: the one which holds the environments in use,
// or $PIPETOWER_CONFIG, or ~/.pipetower when there is none yet.
func (a *App) configPath() string {
	if cfg, err := a.environments(); err == nil && cfg.EnvironmentsPath != "" {
		return cfg.EnvironmentsPath
	}
	return config.DefaultPath(a.env)
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

			team, err := pickTeam(cmd.Context(), a, cfg, optionalArg(argv), all)
			if err != nil {
				return err
			}
			if err := config.SetTeam(cfg.EnvironmentsPath, cfg.Environment, team); err != nil {
				return err
			}

			if team == "" {
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

// pickTeam returns the team to limit the commands to, "" for all teams: the given one (any of the
// user's teams, not only the one currently set), or the user's choice among them and "all teams".
func pickTeam(ctx context.Context, a *App, cfg *config.Config, name string, all bool) (string, error) {
	if all {
		return "", nil
	}
	client, err := a.Client()
	if err != nil {
		return "", err
	}
	if name != "" {
		found, err := (&resolve.Resolver{Client: client, Prompter: a.Prompter()}).Team(ctx, name)
		return found.Name, err
	}

	teams, err := api.ListAll[api.Team](ctx, client, api.Teams, api.ListOptions{}, 100)
	if err != nil {
		return "", err
	}
	mark := func(current bool) string {
		if current {
			return "●"
		}
		return " "
	}
	options := []prompt.Option{{Mark: mark(cfg.Team == ""), Columns: []string{"All teams", "no limit"}}}
	for _, team := range teams {
		options = append(options, prompt.Option{Mark: mark(cfg.Team == team.Name || cfg.Team == team.ID), Columns: []string{team.Name, ui.Plural(len(team.Members), "member")}})
	}
	index, err := a.Prompter().Pick("Limit the commands to which team?", options)
	if err != nil || index == 0 {
		return "", err
	}
	return teams[index-1].Name, nil
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
