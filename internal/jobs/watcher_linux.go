package jobs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func startExitWatcher(process *os.Process) (*exitWatcher, error) {
	return watch(func() error {
		var info unix.Siginfo
		for {
			err := unix.Waitid(unix.P_PID, process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
			if !errors.Is(err, unix.EINTR) {
				return err
			}
		}
	}), nil
}
