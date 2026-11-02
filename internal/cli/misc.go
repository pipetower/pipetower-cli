package cli

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/ui"
	"github.com/pipetower/pipetower-cli/internal/version"
)

// newAPICommand sends a raw authenticated request.
func newAPICommand(a *App) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Send an authenticated request to the REST API and print the JSON",
		Long:  "Send a request to the Pipetower REST API (paths are relative to /api/v1).",
		Example: "  pt api GET servers\n" +
			"  pt api GET 'runs?status=running'\n" +
			"  pt api POST runs -d '{\"action_id\":\"x1\",\"server_ids\":[\"s1\"]}'",
		Args: args("method", "path"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			client, err := a.Client()
			if err != nil {
				return err
			}
			endpoint := rawEndpoint(strings.ToUpper(argv[0]), argv[1], data)
			status, body, err := client.Raw(cmd.Context(), endpoint)
			if err != nil {
				return err
			}
			if len(body) > 0 {
				if err := a.IO.PrintJSON(body); err != nil {
					return err
				}
			}
			if status >= 300 {
				a.IO.Errorln(ui.Danger.Render(" ✗ " + http.StatusText(status)))
				return &fault.Silent{Code: fault.ExitAPI}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&data, "data", "d", "", "request body as JSON")
	return cmd
}

// rawEndpoint builds an endpoint from a path which may contain a query string.
func rawEndpoint(method, path, data string) api.Endpoint {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "/"), "api/v1/")
	endpoint := api.Endpoint{Method: method, Path: path}
	if before, query, found := strings.Cut(path, "?"); found {
		endpoint.Path = before
		endpoint.Query = parseQuery(query)
	}
	if data != "" {
		endpoint.Body = []byte(data)
	}
	return endpoint
}

// newVersionCommand prints the build information.
func newVersionCommand(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of pt",
		Args:  args(),
		Run: func(*cobra.Command, []string) {
			details := []string{version.Date, runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH}
			if !strings.Contains(version.Version, version.Commit) {
				details = append([]string{version.Commit}, details...)
			}
			a.IO.Println(ui.Title.Render("pt") + " " + version.Version + ui.Muted.Render("  "+strings.Join(details, " · ")))
		},
	}
}

// fishRequest is where cobra's fish script asks pt for completions, patched for names with spaces:
//   - It joins the arguments into one string for eval, which splits "Deploy App" into two words, so
//     each argument is escaped first.
//   - It passes the word being completed as typed, "Adjust\ M" or 'Adjust M with its backslash or
//     quote, which no name starts with, so the word is unescaped first.
var fishRequest = strings.NewReplacer(
	`    set -l lastArg (string escape -- (commandline -ct))`,
	"    set -l lastArg (commandline -ct | string unescape)\n    set lastArg (string escape -- \"$lastArg\")",
	`    set -l requestComp "`, "    set -l escapedArgs (string escape -- $args[2..-1])\n    set -l requestComp \"",
	`__complete $args[2..-1] $lastArg"`, `__complete $escapedArgs $lastArg"`,
)

// genFishCompletion writes cobra's fish script, patched for names with spaces.
func genFishCompletion(root *cobra.Command, out io.Writer) error {
	var script strings.Builder
	if err := root.GenFishCompletion(&script, true); err != nil {
		return err
	}
	patched := fishRequest.Replace(script.String())
	if !strings.Contains(patched, "__complete $escapedArgs $lastArg") || strings.Count(patched, "escapedArgs") != 2 ||
		!strings.Contains(patched, "string unescape") {
		return errors.New("cobra's fish completion script has changed, adapt fishRequest")
	}
	_, err := io.WriteString(out, patched)
	return err
}

