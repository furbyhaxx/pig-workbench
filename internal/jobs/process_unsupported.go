//go:build !unix && !windows

package jobs

import (
	"os"
	"os/exec"
	"time"
)

func configureProcess(*exec.Cmd) {}

func signalProcess(*os.Process, os.Signal) error { return ErrGroupKillUnsupported }

func terminateProcess(*os.Process, time.Duration) error { return ErrGroupKillUnsupported }

func waitProcessGroupGone(*os.Process) error { return ErrGroupKillUnsupported }

func processExit(state *os.ProcessState, duration time.Duration) Exit {
	exit := Exit{Code: -1, Duration: duration}
	if state != nil {
		exit.Code = state.ExitCode()
	}
	return exit
}
