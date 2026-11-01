//go:build !windows

package ui

import "os"

// EnableVirtualTerminal is not needed outside of Windows, terminals interpret escape sequences.
func EnableVirtualTerminal(*os.File) func() {
	return func() {}
}
