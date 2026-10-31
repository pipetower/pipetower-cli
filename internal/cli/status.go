package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// statusReport is what pt status checks.
type statusReport struct {
	Instance    string   `json:"instance"`
	URLSource   string   `json:"url_source"`
	TokenSource string   `json:"token_source"`
	User        string   `json:"user"`
	Email       string   `json:"email"`
	Teams       []string `json:"teams"`
	Gateway     string   `json:"gateway"`
	GatewayMS   int64    `json:"gateway_ms"`
	GatewayErr  string   `json:"gateway_error,omitempty"`
}

// newStatusCommand checks configuration, authentication and the gateway.
func newStatusCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check the configuration, the API token and the real-time gateway",
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := a.Config(); err != nil {
				return err
			}
			report, err := ui.Spin(a.IO, "Checking…", func() (statusReport, error) { return checkStatus(cmd.Context(), a) })
			if err != nil {
				return err
			}
			if a.Structured() {
				return a.PrintValue(report)
			}
			a.IO.Println(renderStatus(report))
			return nil
		},
	}
}

// checkStatus loads the configuration and checks the API and the gateway.
func checkStatus(ctx context.Context, a *App) (statusReport, error) {
	cfg, err := a.Config()
	if err != nil {
		return statusReport{}, err
	}
	client, err := a.Client()
	if err != nil {
		return statusReport{}, err
	}

	account, err := client.Account(ctx)
	if err != nil {
		return statusReport{}, err
	}
	teams, err := api.ListAll[api.Team](ctx, client, api.Teams, api.ListOptions{}, 100)
	if err != nil {
		return statusReport{}, err
	}

	report := statusReport{
		Instance: cfg.URL, URLSource: cfg.URLSource.String(), TokenSource: cfg.TokenSource.String(),
		User: account.Name, Email: account.Email, Teams: teamNames(teams),
	}
	checkGateway(ctx, a, &report)
	return report, nil
}

// checkGateway connects to the gateway and measures the time until it is ready.
func checkGateway(ctx context.Context, a *App, report *statusReport) {
	started := time.Now()
	client, err := a.Connector()(ctx)
	if err != nil {
		report.Gateway, report.GatewayErr = "unreachable", ui.ProblemOf(err).Title+": "+ui.ProblemOf(err).Detail
		return
	}
	defer client.Close()

	report.Gateway, report.GatewayMS = "connected", time.Since(started).Milliseconds()
	report.Gateway += " · " + ui.Plural(len(client.Ready().Sessions), "live terminal session")
}

// renderStatus renders the report as aligned key-value lines.
func renderStatus(r statusReport) string {
	gatewayText := ui.Success.Render("● ") + r.Gateway + ui.Faint.Render(" · "+ui.Duration(time.Duration(r.GatewayMS)*time.Millisecond))
	if r.GatewayErr != "" {
		gatewayText = ui.Danger.Render("✗ "+r.Gateway) + "\n" + strings.Repeat(" ", 15) + ui.Muted.Render(r.GatewayErr)
	}

	lines := []string{
		"",
		" " + ui.Title.Render("Pipetower") + "  " + ui.Muted.Render(r.Instance),
		"",
		line("User", ui.Bold.Render(r.User)+ui.Muted.Render("  "+r.Email)),
		line("Teams", strings.Join(r.Teams, ", ")),
		line("API", ui.Success.Render("● ")+"authenticated"),
		line("Gateway", gatewayText),
		"",
		line("URL from", ui.Muted.Render(r.URLSource)),
		line("Token from", ui.Muted.Render(r.TokenSource)),
	}
	return strings.Join(lines, "\n") + "\n"
}

// line renders a labeled status line.
func line(label, value string) string {
	return " " + ui.Label.Render(label) + value
}

// teamNames returns the names of teams.
func teamNames(teams []api.Team) []string {
	names := make([]string, len(teams))
	for i, team := range teams {
		names[i] = team.Name
	}
	return names
}
