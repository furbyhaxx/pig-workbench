package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogBytes        = 10 * 1024 * 1024
	defaultSessionLogBytes = 100 * 1024 * 1024
	defaultGrace           = 2 * time.Second
	defaultMaxActive       = 32
	defaultMaxFinished     = 20
	identityPrefixBytes    = 8
	identitySequenceWidth  = 16
)

// JobNotFoundError means the manager has no job with the requested ID.
type JobNotFoundError struct{}

// Error describes the missing job.
func (*JobNotFoundError) Error() string { return "job not found" }

// ErrJobNotFound identifies a missing job.
var ErrJobNotFound = &JobNotFoundError{}

// JobRetiredError means the job was admitted but its retained terminal state expired.
type JobRetiredError struct{}

// Error describes the retired job.
func (*JobRetiredError) Error() string { return "job state was retired" }

// Unwrap reports the job as missing for callers that do not distinguish retention.
func (*JobRetiredError) Unwrap() error { return ErrJobNotFound }

// ErrJobRetired identifies a removed terminal state.
var ErrJobRetired = &JobRetiredError{}

// ErrFailedToStart reports a command that could not be launched; its ID stays answerable.
var ErrFailedToStart = errors.New("job failed to start")

// ErrManagerClosed means admission is closed.
var ErrManagerClosed = errors.New("job manager is closed")

// ErrJobLimit means the concurrent process limit has been reached.
var ErrJobLimit = errors.New("active job limit reached")

// ErrJobCleanupIncomplete means child termination could not be established.
var ErrJobCleanupIncomplete = errors.New("job cleanup is incomplete; child or descendants may still be running")

// ManagerOptions configures output limits, background process ownership, and how many
// terminal states stay readable. Zero fields use the defaults: 10 MiB, 100 MiB, two
// seconds, 32 active jobs, and 20 finished jobs.
type ManagerOptions struct {
	LogBytes        int64
	SessionLogBytes int64
	Grace           time.Duration
	MaxActive       int
	MaxFinished     int
}

// JobState is a snapshot of a managed job and its terminal facts.
// Running remains true when termination could not be proven, because no leader status
// was obtained and descendants may still run.
// Cancelled records whether cancellation or shutdown won before observed exit.
type JobState struct {
	ID        string
	Running   bool
	Cancelled bool
	// Stopping is set while a requested termination has not produced an exit yet.
	Stopping bool
	// FailedToStart marks a command that never ran; Error says why.
	FailedToStart bool
	// Unknown marks a job whose supervision ended without a leader status: whether it
	// still runs is not known, so Running stays true and no exit is invented.
	Unknown bool
	Exit    *Exit
	Error   string
}

type managedJob struct {
	process       *Process
	streams       *streamTrack
	sequence      uint64
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
	maxFinished     int
	active          int
	closed          bool
	jobs            map[string]*managedJob
	finished        []*managedJob
	retired         int
	prefix          string
	sequence        uint64
	owned           sync.WaitGroup
	admissionErrors []error
	unresolved      []*Process
	closeDone       chan struct{}
	closeErr        error
}

