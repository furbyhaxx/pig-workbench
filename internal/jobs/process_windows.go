package jobs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// groupOwner owns the direct child only: Windows has no process group to terminate.
type groupOwner struct {
	process *os.Process
}

func newGroupOwner() (*groupOwner, error) { return &groupOwner{}, nil }

func (o *groupOwner) configure(*exec.Cmd) {}

func (o *groupOwner) bind(process *os.Process) error {
	o.process = process
	return nil
}

func (o *groupOwner) signal(signal os.Signal) error {
	return errors.Join(ErrGroupKillUnsupported, o.process.Signal(signal))
}

func (o *groupOwner) terminate(time.Duration) error {
	err := o.process.Kill()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return ErrGroupKillUnsupported
}

func (o *groupOwner) gone() error { return ErrGroupKillUnsupported }

// observeExit waits on the process handle, bounded by ctx, without reaping the child.
func (o *groupOwner) observeExit(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var result uint32
		var waitErr error
		err := o.process.WithHandle(func(handle uintptr) {
			r, err := syscall.WaitForSingleObject(syscall.Handle(handle), 10)
			if err != nil {
				waitErr = err
			} else {
				result = r
			}
		})
		if err != nil {
			return err
		}
		if waitErr != nil {
			return waitErr
		}
		if result == syscall.WAIT_OBJECT_0 {
			return nil
		}
	}
}

func (o *groupOwner) abortStart() error { return o.terminate(0) }

func (o *groupOwner) release() error { return nil }

func processExit(state *os.ProcessState, duration time.Duration) Exit {
	exit := Exit{Code: -1, Duration: duration}
	if state != nil {
		exit.Code = state.ExitCode()
	}
	return exit
}
