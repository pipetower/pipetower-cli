// Package gitlog reads from a git repository what a run delivers: the commit of a ref and the
// commits since an earlier one. It only reads, apart from fetching the ref from its remote.
package gitlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Upstream is the ref of the branch the current branch tracks.
const Upstream = "@{upstream}"

// A log lists at most this many commits and has at most this many bytes, whatever is reached
// first. Pipetower takes metadata of up to 100,000 bytes in total.
const (
	maxCommits = 200
	maxLength  = 40_000
)

// A log has one line per commit: short hash, date, author and subject. The date looks the same everywhere.
const (
	logFormat  = "--format=%h  %ad  %an  %s"
	dateFormat = "--date=format:%Y-%m-%d %H:%M"
)

// ErrNoRepository says that the directory is not inside a git repository.
var ErrNoRepository = errors.New("not a git repository")

// Error is a git command which failed.
type Error struct {
	Args   []string
	Output string
}

// Error names the command with what git answered.
func (e *Error) Error() string {
	return "git " + strings.Join(e.Args, " ") + ": " + e.Output
}

// Repo is a git repository.
type Repo struct {
	dir string
}

// Target is the commit a ref points to.
type Target struct {
	// Ref is the full name of the ref, like origin/main, or the ref as given if it has no name.
	Ref string
	// Branch is the ref without its remote.
	Branch string
	// Commit is the full hash.
	Commit string
	// Repository is the web address of the remote, empty if there is none.
	Repository string
}

// Log is the commits a target adds to what was delivered before.
type Log struct {
	// Previous is the commit the log starts after, empty if it is not known.
	Previous string
	// Commits is the log as text, one line per commit, empty if the previous commit is not known.
	Commits string
}

// Open returns the repository the directory belongs to.
func Open(ctx context.Context, dir string) (Repo, error) {
	repo := Repo{dir: dir}
	if _, err := repo.git(ctx, "rev-parse", "--is-inside-work-tree"); err != nil {
		return Repo{}, ErrNoRepository
	}
	return repo, nil
}

// Target resolves the ref to its commit. A ref of a remote, like origin/main or the upstream
// of the current branch, is fetched before, so the commit is the one the remote has now. The
// checked out branch and the working tree stay as they are.
func (r Repo) Target(ctx context.Context, ref string) (Target, error) {
	name, err := r.git(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", ref)
	if err != nil {
		return Target{}, err
	}
	target := Target{Ref: name, Branch: name}
	if name == "" {
		target.Ref, target.Branch = ref, ""
	}

	remote := r.remoteOf(ctx, name)
	if remote != "" {
		target.Branch = strings.TrimPrefix(name, remote+"/")
		if _, err := r.git(ctx, "fetch", "--quiet", remote, target.Branch); err != nil {
			return Target{}, err
		}
	}
	if target.Commit, err = r.git(ctx, "rev-parse", "--verify", target.Ref+"^{commit}"); err != nil {
		return Target{}, err
	}
	target.Repository = r.repository(ctx, remote)
	return target, nil
}

// Log lists the commits of the target which the previous commits don't have: those since the
// commit all of them share. Without previous commits, or if this repository doesn't know one
// of them, nobody can tell which commits are new, so the log is empty.
func (r Repo) Log(ctx context.Context, previous []string, commit string) (Log, error) {
	base := r.base(ctx, previous)
	if base == "" {
		return Log{}, nil
	}

	added, err := r.count(ctx, base+".."+commit)
	if err != nil {
		return Log{}, err
	}
	if added == 0 {
		return r.unchanged(ctx, base, commit)
	}
	commits, err := r.git(ctx, "log", fmt.Sprintf("--max-count=%d", maxCommits), logFormat, dateFormat, base+".."+commit)
	return Log{Previous: base, Commits: fit(strings.Split(commits, "\n"), added)}, err
}

// fit joins the lines of the listed commits as long as they fit into the length of a log, and
// tells in a last line how many of all commits are not listed. A first line which alone is too
// long is cut.
func fit(lines []string, total int) string {
	length, listed := 0, 0
	for _, line := range lines {
		if length += len(line) + 1; length > maxLength {
			break
		}
		listed++
	}
	if listed == 0 {
		// cut at the limit, without half a character at the end
		lines, listed = []string{strings.ToValidUTF8(lines[0][:maxLength-len("…")-1], "") + "…"}, 1
	}

	log := strings.Join(lines[:listed], "\n")
	if total > listed {
		log += fmt.Sprintf("\n… and %d more", total-listed)
	}
	return log
}

// unchanged describes a target without new commits: the same commit as before, or an older one.
func (r Repo) unchanged(ctx context.Context, base, commit string) (Log, error) {
	removed, err := r.count(ctx, commit+".."+base)
	if err != nil {
		return Log{}, err
	}
	if removed == 0 {
		return Log{Previous: base, Commits: "No new commits"}, nil
	}
	return Log{Previous: base, Commits: fmt.Sprintf("No new commits, %d of the previous commits are not included", removed)}, nil
}

// base is the newest commit all previous commits share, empty without previous commits or if
// one of them is not known.
func (r Repo) base(ctx context.Context, previous []string) string {
	if len(previous) == 0 {
		return ""
	}
	for _, commit := range previous {
		if _, err := r.git(ctx, "cat-file", "-e", commit+"^{commit}"); err != nil {
			return ""
		}
	}
	base, err := r.git(ctx, append([]string{"merge-base", "--octopus"}, previous...)...)
	if err != nil {
		return ""
	}
	return base
}

// count counts the commits of a range.
func (r Repo) count(ctx context.Context, commits string) (int, error) {
	out, err := r.git(ctx, "rev-list", "--count", commits)
	if err != nil {
		return 0, err
	}
	var count int
	_, err = fmt.Sscan(out, &count)
	return count, err
}

// remoteOf returns the remote a ref name like origin/main belongs to, empty for other refs.
func (r Repo) remoteOf(ctx context.Context, name string) string {
	remotes, err := r.git(ctx, "remote")
	if err != nil {
		return ""
	}
	for _, remote := range strings.Fields(remotes) {
		if strings.HasPrefix(name, remote+"/") {
			return remote
		}
	}
	return ""
}

// repository is the web address of the remote, or of origin for a ref without remote.
func (r Repo) repository(ctx context.Context, remote string) string {
	if remote == "" {
		remote = "origin"
	}
	address, err := r.git(ctx, "remote", "get-url", remote)
	if err != nil {
		return ""
	}
	return WebURL(address)
}

// git runs a git command in the repository and returns its trimmed output.
func (r Repo) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		output := strings.TrimSpace(stderr.String())
		if output == "" {
			output = err.Error()
		}
		return "", &Error{Args: args, Output: output}
	}
	return strings.TrimSpace(stdout.String()), nil
}
