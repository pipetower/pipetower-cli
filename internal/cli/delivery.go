package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/fault"
	"github.com/pipetower/pipetower-cli/internal/gitlog"
	"github.com/pipetower/pipetower-cli/internal/resolve"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// commitVariable is the variable of an action or workflow which takes the commit to deliver.
const commitVariable = "COMMIT"

// The names of the metadata --meta-git adds to a run.
const (
	metaRepository     = "repository"
	metaBranch         = "branch"
	metaCommit         = "commit"
	metaPreviousCommit = "previous_commit"
	metaCompare        = "compare"
	metaCommits        = "commits"
)

// delivery is what a run delivers from the git repository pt runs in: the commit of a ref and
// the commits since the last run, if that run told its commit. It is nil without --meta-git.
type delivery struct {
	target gitlog.Target
	log    gitlog.Log
}

// gitDelivery collects the delivery of --meta-git: the commit of the ref, and its commits since
// the last successful run of the action or workflow on each server. Only if every one of these
// runs told its commit, it is known which commits are new. The repository must have these
// commits, otherwise it is not the one which is deployed.
func gitDelivery(ctx context.Context, a *App, runnable resolve.Runnable, servers []api.Server, flags runFlags) (*delivery, error) {
	ref := flags.meta.git
	if ref == "" {
		return nil, nil
	}

	d, err := ui.Spin(a.IO, "Collecting the commits…", func() (*delivery, error) {
		repo, err := gitlog.Open(ctx, a.env.Cwd)
		if err != nil {
			return nil, err
		}
		target, err := repo.Target(ctx, ref)
		if err != nil {
			return nil, err
		}
		previous, err := previousCommits(ctx, a, runnable, servers)
		if err != nil {
			return nil, err
		}
		if err := repo.Require(ctx, target, previous); err != nil {
			return nil, err
		}
		log, err := repo.Log(ctx, previous, target.Commit)
		return &delivery{target: target, log: log}, err
	})
	if err != nil {
		return nil, metaGitError(err, ref)
	}
	if !flags.follow.quiet {
		a.IO.Errorln(" " + ui.Muted.Render("Commit ") + ui.Bold.Render(shortCommit(d.target.Commit)) + ui.Muted.Render(" of "+d.target.Ref+d.note()))
	}
	return d, nil
}

// note tells why the commits are not listed, nothing if they are.
func (d *delivery) note() string {
	if d.log.Commits != "" {
		return ""
	}
	return ", new commits are not listed because the last run recorded no commit"
}

// metadata is the delivery as metadata of the run, nothing without a delivery.
func (d *delivery) metadata() map[string]string {
	if d == nil {
		return nil
	}
	metadata := map[string]string{
		metaRepository:     d.target.Repository,
		metaBranch:         d.target.Branch,
		metaCommit:         d.target.Commit,
		metaPreviousCommit: d.log.Previous,
		metaCompare:        gitlog.CompareURL(d.target.Repository, shortCommit(d.log.Previous), shortCommit(d.target.Commit)),
		metaCommits:        d.log.Commits,
	}
	for name, value := range metadata {
		if value == "" {
			delete(metadata, name)
		}
	}
	return metadata
}

// variables adds the commit of the delivery to the --var flags, if the action or workflow has
// the variable for it and no flag gives it.
func (d *delivery) variables(runnable resolve.Runnable, flags []string) []string {
	if d == nil || !hasVariable(runnable, commitVariable) {
		return flags
	}
	for _, flag := range flags {
		if strings.HasPrefix(flag, commitVariable+"=") {
			return flags
		}
	}
	return append(append([]string{}, flags...), commitVariable+"="+d.target.Commit)
}

// hasVariable tells whether the action or workflow takes a variable of the name.
func hasVariable(runnable resolve.Runnable, name string) bool {
	for _, definition := range runnable.Variables() {
		if definition.Name == name {
			return true
		}
	}
	return false
}

// previousCommits returns, per server, the commit of the last successful run of the action or
// workflow there. It returns none if one of these runs told no commit or a server had no run:
// what was delivered there since is not known then.
func previousCommits(ctx context.Context, a *App, runnable resolve.Runnable, servers []api.Server) ([]string, error) {
	client, err := a.Client()
	if err != nil {
		return nil, err
	}

	var commits []string
	for _, server := range servers {
		params := map[string]string{strings.ToLower(runnable.Type()): runnable.ID(), "server": server.ID, "status": api.StatusSuccessful}
		page, err := api.Get[api.Page[api.Run]](ctx, client, api.List(api.Runs, api.ListOptions{Limit: 1, Params: params}))
		if err != nil {
			return nil, err
		}
		if len(page.Data) == 0 || page.Data[0].Metadata[metaCommit] == "" {
			return nil, nil
		}
		commits = append(commits, page.Data[0].Metadata[metaCommit])
	}
	return commits, nil
}

// metaGitError explains why --meta-git could not collect the delivery.
func metaGitError(err error, ref string) error {
	var gitErr *gitlog.Error
	var foreign *gitlog.ForeignCommitError
	var fetch *gitlog.FetchError
	switch {
	case errors.As(err, &fetch):
		return fault.New(fault.Problem{
			Title:  "Git could not fetch from “" + fetch.Remote + "”",
			Detail: fetch.Output + "\nCheck the network and your login for the repository, or run it without --meta-git.",
			Code:   fault.ExitFailure,
		})
	case errors.As(err, &foreign):
		return fault.New(fault.Problem{
			Title: "This is not the repository which was deployed last",
			Detail: "The last run deployed the commit " + shortCommit(foreign.Commit) + ", which " + ui.Or(foreign.Repository, "this repository") + " does not have.\n" +
				"Run pt inside the repository which is deployed, or without --meta-git.",
			Code: fault.ExitUsage,
		})
	case errors.Is(err, gitlog.ErrNoRepository):
		return usageError("--meta-git needs a git repository", "Run pt inside the repository which is deployed.")
	case errors.As(err, &gitErr) && ref == gitlog.Upstream:
		return fault.New(fault.Problem{
			Title:  "Git could not find the remote branch of your current branch",
			Detail: gitErr.Output,
			Hints:  []fault.Hint{{Label: "Name the ref", Value: "--meta-git=origin/main"}},
			Code:   fault.ExitUsage,
		})
	case errors.As(err, &gitErr):
		return fault.New(fault.Problem{Title: "Git could not find “" + ref + "”", Detail: gitErr.Output, Code: fault.ExitUsage})
	}
	return err
}

// shortCommit is the short form of a commit hash.
func shortCommit(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}
