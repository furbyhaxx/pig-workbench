//go:build !windows

package sessionstore

import (
	"errors"
	"os"
	"syscall"
)

// device identifies the filesystem holding path, so conversion defers across filesystems.
func device(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("filesystem identity is unavailable on this platform")
	}
	return uint64(stat.Dev), nil
}
