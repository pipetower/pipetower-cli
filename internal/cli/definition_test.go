package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// definitionAPI answers like fakeAPI, with a status per path and the request bodies recorded.
type definitionAPI struct {
	fakeAPI
	statuses map[string]int
	bodies   map[string]string
}

func newDefinitionAPI(responses fakeAPI, statuses map[string]int) *definitionAPI {
	return &definitionAPI{fakeAPI: responses, statuses: statuses, bodies: map[string]string{}}
}

func (d *definitionAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.RequestURI()
	if r.Body != nil {
		d.bodies[key] = readAll(r)
	}
	if status, ok := d.statuses[key]; ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(d.fakeAPI[key]))
		return
	}
	d.fakeAPI.ServeHTTP(w, r)
}

// readAll reads the request body.
func readAll(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	return string(body)
}

// deployFile writes a definition file of the action "Deploy App" and returns its path.
func deployFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deploy.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const deployDefinition = "kind: action\nname: Deploy App\nsteps:\n  - name: Pull\n    type: execute_command\n    command: git pull\n"

const noActions = `{"data":[],"meta":{"has_more":false}}`

func TestApplyCreatesANewActionInTheTeam(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/teams?limit=100&search=Ops":                  `{"data":[{"id":"t2","name":"Ops"}],"meta":{"has_more":false}}`,
		"GET /api/v1/actions?limit=100&search=Deploy+App&team=t2": noActions,
		"POST /api/v1/actions":                                    `{"id":"x9","team_id":"t2","name":"Deploy App","steps":[],"variables":[],"tags":[]}`,
	}, nil)

	out := execute(t, api, "test-token", "action", "apply", "-f", deployFile(t, deployDefinition), "--team", "Ops")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Created action Deploy App") || !strings.Contains(out.stdout, "x9") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(api.bodies["POST /api/v1/actions"]), &body); err != nil || body["team"] != "t2" || body["kind"] != "action" || body["name"] != "Deploy App" {
		t.Errorf("body = %s (err %v)", api.bodies["POST /api/v1/actions"], err)
	}
	if strings.Contains(api.bodies["POST /api/v1/actions"], "dry_run") {
		t.Errorf("a real apply must not be a dry run: %s", api.bodies["POST /api/v1/actions"])
	}
}

func TestApplyUpdatesTheActionOfTheSameName(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/actions?limit=100&search=Deploy+App": `{"data":[{"id":"x1","team_id":"t1","name":"deploy app"},{"id":"x2","team_id":"t1","name":"Deploy App Old"}],"meta":{"has_more":false}}`,
		"PUT /api/v1/actions/x1":                          `{"id":"x1","team_id":"t1","name":"Deploy App","steps":[],"variables":[],"tags":[]}`,
	}, nil)

	out := execute(t, api, "test-token", "action", "apply", "-f", deployFile(t, deployDefinition))

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Updated action Deploy App") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	if body := api.bodies["PUT /api/v1/actions/x1"]; strings.Contains(body, `"team"`) || !strings.Contains(body, `"command":"git pull"`) {
		t.Errorf("body = %s", body)
	}
}

func TestDryRunOnlyChecksTheFile(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/actions?limit=100&search=Deploy+App": noActions,
		"POST /api/v1/actions":                            `{"valid":true,"definition":{"kind":"action","name":"Deploy App"}}`,
	}, nil)

	out := execute(t, api, "test-token", "action", "apply", "-f", deployFile(t, deployDefinition), "--dry-run")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "is valid, action Deploy App would be created") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	if !strings.Contains(api.bodies["POST /api/v1/actions"], `"dry_run":true`) {
		t.Errorf("body = %s", api.bodies["POST /api/v1/actions"])
	}
}

func TestAnInvalidDefinitionListsTheProblemsByPath(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/actions?limit=100&search=Deploy+App": noActions,
		"POST /api/v1/actions": `{"message":"The steps.0.command field is required. (and 1 more error)",
			"errors":{"steps.0.command":["The steps.0.command field is required."],"steps.0.sudoo":["Unknown key \"sudoo\""]}}`,
	}, map[string]int{"POST /api/v1/actions": http.StatusUnprocessableEntity})

	out := execute(t, api, "test-token", "action", "apply", "-f", deployFile(t, deployDefinition))

	if out.code != fault.ExitAPI || !strings.Contains(out.stderr, "deploy.yml is not valid") ||
		!strings.Contains(out.stderr, "steps.0.command: The steps.0.command field is required.") || !strings.Contains(out.stderr, "steps.0.sudoo: Unknown key") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestApplyNeedsTheTeamWhenTheUserHasSeveral(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/actions?limit=100&search=Deploy+App": noActions,
		"POST /api/v1/actions":                            `{"message":"The team is required.","errors":{"team":["The team (id or name) is required, you are a member of several teams."]}}`,
	}, map[string]int{"POST /api/v1/actions": http.StatusUnprocessableEntity})

	out := execute(t, api, "test-token", "action", "apply", "-f", deployFile(t, deployDefinition))

	if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "Which team?") || !strings.Contains(out.stderr, "--team <name>") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestApplyRejectsAFileOfAnotherKindAndNeedsTheFile(t *testing.T) {
	wrongKind := execute(t, fakeAPI{}, "test-token", "workflow", "apply", "-f", deployFile(t, deployDefinition))
	noFile := execute(t, fakeAPI{}, "test-token", "action", "apply")
	noDefinition := execute(t, fakeAPI{}, "test-token", "action", "apply", "-f", deployFile(t, "name: x"))

	if wrongKind.code != fault.ExitUsage || !strings.Contains(wrongKind.stderr, "is an action, not a workflow") || !strings.Contains(wrongKind.stderr, "pt action apply") {
		t.Errorf("exit %d, stderr:\n%s", wrongKind.code, wrongKind.stderr)
	}
	if noFile.code != fault.ExitUsage || !strings.Contains(noFile.stderr, "Missing --file") {
		t.Errorf("exit %d, stderr:\n%s", noFile.code, noFile.stderr)
	}
	if noDefinition.code != fault.ExitUsage || !strings.Contains(noDefinition.stderr, "kind: action|workflow|template") {
		t.Errorf("exit %d, stderr:\n%s", noDefinition.code, noDefinition.stderr)
	}
}

