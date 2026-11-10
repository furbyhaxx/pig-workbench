package jobs

import (
	"errors"
	"fmt"
	"os"
)

// MaxInput bounds one stdin write.
const MaxInput = 64 << 10

// ErrInputTooLarge reports a stdin write beyond MaxInput; nothing was sent.
var ErrInputTooLarge = errors.New("stdin input exceeds 64 KiB; send it in smaller writes")

// Write sends bytes to the stdin pipe of a retained live job.
func (m *Manager) Write(id string, data []byte) (int, error) {
	if len(data) > MaxInput {
		return 0, ErrInputTooLarge
	}
	job, err := m.job(id)
	if err != nil {
		return 0, err
	}
	if job.process == nil {
		return 0, fmt.Errorf("%w: %s", ErrFailedToStart, job.state.Error)
	}
	return job.process.Write(data)
}

// EOF closes a retained job's stdin without sending a process signal.
func (m *Manager) EOF(id string) error {
	job, err := m.job(id)
	if err != nil {
		return err
	}
	if job.process == nil {
		return fmt.Errorf("%w: %s", ErrFailedToStart, job.state.Error)
	}
	return job.process.CloseStdin()
}

// Signal forwards a process-group signal through the retained identity guard.
func (m *Manager) Signal(id string, signal os.Signal) error {
	job, err := m.job(id)
	if err != nil {
		return err
	}
	if job.process == nil {
		return fmt.Errorf("%w: %s", ErrFailedToStart, job.state.Error)
	}
	return job.process.Signal(signal)
}
