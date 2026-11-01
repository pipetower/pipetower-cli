package cli

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pipetower/pipetower-cli/internal/ui"
)

// helpIndent indents the content of the help sections below their titles.
const helpIndent = "    "

// row is a name with its description, in the lists of commands and flags.
type row [2]string

// renderHelp renders the help of a command, the same for every command: the description, the usage,
// the subcommands (by name), the flags, examples and where to learn more. Sections without content
// are left out, the descriptions of all lists start in the same column. Usage and examples are muted,
// so that the commands and flags stand out.
func renderHelp(cmd *cobra.Command) string {
	commands, flags, globals := subcommands(cmd), flagList(cmd.LocalNonPersistentFlags()), flagList(cmd.Root().PersistentFlags())
	width := nameWidth(slices.Concat(commands, flags, globals))
	sections := []string{
		helpSection("", description(cmd)),
		helpSection("USAGE", ui.Muted.Render(usageLine(cmd))),
		helpSection("COMMANDS", columns(commands, width)),
		helpSection("FLAGS", columns(flags, width)),
		helpSection("GLOBAL FLAGS", columns(globals, width)),
		helpSection("EXAMPLES", examples(cmd)),
		helpSection("", learnMore(cmd)),
	}
	sections = slices.DeleteFunc(sections, func(section string) bool { return section == "" })
	return "\n" + strings.Join(sections, "\n\n") + "\n\n"
}

// helpSection renders a section below its title, the description has no title and less indent.
func helpSection(title, content string) string {
	if content == "" {
		return ""
	}
	if title == "" {
		return indent(content, "  ")
	}
	return "  " + ui.Title.Render(title) + "\n" + indent(content, helpIndent)
}

// description is the long description of a command, or its short one.
func description(cmd *cobra.Command) string {
	if cmd.Long != "" {
		return cmd.Long
	}
	return cmd.Short
}

// usageLine shows how the command is called: its path, arguments, subcommand and flags.
func usageLine(cmd *cobra.Command) string {
	line := cmd.CommandPath() + strings.TrimPrefix(cmd.Use, cmd.Name())
	if cmd.HasAvailableSubCommands() {
		line += " <command>"
	}
	if cmd.HasAvailableFlags() {
		line += " [flags]"
	}
	return line
}

// examples renders the examples of a command, one command per line.
func examples(cmd *cobra.Command) string {
	if cmd.Example == "" {
		return ""
	}
	lines := strings.Split(cmd.Example, "\n")
	for i, line := range lines {
		lines[i] = ui.Muted.Render(strings.TrimSpace(line))
	}
	return strings.Join(lines, "\n")
}

// subcommands lists the subcommands by name with their short description.
func subcommands(cmd *cobra.Command) []row {
	var rows []row
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() {
			rows = append(rows, row{ui.Accent.Render(sub.Name()), sub.Short})
		}
	}
	return rows
}

// flagList lists the flags with their description and default.
func flagList(flags *pflag.FlagSet) []row {
	var rows []row
	flags.VisitAll(func(flag *pflag.Flag) {
		if !flag.Hidden {
			rows = append(rows, row{ui.Success.Render(flagNames(flag)), ui.Capitalize(flag.Usage) + flagDefault(flag)})
		}
	})
	return rows
}

// learnMore tells how to get the help of a subcommand, for commands which have subcommands.
func learnMore(cmd *cobra.Command) string {
	if !cmd.HasAvailableSubCommands() {
		return ""
	}
	return ui.Muted.Render("See " + cmd.CommandPath() + " <command> --help for the help of a command.")
}

// flagNames renders the short and long name of a flag, long names aligned.
func flagNames(flag *pflag.Flag) string {
	if flag.Shorthand == "" {
		return "    --" + flag.Name
	}
	return "-" + flag.Shorthand + ", --" + flag.Name
}

// flagDefault renders the default value of a flag, if it has a meaningful one.
func flagDefault(flag *pflag.Flag) string {
	if slices.Contains([]string{"", "false", "[]", "0"}, flag.DefValue) {
		return ""
	}
	return ui.Faint.Render(" (" + flag.DefValue + ")")
}

// nameWidth is the width of the longest name of the rows.
func nameWidth(rows []row) int {
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(r[0]))
	}
	return width
}

// columns renders the rows with their descriptions three spaces after the given name width.
func columns(rows []row, width int) string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = r[0] + strings.Repeat(" ", width-lipgloss.Width(r[0])+3) + r[1]
	}
	return strings.Join(lines, "\n")
}

// indent indents every line of the text.
func indent(text, prefix string) string {
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}

// helpFunc prints the help of a command with the colors the global flags allow.
func helpFunc(a *App) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, _ []string) {
		_ = a.setup() // help skips the pre-run, the streams follow --no-color nonetheless
		_, _ = fmt.Fprint(a.IO.Out, renderHelp(cmd))
	}
}
