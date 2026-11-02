package ui

import (
	"fmt"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/progress"
)

// progressWidth is the width of the bar in a progress line.
const progressWidth = 32

// Progress shows a transfer on stderr: the text, a progress bar, the percentage and the bytes
// transferred. Outside a terminal only the text is printed once.
type Progress struct {
	o           *IO
	text        string
	done, total atomic.Int64
	stop        chan struct{}
	stopped     chan struct{}
}

// StartProgress starts showing the progress of a transfer, update it with Set and end it with Stop.
func StartProgress(o *IO, text string) *Progress {
	p := &Progress{o: o, text: text}
	if !o.ErrTTY {
		o.Errorln(" " + text + "…")
		return p
	}

	p.stop, p.stopped = make(chan struct{}), make(chan struct{})
	go p.show()
	return p
}

// Set reports the bytes transferred so far and the total, which is 0 when unknown.
func (p *Progress) Set(done, total int64) {
	p.done.Store(done)
	p.total.Store(total)
}

// Stop ends the progress and clears its line.
func (p *Progress) Stop() {
	if p.stop == nil {
		return
	}
	close(p.stop)
	<-p.stopped
}

// show redraws the progress line until it is stopped.
func (p *Progress) show() {
	defer close(p.stopped)
	bar := progress.New(
		progress.WithColors(ColorBrand, ColorBlue),
		progress.WithFillCharacters('━', '━'),
		progress.WithWidth(progressWidth),
		progress.WithoutPercentage(),
	)
	bar.EmptyColor = ColorFaint

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for frame := 0; ; frame++ {
		_, _ = fmt.Fprint(p.o.Err, "\r\x1b[2K"+p.line(bar, frame))
		select {
		case <-p.stop:
			_, _ = fmt.Fprint(p.o.Err, "\r\x1b[2K")
			return
		case <-ticker.C:
		}
	}
}

// line renders the progress: " Downloading pt 0.2.0  ━━━━━━━━━━━━━━━━━━━━  62%  3.1 MB / 5.0 MB".
func (p *Progress) line(bar progress.Model, frame int) string {
	done, total := p.done.Load(), p.total.Load()
	if total <= 0 {
		// the size is unknown: a spinner instead of the bar
		return " " + Accent.Render(spinnerFrames[frame/2%len(spinnerFrames)]) + " " + p.text + "  " + Muted.Render(Bytes(float64(done)))
	}

	ratio := min(1, float64(done)/float64(total))
	return " " + p.text + "  " + bar.ViewAs(ratio) + "  " + Bold.Render(fmt.Sprintf("%3.0f%%", ratio*100)) +
		"  " + Muted.Render(Bytes(float64(done))+" / "+Bytes(float64(total)))
}
