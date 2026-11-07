package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/config"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// result is the outcome of one pt invocation.
type result struct {
	stdout string
	stderr string
	code   int
	opened string
}

// fakeAPI answers API paths (with query) with fixed JSON and rejects other tokens.
type fakeAPI map[string]string

func (f fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer test-token" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
		return
	}
	body, ok := f[r.Method+" "+r.URL.RequestURI()]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"The requested resource could not be found."}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// execute runs pt with the arguments against the API, configured by a .pipetower file with the token.
func execute(t *testing.T, api http.Handler, token string, args ...string) result {
	t.Helper()
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	if token != "" {
		writeConfig(t, dir, configFile(server.URL, token, ""))
	}

	return run(t, dir, args...)
}

// configFile is a .pipetower with one environment "test" of the URL, token and team.
func configFile(url, token, team string) string {
	content := `{"environment": "test", "environments": {"test": {"url": "` + url + `", "token": "` + token + `"`
	if team != "" {
		content += `, "team": "` + team + `"`
	}
	return content + "}}}\n"
}

// writeConfig writes the .pipetower of the directory.
func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, ".pipetower")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// run executes the command tree like main does, with plain non-interactive streams.
func run(t *testing.T, dir string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := config.Env{
		Getenv:     func(string) string { return "" },
		Cwd:        dir,
		Home:       dir,
		GitIgnored: func(string) (bool, bool) { return false, false },
	}
	newIO := func(bool, bool) *ui.IO {
		plain := func(w *bytes.Buffer) *colorprofile.Writer {
			return &colorprofile.Writer{Forward: w, Profile: colorprofile.NoTTY}
		}
		return &ui.IO{Out: plain(&stdout), Err: plain(&stderr), RawOut: &stdout, Profile: colorprofile.NoTTY}
	}

	opened := ""
	app := NewAppWith(env, newIO)
	app.openURL = func(url string) error {
		opened = url
		return nil
	}
	root := NewRootCommand(app)
	root.SetArgs(args)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SilenceErrors, root.SilenceUsage = true, true

	err := root.ExecuteContext(context.Background())
	if err != nil {
		app.HandleError(plainWriter(&stderr), err)
	}

	return result{stdout: stdout.String(), stderr: stderr.String(), code: ExitCode(err), opened: opened}
}

// plainWriter writes without colors.
func plainWriter(w *bytes.Buffer) *colorprofile.Writer {
	return &colorprofile.Writer{Forward: w, Profile: colorprofile.NoTTY}
}

const serversPage = `{"data":[
	{"id":"a1","team_id":"t1","name":"web-01","host":"10.0.0.1","port":22,"connected":true,"tags":[],
	 "metrics":{"cpu":{"usage_percent":12.5},"load":{"avg_1":0.4},"memory":{"used_percent":40},"disk":{"used_percent":70}}},
	{"id":"a2","team_id":"t1","name":"web-02","host":"10.0.0.2","port":22,"connected":false,"tags":[],"metrics":null}
],"meta":{"from":1,"to":2,"total":2,"per_page":100,"has_more":false}}`

func TestServerListTable(t *testing.T) {
	out := execute(t, fakeAPI{"GET /api/v1/servers?limit=25": serversPage}, "test-token", "server", "list")

	if out.code != fault.ExitOK {
		t.Fatalf("exit %d: %s", out.code, out.stderr)
	}
	golden.RequireEqual(t, out.stdout)
}

func TestServerListAsJSONKeepsTheAPIData(t *testing.T) {
	out := execute(t, fakeAPI{"GET /api/v1/servers?limit=25": serversPage}, "test-token", "server", "list", "-o", "json")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, `"name": "web-01"`) || !strings.Contains(out.stdout, `"usage_percent": 12.5`) {
		t.Errorf("exit %d, stdout:\n%s", out.code, out.stdout)
	}
}

func TestMissingConfigurationExplainsTheFile(t *testing.T) {
	out := run(t, "/nonexistent/project", "status")

	if out.code != fault.ExitConfig {
		t.Fatalf("exit %d, want %d", out.code, fault.ExitConfig)
	}
	golden.RequireEqual(t, out.stderr)
}

