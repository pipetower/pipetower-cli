// Package present renders the domain objects of the API (statuses, runs) with the ui building blocks.
package present

import (
	"image/color"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// StatusLook is the visual representation of a run status.
type StatusLook struct {
	Icon  string
	Label string
	Color color.Color
}

var statusLooks = map[string]StatusLook{
	api.StatusPending:         {"○", "Pending", ui.ColorMuted},
	api.StatusRunning:         {"●", "Running", ui.ColorBrand},
	api.StatusSuccessful:      {"✓", "Successful", ui.ColorGreen},
	api.StatusFailed:          {"✗", "Failed", ui.ColorRed},
	api.StatusPartiallyFailed: {"◐", "Partially failed", ui.ColorOrange},
	api.StatusCanceled:        {"◌", "Canceled", ui.ColorFaint},
	api.StatusTerminated:      {"■", "Terminated", ui.ColorOrange},
	api.StatusSkipped:         {"⊘", "Skipped", ui.ColorMuted},
	api.StatusNotAllowed:      {"⚠", "Not allowed", ui.ColorYellow},
}

// Status returns the look of a status, unknown statuses are shown as they are.
func Status(status string) StatusLook {
	if look, ok := statusLooks[status]; ok {
		return look
	}
	return StatusLook{"?", strings.ReplaceAll(status, "_", " "), ui.ColorMuted}
}

// StatusIcon renders the colored icon of a status.
func StatusIcon(status string) string {
	look := Status(status)
	return ui.Fg(look.Color).Render(look.Icon)
}

// StatusBadge renders the colored icon and label of a status.
func StatusBadge(status string) string {
	look := Status(status)
	return ui.Fg(look.Color).Render(look.Icon + " " + look.Label)
}
