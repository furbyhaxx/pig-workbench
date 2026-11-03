package jobs

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

func configureProcess(*exec.Cmd) {}

func signalProcess(process *os.Process, signal os.Signal) error {
	return errors.Join(ErrGroupKillUnsupported, process.Signal(signal))
}

func terminateProcess(process *os.Process, _ time.Duration) error {
	err := process.Kill()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return ErrGroupKillUnsupported
}

func waitProcessGroupGone(*os.Process) error { return ErrGroupKillUnsupported }

func processExit(state *os.ProcessState, duration time.Duration) Exit {
	exit := Exit{Code: -1, Duration: duration}
	if state != nil {
		exit.Code = state.ExitCode()
	}
	return exit
}
