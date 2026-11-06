package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/definition"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// definitionKind describes a kind whose objects are described by definition files: how one is
// resolved from an argument and how its id and name are read.
type definitionKind[T any] struct {
	kind resolve.Kind
	find func(*resolve.Resolver, context.Context, string) (T, error)
	id   func(T) string
	name func(T) string
}

// newDefinitionCommands builds export, apply, delete and example of a kind.
func newDefinitionCommands[T any](a *App, k definitionKind[T]) []*cobra.Command {
	return []*cobra.Command{newExportCommand(a, k), newApplyCommand(a, k.kind), newDeleteCommand(a, k), newExampleCommand(a, k.kind)}
}

// newExportCommand prints the definition of an object, as YAML (the format of definition files) or as JSON with -o json.
func newExportCommand[T any](a *App, k definitionKind[T]) *cobra.Command {
	noun := k.kind.Singular
	return &cobra.Command{
		Use:   "export <" + noun + ">",
		Short: "Print the definition of " + withArticle(noun) + " as YAML",
		Long: "Print the definition of " + withArticle(noun) + ": the file which describes it completely, as YAML (or\n" +
			"JSON with -o json). Keep it in a file, edit it and apply it again with “pt " + noun + " apply”.",
		Example: "  pt " + noun + " export \"Deploy App\" > deploy.yml",
		Args:    argOrPick(a, noun),
		RunE: func(cmd *cobra.Command, argv []string) error {
			resolver, err := a.Resolver(cmd.Context())
			if err != nil {
				return err
			}
			item, err := k.find(resolver, cmd.Context(), optionalArg(argv))
			if err != nil {
				return err
			}
			return a.printDefinition(cmd.Context(), api.Definition(k.kind.Resource, k.id(item)))
		},
	}
}

// newApplyCommand creates or updates an object from a definition file: the object of the file's
// name in the team is replaced, otherwise it is created.
func newApplyCommand(a *App, kind resolve.Kind) *cobra.Command {
	var file string
	var dryRun bool
	noun := kind.Singular
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Create or update " + withArticle(noun) + " from a definition file",
		Long: "Create or update " + withArticle(noun) + " from a definition file (YAML or JSON, see “pt " + noun + " example”).\n" +
			"The file describes the whole " + noun + " by its name: " + withArticle(noun) + " of that name in the team is\n" +
			"replaced, otherwise it is created in the team (--team or PIPETOWER_TEAM, needed with several\n" +
			"teams). Names in the file (templates, actions, servers, tags) are resolved in that team.\n" +
			"--dry-run only checks the file, nothing is saved. Problems are listed by their path in the\n" +
			"file, like steps.1.conditions.0.0.rule.",
		Example: "  pt " + noun + " apply -f deploy.yml --dry-run\n  pt " + noun + " apply -f deploy.yml\n  cat deploy.yml | pt " + noun + " apply -f -",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				return usageProblem(cmd, "Missing --file")
			}
			d, err := definition.Load(file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if d.Kind != noun {
				return usageError(d.Source+" is "+withArticle(d.Kind)+", not "+withArticle(noun), "Use “pt "+d.Kind+" apply”.")
			}
			return apply(cmd.Context(), a, kind, d, dryRun)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "the definition file, - for stdin")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only check the file, save nothing")
	return cmd
}

// apply sends the definition: PUT to the object of its name, or POST for a new one.
func apply(ctx context.Context, a *App, kind resolve.Kind, d definition.Definition, dryRun bool) error {
	resolver, err := a.Resolver(ctx)
	if err != nil {
		return err
	}
	existing, err := resolver.Named(ctx, kind, d.Name)
	var ambiguous *resolve.AmbiguousError
	if errors.As(err, &ambiguous) {
		return fault.New(fault.Problem{
			Title:  ui.Capitalize(kind.Singular) + " “" + d.Name + "” exists in several of your teams",
			Detail: "Say which one to update.",
			Hints:  []fault.Hint{{Label: "Use", Value: "--team <name>"}},
			Code:   fault.ExitUsage,
		})
	}
	if err != nil {
		return err
	}

	extra := map[string]any{}
	if dryRun {
		extra["dry_run"] = true
	}
	endpoint := api.Put(kind.Resource+"/"+existing, d.Body(extra))
	if existing == "" {
		if resolver.TeamID != "" {
			extra["team"] = resolver.TeamID
		}
		endpoint = api.Post(kind.Resource, d.Body(extra))
	}

	client, err := a.Client()
	if err != nil {
		return err
	}
	raw, err := client.JSON(ctx, endpoint)
	if err != nil {
		return applyProblem(err, d.Source)
	}
	if a.Structured() {
		return a.PrintRaw(raw)
	}

	verb, done := "Created", "created"
	if existing != "" {
		verb, done = "Updated", "updated"
	}
	if dryRun {
		a.IO.Println(" " + ui.Success.Render("✓") + " " + d.Source + " is valid, " + kind.Singular + " " + ui.Bold.Render(d.Name) + " would be " + done)
		return nil
	}
	var saved api.Reference
	_ = json.Unmarshal(raw, &saved)
	a.IO.Println(" " + ui.Success.Render("✓") + " " + verb + " " + kind.Singular + " " + ui.Bold.Render(saved.Name) + ui.Faint.Render("  "+saved.ID))
	return nil
}

