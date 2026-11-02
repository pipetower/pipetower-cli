package cli

import (
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// restrictedWorkflow is "Deploy App" of team t1, which may only run on UMEX.
const restrictedWorkflow = `{"id":"w1","team_id":"t1","name":"Deploy App","steps":[],"variables":[],
	"allowed_servers":[{"id":"u1","name":"UMEX"}],"tags":[]}`

// teamServers are UMEX (#web) and Shop (#web) of team t1.
const teamServers = `{"data":[
	{"id":"u1","team_id":"t1","name":"UMEX","host":"10.0.0.1","tags":[{"name":"web","color":""}]},
	{"id":"s1","team_id":"t1","name":"Shop","host":"10.0.0.2","tags":[{"name":"web","color":""},{"name":"shop","color":""}]}
],"meta":{"has_more":false}}`

func restrictedAPI() fakeAPI {
	return fakeAPI{
		"GET /api/v1/workflows?limit=100&search=Deploy+App": `{"data":[` + restrictedWorkflow + `],"meta":{"has_more":false}}`,
		"GET /api/v1/workflows/w1":                          restrictedWorkflow,
		"GET /api/v1/servers?limit=100&search=Shop":         teamServers,
		"GET /api/v1/servers?limit=100&tag=web":             teamServers,
		"GET /api/v1/servers?limit=100&team=t1":             teamServers,
		"POST /api/v1/runs":                                 `{"id":"r9","team_id":"t1","status":"pending","executions":[]}`,
	}
}

func TestARestrictedWorkflowRunsOnItsOnlyServerWithoutAsking(t *testing.T) {
	out := execute(t, restrictedAPI(), "test-token", "workflow", "run", "Deploy App", "--detach")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Started run #r9") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
	if !strings.Contains(out.stderr, "On UMEX, the only server Deploy App may run on") {
		t.Errorf("stderr:\n%s", out.stderr)
	}
}

func TestAServerTheWorkflowMayNotRunOnIsRejectedBeforeStarting(t *testing.T) {
	out := execute(t, restrictedAPI(), "test-token", "workflow", "run", "Deploy App", "-s", "Shop", "--detach")

	if out.code != fault.ExitUsage || !strings.Contains(out.stderr, "Deploy App may not run on Shop") || !strings.Contains(out.stderr, "It may run on   UMEX") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestATagRunsOnlyOnTheServersTheWorkflowMayRunOn(t *testing.T) {
	out := execute(t, restrictedAPI(), "test-token", "workflow", "run", "Deploy App", "-s", "tag:web", "--detach")

	if out.code != fault.ExitOK || !strings.Contains(out.stderr, "Skipping Shop: Deploy App may not run there") {
		t.Errorf("exit %d, stderr:\n%s", out.code, out.stderr)
	}
}

func TestServersToRunOnCompleteOnlyThoseTheWorkflowMayRunOn(t *testing.T) {
	completeArgs(t, restrictedAPI(), "workflow", "run", "Deploy App", "-s", "").is(t, "UMEX", "tag:web")
}

func TestViewNamesTheServersTheWorkflowMayRunOn(t *testing.T) {
	out := execute(t, restrictedAPI(), "test-token", "workflow", "view", "Deploy App")

	if out.code != fault.ExitOK || !strings.Contains(out.stdout, "Runs on UMEX") {
		t.Errorf("exit %d\nstdout:\n%s\nstderr:\n%s", out.code, out.stdout, out.stderr)
	}
}
