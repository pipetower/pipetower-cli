// Command pt is the command line of Pipetower.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"

	"github.com/pipetower/pipetower-cli/internal/cli"
	"github.com/pipetower/pipetower-cli/internal/ui"
	"github.com/pipetower/pipetower-cli/internal/version"
)

func main() {
	ctx, stop := interruptible()
	app := cli.NewApp()
	root := cli.NewRootCommand(app)

	err := fang.Execute(ctx, root,
		fang.WithVersion(version.Version),
		fang.WithCommit(version.Commit),
		fang.WithColorSchemeFunc(colorScheme),
		fang.WithErrorHandler(func(w io.Writer, _ fang.Styles, err error) { app.HandleError(w, err) }),
	)
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

// colorScheme is the help theme of fang in Pipetower colors.
func colorScheme(c lipgloss.LightDarkFunc) fang.ColorScheme {
	scheme := fang.DefaultColorScheme(c)
	scheme.Title = ui.ColorBrand
	scheme.Program = ui.ColorBrand
	scheme.Command = ui.ColorBrand
	scheme.Flag = ui.ColorGreen
	scheme.QuotedString = ui.ColorYellow
	return scheme
}
