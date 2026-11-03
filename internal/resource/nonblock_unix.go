//go:build unix

package resource

import "syscall"

func nonblockFlag() int { return syscall.O_NONBLOCK }
