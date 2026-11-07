package cli

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// twoEnvironments writes a .pipetower with "local" (the fake API, team Ops) and "prod", prod active,
// and returns the directory and the path of the file.
func twoEnvironments(t *testing.T, api fakeAPI) (string, string) {
	t.Helper()
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	path := writeConfig(t, dir, `{"environment": "prod", "environments": {
		"local": {"url": "`+server.URL+`", "token": "test-token", "team": "Ops"},
		"prod": {"url": "https://app.pipetower.com", "token": "prod-token"}}}`)
	return dir, path
}

func TestEnvListMarksTheActiveEnvironment(t *testing.T) {
	dir, _ := twoEnvironments(t, fakeAPI{})

	out := run(t, dir, "env", "list")
	js := run(t, dir, "env", "list", "-o", "json")

	if out.code != fault.ExitOK || !regexp.MustCompile(`●\s+prod`).MatchString(out.stdout) || !strings.Contains(out.stdout, "local") || !strings.Contains(out.stdout, "Ops") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(js.stdout), &list); err != nil || len(list) != 2 || list[1]["active"] != true || list[0]["team"] != "Ops" {
		t.Errorf("json (err %v):\n%s", err, js.stdout)
	}
}

func TestEnvUseSwitchesTheEnvironmentForTheNextCommand(t *testing.T) {
	dir, path := twoEnvironments(t, fakeAPI{"GET /api/v1/teams?limit=100&search=Ops": opsTeam, "GET /api/v1/servers?limit=25&team=t1": serversPage})

	use := run(t, dir, "env", "use", "local")
	list := run(t, dir, "server", "list")
	unknown := run(t, dir, "env", "use", "staging")

	if use.code != fault.ExitOK || !strings.Contains(use.stdout, "Using local") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", use.code, use.stdout, use.stderr)
	}
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), `"environment": "local"`) || !strings.Contains(string(content), `"prod-token"`) {
		t.Errorf("the file must hold the new environment and keep the rest:\n%s", content)
	}
	if list.code != fault.ExitOK || !strings.Contains(list.stdout, "web-01") {
		t.Errorf("the next command must use local (team Ops): exit %d\nstdout:\n%s\nstderr:\n%s", list.code, list.stdout, list.stderr)
	}
	if unknown.code != fault.ExitConfig || !strings.Contains(unknown.stderr, "Unknown environment “staging”") || !strings.Contains(unknown.stderr, "local, prod") {
		t.Errorf("exit %d, stderr:\n%s", unknown.code, unknown.stderr)
	}
}

func TestTeamUseLimitsTheActiveEnvironmentAndAllLiftsIt(t *testing.T) {
	dir, path := twoEnvironments(t, fakeAPI{
		"GET /api/v1/teams?limit=100&search=Shop": `{"data":[{"id":"t2","name":"Shop","members":[]}],"meta":{"has_more":false}}`,
		"GET /api/v1/servers?limit=25&team=t2":    serversPage,
		"GET /api/v1/servers?limit=25":            serversPage,
	})
	run(t, dir, "env", "use", "local")

	use := run(t, dir, "team", "use", "Shop")
	if use.code != fault.ExitOK || !strings.Contains(use.stdout, "Limited to Shop in local") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", use.code, use.stdout, use.stderr)
	}
	if content, _ := os.ReadFile(path); !strings.Contains(string(content), `"team": "Shop"`) {
		t.Errorf("the file must hold the team:\n%s", content)
	}
	if limited := run(t, dir, "server", "list"); limited.code != fault.ExitOK || !strings.Contains(limited.stdout, "web-01") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", limited.code, limited.stdout, limited.stderr)
	}

	all := run(t, dir, "team", "use", "--all")
	if all.code != fault.ExitOK || !strings.Contains(all.stdout, "Not limited to a team in local") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", all.code, all.stdout, all.stderr)
	}
	if content, _ := os.ReadFile(path); strings.Contains(string(content), `"team"`) {
		t.Errorf("the team must be gone:\n%s", content)
	}
	if unlimited := run(t, dir, "server", "list"); unlimited.code != fault.ExitOK || !strings.Contains(unlimited.stdout, "web-01") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", unlimited.code, unlimited.stdout, unlimited.stderr)
	}

	missing := run(t, dir, "team", "use")
	if missing.code != fault.ExitUsage || !strings.Contains(missing.stderr, "Missing team") {
		t.Errorf("exit %d, stderr:\n%s", missing.code, missing.stderr)
	}
}

func TestSeveralEnvironmentsWithoutAnActiveOneExplainTheSwitch(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"environments": {"local": {"url": "http://localhost", "token": "l"}, "prod": {"url": "https://app", "token": "p"}}}`)

	out := run(t, dir, "server", "list")
	list := run(t, dir, "env", "list")

	if out.code != fault.ExitConfig || !strings.Contains(out.stderr, "Which environment?") || !strings.Contains(out.stderr, "pt env use <environment>") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
	if list.code != fault.ExitOK || !strings.Contains(list.stdout, "local") || !strings.Contains(list.stdout, "prod") {
		t.Errorf("env list must work without an active environment: exit %d\nstdout:\n%s\nstderr:\n%s", list.code, list.stdout, list.stderr)
	}
}

func TestEnvironmentsAreCompleted(t *testing.T) {
	dir, _ := twoEnvironments(t, fakeAPI{})

	out := run(t, dir, "__complete", "env", "use", "")

	if !strings.Contains(out.stdout, "local\t") || !strings.Contains(out.stdout, "prod\thttps://app.pipetower.com") {
		t.Errorf("stdout:\n%s", out.stdout)
	}
}
