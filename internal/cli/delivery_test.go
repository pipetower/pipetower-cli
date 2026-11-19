package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/resolve"
)

// runAPI answers what starting a run of the action "Deploy" needs and keeps the request which starts it.
type runAPI struct {
	// variables are the variable definitions of the action, as JSON.
	variables string
	// previousRuns is the list of the runs the action had before, as JSON.
	previousRuns string
	started      api.RunRequest
	runsQuery    string
}

func (f *runAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	page := func(items string) string {
		return `{"data":[` + items + `],"meta":{"from":1,"to":1,"total":1,"per_page":100,"has_more":false}}`
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/runs":
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &f.started)
		_, _ = w.Write([]byte(finishedRun))
	case r.URL.Path == "/api/v1/runs":
		f.runsQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(page(f.previousRuns)))
	case r.URL.Path == "/api/v1/actions":
		_, _ = w.Write([]byte(page(`{"id":"a1","team_id":"t1","name":"Deploy","steps":[],"variables":[` + f.variables + `],"tags":[]}`)))
	case r.URL.Path == "/api/v1/servers":
		_, _ = w.Write([]byte(serversPage))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"The requested resource could not be found."}`))
	}
}

// start runs pt in the directory against the API.
func (f *runAPI) start(t *testing.T, dir string, args ...string) result {
	t.Helper()
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	writeConfig(t, dir, configFile(server.URL, "test-token", ""))

	return run(t, dir, args...)
}

// git runs a git command in the directory with a fixed author.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=ada@example.com", "GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=ada@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit adds a commit with the subject to the repository and returns its hash.
func commit(t *testing.T, dir, subject string) string {
	t.Helper()
	git(t, dir, "commit", "--quiet", "--allow-empty", "--message", subject)
	return git(t, dir, "rev-parse", "HEAD")
}

// clonedRepository creates a repository with two commits and a clone of it, which got a third
// commit only the origin has. It returns the directory of the clone and the three commits.
func clonedRepository(t *testing.T) (string, []string) {
	t.Helper()
	origin, clone := t.TempDir(), t.TempDir()
	git(t, origin, "init", "--quiet", "--initial-branch=main")
	commits := []string{commit(t, origin, "First"), commit(t, origin, "Second")}
	git(t, clone, "clone", "--quiet", origin, ".")

	return clone, append(commits, commit(t, origin, "Third"))
}

func TestMetaGitSendsTheCommitsSinceTheLastRunAsMetadata(t *testing.T) {
	clone, commits := clonedRepository(t)
	fake := &runAPI{
		variables:    `{"name":"COMMIT","type":"text","optional":true}`,
		previousRuns: `{"id":"r8","status":"successful","metadata":{"commit":"` + commits[0] + `"}}`,
	}

	out := fake.start(t, clone, "action", "run", "Deploy", "-s", "web-01", "--detach", "--meta-git", "--meta", "ticket=OPS-12")

	metadata := fake.started.Metadata
	if out.code != fault.ExitOK || metadata["commit"] != commits[2] || metadata["previous_commit"] != commits[0] || metadata["branch"] != "main" || metadata["ticket"] != "OPS-12" {
		t.Fatalf("exit %d, metadata = %v, stderr:\n%s", out.code, metadata, out.stderr)
	}
	if lines := strings.Split(metadata["commits"], "\n"); len(lines) != 2 || !strings.HasSuffix(lines[0], "Ada  Third") || !strings.HasSuffix(lines[1], "Ada  Second") {
		t.Errorf("commits = %q", metadata["commits"])
	}
	if fake.started.Variables["COMMIT"] != commits[2] {
		t.Errorf("variables = %v", fake.started.Variables)
	}
	if fake.runsQuery != "action=a1&limit=1&server=a1&status=successful" {
		t.Errorf("runs query = %q", fake.runsQuery)
	}
	if !strings.Contains(out.stderr, "Commit "+commits[2][:8]+" of origin/main") {
		t.Errorf("stderr:\n%s", out.stderr)
	}
}

func TestMetaGitListsNoCommitsUnlessTheLastRunToldItsCommit(t *testing.T) {
	clone, commits := clonedRepository(t)

	for name, previousRuns := range map[string]string{
		"no run yet":                     "",
		"the last run told no commit":    `{"id":"r9","status":"successful","metadata":null},{"id":"r8","status":"successful","metadata":{"commit":"` + commits[0] + `"}}`,
		"the last run told other things": `{"id":"r9","status":"successful","metadata":{"ticket":"OPS-12"}}`,
	} {
		fake := &runAPI{previousRuns: previousRuns}

		out := fake.start(t, clone, "action", "run", "Deploy", "-s", "web-01", "--detach", "--meta-git="+commits[1])

		metadata := fake.started.Metadata
		if out.code != fault.ExitOK || len(metadata) != 1 || metadata["commit"] != commits[1] {
			t.Errorf("%s: exit %d, metadata = %v, stderr:\n%s", name, out.code, metadata, out.stderr)
		}
		if !strings.Contains(out.stderr, "new commits are not listed because the last run recorded no commit") {
			t.Errorf("%s: stderr:\n%s", name, out.stderr)
		}
		if len(fake.started.Variables) != 0 {
			t.Errorf("%s: an action without the variable gets none: %v", name, fake.started.Variables)
		}
	}
}

func TestMetadataFlagsWinOverMetaGit(t *testing.T) {
	clone, commits := clonedRepository(t)
	fake := &runAPI{}

	fake.start(t, clone, "action", "run", "Deploy", "-s", "web-01", "--detach", "--meta-git="+commits[1], "--meta", "commit=mine")

	if fake.started.Metadata["commit"] != "mine" {
		t.Errorf("metadata = %v", fake.started.Metadata)
	}
}

func TestMetaGitStopsInARepositoryWhichDoesNotHaveTheCommitDeployedLast(t *testing.T) {
	clone, _ := clonedRepository(t)
	other, _ := clonedRepository(t)
	foreign := commit(t, other, "Of another project")
	fake := &runAPI{previousRuns: `{"id":"r8","status":"successful","metadata":{"commit":"` + foreign + `"}}`}

	out := fake.start(t, clone, "action", "run", "Deploy", "-s", "web-01", "--detach", "--meta-git=origin/main")

	if out.code != fault.ExitUsage || fake.started.ActionID != "" {
		t.Errorf("exit %d, started %+v", out.code, fake.started)
	}
	for _, want := range []string{"This is not the repository which was deployed last", "the commit " + foreign[:8], "or without --meta-git"} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, out.stderr)
		}
	}
}

func TestMetaGitTellsWhenTheRemoteCanNotBeFetchedFrom(t *testing.T) {
	clone, _ := clonedRepository(t)
	git(t, clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	fake := &runAPI{}

	out := fake.start(t, clone, "action", "run", "Deploy", "-s", "web-01", "--detach", "--meta-git=origin/main")

	if out.code != fault.ExitFailure || fake.started.ActionID != "" || !strings.Contains(out.stderr, "Git could not fetch from “origin”") || strings.Contains(out.stderr, "not the repository") {
		t.Errorf("exit %d, started %+v, stderr:\n%s", out.code, fake.started, out.stderr)
	}
}

func TestMetaGitNeedsARepositoryAndAKnownRef(t *testing.T) {
	clone, _ := clonedRepository(t)
	cases := map[string]struct {
		dir string
		ref string
	}{
		"--meta-git needs a git repository": {t.TempDir(), "--meta-git"},
		"Git could not find “nope”":         {clone, "--meta-git=nope"},
	}
	for want, c := range cases {
		fake := &runAPI{}
		out := fake.start(t, c.dir, "action", "run", "Deploy", "-s", "web-01", "--detach", c.ref)
		if out.code != fault.ExitUsage || !strings.Contains(out.stderr, want) || fake.started.ActionID != "" {
			t.Errorf("%s: exit %d, started %+v, stderr:\n%s", want, out.code, fake.started, out.stderr)
		}
	}
}

func TestMetadataOfFlagsAndFiles(t *testing.T) {
	file := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(file, []byte("## 1.2.0\n- Faster\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := metadataFlags{values: []string{"ticket=OPS-12", "note=a=b"}, files: []string{"changelog=" + file, "ticket=" + file}}

	metadata, err := collectMetadata(flags, map[string]string{"commit": "ab09ade5", "note": "collected"})

	want := map[string]string{"commit": "ab09ade5", "note": "a=b", "ticket": "OPS-12", "changelog": "## 1.2.0\n- Faster"}
	if err != nil || len(metadata) != len(want) {
		t.Fatalf("metadata = %v, err = %v", metadata, err)
	}
	for name, value := range want {
		if metadata[name] != value {
			t.Errorf("%s = %q, want %q", name, metadata[name], value)
		}
	}
}

func TestInvalidMetadataIsAUsageError(t *testing.T) {
	for _, flags := range []metadataFlags{
		{values: []string{"novalue"}},
		{values: []string{"=value"}},
		{files: []string{"changelog"}},
		{files: []string{"changelog=" + filepath.Join(t.TempDir(), "missing.md")}},
	} {
		if _, err := collectMetadata(flags, nil); fault.ExitCode(err) != fault.ExitUsage {
			t.Errorf("%+v: err = %v", flags, err)
		}
	}
}

func TestCommitVariableIsOnlyFilledWhenItExistsAndIsNotGiven(t *testing.T) {
	d := &delivery{}
	d.target.Commit = "ab09ade5"
	with := resolve.Runnable{Action: &api.Action{Variables: []api.VariableDefinition{{Name: "COMMIT"}}}}
	without := resolve.Runnable{Action: &api.Action{}}

	cases := []struct {
		delivery *delivery
		runnable resolve.Runnable
		flags    []string
		want     string
	}{
		{d, with, []string{"BRANCH=main"}, "BRANCH=main COMMIT=ab09ade5"},
		{d, with, []string{"COMMIT=mine"}, "COMMIT=mine"},
		{d, without, nil, ""},
		{nil, with, nil, ""},
	}
	for _, c := range cases {
		if got := strings.Join(c.delivery.variables(c.runnable, c.flags), " "); got != c.want {
			t.Errorf("variables = %q, want %q", got, c.want)
		}
	}
}

func TestDeliveryMetadataLinksTheComparisonAndLeavesOutWhatIsUnknown(t *testing.T) {
	d := &delivery{}
	d.target.Repository, d.target.Commit = "https://github.com/pipetower/pipetower", "ab09ade5820c637ac16b8726f1ecfaddb8da30c4"
	d.log.Previous, d.log.Commits = "76000f98477726ae09b1c4cc7c25aead219827b1", "ab09ade5  Pin actions"

	metadata := d.metadata()

	if metadata["compare"] != "https://github.com/pipetower/pipetower/compare/76000f98...ab09ade5" {
		t.Errorf("compare = %q", metadata["compare"])
	}
	if _, ok := metadata["branch"]; ok || len(metadata) != 5 {
		t.Errorf("metadata = %v", metadata)
	}
}
