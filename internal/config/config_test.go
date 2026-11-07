package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testEnv(t *testing.T, cwd, home string, vars map[string]string) Env {
	t.Helper()
	return Env{
		Getenv:     func(key string) string { return vars[key] },
		Cwd:        cwd,
		Home:       home,
		GitIgnored: func(string) (bool, bool) { return false, false },
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// twoEnvironments is a file with a local and a production environment, prod active, local limited to Ops.
const twoEnvironments = `{
  "environment": "prod",
  "environments": {
    "local": {"url": "http://pipetower.localhost/", "token": "local-token", "team": "Ops"},
    "prod": {"url": "https://app.pipetower.com", "token": "prod-token"}
  }
}`

func TestTheActiveEnvironmentOfTheFileIsUsed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	writeFile(t, path, twoEnvironments)

	cfg, err := Load(testEnv(t, root, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "prod" || cfg.URL != "https://app.pipetower.com" || cfg.Token != "prod-token" || cfg.Team != "" {
		t.Errorf("environment %q, url %q, token %q, team %q", cfg.Environment, cfg.URL, cfg.Token, cfg.Team)
	}
	if cfg.URLSource.Path != path || cfg.TokenSource.Path != path || cfg.EnvironmentsPath != path || strings.Join(cfg.EnvironmentNames(), ",") != "local,prod" {
		t.Errorf("sources: url %v, token %v, file %q, names %v", cfg.URLSource, cfg.TokenSource, cfg.EnvironmentsPath, cfg.EnvironmentNames())
	}
}

func TestTheEnvironmentVariableAndTheNearestFileChooseTheEnvironment(t *testing.T) {
	root := t.TempDir()
	home, project := filepath.Join(root, "home"), filepath.Join(root, "project")
	writeFile(t, filepath.Join(home, FileName), twoEnvironments)
	writeFile(t, filepath.Join(project, FileName), `{"environment": "local"}`)

	inProject, err := Load(testEnv(t, filepath.Join(project, "src"), home, nil))
	fromEnv, _ := Load(testEnv(t, project, home, map[string]string{EnvName: "prod"}))
	if err != nil {
		t.Fatal(err)
	}
	if inProject.Environment != "local" || inProject.EnvironmentSource.Path != filepath.Join(project, FileName) || inProject.Token != "local-token" || inProject.Team != "Ops" {
		t.Errorf("in the project: %q from %v, token %q, team %q", inProject.Environment, inProject.EnvironmentSource, inProject.Token, inProject.Team)
	}
	if fromEnv.Environment != "prod" || fromEnv.EnvironmentSource.Env != EnvName || fromEnv.Token != "prod-token" {
		t.Errorf("from $PIPETOWER_ENV: %q from %v, token %q", fromEnv.Environment, fromEnv.EnvironmentSource, fromEnv.Token)
	}
	if len(inProject.Warnings) != 0 {
		t.Errorf("a file which only names the environment is no reason to warn: %v", inProject.Warnings)
	}
}

func TestTheOnlyEnvironmentIsActiveWithoutBeingNamed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), `{"environments": {"prod": {"url": "https://app", "token": "t"}}}`)

	cfg, err := Load(testEnv(t, root, "", nil))

	if err != nil || cfg.Environment != "prod" || cfg.Token != "t" {
		t.Errorf("err = %v, environment %q, token %q", err, cfg.Environment, cfg.Token)
	}
}

func TestSeveralEnvironmentsNeedAnActiveOne(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), `{"environments": {"a": {"url": "https://a", "token": "t"}, "b": {"url": "https://b", "token": "t"}}}`)

	_, err := Load(testEnv(t, root, "", nil))
	_, unknown := Load(testEnv(t, root, "", map[string]string{EnvName: "c"}))

	var none *NoEnvironmentError
	var unknownErr *UnknownEnvironmentError
	if !errors.As(err, &none) || strings.Join(none.Names, ",") != "a,b" {
		t.Errorf("err = %v", err)
	}
	if !errors.As(unknown, &unknownErr) || unknownErr.Name != "c" || unknownErr.Source.Env != EnvName {
		t.Errorf("err = %v", unknown)
	}
}

func TestLoadPrefersTheProcessEnvironment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), twoEnvironments)

	cfg, err := Load(testEnv(t, root, "", map[string]string{EnvToken: "env-token", EnvURL: "https://env"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "env-token" || cfg.TokenSource.Env != EnvToken || cfg.URL != "https://env" || cfg.Environment != "" {
		t.Errorf("token = %q from %v, url = %q, environment %q", cfg.Token, cfg.TokenSource, cfg.URL, cfg.Environment)
	}
}

func TestTokenFromTheProcessEnvironmentNeverGoesToTheURLOfAFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), `{"environments": {"x": {"url": "https://attacker.example"}}}`)

	cfg, err := Load(testEnv(t, root, "", map[string]string{EnvToken: "env-token"}))

	var missing *MissingError
	if !errors.As(err, &missing) || missing.Key != EnvURL || cfg.URL != "" {
		t.Fatalf("err = %v, url = %q", err, cfg.URL)
	}
}

