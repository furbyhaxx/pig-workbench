// Package jobs supervises local processes for session-owned jobs.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ErrReaped means the leader was reaped and its group identity can no longer be used.
var ErrReaped = errors.New("process leader has been reaped")

const (
	// exitObservationTimeout bounds the identity observation that replaces a failed watcher
	// during cleanup, so a survivor cannot hold the caller.
	exitObservationTimeout = 5 * time.Second
	// abandonTimeout bounds the reap of a child whose supervision failed to start.
	abandonTimeout = 5 * time.Second
)

// StdinClosedError means process input is disabled or closed.
type StdinClosedError struct{}

func (*StdinClosedError) Error() string { return "process stdin is closed" }

// ErrStdinClosed reports that no more input can be written.
var ErrStdinClosed = &StdinClosedError{}

// ErrOutputIncomplete means output remained open after the leader was reaped.
var ErrOutputIncomplete = errors.New("process output did not close after leader exit")

// ErrGroupNotGone means group disappearance could not be observed after termination.
var ErrGroupNotGone = errors.New("process group remains after termination")

// ErrProcessIdentityUnsupported means group ownership could not be bound to the identity
// created with the leader, so group cleanup cannot be verified.
var ErrProcessIdentityUnsupported = errors.New("identity-bound process-group signalling is unavailable")

// ErrExitUnavailable means the leader was reaped outside this package, so no exit status exists.
var ErrExitUnavailable = errors.New("process leader exit status is unavailable")

// GroupKillUnsupportedError means a platform cannot terminate a whole process group.
type GroupKillUnsupportedError struct{}

func (*GroupKillUnsupportedError) Error() string { return "process-group termination is unsupported" }

// ErrGroupKillUnsupported reports that only the direct child can be terminated.
var ErrGroupKillUnsupported = &GroupKillUnsupportedError{}

// WatcherUnsupportedError means a platform cannot observe exit without reaping.
type WatcherUnsupportedError struct{}

func (*WatcherUnsupportedError) Error() string {
	return "non-reaping process exit observation is unsupported"
}

// ErrWatcherUnsupported reports that safe group supervision cannot be started.
var ErrWatcherUnsupported = &WatcherUnsupportedError{}

// Spec describes a process and its output destinations.
// For Shell, Command[0] is sh source and remaining entries become positional parameters.
// A nil Env inherits the current environment; a nil Output discards output.
// Stdin enables a writable pipe; otherwise the child receives EOF.
// A non-nil Errors receives stderr through its own pipe; otherwise stderr joins Output.
// Output.Write and Errors.Write may run concurrently, must eventually return and must not
// call methods on the Process.
type Spec struct {
	Command []string
	Dir     string
	Env     []string
	Shell   bool
	Stdin   bool
	Output  io.Writer
	Errors  io.Writer
}

// Exit records the observed status of a reaped process.
type Exit struct {
	Code     int
	Signaled bool
	Signal   string
	Duration time.Duration
}

// Process owns a child until it has been reaped and its output drain has stopped.
// Ownership splits into the group identity in owner, the exit observation in watcher,
// and the reap this process completed under mu. Losing the observation never loses the
// identity, so cleanup and signalling outlive a broken watcher. A reap needs two things
// that the watcher cannot provide: independent evidence that the leader terminated, and
// the gate, which an admitted escalation holds until its signals are finished.
type Process struct {
	cmd         *exec.Cmd
	started     time.Time
	owner       *groupOwner
	watcher     *exitWatcher
	output      *os.File
	errOutput   *os.File
	outputDone  chan struct{}
	outputErr   error
	stdin       io.WriteCloser
	stdinOnce   sync.Once
	stdinClosed atomic.Bool
	mu          sync.Mutex // guards the reap transition and the status it produced
	gate        sync.Mutex // holds signalling, escalation and our own reap apart
	killMu      sync.Mutex // serializes Kill and Close
	killErr     error
	closed      bool
	reaped      bool
	exitKnown   bool
	result      Exit
	reapErr     error
	waitErr     error
	reapOnce    sync.Once // runs cmd.Wait once, under the gate
	drainOnce   sync.Once // drains output once, without the gate
	collectOnce sync.Once // admits the single background collector
	reapDone    chan struct{}
}

