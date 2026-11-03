package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

const (
	defaultLogBytes        = 10 * 1024 * 1024
	defaultSessionLogBytes = 100 * 1024 * 1024
	defaultGrace           = 2 * time.Second
	defaultMaxActive       = 32
)

// JobNotFoundError means the manager has no job with the requested ID.
type JobNotFoundError struct{}

// Error describes the missing job.
func (*JobNotFoundError) Error() string { return "job not found" }

// ErrJobNotFound identifies a missing job.
var ErrJobNotFound = &JobNotFoundError{}

// ErrManagerClosed means admission is closed.
var ErrManagerClosed = errors.New("job manager is closed")

// ErrJobLimit means the concurrent process limit has been reached.
var ErrJobLimit = errors.New("active job limit reached")

// ErrJobCleanupIncomplete means child termination could not be established.
var ErrJobCleanupIncomplete = errors.New("job cleanup is incomplete; child or descendants may still be running")

// ManagerOptions configures output limits and background process ownership.
// Zero fields use the defaults: 10 MiB, 100 MiB, two seconds, and 32 jobs.
type ManagerOptions struct {
	LogBytes        int64
	SessionLogBytes int64
	Grace           time.Duration
	MaxActive       int
}

// JobState is a snapshot of a managed job and its terminal facts.
// Running remains true when termination could not be proven, because no leader status
// was obtained and descendants may still run.
// Cancelled records whether cancellation or shutdown won before observed exit.
type JobState struct {
	ID        string
	Running   bool
	Cancelled bool
	Exit      *Exit
	Error     string
}

type managedJob struct {
	process       *Process
	state         JobState
	done          chan struct{}
	stop          chan struct{}
	selectionMu   sync.Mutex
	selected      bool
	stopRequested bool
	err           error
}

func (j *managedJob) requestStop() {
	j.selectionMu.Lock()
	defer j.selectionMu.Unlock()
	if j.selected || j.stopRequested {
		return
	}
	select {
	case <-j.process.Exited():
		return
	default:
	}
	j.stopRequested = true
	close(j.stop)
}

// Manager owns background jobs and their logs.
type Manager struct {
	mu              sync.Mutex
	logs            *LogStore
	grace           time.Duration
	maxActive       int
	active          int
	closed          bool
	jobs            map[string]*managedJob
	issued          map[string]struct{}
	owned           sync.WaitGroup
	admissionErrors []error
	unresolved      []*Process
	closeDone       chan struct{}
	closeErr        error
}

// NewManager creates a background supervisor with independently owned logs.
func NewManager(options ManagerOptions) (*Manager, error) {
	if options.LogBytes < 0 || options.SessionLogBytes < 0 || options.Grace < 0 || options.MaxActive < 0 {
		return nil, errors.New("job manager options must not be negative")
	}
	if options.LogBytes == 0 {
		options.LogBytes = defaultLogBytes
	}
	if options.SessionLogBytes == 0 {
		options.SessionLogBytes = defaultSessionLogBytes
	}
	if options.Grace == 0 {
		options.Grace = defaultGrace
	}
	if options.MaxActive == 0 {
		options.MaxActive = defaultMaxActive
	}
	logs, err := NewLogStore(options.LogBytes, options.SessionLogBytes)
	if err != nil {
		return nil, err
	}
	return &Manager{
		logs:      logs,
		grace:     options.Grace,
		maxActive: options.MaxActive,
		jobs:      make(map[string]*managedJob),
		issued:    make(map[string]struct{}),
		closeDone: make(chan struct{}),
	}, nil
}

type managerLogWriter struct {
	logs *LogStore
	id   string
}

