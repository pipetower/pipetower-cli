// Command pt is the command line of Pipetower.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/colorprofile"

	"github.com/pipetower/pipetower-cli/internal/cli"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

func main() {
	ctx, stop := interruptible()
	restoreOut, restoreErr := ui.EnableVirtualTerminal(os.Stdout), ui.EnableVirtualTerminal(os.Stderr)
	app := cli.NewApp()

	err := cli.NewRootCommand(app).ExecuteContext(ctx)
	if err != nil {
		app.HandleError(colorprofile.NewWriter(os.Stderr, os.Environ()), err)
	}

	restoreOut()
	restoreErr()
	stop()
	os.Exit(cli.ExitCode(err))
}

// interruptible returns a context which ends on Ctrl+C or SIGTERM, so that every command can stop
// gracefully (e.g. detach from a run). A second Ctrl+C terminates immediately as usual
func interruptible() (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop)

	return ctx, stop
}
