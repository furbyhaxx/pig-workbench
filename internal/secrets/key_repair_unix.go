//go:build unix

package secrets

import (
	"fmt"
	"os"
	"syscall"
)

// narrowKeyMode chmods a key file the current user owns to 0600; anything else is refused.
func narrowKeyMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("secret key %q is not a regular file owned by this user", path)
	}
	return os.Chmod(path, 0600)
}
