package jobs

import (
	"fmt"
	"os"
)

// Write sends bytes to the stdin pipe of a retained live job.
func (m *Manager) Write(id string, data []byte) (int, error) {
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
