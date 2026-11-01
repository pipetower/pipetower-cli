package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/serverview"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

const latestRunsLimit = 5

// newServerCommand groups the server commands.
func newServerCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "List and view servers, their health and connections",
	}
	cmd.AddCommand(
		newServerListCommand(a),
		newServerViewCommand(a),
		newServerConnectCommand(a, true),
		newServerConnectCommand(a, false),
	)
	return cmd
}

// newServerListCommand lists servers with their usage.
func newServerListCommand(a *App) *cobra.Command {
	var page pageOptions
	var tag, search string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List servers with CPU, memory, disk and load",
		Args:  args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := searchOptions(page, search)
			opts.Params = map[string]string{"tag": tag}
			return showList(cmd.Context(), a, listing[api.Server]{
				path:    api.Servers,
				opts:    opts,
				headers: serverview.ListHeaders,
				row:     serverview.ListRow,
				byTeam:  true,
				empty:   "No servers yet. Add one in the web app.",
			})
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&tag, "tag", "", "only servers with this tag")
	addSearchFlag(cmd, &search, "servers")
	return cmd
}

// newServerViewCommand views the dashboard of a server.
func newServerViewCommand(a *App) *cobra.Command {
	var watch, web bool
	cmd := &cobra.Command{
		Use:        "view <server>",
		SuggestFor: []string{"show"},
		Short:      "View the dashboard of a server: system, usage and latest runs",
		Example:    "  pt server view prod\n  pt server view prod --watch",
		Args:       argOrPick(a, "server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			return viewServer(cmd.Context(), a, optionalArg(argv), watch, web)
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "keep the dashboard open and update it live")
	addWebFlag(cmd, &web)
	return cmd
}

// viewServer prints the dashboard once, keeps it updated or opens the server in the web.
func viewServer(ctx context.Context, a *App, arg string, watch, web bool) error {
	server, err := resolveServer(ctx, a, arg)
	if err != nil {
		return err
	}
	if web {
		return a.openWeb("servers/" + server.ID)
	}
	if a.Structured() {
		return a.printEndpoint(ctx, api.Show(api.Servers, server.ID))
	}

	fetch := serverSnapshot(a, server.ID)
	snapshot, err := fetch(ctx)
	if err != nil {
		return err
	}
	if !watch {
		a.IO.Println(serverview.Dashboard(snapshot.Server, snapshot.Runs, time.Now(), a.IO.Width()))
		return nil
	}
	if !a.IO.Terminal {
		return usageError("--watch needs an interactive terminal", "Run it without --watch to print the dashboard once.")
	}
	return serverview.Watch(ctx, snapshot, fetch, a.Connector(), a.IO.RawOut, a.IO.Profile, a.IO.Width())
}

// serverSnapshot loads a server with its latest runs.
func serverSnapshot(a *App, id string) serverview.Fetcher {
	return func(ctx context.Context) (serverview.Snapshot, error) {
		client, err := a.Client()
		if err != nil {
			return serverview.Snapshot{}, err
		}
		server, err := client.Server(ctx, id)
		if err != nil {
			return serverview.Snapshot{}, err
		}
		latest := api.ListOptions{Limit: latestRunsLimit, Params: map[string]string{"server": id}}
		runs, err := api.Get[api.Page[api.Run]](ctx, client, api.List(api.Runs, latest))
		return serverview.Snapshot{Server: server, Runs: runs.Data}, err
	}
}

// newServerConnectCommand connects or disconnects the SSH connection of a server.
func newServerConnectCommand(a *App, connect bool) *cobra.Command {
	use, short, verb := "connect <server>", "Open the SSH connection of a server", "Connecting"
	if !connect {
		use, short, verb = "disconnect <server>", "Close the SSH connection of a server", "Disconnecting"
	}

	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  argOrPick(a, "server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			server, err := resolveServer(cmd.Context(), a, optionalArg(argv))
			if err != nil {
				return err
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			updated, err := ui.Spin(a.IO, verb+" "+server.Name+"…", func() (api.Server, error) {
				return client.SetConnected(cmd.Context(), server.ID, connect)
			})
			if err != nil {
				return err
			}
			if a.Structured() {
				return a.PrintValue(updated)
			}
			a.IO.Println(" " + ui.Connection(updated.Connected) + "  " + ui.Bold.Render(updated.Name) + "  " + ui.Faint.Render(updated.Host))
			return nil
		},
	}
}

// resolveServer resolves a server argument.
func resolveServer(ctx context.Context, a *App, arg string) (api.Server, error) {
	resolver, err := a.Resolver(ctx)
	if err != nil {
		return api.Server{}, err
	}
	return resolver.Server(ctx, arg)
}