type exitWatcher struct {
	signal   chan struct{}
	done     chan struct{}
	observed time.Time
	err      error
}

func watch(fn func() error) *exitWatcher {
	w := &exitWatcher{signal: make(chan struct{}), done: make(chan struct{})}
	go func() {
		w.err = fn()
		if w.err == nil {
			w.observed = time.Now()
			close(w.signal)
		}
		close(w.done)
	}()
	return w
}

// failure reports the observation diagnostic once the watcher has finished.
// It never decides whether cleanup may proceed.
func (w *exitWatcher) failure() error {
	select {
	case <-w.done:
		return w.err
	default:
		return nil
	}
}

// observationTime reports when the watcher saw the exit, or the zero time while it is pending.
func (w *exitWatcher) observationTime() time.Time {
	select {
	case <-w.done:
		return w.observed
	default:
		return time.Time{}
	}
}

// watchProcess observes leader exit without reaping it. Tests replace it to reach the
// initialization failures the kernel does not produce here.
var watchProcess = startExitWatcher

// observeIdentityExit establishes leader exit through the group identity without reaping it.
// Tests replace it to hold a passive observation open while cleanup runs.
var observeIdentityExit = func(owner *groupOwner, ctx context.Context) error {
	return owner.observeExit(ctx)
}

// StartProcess starts a process in a dedicated Unix process group.
// Callers must call Wait or Close to reap the child.
func StartProcess(spec Spec) (*Process, error) {
	if len(spec.Command) == 0 || spec.Command[0] == "" {
		return nil, errors.New("empty process command")
	}
	args := spec.Command
	if spec.Shell {
		args = append([]string{"sh", "-c", spec.Command[0], "sh"}, spec.Command[1:]...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	output := spec.Output
	if output == nil {
		output = io.Discard
	}
	owner, err := newGroupOwner()
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create process output pipe: %w", err)
	}
	cmd.Stdout, cmd.Stderr = writer, writer
	var errReader, errWriter *os.File
	if spec.Errors != nil {
		errReader, errWriter, err = os.Pipe()
		if err != nil {
			reader.Close()
			writer.Close()
			return nil, fmt.Errorf("create process error pipe: %w", err)
		}
		cmd.Stderr = errWriter
	}
	closePipes := func() {
		reader.Close()
		writer.Close()
		if errReader != nil {
			errReader.Close()
			errWriter.Close()
		}
	}
	var stdin io.WriteCloser
	if spec.Stdin {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			closePipes()
			return nil, fmt.Errorf("create process stdin pipe: %w", err)
		}
	}
	owner.configure(cmd)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		closePipes()
		if stdin != nil {
			stdin.Close()
		}
		return nil, fmt.Errorf("start process: %w", err)
	}
	writer.Close()
	if errWriter != nil {
		errWriter.Close()
	}
	if err := owner.bind(cmd.Process); err != nil {
		return nil, abandonStart(cmd, owner, reader, errReader, stdin, fmt.Errorf("bind process identity: %w", err))
	}
	watcher, err := watchProcess(owner, cmd.Process)
	if err != nil {
		return nil, abandonStart(cmd, owner, reader, errReader, stdin, fmt.Errorf("observe process exit: %w", err))
	}
	p := &Process{cmd: cmd, started: started, owner: owner, watcher: watcher, output: reader, errOutput: errReader, outputDone: make(chan struct{}), stdin: stdin, reapDone: make(chan struct{})}
	var errCopy error
	errDone := make(chan struct{})
	if errReader != nil {
		go func() {
			_, errCopy = io.Copy(spec.Errors, errReader)
			errReader.Close()
			close(errDone)
		}()
	} else {
		close(errDone)
	}
	go func() {
		_, copyErr := io.Copy(output, reader)
		reader.Close()
		<-errDone
		p.outputErr = errors.Join(copyErr, errCopy)
		close(p.outputDone)
	}()
	return p, nil
}

