//go:build unix

package jobs

import (
	"os"
	"syscall"
	"time"
)

func processExit(state *os.ProcessState, duration time.Duration) Exit {
	exit := Exit{Code: -1, Duration: duration}
	if state == nil {
		return exit
	}
	exit.Code = state.ExitCode()
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		exit.Signaled = true
		exit.Signal = status.Signal().String()
	}
	return exit
}
