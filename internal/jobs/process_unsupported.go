//go:build !unix && !windows

package jobs

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// groupOwner refuses every group operation: startup already fails without an exit watcher.
type groupOwner struct {
	process *os.Process
}

func newGroupOwner() (*groupOwner, error) { return &groupOwner{}, nil }

func (o *groupOwner) configure(*exec.Cmd) {}

func (o *groupOwner) bind(process *os.Process) error {
	o.process = process
	return nil
}

func (o *groupOwner) signal(os.Signal) error { return ErrGroupKillUnsupported }

func (o *groupOwner) terminate(time.Duration) error { return ErrGroupKillUnsupported }

func (o *groupOwner) gone() error { return ErrGroupKillUnsupported }

func (o *groupOwner) observeExit(context.Context) error { return ErrGroupKillUnsupported }

func (o *groupOwner) abortStart() error { return ErrGroupKillUnsupported }

func (o *groupOwner) release() error { return nil }

func processExit(state *os.ProcessState, duration time.Duration) Exit {
	exit := Exit{Code: -1, Duration: duration}
	if state != nil {
		exit.Code = state.ExitCode()
	}
	return exit
}
