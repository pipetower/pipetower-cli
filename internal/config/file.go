package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// readFile parses a KEY=VALUE file. Blank lines, comments and an "export " prefix are allowed.
func readFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if key, value, ok := parseLine(scanner.Text()); ok {
			values[key] = value
		}
	}

	return values, scanner.Err()
}

// parseLine parses one KEY=VALUE line.
func parseLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	key, value, found := strings.Cut(strings.TrimPrefix(line, "export "), "=")
	if !found {
		return "", "", false
	}

	return strings.TrimSpace(key), unquote(strings.TrimSpace(value)), true
}

// unquote removes matching single or double quotes around a value.
func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
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
