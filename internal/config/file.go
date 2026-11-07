package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// file is the content of a .pipetower file: the environments, the active one and the team, all
// optional. Other keys are kept as they are when the file is written.
type file struct {
	Path         string
	Environment  string
	Team         string
	Environments map[string]Environment
	raw          map[string]json.RawMessage
}

// readFile parses a .pipetower file (JSON).
func readFile(path string) (*file, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	f := &file{Path: path, raw: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(content)) == 0 {
		return f, nil
	}
	if err := json.Unmarshal(content, &f.raw); err != nil {
		return nil, fmt.Errorf("not valid JSON (%s)", jsonError(err))
	}
	if err := decode(f.raw, "environment", &f.Environment); err != nil {
		return nil, err
	}
	if err := decode(f.raw, "team", &f.Team); err != nil {
		return nil, err
	}
	if err := decode(f.raw, "environments", &f.Environments); err != nil {
		return nil, err
	}
	return f, nil
}

// decode reads one key of the file, if present.
func decode(raw map[string]json.RawMessage, key string, into any) error {
	value, ok := raw[key]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(value, into); err != nil {
		return fmt.Errorf("%q has the wrong type (%s)", key, jsonError(err))
	}
	return nil
}

// jsonError shortens a JSON error to its reason.
func jsonError(err error) string {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf("%s at byte %d", syntax.Error(), syntax.Offset)
	}
	return err.Error()
}

// write saves the file with the given keys changed, other keys kept, readable by the user only.
func (f *file) write(changes map[string]any) error {
	for key, value := range changes {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		f.raw[key] = encoded
	}
	content, err := json.MarshalIndent(f.raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, append(content, '\n'), 0o600)
}

// AddEnvironment adds an environment to the file, which is created if it doesn't exist. The
// first environment of a file becomes the active one. An environment of that name must not exist.
func AddEnvironment(path, name string, env Environment) (activated bool, err error) {
	f, err := readFile(path)
	if os.IsNotExist(err) {
		f, err = &file{Path: path, raw: map[string]json.RawMessage{}}, nil
	}
	if err != nil {
		return false, err
	}
	if _, exists := f.Environments[name]; exists {
		return false, &ExistingEnvironmentError{Name: name, Path: path}
	}
	if err := validateURL(env.URL, Source{Path: path}); err != nil {
		return false, err
	}
	if f.Environments == nil {
		f.Environments = map[string]Environment{}
	}
	f.Environments[name] = env

	changes := map[string]any{"environments": f.Environments}
	if activated = f.Environment == ""; activated {
		changes["environment"] = name
	}
	return activated, f.write(changes)
}

// SetEnvironment makes the named environment of the file the active one.
func SetEnvironment(path, name string) error {
	f, err := readFile(path)
	if err != nil {
		return err
	}
	if _, ok := f.Environments[name]; !ok {
		return &UnknownEnvironmentError{Name: name, Names: environmentNames(f.Environments), Path: path}
	}
	return f.write(map[string]any{"environment": name})
}

// SetTeam limits the named environment of the file to the team, or to no team (all teams) with "".
func SetTeam(path, environment, team string) error {
	f, err := readFile(path)
	if err != nil {
		return err
	}
	active, ok := f.Environments[environment]
	if !ok {
		return &UnknownEnvironmentError{Name: environment, Names: environmentNames(f.Environments), Path: path}
	}
	active.Team = team
	f.Environments[environment] = active
	return f.write(map[string]any{"environments": f.Environments})
}

// environmentNames lists the environments in a stable order.
func environmentNames(environments map[string]Environment) []string {
	return (&Config{Environments: environments}).EnvironmentNames()
}

// fileWarnings checks a used config file for loose permissions and the risk of committing it.
func fileWarnings(path string, env Env) []string {
	var warnings []string

	if info, err := os.Stat(path); err == nil && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		warnings = append(warnings, fmt.Sprintf("%s is readable by other users, restrict it with: chmod 600 %s", path, path))
	}

	if env.GitIgnored != nil {
		if inRepo, ignored := env.GitIgnored(path); inRepo && !ignored {
			warnings = append(warnings, fmt.Sprintf("%s is inside a git repository and not ignored, add it to .gitignore so the token is never committed", path))
		}
	}

	return warnings
}

// gitIgnored asks git whether the file is inside a repository and ignored there.
func gitIgnored(path string) (bool, bool) {
	// no fsmonitor: a planted repository config must not run programs
	cmd := exec.Command("git", "-c", "core.fsmonitor=false", "-C", filepath.Dir(path), "check-ignore", "-q", filepath.Base(path))
	err := cmd.Run()

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return true, true
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return true, false
	default:
		return false, false
	}
}