// NewManager creates a background supervisor with independently owned logs.
func NewManager(options ManagerOptions) (*Manager, error) {
	if options.LogBytes < 0 || options.SessionLogBytes < 0 || options.Grace < 0 || options.MaxActive < 0 || options.MaxFinished < 0 {
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
	if options.MaxFinished == 0 {
		options.MaxFinished = defaultMaxFinished
	}
	prefix, err := newIdentityPrefix()
	if err != nil {
		return nil, err
	}
	logs, err := NewLogStore(options.LogBytes, options.SessionLogBytes)
	if err != nil {
		return nil, err
	}
	return &Manager{
		logs:        logs,
		grace:       options.Grace,
		maxActive:   options.MaxActive,
		maxFinished: options.MaxFinished,
		prefix:      prefix,
		jobs:        make(map[string]*managedJob),
		closeDone:   make(chan struct{}),
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

func newIdentityPrefix() (string, error) {
	var raw [identityPrefixBytes]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", fmt.Errorf("generate job identity prefix: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// admitIdentityLocked reserves this manager's next identity. Sequences only grow, so an
// ID is never handed out twice and a retired one stays identifiable as issued.
func (m *Manager) admitIdentityLocked() (string, uint64, error) {
	if m.sequence == math.MaxUint64 {
		return "", 0, errors.New("job identity space is exhausted")
	}
	m.sequence++
	return fmt.Sprintf("%s%0*x", m.prefix, identitySequenceWidth, m.sequence), m.sequence, nil
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
	id, sequence, err := m.admitIdentityLocked()
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	m.active++
	m.owned.Add(1)
	m.mu.Unlock()
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

	streams := &streamTrack{logs: m.logs, id: id}
	spec.Output = streamWriter{track: streams}
	spec.Errors = streamWriter{track: streams, stderr: true}
	process, err := StartProcess(spec)
	if err != nil {
		// The reserved ID stays answerable as a terminal state with an empty log.
		_ = m.logs.Append(id, nil)
		job := &managedJob{sequence: sequence, state: JobState{ID: id, FailedToStart: true, Error: err.Error()}, done: make(chan struct{}), stop: make(chan struct{}), selected: true, err: err}
		close(job.done)
		m.mu.Lock()
		m.jobs[id] = job
		m.finished = append(m.finished, job)
		retired := m.retireFinishedLocked()
		m.mu.Unlock()
		m.forgetRetired(retired)
		return id, fmt.Errorf("%w: %w", ErrFailedToStart, err)
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
	job := &managedJob{process: process, streams: streams, sequence: sequence, state: JobState{ID: id, Running: true}, done: make(chan struct{}), stop: make(chan struct{})}
	m.jobs[id] = job
	admitted = true
	go m.supervise(job)
	m.mu.Unlock()
	return id, nil
}

func (m *Manager) cleanupAdmission(process *Process, id string, cause error) (error, bool) {
	exit, cleanupErr := m.finishProcess(process, 0)
	unresolved := exit == nil
	var logErr error
	if unresolved {
		logErr = m.logs.Drop(id)
	} else {
		logErr = m.logs.Forget(id)
	}
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
	m.finished = append(m.finished, job)
	retired := m.retireFinishedLocked()
	close(job.done)
	m.mu.Unlock()
	m.forgetRetired(retired)
}

// retireFinishedLocked drops the oldest terminal states beyond the cap and returns them,
// so their log metadata can be forgotten now that no producer remains.
func (m *Manager) retireFinishedLocked() []*managedJob {
	excess := len(m.finished) - m.maxFinished
	if excess <= 0 {
		return nil
	}
	retired := m.finished[:excess]
	m.retired += excess
	m.finished = append([]*managedJob(nil), m.finished[excess:]...)
	for _, job := range retired {
		delete(m.jobs, job.state.ID)
	}
	return retired
}

func (m *Manager) forgetRetired(retired []*managedJob) {
	for _, job := range retired {
		// Only a closed store at shutdown or an already-forgotten log fails here.
		_ = m.logs.Forget(job.state.ID)
	}
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
	outputErr := process.closeOutput()
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
	job.state.Unknown = true
	job.state.Error = err.Error()
	job.err = err
	close(job.done)
	m.mu.Unlock()
}

func (m *Manager) job(id string) (*managedJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lookupLocked(id)
}

func (m *Manager) lookupLocked(id string) (*managedJob, error) {
	if job := m.jobs[id]; job != nil {
		return job, nil
	}
	if m.retiredLocked(id) {
		return nil, fmt.Errorf("%w: %s", ErrJobRetired, id)
	}
	return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
}

// retiredLocked reports whether the ID belongs to this manager and was already issued, so
// an unknown ID stays distinguishable from a retired one without tracking every ID.
func (m *Manager) retiredLocked(id string) bool {
	if len(id) != len(m.prefix)+identitySequenceWidth || !strings.HasPrefix(id, m.prefix) {
		return false
	}
	sequence, err := strconv.ParseUint(id[len(m.prefix):], 16, 64)
	if err != nil {
		return false
	}
	return sequence <= m.sequence
}

// Active reports a job that is known to be running; an unknown job is not waited on.
func (s JobState) Active() bool { return s.Running && !s.Unknown }

// snapshot copies the job's state and derives Stopping from a pending stop request.
func (j *managedJob) snapshot() JobState {
	state := j.state
	if state.Exit != nil {
		exit := *state.Exit
		state.Exit = &exit
	}
	if state.Running && !state.Unknown {
		j.selectionMu.Lock()
		state.Stopping = j.stopRequested
		j.selectionMu.Unlock()
	}
	return state
}

// Status returns an independent snapshot of a job.
func (m *Manager) Status(id string) (JobState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, err := m.lookupLocked(id)
	if err != nil {
		return JobState{}, err
	}
	return job.snapshot(), nil
}

// List returns independent snapshots of every retained job, oldest admission first.
// Running jobs are always retained; terminal states beyond MaxFinished are retired.
// ProcessID returns a job's process-group leader identity for the registry that
// reclaims jobs whose runtime went away without shutting them down.
func (m *Manager) ProcessID(id string) (int, string, bool) {
	job, err := m.job(id)
	if err != nil || job.process == nil {
		return 0, "", false
	}
	pid := job.process.PID()
	if pid <= 0 {
		return 0, "", false
	}
	identity, err := processIdentity(pid)
	if err != nil {
		identity = ""
	}
	return pid, identity, true
}

// Retired counts the terminal states dropped beyond MaxFinished since the manager started.
func (m *Manager) Retired() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retired
}

func (m *Manager) List() []JobState {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]*managedJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].sequence < jobs[j].sequence })
	states := make([]JobState, len(jobs))
	for i, job := range jobs {
		states[i] = job.snapshot()
	}
	return states
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
		// The job's own terminal state, so retirement cannot turn an attached wait
		// into a missing-job error.
		m.mu.Lock()
		defer m.mu.Unlock()
		return job.snapshot(), nil
	}
}

