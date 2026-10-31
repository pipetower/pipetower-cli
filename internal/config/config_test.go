package config

import (
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

func TestLoadPrefersEnvironment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), "PIPETOWER_URL=https://file\nPIPETOWER_TOKEN=file-token\n")

	cfg, err := Load(testEnv(t, root, "", map[string]string{EnvToken: "env-token", EnvURL: "https://env"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "env-token" || cfg.TokenSource.Env != EnvToken || cfg.URL != "https://env" {
		t.Errorf("token = %q from %v, url = %q", cfg.Token, cfg.TokenSource, cfg.URL)
	}
}

func TestTokenFromEnvironmentNeverGoesToTheURLOfAFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), "PIPETOWER_URL=https://attacker.example\n")

	cfg, err := Load(testEnv(t, root, "", map[string]string{EnvToken: "env-token"}))

	var missing *MissingError
	if !errors.As(err, &missing) || missing.Key != EnvURL || cfg.URL != "" {
		t.Fatalf("err = %v, url = %q", err, cfg.URL)
	}
}

func TestFileWithoutTokenCannotRedirectTheTokenOfAnotherFile(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "cloned-repo")
	nested := filepath.Join(project, "a", "b")
	home := filepath.Join(root, "home")
	writeFile(t, filepath.Join(project, FileName), "PIPETOWER_URL=https://attacker.example\n")
	writeFile(t, filepath.Join(home, FileName), "PIPETOWER_URL=https://prod\nPIPETOWER_TOKEN='12|abc'\n")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(testEnv(t, nested, home, nil))
	if err != nil {
		t.Fatal(err)
	}
	homeFile := filepath.Join(home, FileName)
	if cfg.URL != "https://prod" || cfg.URLSource.Path != homeFile || cfg.TokenSource.Path != homeFile {
		t.Errorf("url = %q from %v, token from %v", cfg.URL, cfg.URLSource, cfg.TokenSource)
	}
	if len(cfg.Warnings) == 0 || !strings.Contains(cfg.Warnings[0], "Ignored the PIPETOWER_URL in "+filepath.Join(project, FileName)) {
		t.Errorf("the ignored URL must be announced: %v", cfg.Warnings)
	}
}

func TestNearestFileWithTokenWins(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	home := filepath.Join(root, "home")
	writeFile(t, filepath.Join(project, FileName), "# local instance\nexport PIPETOWER_URL=\"http://pipetower.localhost/\"\nPIPETOWER_TOKEN=local\n")
	writeFile(t, filepath.Join(home, FileName), "PIPETOWER_URL=https://prod\nPIPETOWER_TOKEN=prod\n")

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
	if !strings.Contains(missing.Problem().Hints[0].Value, "https://x/account") {
		t.Errorf("hint = %v", missing.Problem().Hints[0])
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
	writeFile(t, path, "PIPETOWER_URL=https://x\nPIPETOWER_TOKEN=t\n")
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

func TestParseLine(t *testing.T) {
	cases := map[string][2]string{
		"KEY=value":          {"KEY", "value"},
		"  KEY = 'a b'  ":    {"KEY", "a b"},
		"export KEY=\"x=y\"": {"KEY", "x=y"},
	}
	for line, want := range cases {
		key, value, ok := parseLine(line)
		if !ok || key != want[0] || value != want[1] {
			t.Errorf("parseLine(%q) = %q, %q, %v", line, key, value, ok)
		}
	}
	for _, line := range []string{"", "# comment", "no equals"} {
		if _, _, ok := parseLine(line); ok {
			t.Errorf("parseLine(%q) should be skipped", line)
		}
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