// applyProblem explains a rejected definition: the problems by their path in the file, or the missing team.
func applyProblem(err error, source string) error {
	var apiErr *api.Error
	if !api.IsStatus(err, http.StatusUnprocessableEntity) || !errors.As(err, &apiErr) {
		return err
	}
	if messages, ok := apiErr.Fields["team"]; ok && len(apiErr.Fields) == 1 {
		return fault.New(fault.Problem{
			Title:  "Which team?",
			Detail: strings.Join(messages, " "),
			Hints:  []fault.Hint{{Label: "Use", Value: "--team <name>"}},
			Code:   fault.ExitUsage,
		})
	}
	detail := strings.Join(apiErr.FieldErrors(), "\n")
	if detail == "" {
		detail = apiErr.Message
	}
	return fault.New(fault.Problem{Title: source + " is not valid", Detail: detail, Code: fault.ExitAPI})
}

// newDeleteCommand deletes an object after a confirmation.
func newDeleteCommand[T any](a *App, k definitionKind[T]) *cobra.Command {
	var yes bool
	noun := k.kind.Singular
	cmd := &cobra.Command{
		Use:   "delete <" + noun + ">",
		Short: "Delete " + withArticle(noun),
		Long:  "Delete " + withArticle(noun) + ". " + ui.Capitalize(withArticle(noun)) + " which other objects still use cannot be deleted, they are named instead.",
		Args:  argOrPick(a, noun),
		RunE: func(cmd *cobra.Command, argv []string) error {
			resolver, err := a.Resolver(cmd.Context())
			if err != nil {
				return err
			}
			item, err := k.find(resolver, cmd.Context(), optionalArg(argv))
			if err != nil {
				return err
			}
			id, name := k.id(item), k.name(item)
			if err := confirm(a, yes, "Delete "+noun+" “"+name+"”? There is no undo."); err != nil {
				return err
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			if _, err := client.JSON(cmd.Context(), api.Delete(k.kind.Resource, id)); err != nil {
				return deleteProblem(err, noun, name)
			}
			if a.Structured() {
				return a.PrintValue(map[string]any{"id": id, "name": name, "deleted": true})
			}
			a.IO.Println(" " + ui.Success.Render("✓") + " Deleted " + noun + " " + ui.Bold.Render(name))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}

// deleteProblem explains why an object cannot be deleted: the objects which still use it.
func deleteProblem(err error, noun, name string) error {
	var apiErr *api.Error
	if !api.IsStatus(err, http.StatusConflict) || !errors.As(err, &apiErr) {
		return err
	}
	var payload struct {
		UsedBy []api.Reference `json:"used_by"`
	}
	_ = json.Unmarshal(apiErr.Body, &payload)
	users := make([]string, len(payload.UsedBy))
	for i, user := range payload.UsedBy {
		users[i] = user.Type + " " + user.Name
	}
	return fault.New(fault.Problem{
		Title:  ui.Capitalize(noun) + " " + name + " is still used",
		Detail: "Remove it from these first:\n" + strings.Join(users, "\n"),
		Code:   fault.ExitAPI,
	})
}

// newExampleCommand prints an example definition of a kind which uses every option.
func newExampleCommand(a *App, kind resolve.Kind) *cobra.Command {
	noun := kind.Singular
	return &cobra.Command{
		Use:   "example",
		Short: "Print an example definition with every option",
		Long: "Print an example definition of " + withArticle(noun) + " which uses every option there is, with\n" +
			"comments which explain every key and the possible values (as JSON with -o json, without\n" +
			"the comments): the starting point for a file to apply.",
		Example: "  pt " + noun + " example > " + noun + ".yml",
		Args:    args(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.flags.output == FormatJSON {
				return a.printDefinition(cmd.Context(), api.Example(noun, false))
			}
			client, err := a.Client()
			if err != nil {
				return err
			}
			yaml, err := client.Text(cmd.Context(), api.Example(noun, true))
			if err != nil {
				return err
			}
			_, err = io.WriteString(a.IO.RawOut, yaml)
			return err
		},
	}
}

// printDefinition prints the definition an endpoint returns, as YAML unless JSON is requested.
func (a *App) printDefinition(ctx context.Context, endpoint api.Endpoint) error {
	client, err := a.Client()
	if err != nil {
		return err
	}
	raw, err := client.JSON(ctx, endpoint)
	if err != nil {
		return err
	}
	if a.flags.output == FormatJSON {
		return a.IO.PrintJSON(raw)
	}
	return a.IO.PrintYAML(raw)
}