// bashNames is added to cobra's bash script, which handles names with spaces badly in two cases:
//   - When several names match, cobra hands them to bash unescaped (to list them nicely), and bash
//     inserts their common beginning as it is: for "Adjust MySQL config" and "Adjust PHP config" that
//     is "Adjust ", whose space ends the word. On Tab, bash gets the names escaped instead, so it
//     inserts "Adjust\ "; listed (the next Tab), they are shown as cobra formats them.
//   - A name typed in quotes ("Adjust M) completes nothing: cobra only compares escaped names, and
//     the quote breaks its request. This completes it within the quotes the user opened.
const bashNames = `
# pt: complete names with spaces, escaped or in the quotes the user opened
__pt_complete_names() {
    [[ -z ${COMP_TYPE-} || $COMP_TYPE -eq 9 ]] || return 0 # Tab inserts, cobra lists for other keys
    local tab=$'\t' comp escaped quote="" matches=()
    [[ ${cur:0:1} == [\"\'] ]] && quote=${cur:0:1}
    for comp in "${rawCompletions[@]}"; do
        comp=${comp%%$tab*}
        if [[ -n $quote ]]; then
            # bash inserts the common beginning within the quote, and closes it after the last name
            [[ $comp == "${cur:1}"* ]] && matches+=("$quote$comp")
        else
            printf -v escaped "%q" "$comp"
            [[ $escaped == "$cur"* ]] && matches+=("$escaped")
        fi
    done
    if [[ -n $quote ]] || (( ${#matches[@]} > 1 )); then
        COMPREPLY=("${matches[@]}")
    fi
}
`

// bashCompletion patches cobra's bash script to complete names with spaces.
var bashCompletion = strings.NewReplacer(
	"\n__pt_handle_standard_completion_case() {", bashNames+"\n__pt_handle_standard_completion_case() {",
	// a quote the user opened ("Adjust M) would make the request fail, it is closed for it
	`    requestComp="${words[0]} __complete ${args[*]}"`,
	`    requestComp="${words[0]} __complete ${args[*]}"`+"\n"+
		`    if [[ ${cur:0:1} == [\"\'] && ( ${#cur} -eq 1 || ${cur: -1} != "${cur:0:1}" ) ]]; then requestComp+=${cur:0:1}; fi`,
	// cobra escapes the completions while handling them, the function needs them as they are
	"        __pt_handle_completion_types\n    fi",
	"        local rawCompletions=(\"${completions[@]}\")\n        __pt_handle_completion_types\n        __pt_complete_names\n    fi",
)

// genBashCompletion writes cobra's bash script, patched to complete names with spaces.
func genBashCompletion(root *cobra.Command, out io.Writer) error {
	var script strings.Builder
	if err := root.GenBashCompletionV2(&script, true); err != nil {
		return err
	}
	patched := bashCompletion.Replace(script.String())
	if strings.Count(patched, "__pt_complete_names") != 2 || strings.Count(patched, "rawCompletions") != 2 || !strings.Contains(patched, "requestComp+=${cur:0:1}") {
		return errors.New("cobra's bash completion script has changed, adapt bashCompletion")
	}
	_, err := io.WriteString(out, patched)
	return err
}

// zshRequest is where cobra's zsh script asks pt for completions: it evals the words as typed, which
// fails on a quote the user opened ("Adjust M), so the quote is closed for it.
var zshRequest = strings.NewReplacer(
	`    requestComp="${words[1]} __complete ${words[2,-1]}"`,
	`    requestComp="${words[1]} __complete ${words[2,-1]}"`+"\n"+
		`    [[ -n ${compstate[quote]} ]] && requestComp+=${compstate[quote]}`,
)

// genZshCompletion writes cobra's zsh script, patched for names in quotes.
func genZshCompletion(root *cobra.Command, out io.Writer) error {
	var script strings.Builder
	if err := root.GenZshCompletion(&script); err != nil {
		return err
	}
	patched := zshRequest.Replace(script.String())
	if !strings.Contains(patched, "requestComp+=${compstate[quote]}") {
		return errors.New("cobra's zsh completion script has changed, adapt zshRequest")
	}
	_, err := io.WriteString(out, patched)
	return err
}

// parseQuery parses a query string, invalid parts are dropped.
func parseQuery(query string) url.Values {
	values, _ := url.ParseQuery(query)
	return values
}

// newCompletionCommand prints the shell completion script.
func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion <shell>",
		Short: "Print the shell completion script (bash, zsh, fish, powershell)",
		Long: "Print the shell completion script (bash, zsh, fish, powershell). The install script sets it up\n" +
			"for bash, zsh and fish, this is for setting it up by hand.",
		Example: "  source <(pt completion bash)\n  pt completion zsh > \"${fpath[1]}/_pt\"",
		Args:    args("shell"),
		RunE: func(cmd *cobra.Command, argv []string) error {
			root, out := cmd.Root(), cmd.OutOrStdout()
			switch argv[0] {
			case "bash":
				return genBashCompletion(root, out)
			case "zsh":
				return genZshCompletion(root, out)
			case "fish":
				return genFishCompletion(root, out)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return usageError("Unknown shell “"+argv[0]+"”", "Use bash, zsh, fish or powershell.")
		},
	}
}