func TestAProjectFileCannotRedirectTheTokenOfHome(t *testing.T) {
	root := t.TempDir()
	project, home := filepath.Join(root, "cloned-repo"), filepath.Join(root, "home")
	writeFile(t, filepath.Join(project, FileName), `{"environment": "prod", "environments": {"prod": {"url": "https://attacker.example"}}}`)
	writeFile(t, filepath.Join(home, FileName), twoEnvironments)

	_, err := Load(testEnv(t, filepath.Join(project, "a", "b"), home, nil))

	// the nearest file with environments wins, but its environment has no token: nothing of home is used
	var missing *MissingError
	if !errors.As(err, &missing) || missing.Key != EnvToken || missing.Environment != "prod" {
		t.Fatalf("err = %v", err)
	}
}

func TestNearestFileWithEnvironmentsWinsAndIsAnnounced(t *testing.T) {
	root := t.TempDir()
	project, home := filepath.Join(root, "project"), filepath.Join(root, "home")
	writeFile(t, filepath.Join(project, FileName), `{"environments": {"local": {"url": "http://pipetower.localhost", "token": "local"}}}`)
	writeFile(t, filepath.Join(home, FileName), twoEnvironments)

	cfg, err := Load(testEnv(t, project, home, nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://pipetower.localhost" || cfg.Token != "local" {
		t.Errorf("url = %q, token = %q", cfg.URL, cfg.Token)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "instead of ~/.pipetower") {
		t.Errorf("a project file shadowing ~/.pipetower must be announced: %v", cfg.Warnings)
	}
}

func TestTheTeamComesFromTheVariableTheNearestFileOrTheEnvironment(t *testing.T) {
	root := t.TempDir()
	home, project := filepath.Join(root, "home"), filepath.Join(root, "project")
	writeFile(t, filepath.Join(home, FileName), twoEnvironments)
	writeFile(t, filepath.Join(project, FileName), `{"environment": "local", "team": "Shop"}`)

	fromProject, _ := Load(testEnv(t, project, home, nil))
	fromEnvironment, _ := Load(testEnv(t, root, home, map[string]string{EnvName: "local"}))
	none, _ := Load(testEnv(t, root, home, nil))
	fromVariable, _ := Load(testEnv(t, project, home, map[string]string{EnvTeam: "Sales"}))

	if fromProject.Team != "Shop" || fromProject.TeamSource.Path != filepath.Join(project, FileName) {
		t.Errorf("project: team %q from %v", fromProject.Team, fromProject.TeamSource)
	}
	if fromEnvironment.Team != "Ops" || fromEnvironment.TeamSource.Path != filepath.Join(home, FileName) {
		t.Errorf("environment: team %q from %v", fromEnvironment.Team, fromEnvironment.TeamSource)
	}
	if none.Team != "" || none.TeamSource != (Source{}) {
		t.Errorf("prod has no team: %q from %v", none.Team, none.TeamSource)
	}
	if fromVariable.Team != "Sales" || fromVariable.TeamSource.Env != EnvTeam {
		t.Errorf("variable: team %q from %v", fromVariable.Team, fromVariable.TeamSource)
	}
}

func TestSetEnvironmentAndSetTeamRewriteTheFileAndKeepTheRest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	writeFile(t, path, `{"environment": "prod", "custom": {"keep": true}, "environments": {"local": {"url": "http://pipetower.localhost", "token": "l"}, "prod": {"url": "https://app", "token": "p", "team": "Ops"}}}`)

	if err := SetEnvironment(path, "local"); err != nil {
		t.Fatal(err)
	}
	if err := SetTeam(path, "local", "Shop"); err != nil {
		t.Fatal(err)
	}
	if err := SetTeam(path, "prod", ""); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(testEnv(t, root, "", nil))
	if err != nil || cfg.Environment != "local" || cfg.Team != "Shop" || cfg.Environments["prod"].Team != "" {
		t.Errorf("err = %v, environment %q, team %q, prod team %q", err, cfg.Environment, cfg.Team, cfg.Environments["prod"].Team)
	}
	content, _ := os.ReadFile(path)
	var raw map[string]any
	if json.Unmarshal(content, &raw) != nil || raw["custom"] == nil || strings.Contains(string(content), `"team": ""`) {
		t.Errorf("other keys must be kept and empty teams left out:\n%s", content)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}

	var unknown *UnknownEnvironmentError
	if err := SetEnvironment(path, "staging"); !errors.As(err, &unknown) {
		t.Errorf("err = %v", err)
	}
}

