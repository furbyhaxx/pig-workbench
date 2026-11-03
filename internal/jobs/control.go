package jobs

import "os"

// Write sends bytes to the stdin pipe of a retained live job.
func (m *Manager) Write(id string, data []byte) (int, error) {
	job, err := m.job(id)
	if err != nil {
		return 0, err
	}
	return job.process.Write(data)
}

// EOF closes a retained job's stdin without sending a process signal.
func (m *Manager) EOF(id string) error {
	job, err := m.job(id)
	if err != nil {
		return err
	}
	return job.process.CloseStdin()
}

// Signal forwards a process-group signal through the retained identity guard.
func (m *Manager) Signal(id string, signal os.Signal) error {
	job, err := m.job(id)
	if err != nil {
		return err
	}
	return job.process.Signal(signal)
}
