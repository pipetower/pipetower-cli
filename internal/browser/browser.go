// Package browser opens web pages in the user's browser.
package browser

import (
	"os"
	"os/exec"
	"runtime"
)

// Open opens the URL with the browser of $BROWSER or the default browser of the system.
func Open(url string) error {
	name, args := command(url, os.Getenv("BROWSER"), runtime.GOOS)
	return exec.Command(name, args...).Start()
}

// command returns the program and arguments which open the URL.
func command(url, browser, goos string) (string, []string) {
	switch {
	case browser != "":
		return browser, []string{url}
	case goos == "darwin":
		return "open", []string{url}
	case goos == "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
