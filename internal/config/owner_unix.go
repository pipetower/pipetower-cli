//go:build !windows

package config

import (
	"os"
	"syscall"
)

// ownedByCurrentUser tells whether the file belongs to the user running pt.
func ownedByCurrentUser(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || int(stat.Uid) == os.Getuid()
}
