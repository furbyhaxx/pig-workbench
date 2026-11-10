//go:build unix

package fileio

import (
	"errors"
	"os"
	"syscall"
)

// SyncDirectory makes a rename or removal inside path durable; filesystems that cannot sync a directory succeed.
func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	err = directory.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return err
}
