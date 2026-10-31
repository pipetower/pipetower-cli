package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/gateway"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/terminal"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// endReasons explains why a session ended.
var endReasons = map[string]string{
	"exited":    "the shell exited",
	"closed":    "the owner ended the session",
	"abandoned": "nobody was attached anymore",
	"failed":    "the shell could not be started",
	"gone":      "the session does not run anymore",
}

// newSSHCommand opens a terminal on a server, like ssh.
func newSSHCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "ssh <server>",
		Short: "Open a shared terminal session on a server",
		Long: "Open an interactive shell on a server through Pipetower. The session is shared:\n" +
			"teammates can join it from the web or with pt terminal join.\n\n" + terminal.HelpText,
		Args: args("server"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			return openTerminal(cmd.Context(), a, argv[0])
		},
	}
}

// newTerminalCommand groups the terminal session commands.
func newTerminalCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "terminal",
		Aliases: []string{"term"},
		Short:   "Open, join and list shared terminal sessions",
	}

	open := newSSHCommand(a)
	open.Use, open.GroupID = "open <server>", ""

	cmd.AddCommand(newTerminalListCommand(a), open, &cobra.Command{
		Use:   "join <session>",
		Short: "Join a running terminal session of your team",
		Args:  args("session"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			return joinTerminal(cmd.Context(), a, argv[0])
		},
	})
	return cmd
}

// newTerminalListCommand lists the live sessions of the user's teams.
func newTerminalListCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List the live terminal sessions of your teams",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := connectGateway(cmd.Context(), a)
			if err != nil {
				return err
			}
			defer client.Close()

			sessions := client.Ready().Sessions
			if a.Structured() {
				return a.PrintValue(sessions)
			}
			printSessions(a, sessions)
			return nil
		},
	}
}

// printSessions prints the lobby as a table.
func printSessions(a *App, sessions []gateway.LobbySession) {
	if len(sessions) == 0 {
		a.IO.Println(ui.Empty(" No live sessions right now. Open one with pt ssh <server>."))
		return
	}

	rows := make([][]string, len(sessions))
	for i, s := range sessions {
		started, _ := time.Parse(time.RFC3339, s.StartedAt)
		rows[i] = []string{
			ui.Success.Render("●") + " " + ui.Bold.Render(s.Server.Name),
			s.Owner.Name,
			fmt.Sprint(len(s.Participants)),
			ui.Muted.Render(ui.Ago(&started, time.Now())),
			ui.Code.Render("pt terminal join " + s.ID),
		}
	}
	a.IO.Println(ui.Table([]string{"SERVER", "OWNER", "ATTACHED", "STARTED", "JOIN"}, rows))
}

// openTerminal opens a new session on a server and attaches to it.
func openTerminal(ctx context.Context, a *App, arg string) error {
	if err := requireTerminal(a); err != nil {
		return err
	}
	server, err := resolveServer(ctx, a, arg)
	if err != nil {
		return err
	}
	return attachTerminal(ctx, a, "Opening a terminal on "+server.Name+"…", func(ctx context.Context, client *api.Client) (api.Session, error) {
		return client.OpenSession(ctx, server.ID)
	})
}

// joinTerminal attaches to an existing session.
func joinTerminal(ctx context.Context, a *App, id string) error {
	if err := requireTerminal(a); err != nil {
		return err
	}
	return attachTerminal(ctx, a, "Joining the session…", func(ctx context.Context, client *api.Client) (api.Session, error) {
		session, err := client.AttachSession(ctx, id)
		return session, orNotFound(err, resolve.KindSession, id)
	})
}

// requireTerminal fails before anything is opened when pt does not run in a terminal.
func requireTerminal(a *App) error {
	if a.IO.Terminal {
		return nil
	}
	return usageError("A terminal session needs an interactive terminal", "Run pt in a terminal, without redirecting input or output.")
}

// attachTerminal opens the session once the gateway is connected, mirrors it into this terminal
// and reports how it ended.
func attachTerminal(ctx context.Context, a *App, opening string, open func(context.Context, *api.Client) (api.Session, error)) error {
	client, err := a.Client()
	if err != nil {
		return err
	}
	result, err := terminal.Attach(ctx, terminal.Options{
		Open: func(ctx context.Context) (api.Session, error) {
			return ui.Spin(a.IO, opening, func() (api.Session, error) { return open(ctx, client) })
		},
		Reattach: client.AttachSession,
		Connect:  func(ctx context.Context) (*gateway.Client, error) { return connectGateway(ctx, a) },
		In:       a.IO.In,
		Out:      a.IO.Stdout,
	})
	if err != nil {
		return err
	}

	a.IO.Println("\n" + sessionEnd(result.Session, result))
	if result.Reason == "failed" {
		return &fault.Silent{Code: fault.ExitFailure}
	}
	return nil
}

// sessionEnd describes how the session ended for the user.
func sessionEnd(session api.Session, result terminal.Result) string {
	if result.Reason == terminal.ReasonDetached {
		return " " + ui.Accent.Render("◌") + " Detached from " + ui.Bold.Render(session.Server.Name) +
			ui.Muted.Render(", the session keeps running. Rejoin with ") + ui.Code.Render("pt terminal join "+session.ID)
	}
	reason := endReasons[result.Reason]
	if reason == "" {
		reason = result.Reason
	}
	return " " + ui.Muted.Render("■") + " Session on " + ui.Bold.Render(session.Server.Name) + " ended" + ui.Muted.Render(": "+reason)
}

// connectGateway connects to the gateway behind a spinner. The configuration is loaded first,
// so that its warnings are not mixed into the spinner line.
func connectGateway(ctx context.Context, a *App) (*gateway.Client, error) {
	if _, err := a.Client(); err != nil {
		return nil, err
	}
	return ui.Spin(a.IO, "Connecting to the gateway…", func() (*gateway.Client, error) {
		return a.Connector()(ctx)
	})
}
