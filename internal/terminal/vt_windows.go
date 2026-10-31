//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVirtualTerminal lets the Windows console interpret the escape sequences of the remote
// shell, and returns the function which restores the previous console mode.
func enableVirtualTerminal(out *os.File) func() {
	handle := windows.Handle(out.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return func() {}
	}
	_ = windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)

	return func() { _ = windows.SetConsoleMode(handle, mode) }
}