func (w managerLogWriter) Write(data []byte) (int, error) {
	if err := w.logs.Append(w.id, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Start admits a background job and returns its opaque ID after startup.
// Context cancellation only affects admission; it does not cancel an admitted job.
// The job's output goes exclusively to the manager's log store.
func (m *Manager) Start(ctx context.Context, spec Spec) (string, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", ErrManagerClosed
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return "", err
	}
	if m.active >= m.maxActive {
		m.mu.Unlock()
		return "", ErrJobLimit
	}
	m.mu.Unlock()
	var id string
	for {
		var raw [16]byte
		if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
			return "", fmt.Errorf("generate job ID: %w", err)
		}
		id = hex.EncodeToString(raw[:])
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return "", ErrManagerClosed
		}
		if err := ctx.Err(); err != nil {
			m.mu.Unlock()
			return "", err
		}
		if m.active >= m.maxActive {
			m.mu.Unlock()
			return "", ErrJobLimit
		}
		if _, exists := m.issued[id]; exists {
			m.mu.Unlock()
			continue
		}
		m.issued[id] = struct{}{}
		m.active++
		m.owned.Add(1)
		m.mu.Unlock()
		break
	}
	admitted := false
	unresolved := false
	defer func() {
		if !admitted {
			if !unresolved {
				m.mu.Lock()
				m.active--
				m.mu.Unlock()
			}
			m.owned.Done()
		}
	}()

	spec.Output = managerLogWriter{logs: m.logs, id: id}
	process, err := StartProcess(spec)
	if err != nil {
		return "", err
	}
	if err := m.logs.Append(id, nil); err != nil {
		var cleanupErr error
		cleanupErr, unresolved = m.cleanupAdmission(process, id, err)
		return "", cleanupErr
	}
	m.mu.Lock()
	var admissionErr error
	if m.closed {
		admissionErr = ErrManagerClosed
	} else {
		admissionErr = ctx.Err()
	}
	if admissionErr != nil {
		m.mu.Unlock()
		var cleanupErr error
		cleanupErr, unresolved = m.cleanupAdmission(process, id, admissionErr)
		return "", cleanupErr
	}
	job := &managedJob{process: process, state: JobState{ID: id, Running: true}, done: make(chan struct{}), stop: make(chan struct{})}
	m.jobs[id] = job
	admitted = true
	go m.supervise(job)
	m.mu.Unlock()
	return id, nil
}

func (m *Manager) cleanupAdmission(process *Process, id string, cause error) (error, bool) {
	exit, cleanupErr := m.finishProcess(process, 0)
	unresolved := exit == nil
	logErr := m.logs.Drop(id)
	if errors.Is(logErr, ErrLogNotFound) {
		logErr = nil
	}
	cleanupErr = errors.Join(cleanupErr, logErr)
	if cleanupErr != nil || unresolved {
		m.mu.Lock()
		if cleanupErr != nil {
			m.admissionErrors = append(m.admissionErrors, cleanupErr)
		}
		if unresolved {
			m.unresolved = append(m.unresolved, process)
		}
		m.mu.Unlock()
	}
	return errors.Join(cause, cleanupErr), unresolved
}

func (m *Manager) supervise(job *managedJob) {
	defer m.owned.Done()
	select {
	case <-job.stop:
	case <-job.process.watcher.done:
	}
	job.selectionMu.Lock()
	grace := time.Duration(0)
	cancelled := job.stopRequested
	if cancelled {
		grace = m.grace
	}
	job.selected = true
	job.selectionMu.Unlock()
	exit, resultErr := m.finishProcess(job.process, grace)
	if exit == nil {
		m.finishUnknown(job, cancelled, resultErr)
		return
	}
	m.mu.Lock()
	job.state.Running = false
	job.state.Cancelled = cancelled
	job.state.Exit = exit
	if resultErr != nil {
		job.state.Error = resultErr.Error()
	}
	job.err = resultErr
	m.active--
	close(job.done)
	m.mu.Unlock()
}

