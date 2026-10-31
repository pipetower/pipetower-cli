package debounce

import (
	"testing"
	"time"
)

func TestFiresOnceAfterABurst(t *testing.T) {
	d := New(20*time.Millisecond, time.Second)
	for range 5 {
		d.Trigger()
	}

	select {
	case <-d.C():
		d.Fired()
	case <-time.After(200 * time.Millisecond):
		t.Fatal("did not fire")
	}
	select {
	case <-d.C():
		t.Fatal("fired twice")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSteadyEventsCannotStarveTheFiring(t *testing.T) {
	d := New(30*time.Millisecond, 80*time.Millisecond)
	start := time.Now()
	stop := time.After(400 * time.Millisecond)

	for {
		select {
		case <-d.C():
			if waited := time.Since(start); waited > 200*time.Millisecond {
				t.Errorf("fired only after %v", waited)
			}
			return
		case <-stop:
			t.Fatal("never fired under steady events")
		case <-time.After(10 * time.Millisecond):
			d.Trigger()
		}
	}
}
