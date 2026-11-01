package cli

import (
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
	"github.com/pipetower/pipetower-cli/internal/version"
)

// newAPICommand sends a raw authenticated request.
func newAPICommand(a *App) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Send an authenticated request to the REST API and print the JSON",
		Long:  "Send a request to the Pipetower REST API (paths are relative to /api/v1).",
		Example: "  pt api GET servers\n" +
			"  pt api GET 'runs?status=running'\n" +
			"  pt api POST runs -d '{\"action_id\":\"x1\",\"server_ids\":[\"s1\"]}'",
		Args: args("method", "path"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			client, err := a.Client()
			if err != nil {
				return err
			}
			endpoint := rawEndpoint(strings.ToUpper(argv[0]), argv[1], data)
			status, body, err := client.Raw(cmd.Context(), endpoint)
			if err != nil {
				return err
			}
			if len(body) > 0 {
				if err := a.IO.PrintJSON(body); err != nil {
					return err
				}
			}
			if status >= 300 {
				a.IO.Errorln(ui.Danger.Render(" ✗ " + http.StatusText(status)))
				return &fault.Silent{Code: fault.ExitAPI}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&data, "data", "d", "", "request body as JSON")
	return cmd
}

// rawEndpoint builds an endpoint from a path which may contain a query string.
func rawEndpoint(method, path, data string) api.Endpoint {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "/"), "api/v1/")
	endpoint := api.Endpoint{Method: method, Path: path}
	if before, query, found := strings.Cut(path, "?"); found {
		endpoint.Path = before
		endpoint.Query = parseQuery(query)
	}
	if data != "" {
		endpoint.Body = []byte(data)
	}
	return endpoint
}

// newVersionCommand prints the build information.
func newVersionCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of pt",
		Args:  args(),
		Run: func(*cobra.Command, []string) {
			details := []string{version.Date, runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH}
			if !strings.HasPrefix(version.Version, version.Commit) {
				details = append([]string{version.Commit}, details...)
			}
			a.IO.Println(ui.Title.Render("pt") + " " + version.Version + ui.Muted.Render("  "+strings.Join(details, " · ")))
		},
	}
}

// parseQuery parses a query string, invalid parts are dropped.
func parseQuery(query string) url.Values {
	values, _ := url.ParseQuery(query)
	return values
}

// newCompletionCommand prints the shell completion script.
func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "completion <shell>",
		Short:   "Print the shell completion script (bash, zsh, fish, powershell)",
		Example: "  source <(pt completion bash)\n  pt completion zsh > \"${fpath[1]}/_pt\"",
		Args:    args("shell"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			root, out := cmd.Root(), cmd.OutOrStdout()
			switch argv[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return usageError("Unknown shell “"+argv[0]+"”", "Use bash, zsh, fish or powershell.")
		},
	}
}
