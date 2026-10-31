package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/pipetower/pipetower-cli/internal/config"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// result is the outcome of one pt invocation.
type result struct {
	stdout string
	stderr string
	code   int
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
		file := "PIPETOWER_URL=" + server.URL + "\nPIPETOWER_TOKEN=" + token + "\n"
		if err := os.WriteFile(filepath.Join(dir, ".pipetower"), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return run(t, dir, args...)
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

	app := NewAppWith(env, newIO)
	root := NewRootCommand(app)
	root.SetArgs(args)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SilenceErrors, root.SilenceUsage = true, true

	err := root.ExecuteContext(context.Background())
	if err != nil {
		app.HandleError(plainWriter(&stderr), err)
	}

	return result{stdout: stdout.String(), stderr: stderr.String(), code: ExitCode(err)}
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

	out := execute(t, api, "test-token", "server", "show", "web")

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

const finishedRun = `{"id":"r1","team_id":"t1","status":"successful","executable":{"id":"a1","type":"Action","name":"Deploy"},
	"executions":[{"id":"e1","server":{"id":"a1","name":"web-01","host":"10.0.0.1"},"status":"successful","steps":[],
	"started_at":"2026-10-31T12:00:00+00:00","finished_at":"2026-10-31T12:00:05+00:00"}],
	"created_at":"2026-10-31T12:00:00+00:00","started_at":"2026-10-31T12:00:00+00:00","finished_at":"2026-10-31T12:00:05+00:00"}`

func TestFollowingARunAsJSONPrintsOnlyJSON(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/runs/r1?include=output": finishedRun,
		"GET /api/v1/runs/r1":                finishedRun,
	}

	out := execute(t, api, "test-token", "runs", "show", "r1", "-f", "-o", "json")

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
	out := execute(t, fakeAPI{}, "test-token", "runs", "show", "zzz")

	if out.code != fault.ExitAPI || !strings.Contains(out.stderr, "Run “zzz” not found") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}
