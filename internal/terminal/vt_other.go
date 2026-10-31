//go:build !windows

package terminal

import "os"

// enableVirtualTerminal is not needed outside of Windows, terminals interpret escape sequences.
func enableVirtualTerminal(*os.File) func() {
	return func() {}
}
