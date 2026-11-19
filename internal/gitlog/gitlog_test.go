package gitlog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repository is a git repository in a temporary directory.
type repository struct {
	t   *testing.T
	dir string
}

// newRepository creates an empty repository with the branch main.
func newRepository(t *testing.T) repository {
	t.Helper()
	r := repository{t: t, dir: t.TempDir()}
	r.git("init", "--quiet", "--initial-branch=main")
	return r
}

// clone clones the repository, so the clone tracks its branches as origin.
func (r repository) clone() repository {
	r.t.Helper()
	clone := repository{t: r.t, dir: r.t.TempDir()}
	clone.git("clone", "--quiet", r.dir, ".")
	return clone
}

// git runs a git command in the repository, with a fixed author and date.
func (r repository) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=ada@example.com", "GIT_AUTHOR_DATE=2026-11-17T12:00:00+00:00",
		"GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=ada@example.com", "GIT_COMMITTER_DATE=2026-11-17T12:00:00+00:00",
		"TZ=UTC",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit adds a commit with the subject and returns its hash.
func (r repository) commit(subject string) string {
	r.t.Helper()
	r.git("commit", "--quiet", "--allow-empty", "--message", subject)
	return r.git("rev-parse", "HEAD")
}

// open opens the repository like pt does.
func (r repository) open() Repo {
	r.t.Helper()
	r.t.Setenv("TZ", "UTC")
	repo, err := Open(context.Background(), r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	return repo
}

func TestOpenNeedsARepository(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); !errors.Is(err, ErrNoRepository) {
		t.Errorf("err = %v", err)
	}
}

func TestTargetOfTheUpstreamIsFetchedWithoutTouchingTheCheckout(t *testing.T) {
	origin := newRepository(t)
	origin.commit("First")
	clone := origin.clone()
	local := clone.commit("Local work which is not pushed")
	pushed := origin.commit("Second")
	if err := os.WriteFile(filepath.Join(clone.dir, "draft.txt"), []byte("draft"), 0o600); err != nil {
		t.Fatal(err)
	}

	target, err := clone.open().Target(context.Background(), Upstream)

	if err != nil || target.Ref != "origin/main" || target.Branch != "main" || target.Commit != pushed {
		t.Fatalf("target = %+v, err = %v", target, err)
	}
	if head := clone.git("rev-parse", "HEAD"); head != local {
		t.Errorf("HEAD moved to %s", head)
	}
	if _, err := os.Stat(filepath.Join(clone.dir, "draft.txt")); err != nil {
		t.Errorf("the working tree changed: %v", err)
	}
}

func TestTargetOfALocalRefIsNotFetched(t *testing.T) {
	origin := newRepository(t)
	first := origin.commit("First")
	clone := origin.clone()
	origin.commit("Second")
	repo := clone.open()

	for ref, branch := range map[string]string{"main": "main", first: ""} {
		target, err := repo.Target(context.Background(), ref)
		if err != nil || target.Commit != first || target.Branch != branch {
			t.Errorf("%s: target = %+v, err = %v", ref, target, err)
		}
	}
}

func TestTargetOfAnUnknownRefFails(t *testing.T) {
	repo := newRepository(t)
	repo.commit("First")

	for _, ref := range []string{"nope", Upstream} {
		var gitErr *Error
		if _, err := repo.open().Target(context.Background(), ref); !errors.As(err, &gitErr) {
			t.Errorf("%s: err = %v", ref, err)
		}
	}
}

func TestRequireFetchesACommitOfAnotherBranchOrWithoutBranch(t *testing.T) {
	origin := newRepository(t)
	origin.commit("First")
	origin.git("config", "uploadpack.allowAnySHA1InWant", "true")
	clone := origin.clone()
	origin.git("checkout", "--quiet", "-b", "preview")
	onBranch := origin.commit("On a branch the clone never fetched")
	origin.git("checkout", "--quiet", "--detach")
	withoutBranch := origin.commit("On no branch, like after a branch was deleted")
	origin.git("checkout", "--quiet", "main")
	repo := clone.open()
	target, err := repo.Target(context.Background(), "origin/main")
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Require(context.Background(), target, []string{onBranch, withoutBranch}); err != nil {
		t.Errorf("err = %v", err)
	}
	if log, err := repo.Log(context.Background(), []string{onBranch}, target.Commit); err != nil || log.Previous == "" {
		t.Errorf("the fetched commit must be usable: log = %+v, err = %v", log, err)
	}
}

func TestRequireRejectsACommitOfAnotherRepository(t *testing.T) {
	origin := newRepository(t)
	origin.commit("First")
	clone := origin.clone()
	other := newRepository(t)
	foreign := other.commit("Of another project")
	repo := clone.open()
	target, err := repo.Target(context.Background(), "origin/main")
	if err != nil {
		t.Fatal(err)
	}

	var foreignErr *ForeignCommitError
	if err := repo.Require(context.Background(), target, []string{target.Commit, foreign}); !errors.As(err, &foreignErr) || foreignErr.Commit != foreign {
		t.Errorf("err = %v", err)
	}
}

func TestARemoteWhichCanNotBeFetchedFromIsReportedAsThat(t *testing.T) {
	origin := newRepository(t)
	origin.commit("First")
	clone := origin.clone()
	repo := clone.open()
	target, err := repo.Target(context.Background(), "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	clone.git("remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))

	var fetchErr *FetchError
	if _, err := repo.Target(context.Background(), "origin/main"); !errors.As(err, &fetchErr) || fetchErr.Remote != "origin" {
		t.Errorf("target: err = %v", err)
	}
	if err := repo.Require(context.Background(), target, []string{"0000000000000000000000000000000000000000"}); !errors.As(err, &fetchErr) {
		t.Errorf("require: err = %v", err)
	}
}