// finishProcess terminates the group and reaps the leader. A failed exit observation is
// reported next to the result instead of vetoing cleanup, which the group identity still
// permits. Exit is nil, and termination stays unproven, unless this reap produced a real
// leader status.
func (m *Manager) finishProcess(process *Process, grace time.Duration) (*Exit, error) {
	killErr := process.Kill(grace)
	closeErr := process.Close()
	// Read after Close, which joins the watcher when the leader was reaped here.
	watchErr := process.watcher.failure()
	resultErr := errors.Join(watchErr, killErr, closeErr)
	exit, known := process.terminalExit()
	if !known {
		return nil, errors.Join(resultErr, m.closeUnobserved(process, ErrJobCleanupIncomplete))
	}
	return exit, resultErr
}

func (m *Manager) closeUnobserved(process *Process, cause error) error {
	stdinErr := process.CloseStdin()
	outputErr := process.output.Close()
	if errors.Is(outputErr, os.ErrClosed) {
		outputErr = nil
	}
	var drainErr error
	timer := time.NewTimer(time.Second)
	select {
	case <-process.outputDone:
		drainErr = process.outputErr
		if errors.Is(drainErr, os.ErrClosed) {
			drainErr = nil
		}
	case <-timer.C:
		drainErr = errors.New("process output drain did not stop")
	}
	timer.Stop()
	return errors.Join(cause, ErrJobCleanupIncomplete, stdinErr, outputErr, drainErr)
}

func (m *Manager) finishUnknown(job *managedJob, cancelled bool, err error) {
	m.mu.Lock()
	job.state.Cancelled = cancelled
	job.state.Error = err.Error()
	job.err = err
	close(job.done)
	m.mu.Unlock()
}

func (m *Manager) job(id string) (*managedJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	if job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	return job, nil
}

func cloneJobState(state JobState) JobState {
	if state.Exit != nil {
		exit := *state.Exit
		state.Exit = &exit
	}
	return state
}

// Status returns an independent snapshot of a job.
func (m *Manager) Status(id string) (JobState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	if job == nil {
		return JobState{}, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	return cloneJobState(job.state), nil
}

// Wait returns the supervision result; cancelling ctx does not affect the job.
// A process whose leader status stayed unavailable returns a state whose Running field
// remains true.
func (m *Manager) Wait(ctx context.Context, id string) (JobState, error) {
	job, err := m.job(id)
	if err != nil {
		return JobState{}, err
	}
	if err := ctx.Err(); err != nil {
		return JobState{}, err
	}
	select {
	case <-ctx.Done():
		return JobState{}, ctx.Err()
	case <-job.done:
		return m.Status(id)
	}
}

// Output reads a page from the managed job's bounded log.
func (m *Manager) Output(id string, cursor int64, max int) ([]byte, int64, bool, int64, error) {
	if _, err := m.job(id); err != nil {
		return nil, 0, false, 0, err
	}
	return m.logs.Read(id, cursor, max)
}

// Cancel requests group termination and waits for the supervision result.
func (m *Manager) Cancel(id string) error {
	job, err := m.job(id)
	if err != nil {
		return err
	}
	job.requestStop()
	<-job.done
	return job.err
}

// Close stops admission, requests termination, joins supervisors, and closes logs.
// Concurrent calls share shutdown and report its errors, including unproven cleanup.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		done := m.closeDone
		m.mu.Unlock()
		<-done
		return m.closeErr
	}
	m.closed = true
	jobs := make([]*managedJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}
	m.mu.Unlock()
	for _, job := range jobs {
		job.requestStop()
	}
	m.owned.Wait()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].state.ID < jobs[j].state.ID })
	m.mu.Lock()
	failures := append([]error(nil), m.admissionErrors...)
	for _, job := range jobs {
		if job.err != nil {
			failures = append(failures, fmt.Errorf("job %s: %w", job.state.ID, job.err))
		}
	}
	m.mu.Unlock()
	failures = append(failures, m.logs.Close())
	m.closeErr = errors.Join(failures...)
	close(m.closeDone)
	return m.closeErr
}
