//go:build windows

package terminal

import (
	"os"
	"time"
)

// watchResize polls the console size, Windows has no resize signal.
func watchResize(out *os.File) (<-chan os.Signal, func()) {
	changes := make(chan os.Signal, 1)
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		cols, rows := size(out)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if c, r := size(out); c != cols || r != rows {
					cols, rows = c, r
					changes <- os.Interrupt
				}
			}
		}
	}()

	return changes, func() { close(done) }
}