func TestExportPrintsTheDefinitionAsYAMLOrJSON(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/templates?limit=100&search=Vhost": `{"data":[{"id":"p1","team_id":"t1","name":"Vhost","file_name":"a.conf","content":"x","tags":[]}],"meta":{"has_more":false}}`,
		"GET /api/v1/templates/p1/definition":          `{"kind":"template","name":"Vhost","file_name":"a.conf","content":"server {\n}"}`,
	}

	yaml := execute(t, api, "test-token", "template", "export", "Vhost")
	js := execute(t, api, "test-token", "template", "export", "Vhost", "-o", "json")

	if yaml.code != fault.ExitOK || !strings.HasPrefix(yaml.stdout, "kind: template\nname: Vhost\nfile_name: a.conf\ncontent: |-\n  server {\n  }\n") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", yaml.code, yaml.stdout, yaml.stderr)
	}
	if js.code != fault.ExitOK || !strings.HasPrefix(js.stdout, "{\n  \"kind\": \"template\"") {
		t.Errorf("exit %d\nstdout:\n%s", js.code, js.stdout)
	}
}

func TestExampleIsPrintedAsCommentedYAMLOrAsJSON(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/definitions/workflow/example?format=yaml": "# A workflow\n\nkind: workflow\nname: Release\n",
		"GET /api/v1/definitions/workflow/example":             `{"kind":"workflow","name":"Release","steps":[{"action":"Build"}]}`,
	}

	yaml := execute(t, api, "test-token", "workflow", "example")
	js := execute(t, api, "test-token", "workflow", "example", "-o", "json")

	if yaml.code != fault.ExitOK || yaml.stdout != "# A workflow\n\nkind: workflow\nname: Release\n" {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", yaml.code, yaml.stdout, yaml.stderr)
	}
	if js.code != fault.ExitOK || !strings.HasPrefix(js.stdout, "{\n  \"kind\": \"workflow\"") {
		t.Errorf("exit %d\nstdout:\n%s", js.code, js.stdout)
	}
}

func TestDeleteNeedsConfirmationAndNamesTheUsers(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/actions?limit=100&search=Deploy+App": `{"data":[{"id":"x1","team_id":"t1","name":"Deploy App","steps":[],"variables":[],"tags":[]}],"meta":{"has_more":false}}`,
		"DELETE /api/v1/actions/x1":                       `{"message":"The action is used by other objects and cannot be deleted.","used_by":[{"type":"workflow","id":"w1","name":"Release"}]}`,
	}, map[string]int{"DELETE /api/v1/actions/x1": http.StatusConflict})

	unconfirmed := execute(t, api, "test-token", "action", "delete", "Deploy App")
	used := execute(t, api, "test-token", "action", "delete", "Deploy App", "--yes")

	if unconfirmed.code != fault.ExitUsage || !strings.Contains(unconfirmed.stderr, "--yes") {
		t.Errorf("exit %d, stderr:\n%s", unconfirmed.code, unconfirmed.stderr)
	}
	if used.code != fault.ExitAPI || !strings.Contains(used.stderr, "Action Deploy App is still used") || !strings.Contains(used.stderr, "workflow Release") {
		t.Errorf("exit %d, stderr:\n%s", used.code, used.stderr)
	}

	api.statuses = nil
	api.fakeAPI["DELETE /api/v1/actions/x1"] = ""
	deleted := execute(t, api, "test-token", "action", "delete", "Deploy App", "-y")
	if deleted.code != fault.ExitOK || !strings.Contains(deleted.stdout, "Deleted action Deploy App") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", deleted.code, deleted.stdout, deleted.stderr)
	}
}

func TestApplyAcrossAllTeamsNeedsAnUnambiguousName(t *testing.T) {
	api := newDefinitionAPI(fakeAPI{
		"GET /api/v1/actions?limit=100&search=Deploy+App": `{"data":[{"id":"x1","team_id":"t1","name":"Deploy App"},{"id":"x2","team_id":"t2","name":"Deploy App"}],"meta":{"has_more":false}}`,
	}, nil)

	out := execute(t, api, "test-token", "action", "apply", "-f", deployFile(t, deployDefinition), "--all-teams")

	if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "exists in several of your teams") || !strings.Contains(out.stderr, "--team <name>") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
	if _, sent := api.bodies["POST /api/v1/actions"]; sent {
		t.Error("nothing must be sent for an ambiguous name")
	}
}
