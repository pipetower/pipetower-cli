// Package config discovers the instance URL, the API token and the team scope of the CLI.
//
// They live in a .pipetower file (JSON) with named environments, e.g. a local and a production
// instance, of which one is active:
//
//	{
//	  "environment": "prod",
//	  "environments": {
//	    "local": {"url": "http://pipetower.localhost", "token": "…", "team": "Ops"},
//	    "prod":  {"url": "https://app.pipetower.com", "token": "…"}
//	  }
//	}
//
// The environments are taken from the first file which has some: in the working directory, one of
// its parents or ~/.pipetower. URL and token of an environment always come from the same file, so
// that a file of a cloned repository can never redirect a token to another server. $PIPETOWER_TOKEN
// and $PIPETOWER_URL, which the user sets, replace them (e.g. in CI).
//
// Which environment is active comes from $PIPETOWER_ENV, or else from the nearest file which names
// one: a project's .pipetower may hold only {"environment": "local"} and use the environments of
// ~/.pipetower. The team which limits all commands comes from $PIPETOWER_TEAM, else from the nearest
// file with a "team", else from the active environment. A team or an environment name can't send
// the token anywhere, they only choose among what the trusted file holds.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// Names of the configuration file and the environment variables.
const (
	FileName = ".pipetower"
	EnvURL   = "PIPETOWER_URL"
	EnvToken = "PIPETOWER_TOKEN"
	EnvTeam  = "PIPETOWER_TEAM"
	EnvName  = "PIPETOWER_ENV"
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

// Environment is one configured Pipetower instance.
type Environment struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	// Team (name or id) limits the resources when the environment is active, empty for all teams.
	Team string `json:"team,omitempty"`
}

// Config is the resolved configuration.
type Config struct {
	URL         string
	Token       string
	URLSource   Source
	TokenSource Source
	// Environment is the name of the active environment, empty when URL and token come from $PIPETOWER_*.
	Environment       string
	EnvironmentSource Source
	// Environments are all configured environments by name, from the file at EnvironmentsPath.
	Environments     map[string]Environment
	EnvironmentsPath string
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
	files := cfg.readFiles()
	cfg.loadEnvironments(files, env)

	if token := env.Getenv(EnvToken); token != "" {
		cfg.Token, cfg.TokenSource = token, Source{Env: EnvToken}
		if url := env.Getenv(EnvURL); url != "" {
			cfg.URL, cfg.URLSource = normalizeURL(url), Source{Env: EnvURL}
		}
	} else if err := cfg.activate(files, env); err != nil {
		return cfg, err
	}
	cfg.loadTeam(files, env)

	return cfg, cfg.validate(env.Home)
}

// EnvironmentNames lists the configured environments in a stable order.
func (c *Config) EnvironmentNames() []string {
	names := make([]string, 0, len(c.Environments))
	for name := range c.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// readFiles reads the candidate files which exist and belong to the current user, nearest first.
// Only a file which can't be parsed is worth a warning, a missing or unreadable one is just skipped.
func (c *Config) readFiles() []*file {
	var files []*file
	for _, path := range c.Searched {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		f, err := readFile(path)
		if err != nil {
			if !os.IsPermission(err) {
				c.Warnings = append(c.Warnings, fmt.Sprintf("Ignored %s: %s", path, err))
			}
			continue
		}
		if !ownedByCurrentUser(path) {
			c.Warnings = append(c.Warnings, fmt.Sprintf("Ignored %s, it belongs to another user", path))
			continue
		}
		files = append(files, f)
	}
	return files
}

// loadEnvironments takes the environments from the nearest file which has some.
func (c *Config) loadEnvironments(files []*file, env Env) {
	for _, f := range files {
		if len(f.Environments) == 0 {
			continue
		}
		c.Environments, c.EnvironmentsPath = f.Environments, f.Path
		c.Warnings = append(c.Warnings, fileWarnings(f.Path, env)...)
		c.Warnings = append(c.Warnings, shadowWarning(f.Path, env.Home)...)
		return
	}
}

// activate chooses the active environment and takes URL and token from it: named by $PIPETOWER_ENV
// or the nearest file, or the only one there is.
func (c *Config) activate(files []*file, env Env) error {
	if len(c.Environments) == 0 {
		return &MissingError{Key: EnvToken, Searched: c.Searched, URL: normalizeURL(env.Getenv(EnvURL)), Home: env.Home}
	}

	// a nearer file may name the environment, a farther one names its own environments, not these
	name, source := strings.TrimSpace(env.Getenv(EnvName)), Source{Env: EnvName}
	for _, f := range files {
		if name == "" && f.Environment != "" {
			name, source = f.Environment, Source{Path: f.Path}
		}
		if f.Path == c.EnvironmentsPath {
			break
		}
	}
	if name == "" && len(c.Environments) == 1 {
		name, source = c.EnvironmentNames()[0], Source{Path: c.EnvironmentsPath}
	}
	if name == "" {
		return &NoEnvironmentError{Names: c.EnvironmentNames(), Path: c.EnvironmentsPath}
	}
	active, known := c.Environments[name]
	if !known {
		return &UnknownEnvironmentError{Name: name, Source: source, Names: c.EnvironmentNames(), Path: c.EnvironmentsPath}
	}

	fileSource := Source{Path: c.EnvironmentsPath}
	c.Environment, c.EnvironmentSource = name, source
	c.Token, c.TokenSource = active.Token, fileSource
	c.URL, c.URLSource = normalizeURL(active.URL), fileSource
	if url := env.Getenv(EnvURL); url != "" {
		c.URL, c.URLSource = normalizeURL(url), Source{Env: EnvURL}
	}
	return nil
}

// loadTeam takes the team from $PIPETOWER_TEAM, the nearest file with a team, or the active environment.
func (c *Config) loadTeam(files []*file, env Env) {
	if team := strings.TrimSpace(env.Getenv(EnvTeam)); team != "" {
		c.Team, c.TeamSource = team, Source{Env: EnvTeam}
		return
	}
	for _, f := range files {
		if f.Team != "" {
			c.Team, c.TeamSource = f.Team, Source{Path: f.Path}
			return
		}
	}
	if active, ok := c.Environments[c.Environment]; ok && active.Team != "" {
		c.Team, c.TeamSource = active.Team, Source{Path: c.EnvironmentsPath}
	}
}

// validate returns an explaining error if URL or token is missing or invalid.
func (c *Config) validate(home string) error {
	switch {
	case c.Token == "":
		return &MissingError{Key: EnvToken, Searched: c.Searched, URL: c.URL, Home: home, Environment: c.Environment, Path: c.EnvironmentsPath}
	case c.URL == "":
		return &MissingError{Key: EnvURL, Searched: c.Searched, Home: home, TokenSource: c.TokenSource, Environment: c.Environment, Path: c.EnvironmentsPath}
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

// shadowWarning tells when the environments of a project file are used instead of those of
// ~/.pipetower, so that nobody runs commands on another instance unnoticed.
func shadowWarning(path, home string) []string {
	homeFile := filepath.Join(home, FileName)
	if home == "" || path == homeFile {
		return nil
	}
	if f, err := readFile(homeFile); err != nil || len(f.Environments) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("Using the environments of %s instead of ~/%s", path, FileName)}
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
