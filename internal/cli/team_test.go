package cli

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

// opsTeam is the team Ops (t1), the only one which matches "Ops".
const opsTeam = `{"data":[{"id":"t1","name":"Ops","members":[]}],"meta":{"has_more":false}}`

const emptyPage = `{"data":[],"meta":{"has_more":false}}`

// executeInProject runs pt in a folder whose .pipetower sets the team besides URL and token.
func executeInProject(t *testing.T, routes fakeAPI, team string, args ...string) (result, string) {
	t.Helper()
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	file := filepath.Join(dir, ".pipetower")
	content := "PIPETOWER_URL=" + server.URL + "\nPIPETOWER_TOKEN=test-token\nPIPETOWER_TEAM=" + team + "\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return run(t, dir, args...), file
}

func TestTheTeamOfTheProjectLimitsLists(t *testing.T) {
	routes := fakeAPI{
		"GET /api/v1/teams?limit=100&search=Ops": opsTeam,
		"GET /api/v1/servers?limit=25&team=t1":   serversPage,
	}

	out, _ := executeInProject(t, routes, "Ops", "server", "list")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "web-01") || strings.Contains(out.stderr, "Team Ops") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
}

func TestAllTeamsIgnoresTheTeamOfTheProject(t *testing.T) {
	out, _ := executeInProject(t, fakeAPI{"GET /api/v1/servers?limit=25": serversPage}, "Ops", "server", "list", "--all-teams")

	if out.code != fault.ExitOK || strings.Contains(out.stderr, "Team Ops") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestTheTeamFlagOverridesTheProjectWithoutNote(t *testing.T) {
	routes := fakeAPI{
		"GET /api/v1/teams?limit=100&search=Shop": `{"data":[{"id":"t2","name":"Shop","members":[]}],"meta":{"has_more":false}}`,
		"GET /api/v1/servers?limit=25&team=t2":    serversPage,
	}

	out, _ := executeInProject(t, routes, "Ops", "server", "list", "--team", "Shop")

	if out.code != fault.ExitOK || strings.Contains(out.stderr, "set in") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestTeamAndAllTeamsExcludeEachOther(t *testing.T) {
	out := execute(t, fakeAPI{}, "test-token", "server", "list", "--team", "Ops", "--all-teams")

	if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "Use either --team or --all-teams") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestAnUnknownTeamOfTheProjectNamesItsFile(t *testing.T) {
	out, file := executeInProject(t, fakeAPI{"GET /api/v1/teams?limit=100&search=Gone": emptyPage}, "Gone", "server", "list")

	if out.code != fault.ExitAPI || !strings.Contains(out.stderr, "Team “Gone” not found") || !strings.Contains(out.stderr, file) {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestStatusNamesTheTeam(t *testing.T) {
	r := statusReport{Team: "Ops", TeamSource: "/code/project/.pipetower"}

	if text := teamLine(r); !strings.Contains(text, "Ops") || !strings.Contains(text, "set in /code/project/.pipetower") {
		t.Errorf("line = %q", text)
	}
	if text := teamLine(statusReport{}); !strings.Contains(text, "all your teams") {
		t.Errorf("line = %q", text)
	}
}

func TestTerminalSessionsOfTheTeamOnly(t *testing.T) {
	a := NewApp()
	a.team, a.resolved = api.Team{ID: "t1", Name: "Ops"}, true
	sessions := []gateway.LobbySession{{ID: "s1", Team: "t1"}, {ID: "s2", Team: "t2"}}

	kept, err := teamSessions(context.Background(), a, sessions)

	if err != nil || len(kept) != 1 || kept[0].ID != "s1" {
		t.Errorf("sessions = %v, err = %v", kept, err)
	}
}

func TestVariablesOfAServerInAProjectWithATeam(t *testing.T) {
	variables := `{"data":[{"name":"TOKEN","value":"***","secret":true}],"meta":{"has_more":false}}`
	routes := fakeAPI{
		"GET /api/v1/teams?limit=100&search=Ops":              opsTeam,
		"GET /api/v1/servers?limit=100&search=web-01&team=t1": serversPage,
		"GET /api/v1/servers/a1/vars?limit=25":                variables,
		"GET /api/v1/teams/t1/vars?limit=25":                  variables,
	}

	for _, args := range [][]string{{"variable", "list", "--server", "web-01"}, {"variable", "list"}} {
		if out, _ := executeInProject(t, routes, "Ops", args...); out.code != fault.ExitOK || !strings.Contains(out.stdout, "TOKEN") {
			t.Errorf("%v: exit %d\nstdout:\n%s\nstderr:\n%s", args, out.code, out.stdout, out.stderr)
		}
	}
}