// abandonStart terminates a child whose supervision could not be established. Only stable
// handles are used: a failed verification leaves the group unreclaimed and reports it, and a
// child that outlives termination cannot hold the caller by the reap.
func abandonStart(cmd *exec.Cmd, owner *groupOwner, reader, errReader *os.File, stdin io.WriteCloser, cause error) error {
	var stdinErr error
	if stdin != nil {
		stdinErr = stdin.Close()
	}
	killErr := owner.abortStart()
	waitErr := reapAbandoned(cmd)
	releaseErr := owner.release()
	reader.Close()
	if errReader != nil {
		errReader.Close()
	}
	return errors.Join(cause, stdinErr, killErr, waitErr, releaseErr)
}

// reapAbandoned reaps a launched child after termination without waiting on a survivor:
// cmd.Wait has no deadline of its own.
func reapAbandoned(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(abandonTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return err
	case <-timer.C:
		return errors.New("abandoned child did not terminate before the reap deadline")
	}
}

// PID returns the direct child's process ID.
func (p *Process) PID() int { return p.cmd.Process.Pid }

// Write sends data to the child's stdin and reports partial writes.
// Input is closed by CloseStdin, Kill, Close, or reaping the child.
func (p *Process) Write(data []byte) (int, error) {
	if p.stdin == nil || p.stdinClosed.Load() {
		return 0, ErrStdinClosed
	}
	select {
	case <-p.watcher.signal:
		return 0, ErrStdinClosed
	default:
	}
	n, err := p.stdin.Write(data)
	if err != nil {
		if p.stdinClosed.Load() || errors.Is(err, os.ErrClosed) {
			return n, fmt.Errorf("%w: %w", ErrStdinClosed, err)
		}
		return n, fmt.Errorf("write process stdin: %w", err)
	}
	return n, nil
}

// CloseStdin closes the input pipe, causing the child to receive EOF.
// It is safe to call concurrently with Write and may be repeated.
func (p *Process) CloseStdin() error {
	var err error
	p.stdinOnce.Do(func() {
		p.stdinClosed.Store(true)
		if p.stdin != nil {
			err = p.stdin.Close()
			if errors.Is(err, os.ErrClosed) {
				err = nil
			} else if err != nil {
				err = fmt.Errorf("close process stdin: %w", err)
			}
		}
	})
	return err
}

// Exited closes when the leader exits without reaping it.
// Wait or Close must still be called, and watcher errors are reported by Wait.
func (p *Process) Exited() <-chan struct{} {
	return p.watcher.signal
}

// Signal sends a signal to the owned process group, or to the child on Windows.
// A broken exit observation does not block delivery: the group identity stays valid
// until this process reaps the leader. The signal/reap gate is held across the refusal
// check and the delivery, so nothing reaches the group after our reap and an admitted
// escalation finishes signalling first.
func (p *Process) Signal(sig os.Signal) error {
	if sig == nil {
		return errors.New("nil process signal")
	}
	p.gate.Lock()
	defer p.gate.Unlock()
	p.mu.Lock()
	reaped := p.reaped
	p.mu.Unlock()
	if reaped {
		return ErrReaped
	}
	return p.owner.signal(sig)
}

