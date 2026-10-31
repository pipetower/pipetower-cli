// Package fault describes user-facing problems: what went wrong, why, and how to fix it.
// Packages return errors implementing Explainer, the UI renders them consistently.
package fault

import "errors"

// Exit codes of the CLI.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitConfig      = 3
	ExitAPI         = 4
	ExitInterrupted = 130
)

// Hint is an actionable suggestion: a short label and an optional command or URL.
type Hint struct {
	Label string
	Value string
}

// Problem is the user-facing description of an error.
type Problem struct {
	Title  string
	Detail string
	Hints  []Hint
	Code   int
}

// Explainer is an error which can describe itself as a Problem.
type Explainer interface {
	error
	Problem() Problem
}

// Error is a ready-made Explainer.
type Error struct {
	P     Problem
	Cause error
}

// New creates an Explainer error from a problem.
func New(p Problem) *Error {
	return &Error{P: p}
}

// Error implements error.
func (e *Error) Error() string {
	if e.P.Detail != "" {
		return e.P.Title + ": " + e.P.Detail
	}
	return e.P.Title
}

// Problem implements Explainer.
func (e *Error) Problem() Problem {
	return e.P
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Silent marks an error whose message was already shown, only its exit code matters.
type Silent struct {
	Code int
}

// Error implements error.
func (s *Silent) Error() string {
	return "exit status"
}

// ExitCode returns the exit code for an error.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var silent *Silent
	if errors.As(err, &silent) {
		return silent.Code
	}
	var explainer Explainer
	if errors.As(err, &explainer) && explainer.Problem().Code != 0 {
		return explainer.Problem().Code
	}
	return ExitFailure
}
