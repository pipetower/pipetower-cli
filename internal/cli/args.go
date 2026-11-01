package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// args requires exactly the named positional arguments and explains missing ones.
func args(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, given []string) error {
		if len(given) == len(names) {
			return nil
		}
		if len(given) < len(names) {
			return usageProblem(cmd, "Missing "+names[len(given)])
		}
		return usageProblem(cmd, "Too many arguments: "+strings.Join(given[len(names):], " "))
	}
}

// argOrPick requires the named argument, or none when the user can be asked: then the user picks
// the item from a list. Scripts and --no-input must give the argument.
func argOrPick(a *App, name string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, given []string) error {
		if len(given) > 0 {
			return args(name)(cmd, given)
		}
		if err := a.setup(); err != nil { // arguments are checked before the pre-run applies the global flags
			return err
		}
		if a.Prompter() == nil {
			return args(name)(cmd, given)
		}
		return nil
	}
}

// optionalArg returns the first argument, or "" when none was given.
func optionalArg(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return argv[0]
}

// usageProblem explains a wrong call of the command: what is wrong, how it is called and where its help is.
func usageProblem(cmd *cobra.Command, title string) error {
	return fault.New(fault.Problem{
		Title: title,
		Hints: []fault.Hint{{Label: "Usage", Value: usageLine(cmd)}, {Label: "See", Value: cmd.CommandPath() + " --help"}},
		Code:  fault.ExitUsage,
	})
}

// usageError explains a wrong invocation.
func usageError(title, detail string) error {
	return fault.New(fault.Problem{Title: title, Detail: detail, Code: fault.ExitUsage})
}
