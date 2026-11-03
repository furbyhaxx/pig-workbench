//go:build unix

package jobs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalProcess(process *os.Process, signal os.Signal) error {
	sig, ok := signal.(syscall.Signal)
	if !ok {
		return fmt.Errorf("unsupported process signal %v", signal)
	}
	if err := syscall.Kill(-process.Pid, sig); err != nil {
		return fmt.Errorf("signal process group: %w", err)
	}
	return nil
}

func terminateProcess(process *os.Process, grace time.Duration) error {
	err := syscall.Kill(-process.Pid, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("terminate process group: %w", err)
	}
	if grace > 0 {
		timer := time.NewTimer(grace)
		<-timer.C
	}
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}

func waitProcessGroupGone(process *os.Process) error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(-process.Pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("check process group: %w", err)
		}
		select {
		case <-deadline.C:
			return ErrGroupNotGone
		case <-ticker.C:
		}
	}
}

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
