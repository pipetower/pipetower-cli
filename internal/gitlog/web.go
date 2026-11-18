package gitlog

import (
	"net/url"
	"regexp"
	"strings"
)

// scpAddress is the short SSH form of a remote: git@host:path.
var scpAddress = regexp.MustCompile(`^[^@/]+@([^:/]+):(.+)$`)

// comparePaths are the paths of the comparison of two commits on the hosts which are known.
var comparePaths = map[string]string{
	"github.com": "/compare/",
	"gitlab.com": "/-/compare/",
}

// WebURL turns the address of a remote into the web address of the repository, without
// credentials. It is empty for an address which is none of a host, like a local path.
func WebURL(remote string) string {
	if match := scpAddress.FindStringSubmatch(remote); match != nil {
		return webURL(match[1], match[2])
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	switch parsed.Scheme {
	case "http", "https", "ssh", "git":
		return webURL(parsed.Hostname(), parsed.Path)
	}
	return ""
}

// webURL builds the web address of a repository path on a host.
func webURL(host, path string) string {
	return "https://" + host + "/" + strings.TrimSuffix(strings.Trim(path, "/"), ".git")
}

// CompareURL is the web page which compares two commits of the repository, empty if the host
// of the repository is not known for one.
func CompareURL(repository, previous, commit string) string {
	parsed, err := url.Parse(repository)
	if err != nil || repository == "" || previous == "" || previous == commit {
		return ""
	}
	path, ok := comparePaths[parsed.Hostname()]
	if !ok {
		return ""
	}
	return repository + path + previous + "..." + commit
}
