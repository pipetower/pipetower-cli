// Package config discovers the instance URL and API token of the CLI.
//
// The token is taken from $PIPETOWER_TOKEN, or else from the first .pipetower file with a
// token: in the working directory, one of its parents or ~/.pipetower. The URL always comes
// from the same place as the token, so that a file of a cloned repository can never redirect
// the token to another server. Only $PIPETOWER_URL, which the user sets, may override it.
//
// The team which limits all resources is taken from $PIPETOWER_TEAM, or else from the first file
// with a team, independent of the token: a project's .pipetower may contain only its team and use
// the token of ~/.pipetower. A team can't send the token anywhere, it only narrows what is shown.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// Names of the configuration file and the environment variables.
const (
	FileName = ".pipetower"
	EnvURL   = "PIPETOWER_URL"
	EnvToken = "PIPETOWER_TOKEN"
	EnvTeam  = "PIPETOWER_TEAM"
)

// Source tells where a value came from: an environment variable or a file path.
type Source struct {
	Env  string
	Path string
}

// String describes the source for humans.
func (s Source) String() string {
	if s.Env != "" {
		return "$" + s.Env
	}
	return s.Path
}

// Config is the resolved configuration.
type Config struct {
	URL         string
	Token       string
	URLSource   Source
	TokenSource Source
	// Team (name or id) limits the resources, empty for all teams.
	Team       string
	TeamSource Source
	Searched   []string
	Warnings   []string
}

// Env abstracts the process environment, so discovery is testable.
type Env struct {
	Getenv     func(string) string
	Cwd        string
	Home       string
	GitIgnored func(path string) (inRepo bool, ignored bool)
}

// SystemEnv returns the environment of the running process.
func SystemEnv() Env {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	return Env{Getenv: os.Getenv, Cwd: cwd, Home: home, GitIgnored: gitIgnored}
}

// Load resolves the configuration and validates that URL and token are present and safe.
func Load(env Env) (*Config, error) {
	cfg := &Config{Searched: candidateFiles(env.Cwd, env.Home)}

	if token := env.Getenv(EnvToken); token != "" {
		cfg.Token, cfg.TokenSource = token, Source{Env: EnvToken}
	} else {
		cfg.loadFiles(env)
	}
	if url := env.Getenv(EnvURL); url != "" {
		cfg.URL, cfg.URLSource = normalizeURL(url), Source{Env: EnvURL}
	}
	if team := strings.TrimSpace(env.Getenv(EnvTeam)); team != "" {
		cfg.Team, cfg.TeamSource = team, Source{Env: EnvTeam}
	} else {
		cfg.loadTeam()
	}

	return cfg, cfg.validate(env.Home)
}

// loadFiles takes the token and the URL from the first file with a token.
func (c *Config) loadFiles(env Env) {
	for _, path := range c.Searched {
		values, err := readFile(path)
		if err == nil && values[EnvToken] == "" && values[EnvURL] != "" {
			c.Warnings = append(c.Warnings, fmt.Sprintf("Ignored the %s in %s, it has no %s next to it", EnvURL, path, EnvToken))
		}
		if err != nil || values[EnvToken] == "" {
			continue
		}
		if !ownedByCurrentUser(path) {
			c.Warnings = append(c.Warnings, fmt.Sprintf("Ignored %s, it belongs to another user", path))
			continue
		}

		source := Source{Path: path}
		c.Token, c.TokenSource = values[EnvToken], source
		if values[EnvURL] != "" {
			c.URL, c.URLSource = normalizeURL(values[EnvURL]), source
		}
		c.Warnings = append(c.Warnings, fileWarnings(path, env)...)
		c.Warnings = append(c.Warnings, shadowWarning(path, c.URL, env.Home)...)
		return
	}
}

// loadTeam takes the team from the first file with a team, which may be another than the one of
// the token.
func (c *Config) loadTeam() {
	for _, path := range c.Searched {
		values, err := readFile(path)
		if err != nil || values[EnvTeam] == "" {
			continue
		}
		if !ownedByCurrentUser(path) {
			continue // warned about when it has a token, a team alone can do no harm
		}
		c.Team, c.TeamSource = values[EnvTeam], Source{Path: path}
		return
	}
}

// validate returns an explaining error if URL or token is missing or invalid.
func (c *Config) validate(home string) error {
	switch {
	case c.Token == "":
		return &MissingError{Key: EnvToken, Searched: c.Searched, URL: c.URL, Home: home}
	case c.URL == "":
		return &MissingError{Key: EnvURL, Searched: c.Searched, Home: home, TokenSource: c.TokenSource}
	}
	return validateURL(c.URL, c.URLSource)
}

// validateURL accepts https URLs, and http only for local instances, so the token is never sent in clear text.
func validateURL(raw string, source Source) error {
	parsed, err := url.Parse(raw)
	switch {
	case err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http"):
		return &InvalidURLError{URL: raw, Source: source, Reason: "must start with https:// (or http:// for a local instance)"}
	case parsed.Scheme == "http" && !isLocalHost(parsed.Hostname()):
		return &InvalidURLError{URL: raw, Source: source, Reason: "must use https://, the API token would be sent unencrypted"}
	}
	return nil
}

// isLocalHost tells whether the host is this machine.
func isLocalHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// candidateFiles lists the possible config files: from the working directory up to the root, then home.
func candidateFiles(cwd, home string) []string {
	var files []string
	seen := map[string]bool{}
	add := func(dir string) {
		path := filepath.Join(dir, FileName)
		if dir != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	for dir := cwd; dir != ""; dir = parentDir(dir) {
		add(dir)
	}
	add(home)

	return files
}

// parentDir returns the parent directory or an empty string at the root.
func parentDir(dir string) string {
	parent := filepath.Dir(dir)
	if parent == dir {
		return ""
	}
	return parent
}

// normalizeURL removes surrounding whitespace and trailing slashes.
func normalizeURL(url string) string {
	return strings.TrimRight(strings.TrimSpace(url), "/")
}

// shadowWarning tells when a project file is used instead of ~/.pipetower, so that nobody runs
// commands on another instance unnoticed.
func shadowWarning(path, url, home string) []string {
	homeFile := filepath.Join(home, FileName)
	if home == "" || path == homeFile {
		return nil
	}
	if values, err := readFile(homeFile); err != nil || values[EnvToken] == "" {
		return nil
	}
	return []string{fmt.Sprintf("Using %s (%s) instead of ~/%s", path, url, FileName)}
}

// ValidateGatewayURL accepts the websocket gateway of the instance: wss, and ws only for local
// instances, so that tickets and terminal input are never sent in clear text.
func ValidateGatewayURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err == nil && (parsed.Scheme == "wss" || (parsed.Scheme == "ws" && isLocalHost(parsed.Hostname()))) {
		return nil
	}
	return fault.New(fault.Problem{
		Title:  "Insecure real-time gateway",
		Detail: fmt.Sprintf("The instance announces the gateway %q, terminal input and live output would be sent unencrypted.", raw),
		Hints:  []fault.Hint{{Label: "The instance must serve its gateway with wss:// (WSSH_GATEWAY_URL)"}},
		Code:   fault.ExitConfig,
	})
}
