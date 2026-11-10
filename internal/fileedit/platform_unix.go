//go:build unix

package fileedit

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// checkLockDir requires a directory the current user owns and nobody else can
// enter, so another account cannot pre-create or replace lock files.
func checkLockDir(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("edit lock directory %s must not be group or world accessible", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("edit lock directory %s is not owned by the current user", path)
	}
	return nil
}
