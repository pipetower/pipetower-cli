// Package cli wires the commands of pt: parsing, configuration and output.
// Commands stay thin, the work happens in the api, resolve and view packages.
package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/browser"
	"github.com/pipetower/pipetower-cli/internal/config"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/gateway"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// Output formats.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
)

// flags are the global flags.
type flags struct {
	output  string
	noInput bool
	noColor bool
	team    string
}

// IOFactory creates the streams once the global flags are known.
type IOFactory func(noColor, noInput bool) *ui.IO

// App holds what all commands share: streams, flags, configuration and the API client.
type App struct {
	IO     *ui.IO
	flags  flags
	env    config.Env
	newIO  IOFactory
	cfg    *config.Config
	client *api.Client
	teamID string
	// openURL opens a page in the browser (replaced in tests).
	openURL func(url string) error
}

// NewApp creates the app for the running process.
func NewApp() *App {
	return NewAppWith(config.SystemEnv(), ui.NewIO)
}

// NewAppWith creates an app with the given environment and streams (e.g. in tests).
func NewAppWith(env config.Env, newIO IOFactory) *App {
	return &App{IO: newIO(false, false), env: env, newIO: newIO, openURL: browser.Open}
}

// setup applies the parsed global flags.
func (a *App) setup() error {
	a.IO = a.newIO(a.flags.noColor, a.flags.noInput)

	switch a.flags.output {
	case FormatTable, FormatJSON, FormatYAML:
		return nil
	}
	return fault.New(fault.Problem{
		Title:  fmt.Sprintf("Unknown output format “%s”", a.flags.output),
		Detail: "Use table, json or yaml.",
		Code:   fault.ExitUsage,
	})
}

// Config loads the configuration once and shows its warnings.
func (a *App) Config() (*config.Config, error) {
	if a.cfg != nil {
		return a.cfg, nil
	}

	cfg, err := config.Load(a.env)
	if err != nil {
		return nil, err
	}
	for _, warning := range cfg.Warnings {
		a.IO.Errorln(ui.RenderWarning(warning))
	}

	a.cfg = cfg
	return cfg, nil
}

// Client returns the API client of the configured instance.
func (a *App) Client() (*api.Client, error) {
	if a.client != nil {
		return a.client, nil
	}

	cfg, err := a.Config()
	if err != nil {
		return nil, err
	}

	a.client = api.New(cfg.URL, cfg.Token)
	return a.client, nil
}

// Prompter returns the prompter, or nil if the user must not be asked.
func (a *App) Prompter() prompt.Prompter {
	if !a.IO.CanPrompt {
		return nil
	}
	return prompt.Terminal{Profile: a.IO.Profile}
}

// Resolver returns a resolver which asks the user when allowed, limited to the team of --team.
func (a *App) Resolver(ctx context.Context) (*resolve.Resolver, error) {
	client, err := a.Client()
	if err != nil {
		return nil, err
	}
	teamID, err := a.TeamID(ctx)
	if err != nil {
		return nil, err
	}
	return &resolve.Resolver{Client: client, Prompter: a.Prompter(), TeamID: teamID}, nil
}

// TeamID resolves the team given with --team once, it is empty without the flag.
func (a *App) TeamID(ctx context.Context) (string, error) {
	if a.flags.team == "" || a.teamID != "" {
		return a.teamID, nil
	}
	client, err := a.Client()
	if err != nil {
		return "", err
	}
	team, err := (&resolve.Resolver{Client: client, Prompter: a.Prompter()}).Team(ctx, a.flags.team)
	a.teamID = team.ID
	return a.teamID, err
}

// Connector opens gateway connections with fresh tickets from the API.
func (a *App) Connector() gateway.Connector {
	return func(ctx context.Context) (*gateway.Client, error) {
		client, err := a.Client()
		if err != nil {
			return nil, err
		}
		return gateway.Connect(ctx, func(ctx context.Context) (string, string, error) {
			ticket, err := client.GatewayTicket(ctx)
			if err == nil {
				err = config.ValidateGatewayURL(ticket.URL)
			}
			return ticket.URL, ticket.Ticket, err
		})
	}
}

// Structured tells whether raw data (json or yaml) is printed instead of tables.
func (a *App) Structured() bool {
	return a.flags.output != FormatTable
}

// PrintRaw prints raw API data in the chosen format.
func (a *App) PrintRaw(raw json.RawMessage) error {
	if a.flags.output == FormatYAML {
		return a.IO.PrintYAML(raw)
	}
	return a.IO.PrintJSON(raw)
}

// PrintValue prints a value as structured data.
func (a *App) PrintValue(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.PrintRaw(raw)
}

// openWeb opens the page of the instance (e.g. "runs/30denr") in the browser, or prints its URL
// when no browser can be started.
func (a *App) openWeb(path string) error {
	cfg, err := a.Config()
	if err != nil {
		return err
	}
	url := cfg.URL + "/" + path
	if err := a.openURL(url); err != nil {
		a.IO.Println(url)
		return nil
	}
	a.IO.Errorln(ui.Muted.Render(" Opening ") + url + ui.Muted.Render(" in your browser."))
	return nil
}
