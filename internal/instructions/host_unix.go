//go:build unix

package instructions

import (
	"os"
	"syscall"
)

// untrustedWriter explains why another user could have changed the file, or returns "".
func untrustedWriter(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "has no owner information"
	}
	if stat.Uid != 0 && int(stat.Uid) != os.Getuid() {
		return "is owned by neither root nor the current user"
	}
	if info.Mode().Perm()&0o022 != 0 && !(info.IsDir() && info.Mode()&os.ModeSticky != 0) {
		return "is writable by group or others"
	}
	return ""
}
