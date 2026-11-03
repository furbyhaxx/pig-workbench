//go:build unix

package instructions

import "syscall"

func nonblockFlag() int { return syscall.O_NONBLOCK }
