package jobs

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func startExitWatcher(owner *groupOwner, _ *os.Process) (*exitWatcher, error) {
	fd, err := owner.pollFD()
	if err != nil {
		return nil, err
	}
	return watch(func() error {
		// The descriptor outlives this observer: release waits for the watcher, and the
		// watcher ends before any reap. A finished child stays readable, so a reap that
		// happened elsewhere still ends this observation.
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		for {
			n, err := unix.Poll(fds, -1)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			// Only an exit event is evidence: POLLNVAL/POLLERR describe the descriptor.
			if fds[0].Revents&unix.POLLIN != 0 {
				return nil
			}
			return fmt.Errorf("process identity descriptor reported revents %#x without exit evidence", fds[0].Revents)
		}
	}), nil
}
