//go:build unix

package host

import (
	"os/exec"
	"syscall"
)

// detachTerminal starts cmd without the session's controlling terminal, so it cannot take the
// terminal foreground and get the host suspended.
func detachTerminal(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
