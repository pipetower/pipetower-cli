package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/present"
	"github.com/pipetower/pipetower-cli/internal/serverview"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

const latestRunsLimit = 5

// newServerCommand groups the server commands.
func newServerCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Show servers, their health and connections",
	}
	cmd.AddCommand(
		newServerListCommand(a),
		newServerShowCommand(a),
		newServerConnectCommand(a, true),
		newServerConnectCommand(a, false),
		newServerVarsCommand(a),
		newServerRunsCommand(a),
	)
	return cmd
}

// newServerListCommand lists servers with their usage.
func newServerListCommand(a *App) *cobra.Command {
	var page pageOptions
	var tag, search string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List servers with CPU, memory, disk and load",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := page.options()
			opts.Search, opts.Params = search, map[string]string{"tag": tag}
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
	cmd.Flags().StringVar(&search, "search", "", "only servers whose name contains this text")
	return cmd
}

// newServerShowCommand shows the dashboard of a server.
func newServerShowCommand(a *App) *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:     "show <server>",
		Short:   "Show the dashboard of a server: system, usage and latest runs",
		Example: "  pt server show prod\n  pt server show prod --watch",
		Args:    args("server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			return showServer(cmd.Context(), a, argv[0], watch)
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "keep the dashboard open and update it live")
	return cmd
}

// showServer prints the dashboard once or keeps it updated.
func showServer(ctx context.Context, a *App, arg string, watch bool) error {
	server, err := resolveServer(ctx, a, arg)
	if err != nil {
		return err
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
		a.IO.Println(serverview.Dashboard(snapshot.Server, snapshot.Runs, time.Now()))
		return nil
	}
	if !a.IO.Terminal {
		return usageError("--watch needs an interactive terminal", "Run it without --watch to print the dashboard once.")
	}
	return serverview.Watch(ctx, snapshot, fetch, a.Connector(), a.IO.RawOut, a.IO.Profile)
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
		runs, err := api.Get[api.Page[api.Run]](ctx, client, api.Nested(api.Servers, id, api.Runs, api.ListOptions{Limit: latestRunsLimit}))
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
		Args:  args("server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			server, err := resolveServer(cmd.Context(), a, argv[0])
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

// newServerVarsCommand lists the environment variables of a server.
func newServerVarsCommand(a *App) *cobra.Command {
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "vars <server>",
		Short: "List the environment variables of a server",
		Args:  args("server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			server, err := resolveServer(cmd.Context(), a, argv[0])
			if err != nil {
				return err
			}
			return showList(cmd.Context(), a, variableListing(api.NestedPath(api.Servers, server.ID, "vars"), page))
		},
	}
	addPageFlags(cmd, &page)
	return cmd
}

// newServerRunsCommand lists the runs of a server.
func newServerRunsCommand(a *App) *cobra.Command {
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "runs <server>",
		Short: "List the runs on a server",
		Args:  args("server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			server, err := resolveServer(cmd.Context(), a, argv[0])
			if err != nil {
				return err
			}
			return showList(cmd.Context(), a, runListing(api.NestedPath(api.Servers, server.ID, api.Runs), page.options()))
		},
	}
	addPageFlags(cmd, &page)
	return cmd
}

// variableListing lists variables, secrets stay hidden.
func variableListing(path string, page pageOptions) listing[api.Variable] {
	return listing[api.Variable]{
		path:    path,
		opts:    page.options(),
		headers: []string{"NAME", "VALUE", ""},
		row: func(v api.Variable) []string {
			if v.Secret {
				return []string{ui.Bold.Render(v.Name), ui.Faint.Render("••••••"), ui.Faint.Render("secret")}
			}
			return []string{ui.Bold.Render(v.Name), v.Value, ""}
		},
		empty: "No variables.",
	}
}

// runListing lists runs.
func runListing(path string, opts api.ListOptions) listing[api.Run] {
	return listing[api.Run]{
		path:    path,
		opts:    opts,
		headers: present.RunHeaders,
		row:     func(r api.Run) []string { return present.RunRow(r, time.Now()) },
		byTeam:  path == api.Runs,
		empty:   "No runs yet.",
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

// addPageFlags adds --limit and --page.
func addPageFlags(cmd *cobra.Command, page *pageOptions) {
	cmd.Flags().IntVarP(&page.limit, "limit", "L", 25, "number of items per page (max 100)")
	cmd.Flags().IntVar(&page.page, "page", 1, "page number")
}