// Wait waits for and reaps the leader; cancelling ctx stops the wait without reaping a leader
// that has not been shown to exit. Once exit evidence exists, the single collector completes
// the reap on its own. A nonzero exit is recorded in Exit rather than returned as an error.
// If inherited output descriptors remain open, Wait returns ErrOutputIncomplete.
// Only a leader reaped here has a status: a leader reaped elsewhere yields
// ErrExitUnavailable instead of an invented Exit. A broken exit observation is
// reported next to the result rather than replacing it.
func (p *Process) Wait(ctx context.Context) (Exit, error) {
	// The record of a completed reap decides: observing afterwards could only probe an
	// identity descriptor that Close already released.
	if exit, resultErr, done := p.reapedResult(); done {
		return exit, errors.Join(p.watcher.failure(), resultErr)
	}
	if err := ctx.Err(); err != nil {
		return Exit{}, err
	}
	exited, observationErr := p.observedExit(ctx, p.observeIdentity)
	if !exited {
		if exit, resultErr, done := p.reapedResult(); done {
			return exit, errors.Join(p.watcher.failure(), resultErr)
		}
		// No evidence that the leader terminated: reaping now could block on a live child,
		// so the caller keeps ownership of it.
		return Exit{}, observationErr
	}
	if err := ctx.Err(); err != nil {
		return Exit{}, errors.Join(err, observationErr)
	}
	// The admitted collector finishes independently, so this wait answers its context even
	// while an escalation owns the gate.
	p.admitCollector()
	select {
	case <-ctx.Done():
		return Exit{}, errors.Join(ctx.Err(), observationErr)
	case <-p.reapDone:
	}
	exit, resultErr, _ := p.reapedResult()
	return exit, errors.Join(observationErr, resultErr)
}

// observedExit establishes that the leader terminated without reaping it. A working watcher
// settles it; otherwise the group identity is observed directly through observe, bounded by
// ctx. The error keeps the watcher diagnostic and any observation failure. Neither path holds
// a lifecycle lock while it waits, so a pending wait never stands between cleanup and its reap.
func (p *Process) observedExit(ctx context.Context, observe func(context.Context) error) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-p.watcher.done:
	}
	watchErr := p.watcher.failure()
	if watchErr == nil {
		return true, nil
	}
	if err := observe(ctx); err != nil {
		return false, errors.Join(watchErr, err)
	}
	return true, watchErr
}

// terminalExit reports the exit of a leader this process reaped, if any.
func (p *Process) terminalExit() (*Exit, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.exitKnown {
		return nil, false
	}
	exit := p.result
	return &exit, true
}

// reapedResult reports the record of our own reap once its drain finished, without touching the
// identity. A caller that finds only the published leader status waits for the completion.
func (p *Process) reapedResult() (Exit, error, bool) {
	select {
	case <-p.reapDone:
	default:
		return Exit{}, nil, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.exitKnown {
		return Exit{}, errors.Join(ErrExitUnavailable, p.waitErr), true
	}
	return p.result, p.waitErr, true
}

// recordedErr reports the outcome of the reap this process completed.
func (p *Process) recordedErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

// admitCollector starts the single background collector that reaps an exited leader. It takes
// the gate itself, so an admitted escalation reaps inline and the collector then finds nothing
// to do: neither side waits for the other under the gate.
func (p *Process) admitCollector() {
	p.collectOnce.Do(func() {
		go func() {
			p.gate.Lock()
			p.reap()
			p.gate.Unlock()
			p.drain()
		}()
	})
}

// reap runs cmd.Wait at most once and publishes the leader status. The caller holds the gate and
// established exit evidence first, so the wait cannot stall and no escalation is still
// signalling the group.
func (p *Process) reap() {
	p.reapOnce.Do(p.collect)
}

func (p *Process) collect() {
	err := p.cmd.Wait()
	stdinErr := p.CloseStdin()
	state := p.cmd.ProcessState
	// Without an exit observation the duration can only end at the reap.
	duration := time.Since(p.started)
	if observed := p.watcher.observationTime(); !observed.IsZero() && observed.After(p.started) {
		duration = observed.Sub(p.started)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = nil
	}
	if state == nil {
		// The leader was reaped elsewhere: its status cannot be recovered, and saying so is
		// the record for every later caller.
		err = errors.Join(err, ErrExitUnavailable)
	}
	p.mu.Lock()
	p.reaped = true
	p.exitKnown = state != nil
	p.result = processExit(state, duration)
	p.reapErr = errors.Join(err, stdinErr)
	p.mu.Unlock()
}

// drain joins the output drain once and records the reap outcome before reapDone closes. It runs
// without the gate: a descendant that retains the pipe must not hold off signalling or cleanup.
func (p *Process) drain() {
	p.drainOnce.Do(func() {
		err := p.drainOutput()
		p.mu.Lock()
		p.waitErr = errors.Join(p.reapErr, err)
		p.mu.Unlock()
		close(p.reapDone)
	})
}

// closeOutput closes every output pipe so stuck drains return; it reports the first close error.
func (p *Process) closeOutput() error {
	err := p.output.Close()
	if p.errOutput != nil {
		if errErr := p.errOutput.Close(); err == nil || errors.Is(err, os.ErrClosed) {
			err = errErr
		}
	}
	return err
}

// drainOutput waits for the output drain; a descendant may retain the pipe after its leader
// exited.
func (p *Process) drainOutput() error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-p.outputDone:
		return p.outputErr
	case <-timer.C:
		p.closeOutput()
		<-p.outputDone
		return errors.Join(ErrOutputIncomplete, p.outputErr)
	}
}