func TestABrokenFileIsReportedAndSkippedAndADirectoryIsIgnored(t *testing.T) {
	root := t.TempDir()
	project, nested, home := filepath.Join(root, "project"), filepath.Join(root, "project", "nested"), filepath.Join(root, "home")
	writeFile(t, filepath.Join(project, FileName), `{"environment": "prod",`)
	writeFile(t, filepath.Join(home, FileName), twoEnvironments)
	if err := os.MkdirAll(filepath.Join(nested, FileName), 0o755); err != nil { // a directory of that name, like a temp folder
		t.Fatal(err)
	}

	cfg, err := Load(testEnv(t, nested, home, nil))

	if err != nil || cfg.Token != "prod-token" || len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "not valid JSON") {
		t.Errorf("err = %v, token %q, warnings %v", err, cfg.Token, cfg.Warnings)
	}
}

func TestLoadAcceptsPlainHTTPOnlyForLocalInstances(t *testing.T) {
	cases := map[string]bool{
		"https://app.pipetower.com":  true,
		"http://pipetower.localhost": true,
		"http://localhost:8000":      true,
		"http://127.0.0.1":           true,
		"http://[::1]:8080":          true,
		"http://app.pipetower.com":   false,
		"http://10.0.0.5":            false,
		"ftp://app.pipetower.com":    false,
	}
	for url, valid := range cases {
		_, err := Load(testEnv(t, t.TempDir(), "", map[string]string{EnvURL: url, EnvToken: "t"}))

		var invalid *InvalidURLError
		if valid == errors.As(err, &invalid) {
			t.Errorf("%s: err = %v", url, err)
		}
	}
}

func TestFilesOfOtherUsersAreNotTrusted(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root owns every file")
	}
	if _, err := os.Stat("/etc/hostname"); err != nil {
		t.Skip("no file of another user available")
	}

	if ownedByCurrentUser("/etc/hostname") {
		t.Error("/etc/hostname must not count as owned by the current user")
	}
	if !ownedByCurrentUser(t.TempDir()) {
		t.Error("an own directory must count as owned")
	}
}

func TestLoadReportsMissingToken(t *testing.T) {
	root := t.TempDir()

	_, err := Load(testEnv(t, root, root, map[string]string{EnvURL: "https://x"}))

	var missing *MissingError
	if !errors.As(err, &missing) || missing.Key != EnvToken {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(missing.Problem().Hints[0].Value, "https://x/account") || !strings.Contains(missing.Problem().Hints[1].Value, "pt env add") {
		t.Errorf("hints = %v", missing.Problem().Hints)
	}
}

func TestLoadRejectsURLWithoutScheme(t *testing.T) {
	_, err := Load(testEnv(t, t.TempDir(), "", map[string]string{EnvURL: "pipetower.com", EnvToken: "t"}))

	var invalid *InvalidURLError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadWarnsAboutLooseFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	writeFile(t, path, twoEnvironments)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	env := testEnv(t, root, "", nil)
	env.GitIgnored = func(string) (bool, bool) { return true, false }

	cfg, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 2 {
		t.Errorf("warnings = %v", cfg.Warnings)
	}
}

func TestGatewayURLMustBeEncryptedUnlessLocal(t *testing.T) {
	cases := map[string]bool{
		"wss://app.pipetower.com/gateway":  true,
		"ws://localhost:8081":              true,
		"ws://gateway.pipetower.localhost": true,
		"ws://app.pipetower.com:8081":      false,
		"https://app.pipetower.com":        false,
	}
	for url, valid := range cases {
		if err := ValidateGatewayURL(url); (err == nil) != valid {
			t.Errorf("%s: err = %v", url, err)
		}
	}
}

func TestAddEnvironmentCreatesTheFileAndActivatesTheFirstOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	first, err := AddEnvironment(path, "prod", Environment{URL: "https://app.pipetower.com/", Token: "p"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := AddEnvironment(path, "local", Environment{URL: "http://pipetower.localhost", Token: "l", Team: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	_, exists := AddEnvironment(path, "prod", Environment{URL: "https://x", Token: "t"})
	_, insecure := AddEnvironment(path, "bad", Environment{URL: "http://app.pipetower.com", Token: "t"})

	cfg, err := Load(testEnv(t, filepath.Dir(path), "", nil))
	if err != nil || !first || second || cfg.Environment != "prod" || cfg.Environments["local"].Team != "Ops" || cfg.Environments["prod"].URL != "https://app.pipetower.com/" {
		t.Errorf("err = %v, first %v, second %v, cfg %+v", err, first, second, cfg)
	}
	var existing *ExistingEnvironmentError
	var invalid *InvalidURLError
	if !errors.As(exists, &existing) || !errors.As(insecure, &invalid) {
		t.Errorf("exists = %v, insecure = %v", exists, insecure)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}
