package gateway

import "github.com/pipetower/pipetower-cli/internal/fault"

// UnreachableError tells that the gateway WebSocket could not be opened.
type UnreachableError struct {
	URL string
	Err error
}

// Error implements error.
func (e *UnreachableError) Error() string {
	return "cannot reach the gateway at " + e.URL + ": " + e.Err.Error()
}

// Unwrap returns the cause.
func (e *UnreachableError) Unwrap() error {
	return e.Err
}

// Problem implements fault.Explainer.
func (e *UnreachableError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "Cannot reach the real-time gateway",
		Detail: e.URL + "\n" + e.Err.Error(),
		Hints:  []fault.Hint{{Label: "The gateway (wssh) of your Pipetower instance must be running and reachable"}},
		Code:   fault.ExitAPI,
	}
}

// AuthError tells that the gateway rejected the connection ticket.
type AuthError struct {
	Reason string
}

// Error implements error.
func (e *AuthError) Error() string {
	return "gateway authentication failed: " + e.Reason
}

// Problem implements fault.Explainer.
func (e *AuthError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "The real-time gateway rejected the connection",
		Detail: "Authentication failed: " + e.Reason + ".",
		Hints:  []fault.Hint{{Label: "Check that the clocks of the app and the gateway are in sync and both use the same WSSH_TOKEN"}},
		Code:   fault.ExitAPI,
	}
}
