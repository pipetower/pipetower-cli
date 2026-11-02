package update

import (
	"fmt"
	"runtime"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// NotWritableError means that the binary can't be replaced, e.g. because it was installed with sudo.
type NotWritableError struct {
	Dir string
}

func (e *NotWritableError) Error() string {
	return "no permission to write to " + e.Dir
}

// Problem explains how to update with the permission needed.
func (e *NotWritableError) Problem() fault.Problem {
	command := "curl -fsSL https://pipetower.com/install-cli | sudo bash"
	if e.Dir != "/usr/local/bin" {
		command = "curl -fsSL https://pipetower.com/install-cli | sudo PT_INSTALL_DIR=" + e.Dir + " bash"
	}
	return fault.Problem{
		Title:  "Can't update pt in " + e.Dir,
		Detail: "You have no permission to write to this directory.",
		Hints:  []fault.Hint{{Label: "Update with", Value: command}},
		Code:   fault.ExitFailure,
	}
}

// ChecksumError means that the downloaded archive is not the published one.
type ChecksumError struct {
	Archive string
}

func (e *ChecksumError) Error() string {
	return "checksum mismatch of " + e.Archive
}

// Problem explains the damaged download.
func (e *ChecksumError) Problem() fault.Problem {
	return fault.Problem{
		Title:  "The download of pt is damaged",
		Detail: "The checksum of " + e.Archive + " doesn't match the release, pt stays unchanged.",
		Hints:  []fault.Hint{{Label: "Try again later"}},
		Code:   fault.ExitFailure,
	}
}

// MissingError means that the release lacks a file, e.g. a build for this platform.
type MissingError struct {
	Release Release
	Name    string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("pt %s has no %s", e.Release.Version, e.Name)
}

// Problem explains the missing file.
func (e *MissingError) Problem() fault.Problem {
	return fault.Problem{
		Title:  fmt.Sprintf("pt %s is not available for %s/%s", e.Release.Version, runtime.GOOS, runtime.GOARCH),
		Detail: "The release has no " + e.Name + ".",
		Hints:  []fault.Hint{{Label: "Release", Value: e.Release.URL}},
		Code:   fault.ExitFailure,
	}
}
