package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
	"github.com/pipetower/pipetower-cli/internal/update"
	"github.com/pipetower/pipetower-cli/internal/version"
)

// EnvNoUpdateCheck disables looking for a new release when pt starts.
const EnvNoUpdateCheck = "PT_NO_UPDATE_CHECK"

// updateCheckTimeout limits how long a start waits for GitHub.
const updateCheckTimeout = 2 * time.Second

// updater finds and installs new releases of pt.
type updater struct {
	source   *update.Source
	schedule update.Schedule
	// executable returns the binary to replace (replaced in tests).
	executable func() (string, error)
	now        func() time.Time
}

func newUpdater() updater {
	return updater{source: update.GitHub(), schedule: update.DefaultSchedule(), executable: update.Executable, now: time.Now}
}

// newUpdateCommand updates pt to the latest release.
func newUpdateCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update pt to the latest version",
		Long: "Update pt to the latest version. pt also looks for a new version once a day when it " +
			"starts in a terminal and offers to install it (disable with " + EnvNoUpdateCheck + "=1).",
		Args: args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !version.IsRelease() {
				return fault.New(fault.Problem{
					Title:  "pt " + version.Version + " is a development build",
					Detail: "Only an installed release of pt can update itself.",
					Hints:  []fault.Hint{{Label: "Install with", Value: update.InstallCommand}},
					Code:   fault.ExitUsage,
				})
			}

			release, err := ui.Spin(a.IO, "Looking for updates…", func() (update.Release, error) {
				return a.updater.source.Latest(cmd.Context())
			})
			if err != nil {
				return latestProblem(err)
			}
			a.updater.schedule.Checked(a.updater.now())
			if !version.Newer(release.Version, version.Version) {
				a.IO.Errorln(" " + ui.Success.Render("✓") + " pt " + version.Version + " is the latest version")
				return nil
			}
			return a.installUpdate(cmd.Context(), release)
		},
	}
}

// offerUpdate looks for a new release once a day when pt starts in a terminal and offers to install
// it. After an update pt stops, the command runs with the new version once pt is started again.
func (a *App) offerUpdate(cmd *cobra.Command) error {
	if !a.IO.CanPrompt || !version.IsRelease() || a.env.Getenv(EnvNoUpdateCheck) != "" || skipsUpdateCheck(cmd) {
		return nil
	}
	now := a.updater.now()
	if !a.updater.schedule.Due(now) {
		return nil
	}
	a.updater.schedule.Checked(now)

	ctx, cancel := context.WithTimeout(cmd.Context(), updateCheckTimeout)
	release, err := a.updater.source.Latest(ctx)
	cancel()
	if err != nil || !version.Newer(release.Version, version.Version) {
		return nil // offline or up to date: the command runs as usual
	}

	question := "pt " + release.Version + " is available (you have " + version.Version + "). Update now?"
	accepted, err := a.Prompter().Confirm(question, "Yes, update", "Not now")
	if err != nil {
		return err
	}
	if !accepted {
		a.IO.Errorln(ui.Muted.Render(" Update any time with ") + ui.Code.Render("pt update") + "\n")
		return nil
	}

	if err := a.installUpdate(cmd.Context(), release); err != nil {
		if isQuiet(err) {
			return err
		}
		a.HandleError(a.IO.Err, err) // the command still runs, with the current version
		return nil
	}
	return &fault.Silent{Code: fault.ExitOK}
}

// skipsUpdateCheck tells whether the command never offers an update: pt update itself, and the
// commands whose output other programs read.
func skipsUpdateCheck(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "update", "version", "completion":
		return true
	}
	return strings.HasPrefix(cmd.Name(), cobra.ShellCompRequestCmd) // __complete, __completeNoDesc
}

// installUpdate downloads the release with a progress bar and replaces the installed binary.
func (a *App) installUpdate(ctx context.Context, release update.Release) error {
	target, err := a.updater.executable()
	if err != nil {
		return installProblem(err)
	}

	progress := ui.StartProgress(a.IO, "Downloading pt "+release.Version)
	err = a.updater.source.Install(ctx, release, target, progress.Set)
	progress.Stop()
	if err != nil {
		return installProblem(err)
	}

	a.IO.Errorln(renderUpdated(version.Version, release))
	return nil
}

// renderUpdated tells which version was installed and that it takes effect after a restart.
func renderUpdated(from string, release update.Release) string {
	return "\n " + ui.Success.Render("✓") + " " + ui.Bold.Render("Updated pt ") + ui.Muted.Render(from) +
		ui.Bold.Render(" → "+release.Version) + "\n\n" +
		"   Restart pt to use the new version.\n" +
		"   " + ui.Muted.Render("What's new  ") + ui.Code.Render(release.URL) + "\n"
}

// latestProblem explains why the latest release couldn't be found.
func latestProblem(err error) error {
	if isQuiet(err) {
		return err
	}
	if errors.Is(err, update.ErrNoRelease) {
		return fault.New(fault.Problem{Title: "No version of pt has been released yet", Code: fault.ExitFailure})
	}
	return &fault.Error{P: fault.Problem{
		Title:  "Can't look for updates",
		Detail: ui.Capitalize(err.Error()),
		Code:   fault.ExitFailure,
	}, Cause: err}
}

// installProblem explains a failed update, problems of the update package explain themselves.
func installProblem(err error) error {
	var explainer fault.Explainer
	if isQuiet(err) || errors.As(err, &explainer) {
		return err
	}
	return &fault.Error{P: fault.Problem{
		Title:  "Can't update pt",
		Detail: ui.Capitalize(err.Error()),
		Hints:  []fault.Hint{{Label: "Update with", Value: update.InstallCommand}},
		Code:   fault.ExitFailure,
	}, Cause: err}
}
