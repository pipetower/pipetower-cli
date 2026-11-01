package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// newVariableCommand groups the variable commands.
func newVariableCommand(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "variable", Short: "List the variables of your account, a team or a server"}
	cmd.AddCommand(newVariableListCommand(a))
	return cmd
}

// newVariableListCommand lists the variables of the account, a team or a server, secrets stay hidden.
func newVariableListCommand(a *App) *cobra.Command {
	var page pageOptions
	var server string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List the variables of your account, or of a server or team",
		Example: "  pt variable list\n  pt variable list --server prod\n  pt variable list --team Ops",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := variablesPath(cmd.Context(), a, server)
			if err != nil {
				return err
			}
			return showList(cmd.Context(), a, variableListing(path, page))
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&server, "server", "", "the variables of this server (name or id)")
	return cmd
}

// variablesPath is the endpoint of the variables of the server, the team (global --team) or the account.
func variablesPath(ctx context.Context, a *App, server string) (string, error) {
	if server != "" && a.flags.team != "" {
		return "", usageError("Use either --server or --team", "Variables belong to a server, a team or your account.")
	}
	resolver, err := a.Resolver(ctx)
	if err != nil {
		return "", err
	}
	if server != "" {
		found, err := resolver.Server(ctx, server)
		return api.NestedPath(api.Servers, found.ID, "vars"), err
	}
	if resolver.TeamID != "" {
		return api.NestedPath(api.Teams, resolver.TeamID, "vars"), nil
	}
	return "account/vars", nil
}

// variableListing lists variables, secrets stay hidden.
func variableListing(path string, page pageOptions) listing[api.Variable] {
	return listing[api.Variable]{
		path:    path,
		opts:    page.options(),
		headers: []string{"NAME", "VALUE", ""},
		row: func(v api.Variable) []string {
			if v.Secret {
				return []string{ui.Bold.Render(v.Name), ui.Faint.Render("••••••"), ui.Faint.Render("secret")}
			}
			return []string{ui.Bold.Render(v.Name), v.Value, ""}
		},
		empty: "No variables.",
	}
}
