package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// listing describes how a list is fetched and shown as a table.
type listing[T any] struct {
	path    string
	opts    api.ListOptions
	headers []string
	row     func(T) []string
	// byTeam tells whether the endpoint can be limited with --team.
	byTeam bool
	empty  string
}

// pageOptions are the pagination flags of list commands.
type pageOptions struct {
	limit int
	page  int
}

// searchOptions converts the page flags and the search into list options.
func searchOptions(page pageOptions, search string) api.ListOptions {
	opts := page.options()
	opts.Search = search
	return opts
}

// options converts the flags into list options.
func (p pageOptions) options() api.ListOptions {
	return api.ListOptions{Limit: p.limit, Page: p.page}
}

// showList fetches a list and prints it as a table or as raw data.
func showList[T any](ctx context.Context, a *App, l listing[T]) error {
	client, err := a.Client()
	if err != nil {
		return err
	}
	if l.byTeam {
		if l.opts.Team, err = a.TeamID(ctx); err != nil {
			return err
		}
	}
	page, err := api.Get[api.Page[json.RawMessage]](ctx, client, api.List(l.path, l.opts))
	if err != nil {
		return err
	}

	items, raw, err := decodeItems[T](page.Data)
	if err != nil {
		return err
	}

	if a.Structured() {
		return a.PrintValue(map[string]any{"data": raw, "meta": page.Meta})
	}
	printTable(a, items, l)
	if page.Meta.HasMore {
		a.IO.Errorln(ui.Faint.Render(fmt.Sprintf(" Showing %d–%d of %d · use --page %d for more", page.Meta.From, page.Meta.To, page.Meta.Total, l.opts.Page+1)))
	}
	return nil
}

// printTable prints the items as a table or the empty hint.
func printTable[T any](a *App, items []T, l listing[T]) {
	if len(items) == 0 {
		a.IO.Println(ui.Empty(" " + l.empty))
		return
	}
	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = l.row(item)
	}
	a.IO.Println(ui.Table(l.headers, rows))
}

// decodeItems decodes the raw items while keeping them for structured output.
func decodeItems[T any](raw []json.RawMessage) ([]T, []json.RawMessage, error) {
	items := make([]T, len(raw))
	for i, item := range raw {
		if err := json.Unmarshal(item, &items[i]); err != nil {
			return nil, nil, err
		}
	}
	return items, raw, nil
}

// showOne prints an already resolved resource rendered, or freshly fetched as raw data.
func showOne[T any](ctx context.Context, a *App, value T, endpoint api.Endpoint, render func(T) string) error {
	if a.Structured() {
		return a.printEndpoint(ctx, endpoint)
	}
	a.IO.Println(render(value))
	return nil
}

// printEndpoint prints the raw response of an endpoint.
func (a *App) printEndpoint(ctx context.Context, endpoint api.Endpoint) error {
	client, err := a.Client()
	if err != nil {
		return err
	}
	raw, err := client.JSON(ctx, endpoint)
	if err != nil {
		return err
	}
	return a.PrintRaw(raw)
}

// usageError explains a wrong invocation.
func usageError(title, detail string) error {
	return fault.New(fault.Problem{Title: title, Detail: detail, Code: fault.ExitUsage})
}

// orNotFound replaces a 404 of the API with an error which names the argument.
func orNotFound(err error, kind resolve.Kind, arg string) error {
	if api.IsNotFound(err) {
		return &resolve.NotFoundError{Kind: kind, Arg: arg}
	}
	return err
}

// addWebFlag adds --web, which opens the viewed item in the web app instead.
func addWebFlag(cmd *cobra.Command, web *bool) {
	cmd.Flags().BoolVarP(web, "web", "w", false, "open it in the web app")
}

// newViewCommand builds the view command of a kind: the item rendered in the terminal, as raw data
// with --output, or opened in the web app with --web.
func newViewCommand[T any](a *App, kind resolve.Kind, short string, find func(*resolve.Resolver, context.Context, string) (T, error), id func(T) string, render func(T) string) *cobra.Command {
	var web bool
	cmd := &cobra.Command{
		Use:        "view <" + kind.Singular + ">",
		Short:      short,
		SuggestFor: []string{"show"},
		Args:       args(kind.Singular),
		RunE: func(cmd *cobra.Command, argv []string) error {
			resolver, err := a.Resolver(cmd.Context())
			if err != nil {
				return err
			}
			item, err := find(resolver, cmd.Context(), argv[0])
			if err != nil {
				return err
			}
			if web {
				return a.openWeb(kind.Resource + "/" + id(item))
			}
			return showOne(cmd.Context(), a, item, api.Show(kind.Resource, id(item)), render)
		},
	}
	addWebFlag(cmd, &web)
	return cmd
}
