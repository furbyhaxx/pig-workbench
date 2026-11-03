//go:build !linux && !darwin && !windows

package jobs

import "os"

func startExitWatcher(*os.Process) (*exitWatcher, error) {
	return nil, ErrWatcherUnsupported
}