// Kill terminates the group, escalates after grace, and reaps the child.
// On Windows it kills the direct child and returns ErrGroupKillUnsupported.
func (p *Process) Kill(grace time.Duration) error {
	p.killMu.Lock()
	defer p.killMu.Unlock()
	stdinErr := p.CloseStdin()
	err := p.kill(grace)
	if stdinErr != nil {
		p.killErr = errors.Join(p.killErr, stdinErr)
	}
	return errors.Join(stdinErr, err)
}

func (p *Process) kill(grace time.Duration) error {
	// The gate is held from admission through the reap that publishes the status, so no reap
	// can complete while the escalation still signals the group and no signal slips between
	// the refusal check and our reap.
	p.gate.Lock()
	p.mu.Lock()
	reaped := p.reaped
	p.mu.Unlock()
	if reaped {
		p.gate.Unlock()
		return ErrReaped
	}
	termErr := p.owner.terminate(grace)
	ctx, cancel := context.WithTimeout(context.Background(), exitObservationTimeout)
	exited, exitErr := p.observedExit(ctx, p.owner.observeExit)
	cancel()
	if exited {
		p.reap()
	}
	p.gate.Unlock()
	var waitErr error
	if exited {
		p.drain()
		waitErr = errors.Join(exitErr, p.recordedErr())
	} else {
		// Termination could not be established, so the leader stays unreaped: reaping a
		// survivor would block here, and claiming its exit would be inventing one.
		waitErr = errors.Join(exitErr, ErrJobCleanupIncomplete)
	}
	groupErr := p.owner.gone()
	p.killErr = errors.Join(termErr, waitErr, groupErr)
	return p.killErr
}

// Close immediately terminates and reaps an unreaped child and joins its watcher.
// The first call reports recorded failures; later calls do no work and return nil.
func (p *Process) Close() error {
	p.killMu.Lock()
	defer p.killMu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	stdinErr := p.CloseStdin()
	err := p.kill(0)
	if errors.Is(err, ErrReaped) {
		<-p.reapDone
		err = errors.Join(p.recordedErr(), p.killErr)
	}
	return errors.Join(stdinErr, err, p.watcher.failure(), p.releaseIdentity())
}

// releaseIdentity closes the identity handle exactly once, after the leader was reaped and no
// observer is left. It holds the gate so observation cannot use a recycled descriptor, and a
// reap always ends the watcher first, so the join cannot block on a live leader.
func (p *Process) releaseIdentity() error {
	p.gate.Lock()
	defer p.gate.Unlock()
	p.mu.Lock()
	reaped := p.reaped
	p.mu.Unlock()
	if !reaped {
		return nil
	}
	<-p.watcher.done
	return p.owner.release()
}

// observeIdentity establishes leader termination through the group identity, bounded by ctx.
// It holds no lifecycle lock: the owner observes through a private descriptor copy.
func (p *Process) observeIdentity(ctx context.Context) error {
	return observeIdentityExit(p.owner, ctx)
}
