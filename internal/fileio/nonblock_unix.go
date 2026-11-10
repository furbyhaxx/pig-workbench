//go:build unix

package fileio

import "syscall"

// NonblockFlag keeps an open of a FIFO or device from blocking.
func NonblockFlag() int { return syscall.O_NONBLOCK }

// LockFlags prevents blocking or following a swapped lock-file symlink.
func LockFlags() int {
	return syscall.O_CREAT | syscall.O_RDWR | syscall.O_NONBLOCK | syscall.O_NOFOLLOW
}
