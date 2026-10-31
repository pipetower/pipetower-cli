// Package debounce coalesces bursts of events into one, without starving under steady events.
package debounce

import "time"

// Debounce fires once events pause for the delay, but at the latest maxWait after the first event.
type Debounce struct {
	timer   *time.Timer
	delay   time.Duration
	maxWait time.Duration
	first   time.Time
}

// New creates a debounce which is idle until the first trigger.
func New(delay, maxWait time.Duration) *Debounce {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	return &Debounce{timer: timer, delay: delay, maxWait: maxWait}
}

// Trigger records an event and (re)schedules the firing.
func (d *Debounce) Trigger() {
	now := time.Now()
	if d.first.IsZero() {
		d.first = now
	}
	d.timer.Reset(max(0, min(d.delay, d.maxWait-now.Sub(d.first))))
}

// After schedules the firing after the duration, e.g. to retry.
func (d *Debounce) After(duration time.Duration) {
	d.timer.Reset(duration)
}

// C delivers the firings, every received firing must be confirmed with Fired.
func (d *Debounce) C() <-chan time.Time {
	return d.timer.C
}

// Fired starts a new burst after a firing was handled.
func (d *Debounce) Fired() {
	d.first = time.Time{}
}

// Stop cancels a scheduled firing.
func (d *Debounce) Stop() {
	d.timer.Stop()
	d.first = time.Time{}
}
