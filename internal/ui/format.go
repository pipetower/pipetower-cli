package ui

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Duration renders a duration compactly: 850ms, 34s, 2m 05s, 1h 02m.
func Duration(d time.Duration) string {
	switch {
	case d <= 0:
		return "0s"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// Elapsed renders the time between start and end, or until now if end is nil.
func Elapsed(start, end *time.Time, now time.Time) string {
	if start == nil {
		return ""
	}
	return Duration(ElapsedTime(start, end, now))
}

// ElapsedTime is the time from start to end, or until now while running (zero without start).
func ElapsedTime(start, end *time.Time, now time.Time) time.Duration {
	if start == nil {
		return 0
	}
	if end != nil {
		now = *end
	}
	return now.Sub(*start).Truncate(time.Second)
}

// Clock renders a running duration like a stopwatch: 0:07, 12:34, 1:02:03.
func Clock(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// Bytes renders a byte size with a binary unit: 3.3 GB.
func Bytes(bytes float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for bytes >= 1024 && i < len(units)-1 {
		bytes /= 1024
		i++
	}
	if i == 0 || bytes >= 10 {
		return fmt.Sprintf("%.0f %s", bytes, units[i])
	}
	return fmt.Sprintf("%.1f %s", bytes, units[i])
}

// Ago renders a time relative to now: just now, 5 minutes ago, 2 days ago.
func Ago(t *time.Time, now time.Time) string {
	if t == nil {
		return ""
	}
	d := now.Sub(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return Plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return Plural(int(d.Hours()), "hour") + " ago"
	case d < 30*24*time.Hour:
		return Plural(int(d.Hours()/24), "day") + " ago"
	case d < 365*24*time.Hour:
		return Plural(int(d.Hours()/24/30), "month") + " ago"
	default:
		return Plural(int(d.Hours()/24/365), "year") + " ago"
	}
}

// Plural renders "1 server" or "3 servers".
func Plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Percent renders a percentage without needless decimals.
func Percent(value float64) string {
	return fmt.Sprintf("%.0f%%", math.Round(value))
}

// Float renders an optional number, or a dash.
func Float(value *float64, decimals int) string {
	if value == nil {
		return "–"
	}
	return fmt.Sprintf("%.*f", decimals, *value)
}

// Or returns the value or the fallback when it is empty.
func Or(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// FirstTime returns the first non-nil time.
func FirstTime(times ...*time.Time) *time.Time {
	for _, t := range times {
		if t != nil {
			return t
		}
	}
	return nil
}

// Capitalize upper-cases the first letter.
func Capitalize(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
