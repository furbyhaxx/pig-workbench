//go:build unix

package project

import (
	"os"
	"syscall"
)

func registryPlatformSupport() error { return nil }

func checkRegistryPrivate(path string, info os.FileInfo, allowed os.FileMode) error {
	mode := info.Mode()
	if mode.Perm() & ^allowed != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return &Error{Path: path, Reason: "permissions are too wide"}
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return &Error{Path: path, Reason: "ownership cannot be verified for the current user"}
	}
	return nil
}
