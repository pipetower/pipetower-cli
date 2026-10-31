package serverview

import (
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
)

func TestAnOlderFetchNeverReplacesANewerOne(t *testing.T) {
	m := &watchModel{}
	newer := Snapshot{Server: api.Server{Name: "newer"}}
	older := Snapshot{Server: api.Server{Name: "older"}}

	m.Update(snapshotMsg{snapshot: newer, seq: 2})
	m.Update(snapshotMsg{snapshot: older, seq: 1})

	if m.snapshot.Server.Name != "newer" {
		t.Errorf("snapshot = %q, the newer fetch must stay", m.snapshot.Server.Name)
	}
}
