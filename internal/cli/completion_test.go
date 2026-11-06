package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// taggedServers are two servers, both tagged #web, web-01 also #prod.
const taggedServers = `{"data":[
	{"id":"a1","team_id":"t1","name":"web-01","host":"10.0.0.1","tags":[{"name":"web","color":""},{"name":"prod","color":""}]},
	{"id":"a2","team_id":"t1","name":"web-02","host":"10.0.0.2","tags":[{"name":"web","color":""}]}
],"meta":{"has_more":false}}`

// deployActions is the action "Deploy App" with a selection and a text variable.
const deployActions = `{"data":[
	{"id":"x1","team_id":"t1","name":"Deploy App","variables":[
		{"name":"branch","type":"selection","values":"main, develop"},
		{"name":"message","type":"text","values":""}],"tags":[]}
],"meta":{"has_more":false}}`

// completionResult is the output of pt __complete: the offered values and the directive.
type completionResult struct {
	values    []string
	directive cobra.ShellCompDirective
}

// completeArgs runs the completion of the last argument like a shell on Tab.
func completeArgs(t *testing.T, api fakeAPI, args ...string) completionResult {
	t.Helper()
	out := execute(t, api, "test-token", append([]string{cobra.ShellCompRequestCmd}, args...)...)
	return parseCompletion(t, out)
}

func parseCompletion(t *testing.T, out result) completionResult {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out.stdout), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, ":") {
		t.Fatalf("no directive in:\n%s\nstderr:\n%s", out.stdout, out.stderr)
	}
	var directive int
	for _, c := range last[1:] {
		directive = directive*10 + int(c-'0')
	}
	var values []string
	for _, line := range lines[:len(lines)-1] {
		value, _, _ := strings.Cut(line, "\t")
		values = append(values, value)
	}
	return completionResult{values: values, directive: cobra.ShellCompDirective(directive)}
}

func (c completionResult) is(t *testing.T, want ...string) {
	t.Helper()
	if strings.Join(c.values, "|") != strings.Join(want, "|") {
		t.Errorf("completions = %q, want %q", c.values, want)
	}
	if c.directive&cobra.ShellCompDirectiveNoFileComp == 0 {
		t.Errorf("directive %d completes file names", c.directive)
	}
}

func TestCompleteServerNames(t *testing.T) {
	api := fakeAPI{"GET /api/v1/servers?limit=100&search=web": taggedServers}

	completeArgs(t, api, "server", "view", "web").is(t, "web-01", "web-02")
	completeArgs(t, api, "server", "view", "web-01", "").is(t) // one server only
}

func TestCompleteServersAndTagsToRunOn(t *testing.T) {
	api := fakeAPI{
		"GET /api/v1/servers?limit=100":            taggedServers,
		"GET /api/v1/servers?limit=100&search=tag": `{"data":[],"meta":{"has_more":false}}`,
	}

	completeArgs(t, api, "action", "run", "Deploy App", "-s", "").is(t, "web-01", "web-02", "tag:prod", "tag:web")
	completeArgs(t, api, "action", "run", "Deploy App", "-s", "tag:").is(t, "tag:prod", "tag:web")
	completeArgs(t, api, "action", "run", "Deploy App", "-s", "web-01", "-s", "").is(t, "web-02", "tag:prod", "tag:web")
}

func TestCompleteVariablesAndTheirValues(t *testing.T) {
	api := fakeAPI{"GET /api/v1/actions?limit=100&search=Deploy+App": deployActions}

	names := completeArgs(t, api, "action", "run", "Deploy App", "-v", "")
	names.is(t, "branch=", "message=")
	if names.directive&cobra.ShellCompDirectiveNoSpace == 0 {
		t.Error("a space after name= separates the value")
	}
	completeArgs(t, api, "action", "run", "Deploy App", "-v", "branch=").is(t, "branch=main", "branch=develop")
	completeArgs(t, api, "action", "run", "Deploy App", "-v", "branch=main", "-v", "").is(t, "message=")
}

