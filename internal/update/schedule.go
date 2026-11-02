package update

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CheckInterval is how often pt looks for a new release on its own.
const CheckInterval = 24 * time.Hour

// Schedule remembers when pt last looked for a new release, so that starting it stays fast and a
// declined update is offered again only the next day.
type Schedule struct {
	// Path of the file with the time of the last check, empty disables the checks.
	Path string
}

// DefaultSchedule keeps the time in the cache directory of the user.
func DefaultSchedule() Schedule {
	dir, err := os.UserCacheDir()
	if err != nil {
		return Schedule{}
	}
	return Schedule{Path: filepath.Join(dir, "pipetower", "update-check")}
}

// Due tells whether it is time to look for a new release.
func (s Schedule) Due(now time.Time) bool {
	if s.Path == "" {
		return false
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return true
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	// a time in the future (a clock set back) must not suppress the checks
	return err != nil || now.Sub(last) >= CheckInterval || last.After(now)
}

// Checked records a check, whatever its result: offline, pt tries again the next day instead of
// waiting for the network on every start.
func (s Schedule) Checked(now time.Time) {
	if s.Path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(s.Path, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o600)
}
