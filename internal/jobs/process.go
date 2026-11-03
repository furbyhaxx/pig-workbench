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

// StdinClosedError means process input is disabled or closed.
type StdinClosedError struct{}

func (*StdinClosedError) Error() string { return "process stdin is closed" }

// ErrStdinClosed reports that no more input can be written.
var ErrStdinClosed = &StdinClosedError{}

// ErrOutputIncomplete means output remained open after the leader was reaped.
var ErrOutputIncomplete = errors.New("process output did not close after leader exit")

// ErrGroupNotGone means group disappearance could not be observed after termination.
var ErrGroupNotGone = errors.New("process group remains after termination")

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

// Spec describes a process and its combined output destination.
// For Shell, Command[0] is sh source and remaining entries become positional parameters.
// A nil Env inherits the current environment; a nil Output discards output.
// Stdin enables a writable pipe; otherwise the child receives EOF.
// Output.Write must eventually return and must not call methods on the Process.
type Spec struct {
	Command []string
	Dir     string
	Env     []string
	Shell   bool
	Stdin   bool
	Output  io.Writer
}

// Exit records the observed status of a reaped process.
type Exit struct {
	Code     int
	Signaled bool
	Signal   string
	Duration time.Duration
}

// Process owns a child until it has been reaped and its output drain has stopped.
type Process struct {
	cmd         *exec.Cmd
	started     time.Time
	watcher     *exitWatcher
	output      *os.File
	outputDone  chan struct{}
	outputErr   error
	stdin       io.WriteCloser
	stdinOnce   sync.Once
	stdinClosed atomic.Bool
	mu          sync.Mutex
	killMu      sync.Mutex
	killErr     error
	closed      bool
	reaped      bool
	result      Exit
	waitErr     error
	waitOnce    sync.Once
	waitDone    chan struct{}
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

func (w *exitWatcher) exited() (<-chan struct{}, error) {
	select {
	case <-w.done:
		return w.signal, w.err
	default:
		return w.signal, nil
	}
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
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create process output pipe: %w", err)
	}
	cmd.Stdout, cmd.Stderr = writer, writer
	var stdin io.WriteCloser
	if spec.Stdin {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			reader.Close()
			writer.Close()
			return nil, fmt.Errorf("create process stdin pipe: %w", err)
		}
	}
	configureProcess(cmd)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		if stdin != nil {
			stdin.Close()
		}
		return nil, fmt.Errorf("start process: %w", err)
	}
	writer.Close()
	watcher, err := startExitWatcher(cmd.Process)
	if err != nil {
		var stdinErr error
		if stdin != nil {
			stdinErr = stdin.Close()
		}
		killErr := terminateProcess(cmd.Process, 0)
		waitErr := cmd.Wait()
		reader.Close()
		return nil, errors.Join(fmt.Errorf("observe process exit: %w", err), stdinErr, killErr, waitErr)
	}
	p := &Process{cmd: cmd, started: started, watcher: watcher, output: reader, outputDone: make(chan struct{}), stdin: stdin, waitDone: make(chan struct{})}
	go func() {
		_, p.outputErr = io.Copy(output, reader)
		reader.Close()
		close(p.outputDone)
	}()
	return p, nil
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
func (p *Process) Signal(sig os.Signal) error {
	if sig == nil {
		return errors.New("nil process signal")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reaped {
		return ErrReaped
	}
	if _, err := p.watcher.exited(); err != nil {
		return fmt.Errorf("observe process exit: %w", err)
	}
	return signalProcess(p.cmd.Process, sig)
}

// Wait waits for and reaps the leader; cancelling ctx leaves the child running.
// A nonzero exit is recorded in Exit rather than returned as an error.
// If inherited output descriptors remain open, Wait returns ErrOutputIncomplete.
func (p *Process) Wait(ctx context.Context) (Exit, error) {
	if err := ctx.Err(); err != nil {
		return Exit{}, err
	}
	select {
	case <-ctx.Done():
		return Exit{}, ctx.Err()
	case <-p.watcher.done:
	}
	if _, err := p.watcher.exited(); err != nil {
		return Exit{}, fmt.Errorf("observe process exit: %w", err)
	}
	p.waitOnce.Do(func() { go p.reap() })
	select {
	case <-ctx.Done():
		return Exit{}, ctx.Err()
	case <-p.waitDone:
		return p.result, p.waitErr
	}
}

func (p *Process) reap() {
	p.mu.Lock()
	err := p.cmd.Wait()
	stdinErr := p.CloseStdin()
	p.reaped = true
	result := processExit(p.cmd.ProcessState, p.watcher.observed.Sub(p.started))
	p.mu.Unlock()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = nil
	}
	err = errors.Join(err, stdinErr)
	// A descendant may retain the output pipe after its leader has exited.
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-p.outputDone:
		err = errors.Join(err, p.outputErr)
	case <-timer.C:
		p.output.Close()
		<-p.outputDone
		err = errors.Join(err, ErrOutputIncomplete, p.outputErr)
	}
	timer.Stop()
	p.result, p.waitErr = result, err
	close(p.waitDone)
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
	p.mu.Lock()
	if p.reaped {
		p.mu.Unlock()
		return ErrReaped
	}
	if _, err := p.watcher.exited(); err != nil {
		p.mu.Unlock()
		return fmt.Errorf("observe process exit: %w", err)
	}
	err := terminateProcess(p.cmd.Process, grace)
	p.mu.Unlock()
	if err != nil && !errors.Is(err, ErrGroupKillUnsupported) {
		return err
	}
	_, waitErr := p.Wait(context.Background())
	var groupErr error
	if err == nil {
		groupErr = waitProcessGroupGone(p.cmd.Process)
	}
	p.killErr = errors.Join(err, waitErr, groupErr)
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
		<-p.waitDone
		return errors.Join(stdinErr, p.waitErr, p.killErr)
	}
	return errors.Join(stdinErr, err)
}