func TestCompleteRuns(t *testing.T) {
	api := fakeAPI{"GET /api/v1/runs?limit=30": `{"data":[
		{"id":"r2","status":"running","executable":{"id":"x1","name":"Deploy App"}},
		{"id":"r1","status":"failed","executable":{"id":"x1","name":"Deploy App"},"finished_at":"2026-11-02T10:00:00Z"}
	],"meta":{"has_more":false}}`}

	completeArgs(t, api, "run", "view", "").is(t, "r2", "r1")
	completeArgs(t, api, "run", "cancel", "").is(t, "r2") // finished runs can't be canceled
}

func TestCompleteServersOfARun(t *testing.T) {
	api := fakeAPI{"GET /api/v1/runs/r1": finishedRun}

	completeArgs(t, api, "run", "view", "r1", "--log", "--server", "").is(t, "web-01")
}

func TestCompleteFixedValues(t *testing.T) {
	completeArgs(t, fakeAPI{}, "run", "list", "--status", "f").is(t, "failed")
	completeArgs(t, fakeAPI{}, "server", "list", "-o", "").is(t, "table", "json", "yaml")
	completeArgs(t, fakeAPI{}, "completion", "").is(t, "bash", "zsh", "fish", "powershell")
	completeArgs(t, fakeAPI{}, "api", "P").is(t, "POST", "PUT", "PATCH")
}

func TestDefinitionFilesAreCompletedByTheShell(t *testing.T) {
	c := completeArgs(t, fakeAPI{}, "action", "apply", "-f", "")

	if strings.Join(c.values, "|") != "yml|yaml|json" || c.directive&cobra.ShellCompDirectiveFilterFileExt == 0 || c.directive&cobra.ShellCompDirectiveNoFileComp != 0 {
		t.Errorf("completions = %q, directive = %v", c.values, c.directive)
	}
}

func TestFreeTextAndCommandsWithoutArgumentsCompleteNoFiles(t *testing.T) {
	completeArgs(t, fakeAPI{}, "server", "list", "--search", "").is(t)
	completeArgs(t, fakeAPI{}, "status", "").is(t)
}

func TestCompletionWithoutConfigurationOffersNothingQuietly(t *testing.T) {
	out := run(t, t.TempDir(), cobra.ShellCompRequestCmd, "server", "view", "")
	c := parseCompletion(t, out)

	c.is(t)
	if strings.Contains(out.stderr, "token") {
		t.Errorf("the configuration problem garbles the command line:\n%s", out.stderr)
	}
}

func TestEveryArgumentAndFlagValueIsCompleted(t *testing.T) {
	root := NewRootCommand(NewApp()) // addCompletions panics on what it can't complete
	forEachCommand(root, func(cmd *cobra.Command) {
		if cmd.ValidArgsFunction == nil {
			t.Errorf("%s completes no arguments", cmd.CommandPath())
		}
		cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
			if _, ok := cmd.GetFlagCompletionFunc(flag.Name); !ok && flag.Value.Type() != "bool" {
				t.Errorf("%s --%s completes nothing", cmd.CommandPath(), flag.Name)
			}
		})
	})
}

func TestCompletionScriptsArePatchedForNamesWithSpaces(t *testing.T) {
	patches := map[string][]string{
		// several matches are inserted escaped, quoted names complete within the quote
		"bash": {"__pt_complete_names\n", "local rawCompletions=", "requestComp+=${cur:0:1}"},
		// a quote the user opened is closed for the request
		"zsh": {"requestComp+=${compstate[quote]}"},
		// arguments are escaped for the eval, the word being completed is unescaped
		"fish": {"set -l escapedArgs (string escape -- $args[2..-1])", "__complete $escapedArgs $lastArg", "(commandline -ct | string unescape)"},
	}
	for shell, want := range patches {
		out := execute(t, fakeAPI{}, "test-token", "completion", shell)
		for _, patch := range want {
			if out.code != 0 || !strings.Contains(out.stdout, patch) {
				t.Errorf("%s: exit %d, script lacks %q, stderr:\n%s", shell, out.code, patch, out.stderr)
			}
		}
	}
}
