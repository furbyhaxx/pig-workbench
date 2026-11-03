//go:build linux

package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	groupGoneTimeout = 2 * time.Second
	groupGoneTick    = 5 * time.Millisecond
	observeSlice     = 50 * time.Millisecond
)

// groupOwner binds every group operation to the identity the kernel created with the
// leader. The pidfd comes from clone(CLONE_PIDFD), so pidfd_send_signal and the group
// probe resolve the original process group even after the leader was reaped, and a
// recycled numeric group id cannot be reached.
type groupOwner struct {
	mu       sync.Mutex
	fd       int
	verified bool
	process  *os.Process
	closed   bool
}

// probeIdentity proves the group-signalling capability before any user command runs. Signal 0
// is permission-checked but never delivered, and the descriptor is our own, so the probe has
// no side effects. An unsupported CLONE_PIDFD shows up as a failed start, which runs no user
// code either.
var probeIdentity = probeGroupIdentity

var (
	probeOnce sync.Once
	probeErr  error
)

func probeGroupIdentity() error {
	probeOnce.Do(func() {
		fd, err := unix.PidfdOpen(os.Getpid(), 0)
		if err != nil {
			probeErr = fmt.Errorf("%w: pidfd_open: %w", ErrProcessIdentityUnsupported, err)
			return
		}
		defer unix.Close(fd)
		// ESRCH only means this process does not lead its own group; a rejected flag or a
		// missing syscall is what proves the capability absent.
		if err := unix.PidfdSendSignal(fd, 0, nil, unix.PIDFD_SIGNAL_PROCESS_GROUP); err != nil && !errors.Is(err, unix.ESRCH) {
			probeErr = fmt.Errorf("%w: pidfd_send_signal: %w", ErrProcessIdentityUnsupported, err)
		}
	})
	return probeErr
}

func newGroupOwner() (*groupOwner, error) {
	if err := probeIdentity(); err != nil {
		return nil, err
	}
	return &groupOwner{fd: -1}, nil
}

func (o *groupOwner) configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, PidFD: &o.fd}
}

// bind verifies that the leader has an identity handle and that group signalling through it
// works. Both are checked on the real child: signal 0 delivers nothing and proves the
// capability instead of inferring it from a kernel version.
func (o *groupOwner) bind(process *os.Process) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.process = process
	if o.fd < 0 {
		return fmt.Errorf("%w: kernel provided no pidfd for pid %d", ErrProcessIdentityUnsupported, process.Pid)
	}
	if err := unix.PidfdSendSignal(o.fd, 0, nil, unix.PIDFD_SIGNAL_PROCESS_GROUP); err != nil {
		return fmt.Errorf("%w: %w", ErrProcessIdentityUnsupported, err)
	}
	o.verified = true
	return nil
}

// deliverGroupSignal delivers to the original process group. Tests replace it to sequence a
// delivery against the reap.
var deliverGroupSignal = func(o *groupOwner, sig syscall.Signal) error {
	return o.sendGroup(sig)
}

func (o *groupOwner) signal(sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return fmt.Errorf("unsupported process signal %v", sig)
	}
	if err := deliverGroupSignal(o, s); err != nil {
		return fmt.Errorf("signal process group: %w", err)
	}
	return nil
}

// sendGroup delivers sig to the original process group through the pidfd. There is no numeric
// fallback: an unverified identity refuses and reports instead of guessing a group id.
// Callers hold mu, so the descriptor cannot be released while a call is in flight.
func (o *groupOwner) sendGroup(sig syscall.Signal) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.verified || o.closed {
		return ErrProcessIdentityUnsupported
	}
	return unix.PidfdSendSignal(o.fd, sig, nil, unix.PIDFD_SIGNAL_PROCESS_GROUP)
}

