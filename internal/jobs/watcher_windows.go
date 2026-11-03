package jobs

import (
	"fmt"
	"os"
	"syscall"
)

func startExitWatcher(_ *groupOwner, process *os.Process) (*exitWatcher, error) {
	return watch(func() error {
		var waitErr error
		err := process.WithHandle(func(handle uintptr) {
			result, err := syscall.WaitForSingleObject(syscall.Handle(handle), syscall.INFINITE)
			if err != nil {
				waitErr = err
			} else if result != syscall.WAIT_OBJECT_0 {
				waitErr = fmt.Errorf("unexpected process wait result: %d", result)
			}
		})
		if err != nil {
			return err
		}
		return waitErr
	}), nil
}
