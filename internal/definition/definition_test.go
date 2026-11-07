package definition

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

func TestLoadParsesYAMLIntoAJSONBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yml")
	content := "kind: action\nname: Deploy\nvariables:\n  - name: ENV\n    values: [a, b]\nsteps:\n  - name: Pull\n    type: execute_command\n    command: |\n      git pull\n    sudo: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	d, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(d.Body(map[string]any{"team": "t1"}))

	if d.Kind != "action" || d.Name != "Deploy" || d.Source != path {
		t.Errorf("definition = %+v", d)
	}
	for _, want := range []string{`"kind":"action"`, `"values":["a","b"]`, `"command":"git pull\n"`, `"sudo":true`, `"team":"t1"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
}

func TestLoadReadsStdin(t *testing.T) {
	d, err := Load("-", strings.NewReader(`{"kind": "template", "name": "Vhost", "file_name": "a", "content": "b"}`))

	if err != nil || d.Kind != "template" || d.Name != "Vhost" || d.Source != "The definition on stdin" {
		t.Errorf("definition = %+v, err = %v", d, err)
	}
}

func TestFilesWhichAreNoDefinitionsAreExplained(t *testing.T) {
	for content, want := range map[string]string{
		"name: [":             "not valid YAML",
		"":                    "empty",
		"name: x":             "kind: action|workflow|template",
		"kind: pipe\nname: x": "kind: action|workflow|template",
		"kind: action":        "must have a name",
		"- just\n- a list":    "not valid YAML",
	} {
		_, err := Parse([]byte(content), "deploy.yml")
		if err == nil || fault.ExitCode(err) != fault.ExitUsage || !strings.Contains(err.Error(), want) || !strings.HasPrefix(err.Error(), "deploy.yml is not a definition") {
			t.Errorf("%q: err = %v, want %q", content, err, want)
		}
	}
}

func TestMissingFileIsAUsageError(t *testing.T) {
	_, err := Load("/nonexistent/deploy.yml", nil)

	if fault.ExitCode(err) != fault.ExitUsage || !strings.Contains(err.Error(), "Cannot read /nonexistent/deploy.yml") {
		t.Errorf("err = %v", err)
	}
}

func TestALiteralBlockAtTheEndOfTheFileKeepsItsFinalLineBreak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cron.yml")
	if err := os.WriteFile(path, []byte("kind: template\nname: Cron\nfile_name: a\ncontent: |\n  * * * * * run\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	d, err := Load(path, nil)

	if err != nil || d.Data["content"] != "* * * * * run\n" {
		t.Errorf("content = %q, err = %v", d.Data["content"], err)
	}
}
