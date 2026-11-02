// Package version holds the build information injected at link time.
package version

import "golang.org/x/mod/semver"

// Set via -ldflags "-X github.com/pipetower/pipetower-cli/internal/version.Version=..."
//
// Releases carry the semantic version of their tag without the v (1.4.0, 1.5.0-rc.1), local
// builds dev or dev-<git describe>.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// UserAgent is the HTTP user agent sent with every request.
func UserAgent() string {
	return "pt/" + Version
}

// IsRelease tells whether this build is a published release, which can be updated.
func IsRelease() bool {
	return Valid(Version)
}

// Valid tells whether a version is a semantic version without the v: 1.4.0 or 1.5.0-rc.1.
func Valid(v string) bool {
	// canonical rejects shorthands like 1.4 and build metadata
	return v != "" && v[0] != 'v' && semver.Canonical("v"+v) == "v"+v
}

// Newer tells whether version a is newer than version b, both valid.
func Newer(a, b string) bool {
	return semver.Compare("v"+a, "v"+b) > 0
}
