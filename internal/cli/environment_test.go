package cli

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/prompt"
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

// picker is a prompter which picks the option at the index and records the options offered.
type picker struct {
	index   int
	offered []string
	secret  string
}

func (p *picker) Pick(_ string, options []prompt.Option) (int, error) {
	for _, option := range options {
		p.offered = append(p.offered, strings.TrimSpace(option.Mark+" "+option.Columns[0]))
	}
	return p.index, nil
}
func (p *picker) PickMany(string, []prompt.Option) ([]int, error) { return nil, nil }
func (p *picker) Input(string) (string, error)                    { return "", nil }
func (p *picker) Secret(string) (string, error)                   { return p.secret, nil }
func (p *picker) Confirm(string, string, string) (bool, error)    { return true, nil }

func TestTeamUseOffersAllTeamsAndTheTeamsToPick(t *testing.T) {
	dir, path := twoEnvironments(t, fakeAPI{
		"GET /api/v1/teams?limit=100": `{"data":[{"id":"t1","name":"Ops","members":[{}]},{"id":"t2","name":"Shop","members":[]}],"meta":{"has_more":false}}`,
	})
	run(t, dir, "env", "use", "local")

	all := &picker{index: 0}
	out := runWith(t, dir, all, "team", "use")
	shop := &picker{index: 2}
	picked := runWith(t, dir, shop, "team", "use")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Not limited to a team") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	if strings.Join(all.offered, "|") != "All teams|● Ops|Shop" {
		t.Errorf("offered = %q", all.offered)
	}
	if picked.code != fault.ExitOK || !strings.Contains(picked.stdout, "Limited to Shop") || strings.Join(shop.offered, "|") != "● All teams|Ops|Shop" {
		t.Errorf("exit %d, offered %q\nstdout:\n%s\nstderr:\n%s", picked.code, shop.offered, picked.stdout, picked.stderr)
	}
	if content, _ := os.ReadFile(path); !strings.Contains(string(content), `"team": "Shop"`) {
		t.Errorf("the file must hold Shop:\n%s", content)
	}
}

func TestEnvUseOffersTheEnvironmentsToPick(t *testing.T) {
	dir, _ := twoEnvironments(t, fakeAPI{})

	p := &picker{index: 0}
	out := runWith(t, dir, p, "env", "use")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Using local") || strings.Join(p.offered, "|") != "local|● prod" {
		t.Errorf("exit %d, offered %q\nstdout:\n%s\nstderr:\n%s", out.code, p.offered, out.stdout, out.stderr)
	}
}

func TestEnvAddCreatesTheFileAndFurtherOnesAreAddedToIt(t *testing.T) {
	dir := t.TempDir()

	first := run(t, dir, "env", "add", "--token", "p") // the public instance, named after its host
	second := run(t, dir, "env", "add", "--url", "http://pipetower.localhost/", "--token", "l", "--team", "Ops")
	third := run(t, dir, "env", "add", "staging", "--url", "https://staging.example.com", "--token", "s")
	list := run(t, dir, "env", "list")
	again := run(t, dir, "env", "add", "--token", "t")
	badURL := run(t, dir, "env", "add", "--url", "nonsense", "--token", "t")
	noToken := run(t, dir, "env", "add")

	if first.code != fault.ExitOK || !strings.Contains(first.stdout, "Added app.pipetower.com") || !strings.Contains(first.stdout, "https://app.pipetower.com") || !strings.Contains(first.stdout, "active") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", first.code, first.stdout, first.stderr)
	}
	if second.code != fault.ExitOK || !strings.Contains(second.stdout, "Added pipetower.localhost") || strings.Contains(second.stdout, "active") || !strings.Contains(second.stderr, "pt env use pipetower.localhost") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", second.code, second.stdout, second.stderr)
	}
	if third.code != fault.ExitOK || !strings.Contains(third.stdout, "Added staging") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", third.code, third.stdout, third.stderr)
	}
	if content, _ := os.ReadFile(filepath.Join(dir, ".pipetower")); !strings.Contains(string(content), `"environment": "app.pipetower.com"`) || !strings.Contains(string(content), `"team": "Ops"`) {
		t.Errorf("file:\n%s", content)
	}
	if list.code != fault.ExitOK || !regexp.MustCompile(`●\s+app.pipetower.com`).MatchString(list.stdout) || !strings.Contains(list.stdout, "Ops") {
		t.Errorf("exit %d\nstdout:\n%s", list.code, list.stdout)
	}
	if again.code != fault.ExitUsage || !strings.Contains(again.stderr, "“app.pipetower.com” exists already") {
		t.Errorf("exit %d, stderr:\n%s", again.code, again.stderr)
	}
	if badURL.code != fault.ExitUsage && badURL.code != fault.ExitConfig || noToken.code != fault.ExitUsage || !strings.Contains(noToken.stderr, "Missing --token") {
		t.Errorf("bad url: exit %d %s\nno token: exit %d %s", badURL.code, badURL.stderr, noToken.code, noToken.stderr)
	}
}

func TestEnvAddAsksForTheTokenHiddenInATerminal(t *testing.T) {
	dir := t.TempDir()

	out := runWith(t, dir, &picker{secret: "secret-token"}, "env", "add")

	content, _ := os.ReadFile(filepath.Join(dir, ".pipetower"))
	if out.code != fault.ExitOK || !strings.Contains(string(content), `"token": "secret-token"`) || strings.Contains(out.stdout, "secret-token") {
		t.Errorf("exit %d, file:\n%s\nstdout:\n%s\nstderr:\n%s", out.code, content, out.stdout, out.stderr)
	}
}

func TestEnvAddWritesTheConfiguredFileWhenHomeIsTaken(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".pipetower"), 0o755); err != nil { // the app keeps its scripts there
		t.Fatal(err)
	}
	configured := filepath.Join(dir, "config", ".pipetower")

	taken := run(t, dir, "env", "add", "--token", "t")
	out := runEnv(t, dir, map[string]string{"PIPETOWER_CONFIG": configured}, "env", "add", "--token", "t")
	list := runEnv(t, dir, map[string]string{"PIPETOWER_CONFIG": configured}, "env", "list")

	if taken.code != fault.ExitConfig || !strings.Contains(taken.stderr, "is a directory") || !strings.Contains(taken.stderr, "PIPETOWER_CONFIG") {
		t.Errorf("exit %d, stderr:\n%s", taken.code, taken.stderr)
	}
	if out.code != fault.ExitOK || !strings.Contains(out.stdout, configured) {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	if list.code != fault.ExitOK || !strings.Contains(list.stdout, "app.pipetower.com") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", list.code, list.stdout, list.stderr)
	}
}
