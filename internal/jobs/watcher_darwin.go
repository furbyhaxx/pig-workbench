package jobs

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func startExitWatcher(_ *groupOwner, process *os.Process) (*exitWatcher, error) {
	fd, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	var change syscall.Kevent_t
	syscall.SetKevent(&change, process.Pid, syscall.EVFILT_PROC, syscall.EV_ADD|syscall.EV_ONESHOT)
	change.Fflags = syscall.NOTE_EXIT
	if _, err := syscall.Kevent(fd, []syscall.Kevent_t{change}, nil, nil); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	// NOTE_EXIT is not replayed if the child exited before registration.
	info, err := unix.SysctlKinfoProc("kern.proc.pid", process.Pid)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}
	const zombie = 5
	return watch(func() error {
		defer syscall.Close(fd)
		if info.Proc.P_stat == zombie {
			return nil
		}
		var events [1]syscall.Kevent_t
		for {
			n, err := syscall.Kevent(fd, nil, events[:], nil)
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if err != nil {
				return err
			}
			if n == 1 && events[0].Flags&syscall.EV_ERROR != 0 {
				return syscall.Errno(events[0].Data)
			}
			if n == 1 && events[0].Fflags&syscall.NOTE_EXIT != 0 {
				return nil
			}
		}
	}), nil
}