func TestRequireChecksNothingWithoutARemoteOrACompleteHash(t *testing.T) {
	origin := newRepository(t)
	first := origin.commit("First")
	clone := origin.clone()
	local, err := origin.open().Target(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	remote, err := clone.open().Target(context.Background(), "origin/main")
	if err != nil {
		t.Fatal(err)
	}

	if err := origin.open().Require(context.Background(), local, []string{"0000000000000000000000000000000000000000"}); err != nil {
		t.Errorf("without a remote: err = %v", err)
	}
	if err := clone.open().Require(context.Background(), remote, []string{"abc1234", first}); err != nil {
		t.Errorf("with a short hash: err = %v", err)
	}
}

func TestLogListsTheCommitsSinceThePreviousOne(t *testing.T) {
	repo := newRepository(t)
	first := repo.commit("First")
	second := repo.commit("Second")
	third := repo.commit("Third")

	log, err := repo.open().Log(context.Background(), []string{first}, third)

	want := third[:7] + "  2026-11-17 12:00  Ada  Third\n" + second[:7] + "  2026-11-17 12:00  Ada  Second"
	if err != nil || log.Previous != first || log.Commits != want {
		t.Errorf("log = %+v, err = %v", log, err)
	}
}

func TestLogStartsAtTheOldestOfSeveralPreviousCommits(t *testing.T) {
	repo := newRepository(t)
	first := repo.commit("First")
	second := repo.commit("Second")
	third := repo.commit("Third")

	log, err := repo.open().Log(context.Background(), []string{second, first}, third)

	if err != nil || log.Previous != first || strings.Count(log.Commits, "\n") != 1 {
		t.Errorf("log = %+v, err = %v", log, err)
	}
}

func TestLogIsEmptyUnlessEveryPreviousCommitIsKnown(t *testing.T) {
	repo := newRepository(t)
	first := repo.commit("First")
	second := repo.commit("Second")

	for _, previous := range [][]string{nil, {"0000000000000000000000000000000000000000"}, {first, "0000000000000000000000000000000000000000"}} {
		log, err := repo.open().Log(context.Background(), previous, second)
		if err != nil || log != (Log{}) {
			t.Errorf("%v: log = %+v, err = %v", previous, log, err)
		}
	}
}

func TestLogWithoutNewCommits(t *testing.T) {
	repo := newRepository(t)
	first := repo.commit("First")
	second := repo.commit("Second")
	opened := repo.open()

	cases := map[string][2]string{
		"No new commits": {second, second},
		"No new commits, 1 of the previous commits are not included": {second, first},
	}
	for want, c := range cases {
		log, err := opened.Log(context.Background(), []string{c[0]}, c[1])
		if err != nil || log.Previous != c[0] || log.Commits != want {
			t.Errorf("log = %+v, err = %v, want %q", log, err, want)
		}
	}
}

func TestLogWithLongLinesIsCutByItsLength(t *testing.T) {
	short, long := "abc1234  Short", strings.Repeat("ä", maxLength/4)

	cases := []struct {
		lines []string
		total int
		want  string
	}{
		{[]string{short, short}, 2, short + "\n" + short},
		{[]string{short, long, long, short}, 9, short + "\n" + long + "\n… and 7 more"},
		{[]string{long + long + "x", short}, 2, strings.Repeat("ä", maxLength/2-2) + "…\n… and 1 more"},
	}
	for _, c := range cases {
		if got := fit(c.lines, c.total); got != c.want {
			t.Errorf("fit(%d lines of %d) has %d bytes, want %d", len(c.lines), c.total, len(got), len(c.want))
		}
	}
}

func TestLongLogIsCut(t *testing.T) {
	repo := newRepository(t)
	first := repo.commit("First")
	for range maxCommits + 2 {
		repo.commit("More")
	}

	log, err := repo.open().Log(context.Background(), []string{first}, "HEAD")

	lines := strings.Split(log.Commits, "\n")
	if err != nil || len(lines) != maxCommits+1 || lines[maxCommits] != "… and 2 more" {
		t.Errorf("lines = %d, last = %q, err = %v", len(lines), lines[len(lines)-1], err)
	}
}

func TestWebURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:pipetower/pipetower.git":               "https://github.com/pipetower/pipetower",
		"https://github.com/pipetower/pipetower.git":           "https://github.com/pipetower/pipetower",
		"https://oauth2:secret@github.com/pipetower/pipetower": "https://github.com/pipetower/pipetower",
		"ssh://git@git.example.com:2222/team/app.git":          "https://git.example.com/team/app",
		"/srv/git/app.git": "",
		"":                 "",
	}
	for remote, want := range cases {
		if got := WebURL(remote); got != want {
			t.Errorf("WebURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestCompareURL(t *testing.T) {
	cases := []struct{ repository, previous, commit, want string }{
		{"https://github.com/pipetower/pipetower", "a1", "b2", "https://github.com/pipetower/pipetower/compare/a1...b2"},
		{"https://gitlab.com/team/app", "a1", "b2", "https://gitlab.com/team/app/-/compare/a1...b2"},
		{"https://git.example.com/team/app", "a1", "b2", ""},
		{"https://github.com/pipetower/pipetower", "", "b2", ""},
		{"https://github.com/pipetower/pipetower", "b2", "b2", ""},
		{"", "a1", "b2", ""},
	}
	for _, c := range cases {
		if got := CompareURL(c.repository, c.previous, c.commit); got != c.want {
			t.Errorf("CompareURL(%q, %q, %q) = %q, want %q", c.repository, c.previous, c.commit, got, c.want)
		}
	}
}
