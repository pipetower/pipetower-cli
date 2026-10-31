package fault

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeOfErrors(t *testing.T) {
	explained := New(Problem{Title: "No API token found", Code: ExitConfig})
	cases := map[string]struct {
		err  error
		want int
	}{
		"no error":               {nil, ExitOK},
		"plain error":            {errors.New("boom"), ExitFailure},
		"explained":              {explained, ExitConfig},
		"wrapped explained":      {fmt.Errorf("loading: %w", explained), ExitConfig},
		"explained without code": {New(Problem{Title: "Oops"}), ExitFailure},
		"silent":                 {&Silent{Code: ExitInterrupted}, ExitInterrupted},
	}
	for name, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("%s: ExitCode = %d, want %d", name, got, c.want)
		}
	}
}

func TestErrorMessageAndCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := &Error{P: Problem{Title: "Pipetower is not reachable", Detail: "http://pipetower.localhost"}, Cause: cause}

	if err.Error() != "Pipetower is not reachable: http://pipetower.localhost" {
		t.Errorf("Error() = %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("the cause must be unwrappable")
	}
	if New(Problem{Title: "Only a title"}).Error() != "Only a title" {
		t.Error("a problem without detail is its title")
	}
}
