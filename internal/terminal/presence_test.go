package terminal

import (
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

func TestPresenceNotices(t *testing.T) {
	p := newPresence(api.Session{Role: "guest", Server: api.ServerRef{Name: "prod"}})
	owner := gateway.Participant{ID: "u1", Name: "Jamil", Control: true}
	me := gateway.Participant{ID: "u2", Name: "Anna"}

	if p.hasControl("u2") {
		t.Error("a guest has no control before the first update")
	}
	if notice := p.update("u2", []gateway.Participant{owner, me}, nil); notice != "" {
		t.Errorf("first update should be silent, got %q", notice)
	}

	me.Control = true
	guest := gateway.Participant{ID: "u3", Name: "Ben"}
	notice := p.update("u2", []gateway.Participant{owner, me, guest}, []string{"u3"})

	for _, want := range []string{"you got control", "Ben joined", "Ben requests control"} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice %q misses %q", notice, want)
		}
	}
	if !p.hasControl("u2") || !strings.Contains(p.describe("u2"), "prod · control · 3 attached") {
		t.Errorf("describe = %q", p.describe("u2"))
	}

	if notice := p.update("u2", []gateway.Participant{owner, me}, nil); notice != "Ben left" {
		t.Errorf("notice = %q", notice)
	}
}
