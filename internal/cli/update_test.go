package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/pipetower/pipetower-cli/internal/config"
	"github.com/pipetower/pipetower-cli/internal/prompt"
	"github.com/pipetower/pipetower-cli/internal/ui"
	"github.com/pipetower/pipetower-cli/internal/update"
	"github.com/pipetower/pipetower-cli/internal/version"
)

// answer is a prompter which answers every confirmation the same and records the questions.
type answer struct {
	yes       bool
	questions []string
}

func (a *answer) Pick(string, []prompt.Option) (int, error)       { return 0, nil }
func (a *answer) PickMany(string, []prompt.Option) ([]int, error) { return nil, nil }
func (a *answer) Input(string) (string, error)                    { return "", nil }
func (a *answer) Confirm(title, _, _ string) (bool, error) {
	a.questions = append(a.questions, title)
	return a.yes, nil
}

// updateEnv is a released pt 0.1.0 in a terminal, with 0.3.0 published on a fake GitHub.
type updateEnv struct {
	t        *testing.T
	binary   string
	schedule update.Schedule
	prompter *answer
	requests int
	env      map[string]string
}

func newUpdateEnv(t *testing.T, yes bool) *updateEnv {
	t.Helper()
	previous := version.Version
	version.Version = "0.1.0"
	t.Cleanup(func() { version.Version = previous })

	dir := t.TempDir()
	e := &updateEnv{
		t:        t,
		binary:   filepath.Join(dir, "pt"),
		schedule: update.Schedule{Path: filepath.Join(dir, "cache", "update-check")},
		prompter: &answer{yes: yes},
		env:      map[string]string{},
	}
	if err := os.WriteFile(e.binary, []byte("pt 0.1.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

// github serves the release 0.3.0 for linux/amd64.
func (e *updateEnv) github() *httptest.Server {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "pt", Mode: 0o755, Size: 8, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("pt 0.3.0"))
	_ = tw.Close()
	_ = gz.Close()
	sum := sha256.Sum256(archive.Bytes())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.requests++
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v0.3.0", http.StatusFound)
		case "/releases/download/v0.3.0/checksums.txt":
			_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  pt_0.3.0_linux_amd64.tar.gz\n"))
		case "/releases/download/v0.3.0/pt_0.3.0_linux_amd64.tar.gz":
			_, _ = w.Write(archive.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	e.t.Cleanup(server.Close)
	return server
}

// run executes pt in a terminal.
func (e *updateEnv) run(args ...string) result {
	e.t.Helper()
	server := e.github()
	var stdout, stderr bytes.Buffer
	dir := filepath.Dir(e.binary)
	env := config.Env{
		Getenv:     func(name string) string { return e.env[name] },
		Cwd:        dir,
		Home:       dir,
		GitIgnored: func(string) (bool, bool) { return false, false },
	}
	newIO := func(_, noInput bool) *ui.IO {
		plain := func(w *bytes.Buffer) *colorprofile.Writer {
			return &colorprofile.Writer{Forward: w, Profile: colorprofile.NoTTY}
		}
		return &ui.IO{Out: plain(&stdout), Err: plain(&stderr), RawOut: &stdout, Profile: colorprofile.NoTTY, Terminal: true, CanPrompt: !noInput}
	}

	app := NewAppWith(env, newIO)
	app.prompter = e.prompter
	app.updater = updater{
		source:     &update.Source{Repo: server.URL, Client: server.Client(), OS: "linux", Arch: "amd64"},
		schedule:   e.schedule,
		executable: func() (string, error) { return e.binary, nil },
		now:        func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) },
	}
	root := NewRootCommand(app)
	root.SetArgs(args)
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := root.ExecuteContext(context.Background())
	if err != nil {
		app.HandleError(plainWriter(&stderr), err)
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: ExitCode(err)}
}

func (e *updateEnv) installed() string {
	content, _ := os.ReadFile(e.binary)
	return string(content)
}

func TestStartOffersUpdateAndStopsAfterIt(t *testing.T) {
	e := newUpdateEnv(t, true)

	r := e.run("variable", "list")

	if len(e.prompter.questions) != 1 || !strings.Contains(e.prompter.questions[0], "pt 0.3.0 is available (you have 0.1.0)") {
		t.Errorf("questions = %q", e.prompter.questions)
	}
	if e.installed() != "pt 0.3.0" {
		t.Errorf("installed = %q", e.installed())
	}
	for _, want := range []string{"Updated pt 0.1.0 → 0.3.0", "Restart pt to use the new version.", "/releases/tag/v0.3.0"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.stderr)
		}
	}
	// the command did not run: it would have failed without a configuration
	if r.code != 0 || strings.Contains(r.stderr, "PIPETOWER_TOKEN") {
		t.Errorf("code = %d, stderr:\n%s", r.code, r.stderr)
	}
}

