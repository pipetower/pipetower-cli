//go:build !windows

package terminal

import (
	"os"
	"os/signal"
	"syscall"
)

// watchResize reports terminal size changes (SIGWINCH).
func watchResize(_ *os.File) (<-chan os.Signal, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	return signals, func() { signal.Stop(signals) }
}