func TestRejectedTokenIsAConfigurationError(t *testing.T) {
	out := execute(t, fakeAPI{}, "revoked-token", "server", "list")

	if out.code != fault.ExitConfig || !strings.Contains(out.stderr, "token") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestAmbiguousNameWithoutTerminalListsTheCandidates(t *testing.T) {
	api := fakeAPI{"GET /api/v1/servers?limit=100&search=web": serversPage}

	out := execute(t, api, "test-token", "server", "view", "web")

	if out.code == fault.ExitOK || !strings.Contains(out.stderr, "web-01") || !strings.Contains(out.stderr, "web-02") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	out := execute(t, fakeAPI{}, "test-token", "deploy")

	if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "pt --help") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestTypoInCommandSuggestsTheIntendedOne(t *testing.T) {
	out := execute(t, fakeAPI{}, "test-token", "run", "lst")

	if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "pt run list") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestWrongCallShowsTheUsage(t *testing.T) {
	for _, args := range [][]string{{"run", "view"}, {"run", "view", "r1", "r2"}, {"run", "view", "r1", "--bogus"}} {
		out := execute(t, fakeAPI{}, "test-token", args...)
		if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "pt run view <run> [flags]") {
			t.Errorf("%v: exit %d, stderr:\n%s", args, out.code, out.stderr)
		}
	}
}

func TestWithoutTerminalResourcesMustBeGiven(t *testing.T) {
	for missing, calls := range map[string][][]string{
		"action":      {{"action", "view"}, {"action", "run"}, {"action", "export"}, {"action", "delete"}},
		"workflow":    {{"workflow", "view"}, {"workflow", "run"}, {"workflow", "export"}, {"workflow", "delete"}},
		"run":         {{"run", "view"}, {"run", "watch"}, {"run", "cancel"}},
		"server":      {{"server", "view"}, {"server", "connect"}, {"server", "disconnect"}, {"terminal", "open"}},
		"session":     {{"terminal", "join"}},
		"pipe":        {{"pipe", "view"}},
		"template":    {{"template", "view"}, {"template", "export"}, {"template", "delete"}},
		"team":        {{"team", "view"}, {"team", "use"}},
		"environment": {{"env", "use"}},
	} {
		for _, call := range calls {
			out := execute(t, fakeAPI{}, "test-token", call...)
			if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "Missing "+missing) {
				t.Errorf("pt %v: exit %d, stderr:\n%s", call, out.code, out.stderr)
			}
		}
	}
}

func TestHelpSeparatesOwnAndGlobalFlags(t *testing.T) {
	out := execute(t, fakeAPI{}, "test-token", "run", "view", "--help")
	own, global, found := strings.Cut(out.stdout, "GLOBAL FLAGS")

	if !found || !strings.Contains(own, "--log") || strings.Contains(own, "--help") || !strings.Contains(global, "Show the help") {
		t.Errorf("help:\n%s", out.stdout)
	}
}

const finishedRun = `{"id":"r1","team_id":"t1","status":"successful","executable":{"id":"a1","type":"Action","name":"Deploy"},
	"executions":[{"id":"e1","server":{"id":"a1","name":"web-01","host":"10.0.0.1"},"status":"successful","steps":[],
	"started_at":"2026-10-31T12:00:00+00:00","finished_at":"2026-10-31T12:00:05+00:00"}],
	"created_at":"2026-10-31T12:00:00+00:00","started_at":"2026-10-31T12:00:00+00:00","finished_at":"2026-10-31T12:00:05+00:00"}`

func TestFollowingARunAsJSONPrintsOnlyJSON(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/runs/r1?include=output": finishedRun,
		"GET /api/v1/runs/r1":                finishedRun,
	}

	out := execute(t, api, "test-token", "run", "watch", "r1", "-o", "json")

	var run map[string]any
	if err := json.Unmarshal([]byte(out.stdout), &run); err != nil || run["id"] != "r1" {
		t.Errorf("stdout must be the run as JSON (err %v):\n%s", err, out.stdout)
	}
	if out.code != fault.ExitOK || !strings.Contains(out.stderr, "Successful") {
		t.Errorf("exit %d, the summary belongs to stderr:\n%s", out.code, out.stderr)
	}
}

func TestTeamFilterIsAppliedByTheAPI(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/teams?limit=100&search=Ops": `{"data":[{"id":"t2","name":"Ops"}],"meta":{"from":1,"to":1,"total":1,"per_page":100,"has_more":false}}`,
		"GET /api/v1/servers?limit=25&team=t2":   serversPage,
	}

	out := execute(t, api, "test-token", "server", "list", "--team", "Ops")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "web-01") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
}

