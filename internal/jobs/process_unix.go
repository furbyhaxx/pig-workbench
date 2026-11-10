//go:build unix && !linux

package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// zombieState is the Darwin kstat process state for a terminated, unreaped child.
const zombieState = 5

// groupOwner reaches the group of the original leader through its numeric id. That id is
// stable while the leader is unreaped and pins the group, but a check after the reap cannot
// tell a recycled group apart: group disappearance is reported, not guaranteed.
type groupOwner struct {
	process *os.Process
}

func newGroupOwner() (*groupOwner, error) { return &groupOwner{}, nil }

func (o *groupOwner) configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func (o *groupOwner) bind(process *os.Process) error {
	o.process = process
	return nil
}

func (o *groupOwner) signal(sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return fmt.Errorf("unsupported process signal %v", sig)
	}
	if err := syscall.Kill(-o.process.Pid, s); err != nil {
		return fmt.Errorf("signal process group: %w", err)
	}
	return nil
}

func (o *groupOwner) terminate(grace time.Duration) error {
	err := syscall.Kill(-o.process.Pid, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("terminate process group: %w", err)
	}
	if grace > 0 {
		timer := time.NewTimer(grace)
		<-timer.C
		timer.Stop()
	}
	if err := syscall.Kill(-o.process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}

func (o *groupOwner) gone() error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(-o.process.Pid, 0)
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

// observeExit re-checks the leader directly, because the exit watcher owns the platform
// notification and has already failed here.
func (o *groupOwner) observeExit(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := unix.SysctlKinfoProc("kern.proc.pid", o.process.Pid)
		// A missing process was reaped elsewhere; a zombie has exited.
		if err != nil || info.Proc.P_stat == zombieState {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (o *groupOwner) abortStart() error { return o.terminate(0) }

func (o *groupOwner) release() error { return nil }
