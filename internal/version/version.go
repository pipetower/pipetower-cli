// Package version holds the build information injected at link time.
package version

// Set via -ldflags "-X github.com/pipetower/pipetower-cli/internal/version.Version=..."
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// UserAgent is the HTTP user agent sent with every request.
func UserAgent() string {
	return "pt/" + Version
}
