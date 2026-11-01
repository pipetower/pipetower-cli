//go:build windows

package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableVirtualTerminal lets the Windows console interpret escape sequences (colors, cursor
// movements), and returns the function which restores the previous console mode.
func EnableVirtualTerminal(out *os.File) func() {
	handle := windows.Handle(out.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return func() {}
	}
	_ = windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)

	return func() { _ = windows.SetConsoleMode(handle, mode) }
}