func TestUnknownRunIsNamed(t *testing.T) {
	out := execute(t, fakeAPI{}, "test-token", "run", "view", "zzz")

	if out.code != fault.ExitAPI || !strings.Contains(out.stderr, "Run “zzz” not found") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

// webServers answers the server search for "web" and the runs and variables of web-01.
func webServers(extra fakeAPI) fakeAPI {
	api := fakeAPI{"GET /api/v1/servers?limit=100&search=web-01": serversPage}
	for key, value := range extra {
		api[key] = value
	}
	return api
}

func TestRunListFiltersByNameAreResolvedToIDs(t *testing.T) {
	api := webServers(fakeAPI{"GET /api/v1/runs?limit=25&server=a1&status=failed": `{"data":[],"meta":{"from":null,"to":null,"total":0,"per_page":25,"has_more":false}}`})

	out := execute(t, api, "test-token", "run", "list", "--server", "web-01", "--status", "failed")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "No runs found.") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
}

func TestRunLogOfAServer(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/runs/r1":               finishedRun,
		"GET /api/v1/run-executions/e1/log": "Run #r1 log",
	}

	out := execute(t, api, "test-token", "run", "view", "r1", "--log", "--server", "web-01")
	unknown := execute(t, api, "test-token", "run", "view", "r1", "--log", "--server", "db-01")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Run #r1 log") {
		t.Errorf("exit %d, stdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	if unknown.code != fault.ExitUsage || !strings.Contains(unknown.stderr, "It ran on: web-01") {
		t.Errorf("exit %d, stderr:\n%s", unknown.code, unknown.stderr)
	}
}

func TestViewOpensTheWebPage(t *testing.T) {
	out := execute(t, fakeAPI{}, "test-token", "run", "view", "r1", "--web")

	if out.code != fault.ExitOK || !strings.HasSuffix(out.opened, "/runs/r1") {
		t.Errorf("exit %d, opened %q, stderr:\n%s", out.code, out.opened, out.stderr)
	}
}

func TestVariablesOfTheAccountOrAServer(t *testing.T) {
	variables := `{"data":[{"name":"TOKEN","value":"***","secret":true}],"meta":{"from":1,"to":1,"total":1,"per_page":25,"has_more":false}}`
	api := webServers(fakeAPI{"GET /api/v1/account/vars?limit=25": variables, "GET /api/v1/servers/a1/vars?limit=25": variables})

	for _, args := range [][]string{{"variable", "list"}, {"variable", "list", "--server", "web-01"}} {
		if out := execute(t, api, "test-token", args...); out.code != fault.ExitOK || !strings.Contains(out.stdout, "TOKEN") {
			t.Errorf("%v: exit %d\nstdout:\n%s\nstderr:\n%s", args, out.code, out.stdout, out.stderr)
		}
	}
}

// commandTree lists every command with its short description, nested commands indented.
func commandTree(cmd *cobra.Command, depth int) string {
	text := ""
	for _, child := range cmd.Commands() {
		text += fmt.Sprintf("%s%-*s %s\n", strings.Repeat("  ", depth), 24-2*depth, child.Use, child.Short)
		text += commandTree(child, depth+1)
	}
	return text
}

func TestGoldenCommandTree(t *testing.T) {
	golden.RequireEqual(t, commandTree(NewRootCommand(NewApp()), 0))
}

func TestNoCommandHasAliasesOrIsHidden(t *testing.T) {
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		if len(cmd.Aliases) > 0 || cmd.Hidden {
			t.Errorf("%q has aliases %v or is hidden: every command has one name", cmd.CommandPath(), cmd.Aliases)
		}
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(NewRootCommand(NewApp()))
}

func TestLogoOnlyWhenPtRunsWithoutCommand(t *testing.T) {
	logoLine := "╚═╝     ╚═╝╚═╝     ╚══════╝   ╚═╝    ╚═════╝  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝"

	if out := execute(t, fakeAPI{}, "test-token"); !strings.Contains(out.stdout, logoLine) {
		t.Errorf("pt must show the logo:\n%s", out.stdout)
	}
	if out := execute(t, fakeAPI{}, "test-token", "--help"); strings.Contains(out.stdout, logoLine) {
		t.Errorf("pt --help must not show the logo:\n%s", out.stdout)
	}
}
