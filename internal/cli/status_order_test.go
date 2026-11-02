package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestStatusShowsTheConnectionBeforeTheAccount(t *testing.T) {
	text := ansi.Strip(renderStatus(statusReport{Instance: "https://app", User: "Jamil", Teams: []string{"AT"}, Gateway: "connected"}))

	last := -1
	for _, label := range []string{"API", "Gateway", "User", "Teams", "Limited to", "URL from", "Token from"} {
		at := strings.Index(text, " "+label+" ")
		if at <= last {
			t.Fatalf("%s is out of order:\n%s", label, text)
		}
		last = at
	}
}
