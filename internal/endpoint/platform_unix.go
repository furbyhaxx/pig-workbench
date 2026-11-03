//go:build unix

package endpoint

import (
	"errors"
	"os"
	"syscall"
)

func platformSupport() error { return nil }

func checkPrivate(path string, info os.FileInfo, allowed os.FileMode) error {
	mode := info.Mode()
	if mode.Perm() & ^allowed != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return &PermissionError{Path: path, Mode: mode, Reason: "permissions are too wide"}
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return &PermissionError{Path: path, Mode: mode, Reason: "ownership cannot be verified for the current user"}
	}
	return nil
}

func pidAlive(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return false, err
	}
}

func syncDirectory(path string) error {
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