func TestDeclinedUpdateRunsCommandAndAsksAgainNextDay(t *testing.T) {
	e := newUpdateEnv(t, false)

	r := e.run("variable", "list")

	if e.installed() != "pt 0.1.0" || !strings.Contains(r.stderr, "pt update") {
		t.Errorf("installed = %q, stderr:\n%s", e.installed(), r.stderr)
	}
	if r.code != 3 { // the command ran and missed its configuration
		t.Errorf("code = %d, stderr:\n%s", r.code, r.stderr)
	}

	requests := e.requests
	e.run("variable", "list")
	if len(e.prompter.questions) != 1 || e.requests != requests {
		t.Errorf("asked again on the same day: %q, %d requests", e.prompter.questions, e.requests-requests)
	}
}

func TestNoUpdateCheck(t *testing.T) {
	cases := map[string]func(e *updateEnv) []string{
		"disabled": func(e *updateEnv) []string { e.env[EnvNoUpdateCheck] = "1"; return []string{"variable", "list"} },
		"dev build": func(*updateEnv) []string {
			version.Version = "dev-540980c"
			return []string{"variable", "list"}
		},
		"version":    func(*updateEnv) []string { return []string{"version"} },
		"no input":   func(*updateEnv) []string { return []string{"variable", "list", "--no-input"} },
		"completion": func(*updateEnv) []string { return []string{"__complete", "server", ""} },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			e := newUpdateEnv(t, true)
			e.run(setup(e)...)
			if e.requests != 0 || len(e.prompter.questions) != 0 {
				t.Errorf("%d requests, questions %q", e.requests, e.prompter.questions)
			}
		})
	}
}

func TestUpdateCommand(t *testing.T) {
	e := newUpdateEnv(t, false)

	r := e.run("update")

	if r.code != 0 || e.installed() != "pt 0.3.0" || len(e.prompter.questions) != 0 {
		t.Errorf("code = %d, installed = %q, questions = %q, stderr:\n%s", r.code, e.installed(), e.prompter.questions, r.stderr)
	}
	if !strings.Contains(r.stderr, "Updated pt 0.1.0 → 0.3.0") {
		t.Errorf("stderr:\n%s", r.stderr)
	}
}

func TestUpdateCommandWhenUpToDate(t *testing.T) {
	e := newUpdateEnv(t, false)
	version.Version = "0.3.0"

	r := e.run("update")

	if r.code != 0 || !strings.Contains(r.stderr, "pt 0.3.0 is the latest version") || e.installed() != "pt 0.1.0" {
		t.Errorf("code = %d, installed = %q, stderr:\n%s", r.code, e.installed(), r.stderr)
	}
}

func TestUpdateCommandOfDevBuild(t *testing.T) {
	e := newUpdateEnv(t, false)
	version.Version = "dev"

	r := e.run("update")

	if r.code != 2 || !strings.Contains(r.stderr, "development build") || e.requests != 0 {
		t.Errorf("code = %d, stderr:\n%s", r.code, r.stderr)
	}
}
