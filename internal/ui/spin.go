package ui

import (
	"fmt"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spin shows a spinner with the text on stderr while work runs, only in terminals.
func Spin[T any](o *IO, text string, work func() (T, error)) (T, error) {
	if !o.ErrTTY {
		return work()
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			_, _ = fmt.Fprintf(o.Err, "\r %s %s", Accent.Render(spinnerFrames[i%len(spinnerFrames)]), Muted.Render(text))
			select {
			case <-done:
				_, _ = fmt.Fprint(o.Err, "\r\x1b[2K")
				return
			case <-ticker.C:
			}
		}
	}()

	value, err := work()
	close(done)
	<-stopped
	return value, err
}
