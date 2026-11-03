//go:build unix

package prompt

import "syscall"

func nonblockFlag() int { return syscall.O_NONBLOCK }