func (o *groupOwner) terminate(grace time.Duration) error {
	termErr := o.sendGroup(syscall.SIGTERM)
	if termErr != nil && !errors.Is(termErr, syscall.ESRCH) && !errors.Is(termErr, ErrProcessIdentityUnsupported) {
		return fmt.Errorf("terminate process group: %w", termErr)
	}
	if !errors.Is(termErr, syscall.ESRCH) {
		if grace > 0 {
			timer := time.NewTimer(grace)
			<-timer.C
			timer.Stop()
		}
		killErr := o.sendGroup(syscall.SIGKILL)
		if killErr != nil && !errors.Is(killErr, syscall.ESRCH) && !errors.Is(killErr, ErrProcessIdentityUnsupported) {
			return fmt.Errorf("kill process group: %w", killErr)
		}
	}
	if errors.Is(termErr, ErrProcessIdentityUnsupported) {
		return termErr
	}
	return nil
}

// gone reports whether the original group has no members left. A numeric probe cannot answer
// that once the leader is reaped, because the group id may already belong to another group.
func (o *groupOwner) gone() error {
	deadline := time.NewTimer(groupGoneTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(groupGoneTick)
	defer ticker.Stop()
	for {
		err := o.sendGroup(0)
		switch {
		case err == nil:
		case errors.Is(err, syscall.ESRCH):
			return nil
		default:
			return fmt.Errorf("check process group: %w", err)
		}
		select {
		case <-deadline.C:
			return ErrGroupNotGone
		case <-ticker.C:
		}
	}
}

// observeExit waits, bounded by ctx, until the identity reports leader termination. It reaps
// nothing, so it stays usable while the exit watcher is broken. The descriptor is duplicated
// under mu and polled through that private copy: the wait holds no lifecycle lock, and neither
// a released descriptor nor a reused number can end or redirect it. Only POLLIN counts as exit
// evidence; an invalid or erroring descriptor is reported instead of read as an exit.
func (o *groupOwner) observeExit(ctx context.Context) error {
	fd, err := o.duplicateFD()
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(observeSlice/time.Millisecond))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("observe process exit: %w", err)
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&unix.POLLIN != 0 {
			return nil
		}
		return fmt.Errorf("process identity descriptor reported revents %#x without exit evidence", fds[0].Revents)
	}
}

// duplicateFD hands out a private descriptor for the identity. The caller closes it when its
// observation ends, and release cannot invalidate it.
func (o *groupOwner) duplicateFD() (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.verified || o.closed {
		return -1, ErrProcessIdentityUnsupported
	}
	fd, err := unix.FcntlInt(uintptr(o.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("duplicate process identity descriptor: %w", err)
	}
	return fd, nil
}

// abortStart reclaims a launched child whose ownership could not be verified. It never signals
// a process group: with a verified identity the group is terminated through the pidfd, and
// otherwise only the direct child is killed through the handle the kernel created for it, with
// the unreclaimed group reported.
func (o *groupOwner) abortStart() error {
	o.mu.Lock()
	verified := o.verified && !o.closed
	process := o.process
	o.mu.Unlock()
	if verified {
		return o.terminate(0)
	}
	if process == nil {
		return ErrProcessIdentityUnsupported
	}
	var killErr error
	err := process.WithHandle(func(handle uintptr) {
		if err := unix.PidfdSendSignal(int(handle), syscall.SIGKILL, nil, 0); err != nil && !errors.Is(err, syscall.ESRCH) {
			killErr = err
		}
	})
	if errors.Is(err, os.ErrNoHandle) {
		return fmt.Errorf("%w: no handle to reclaim pid %d", ErrProcessIdentityUnsupported, process.Pid)
	}
	return errors.Join(ErrProcessIdentityUnsupported, err, killErr)
}

// pollFD returns the descriptor exit observation waits on. The caller may use it until its
// observer returns; release happens only after a reap, and a reap starts only after that.
func (o *groupOwner) pollFD() (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.verified || o.closed {
		return -1, ErrProcessIdentityUnsupported
	}
	return o.fd, nil
}

func (o *groupOwner) release() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	if o.fd < 0 {
		return nil
	}
	fd := o.fd
	o.fd = -1
	return unix.Close(fd)
}
