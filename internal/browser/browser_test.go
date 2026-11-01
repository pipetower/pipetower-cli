package browser

import (
	"slices"
	"testing"
)

func TestCommandOpensTheURLWithTheBrowserOfTheSystem(t *testing.T) {
	cases := []struct {
		browser, goos, program string
	}{
		{"firefox", "linux", "firefox"},
		{"", "linux", "xdg-open"},
		{"", "darwin", "open"},
		{"", "windows", "rundll32"},
	}
	for _, c := range cases {
		program, args := command("https://x/runs/1", c.browser, c.goos)
		if program != c.program || !slices.Contains(args, "https://x/runs/1") {
			t.Errorf("%s/%s: %s %v", c.browser, c.goos, program, args)
		}
	}
}
