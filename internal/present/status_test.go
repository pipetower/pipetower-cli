package present

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/pipetower/pipetower-cli/internal/api"
)

func TestEveryStatusHasItsOwnIcon(t *testing.T) {
	seen := map[string]string{}
	for status, look := range statusLooks {
		if other, ok := seen[look.Icon]; ok {
			t.Errorf("%s and %s share the icon %s", status, other, look.Icon)
		}
		seen[look.Icon] = status
	}
}

func TestSuccessfulStepsAreGrayLikeInTheWebApp(t *testing.T) {
	if StepIcon(api.StatusSuccessful) == StatusIcon(api.StatusSuccessful) {
		t.Error("a successful step looks like a successful run")
	}
	if ansi.Strip(StepIcon(api.StatusSuccessful)) != "✓" {
		t.Errorf("step icon = %q", ansi.Strip(StepIcon(api.StatusSuccessful)))
	}
	if StepIcon(api.StatusFailed) != StatusIcon(api.StatusFailed) {
		t.Error("a failed step must look like a failed run")
	}
}