// Output reads a page from the managed job's bounded combined log.
func (m *Manager) Output(id string, cursor int64, max int) ([]byte, int64, bool, int64, error) {
	return m.Stream(id, StreamCombined, cursor, max)
}

// Written reports how many combined output bytes the job has produced: the cursor just past its log.
func (m *Manager) Written(id string) (int64, error) {
	if _, err := m.job(id); err != nil {
		return 0, err
	}
	written, _, _, err := m.logs.Facts(id)
	return written, err
}

// Stream reads one output stream. Stdout and stderr cursors count only that stream's bytes;
// the combined cursor counts both. A page may contain fewer than max bytes when the scan
// crosses the other stream.
func (m *Manager) Stream(id string, stream Stream, cursor int64, max int) ([]byte, int64, bool, int64, error) {
	job, err := m.job(id)
	if err != nil {
		return nil, 0, false, 0, err
	}
	if stream == StreamCombined || job.streams == nil {
		data, next, truncated, gap, readErr := m.logs.Read(id, cursor, max)
		if errors.Is(readErr, ErrLogNotFound) {
			if _, lookupErr := m.job(id); lookupErr != nil {
				return nil, 0, false, 0, lookupErr
			}
		}
		return data, next, truncated, gap, readErr
	}
	const scan = 64 << 10
	var out []byte
	pos := int64(0)
	var truncated bool
	var gap int64
	for len(out) < max || max <= 0 {
		chunk, next, chunkTruncated, chunkGap, readErr := m.logs.Read(id, pos, scan)
		if errors.Is(readErr, ErrLogNotFound) {
			return nil, 0, false, 0, readErr
		}
		if readErr != nil && !errors.Is(readErr, ErrLogEvicted) {
			return nil, 0, false, 0, readErr
		}
		if chunkTruncated {
			truncated, gap = true, chunkGap
		}
		kept := job.streams.filter(chunk, pos, stream)
		if cursor < int64(len(out)+len(kept)) {
			from := int(cursor - int64(len(out)))
			if from < 0 {
				from = 0
			}
			out = append(out, kept[from:]...)
		}
		pos = next
		if len(chunk) == 0 || next >= job.streams.written {
			break
		}
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, cursor + int64(len(out)), truncated, gap, nil
}

// Cancel requests group termination and waits for the supervision result.
func (m *Manager) Cancel(id string) error {
	job, err := m.job(id)
	if err != nil {
		return err
	}
	if job.process == nil {
		return fmt.Errorf("%w: %s", ErrFailedToStart, job.state.Error)
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
		if job.process != nil {
			job.requestStop()
		}
	}
	m.owned.Wait()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].state.ID < jobs[j].state.ID })
	m.mu.Lock()
	failures := append([]error(nil), m.admissionErrors...)
	for _, job := range jobs {
		if job.err != nil && !job.state.FailedToStart {
			failures = append(failures, fmt.Errorf("job %s: %w", job.state.ID, job.err))
		}
	}
	m.mu.Unlock()
	failures = append(failures, m.logs.Close())
	m.closeErr = errors.Join(failures...)
	close(m.closeDone)
	return m.closeErr
}
