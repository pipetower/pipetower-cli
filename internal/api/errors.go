package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// Error is an error response of the API.
type Error struct {
	Instance string
	Status   int
	Method   string
	Path     string
	Message  string
	Fields   map[string][]string
}

// newError decodes an error response.
func newError(instance string, e Endpoint, status int, body []byte) *Error {
	var payload struct {
		Message string              `json:"message"`
		Errors  map[string][]string `json:"errors"`
	}
	_ = json.Unmarshal(body, &payload)

	return &Error{Instance: instance, Status: status, Method: e.Method, Path: e.Path, Message: payload.Message, Fields: payload.Errors}
}

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.Status, e.Message)
}

// IsNotFound tells whether the error is a 404 response.
func IsNotFound(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// IsTransient tells whether retrying the request later can succeed: the instance was
// unreachable, timed out, is overloaded or failed with a server error.
func IsTransient(err error) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Status >= http.StatusInternalServerError || apiErr.Status == http.StatusTooManyRequests
	}
	var connErr *ConnectionError
	return errors.As(err, &connErr)
}

// FieldErrors returns the validation messages sorted by field.
func (e *Error) FieldErrors() []string {
	fields := make([]string, 0, len(e.Fields))
	for field := range e.Fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	var lines []string
	for _, field := range fields {
		lines = append(lines, field+": "+strings.Join(e.Fields[field], " "))
	}
	return lines
}

// Problem implements fault.Explainer.
func (e *Error) Problem() fault.Problem {
	switch e.Status {
	case http.StatusUnauthorized:
		return fault.Problem{
			Title:  "Your API token was rejected",
			Detail: "The token is invalid, expired or was revoked.",
			Hints:  []fault.Hint{{Label: "Create a new token", Value: e.Instance + "/account  →  API tokens"}, {Label: "Replace the configured PIPETOWER_TOKEN"}},
			Code:   fault.ExitConfig,
		}
	case http.StatusForbidden:
		return fault.Problem{Title: "Permission denied", Detail: e.messageOr("You are not allowed to do this."), Code: fault.ExitAPI}
	case http.StatusNotFound:
		return fault.Problem{Title: "Not found", Detail: e.messageOr("The requested resource does not exist."), Code: fault.ExitAPI}
	case http.StatusUnprocessableEntity:
		return fault.Problem{Title: e.messageOr("The request is invalid"), Detail: strings.Join(e.FieldErrors(), "\n"), Code: fault.ExitAPI}
	case http.StatusTooManyRequests:
		return fault.Problem{Title: "Too many requests", Detail: "Wait a moment and try again.", Code: fault.ExitAPI}
	}

	return fault.Problem{
		Title:  fmt.Sprintf("The API answered with %d %s", e.Status, http.StatusText(e.Status)),
		Detail: e.messageOr(e.Method + " " + e.Path),
		Code:   fault.ExitAPI,
	}
}

// messageOr returns the API message or the fallback.
func (e *Error) messageOr(fallback string) string {
	if e.Message != "" {
		return e.Message
	}
	return fallback
}

// ConnectionError tells that the instance could not be reached.
type ConnectionError struct {
	URL string
	Err error
}

// Error implements error.
func (e *ConnectionError) Error() string {
	return "cannot reach " + e.URL + ": " + e.Err.Error()
}

// Unwrap returns the network error.
func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// Problem implements fault.Explainer.
func (e *ConnectionError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "Cannot reach " + e.URL,
		Detail: e.Err.Error(),
		Hints: []fault.Hint{
			{Label: "Check your connection and the configured PIPETOWER_URL"},
		},
		Code: fault.ExitAPI,
	}
}

// RedirectError tells that the instance redirects the API, the configured URL is not the final one.
type RedirectError struct {
	URL      string
	Location string
}

// Error implements error.
func (e *RedirectError) Error() string {
	return e.URL + " redirects to " + e.Location
}

// Problem implements fault.Explainer.
func (e *RedirectError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "The Pipetower URL redirects",
		Detail: fmt.Sprintf("%s redirects to %s. Redirects are not followed, so that no request changes on the way.", e.URL, e.Location),
		Hints:  []fault.Hint{{Label: "Set PIPETOWER_URL to the final URL, e.g. with https://"}},
		Code:   fault.ExitConfig,
	}
}
