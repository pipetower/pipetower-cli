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

// statusLooks follow the status icons of the web app, as close as a terminal gets: the same color,
// and the character closest to its icon. Web icon → character:
//
//	pending      clock (gray)                  ◷
//	running      spinning ring (yellow)        ◌, a spinner in live views
//	successful   check (green)                 ✓
//	failed       x mark (red)                  ✗
//	partially    exclamation circle (amber)    ⚠
//	canceled     no symbol (light gray)        ⊘
//	terminated   stop (gray)                   ■
//	skipped      minus circle (light gray)     ⊖
//	not allowed  raised hand (gray)            ⊗ (a hand exists only as a wide, colored emoji)
var statusLooks = map[string]StatusLook{
	api.StatusPending:         {"◷", "Pending", ui.ColorMuted},
	api.StatusRunning:         {"◌", "Running", ui.ColorYellow},
	api.StatusSuccessful:      {"✓", "Successful", ui.ColorGreen},
	api.StatusFailed:          {"✗", "Failed", ui.ColorRed},
	api.StatusPartiallyFailed: {"⚠", "Partially failed", ui.ColorAmber},
	api.StatusCanceled:        {"⊘", "Canceled", ui.ColorFaint},
	api.StatusTerminated:      {"■", "Terminated", ui.ColorMuted},
	api.StatusSkipped:         {"⊖", "Skipped", ui.ColorFaint},
	api.StatusNotAllowed:      {"⊗", "Not allowed", ui.ColorMuted},
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

// StepIcon renders the icon of an action step within an execution. Like in the web app, a
// successful step is gray, so that the steps which did not succeed stand out.
func StepIcon(status string) string {
	if status == api.StatusSuccessful {
		return ui.Muted.Render(Status(status).Icon)
	}
	return StatusIcon(status)
}
