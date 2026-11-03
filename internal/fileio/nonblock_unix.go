//go:build unix

package fileio

import "syscall"

func nonblockFlag() int { return syscall.O_NONBLOCK }

// LockFlags prevents blocking or following a swapped lock-file symlink.
func LockFlags() int {
	return syscall.O_CREAT | syscall.O_RDWR | syscall.O_NONBLOCK | syscall.O_NOFOLLOW
}
