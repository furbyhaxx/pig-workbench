//go:build linux

package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var errFixtureWatcher = errors.New("watcher fixture failed")

// resistantGroup launches a leader that dies on TERM together with a same-group descendant
// that ignores TERM. It reports the descendant once that handler is installed.
const resistantGroup = `sh -c 'trap "" TERM; : > "$1/ready"; while :; do sleep 0.05; done' sh "$1" & until [ -e "$1/ready" ]; do sleep 0.01; done; echo $! > "$1/pid"; exec sleep 300`

const resistantGroupLines = `sh -c 'trap "" TERM; echo ready; while :; do sleep 0.05; done' & echo $!; exec sleep 300`

// injectWatchFailure makes the exit observation fail at a chosen moment. The returned
// function breaks it and returns once the failure is in effect, so no test has to sleep.
func injectWatchFailure(t *testing.T) func() {
	t.Helper()
	previous := watchProcess
	gate := make(chan struct{})
	var failed sync.WaitGroup
	watchProcess = func(*groupOwner, *os.Process) (*exitWatcher, error) {
		watcher := &exitWatcher{signal: make(chan struct{}), done: make(chan struct{})}
		failed.Add(1)
		go func() {
			defer failed.Done()
			<-gate
			watcher.err = errFixtureWatcher
			close(watcher.done)
		}()
		return watcher, nil
	}
	t.Cleanup(func() { watchProcess = previous })
	return func() {
		close(gate)
		failed.Wait()
	}
}

func awaitFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
		if time.Now().After(deadline) {
			t.Fatalf("fixture did not publish %s: %v", path, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// startGroupFixture starts the resistant group and returns its leader and descendant pid.
func startGroupFixture(t *testing.T) (*Process, int) {
	t.Helper()
	dir := t.TempDir()
	process, err := StartProcess(Spec{Command: []string{resistantGroup, dir}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	descendant, err := strconv.Atoi(awaitFile(t, filepath.Join(dir, "pid")))
	if err != nil {
		t.Fatalf("invalid descendant pid: %v", err)
	}
	return process, descendant
}

// procState reports the state letter and start time of pid. A recycled pid carries a
// different start time, so a zombie is not mistaken for the survivor or the other way round.
func procState(pid int) (byte, string, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", false
	}
	stat := string(data)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, "", false
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 20 {
		return 0, "", false
	}
	return fields[0][0], fields[19], true
}

// requireStopped fails unless pid is gone or waiting to be reaped with its original start time.
func requireStopped(t *testing.T, pid int, startTicks string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, ticks, present := procState(pid)
		switch {
		case !present:
			return
		case ticks != startTicks:
			t.Fatalf("pid %d was recycled: start ticks %s, recorded %s", pid, ticks, startTicks)
		case state == 'Z' || state == 'X':
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant %d still executing in state %q", pid, state)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestWatcherFailureReclaimsRealGroup is the regression for a watcher failure that used to
// skip cleanup: the leader and its TERM-resistant descendant must die, the leader status
// must be the real one, and the original diagnostic must survive.
func TestWatcherFailureReclaimsRealGroup(t *testing.T) {
	failObservation := injectWatchFailure(t)
	output := &lineWriter{lines: make(chan string, 8)}
	process, err := StartProcess(Spec{Command: []string{resistantGroupLines}, Shell: true, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	var descendant int
	ready := false
	for descendant == 0 || !ready {
		select {
		case line := <-output.lines:
			if pid, err := strconv.Atoi(line); err == nil {
				descendant = pid
				continue
			}
			if line != "ready" {
				t.Fatalf("unexpected fixture line %q", line)
			}
			ready = true
		case <-time.After(10 * time.Second):
			t.Fatal("fixture did not report a TERM-resistant descendant")
		}
	}
	_, startTicks, present := procState(descendant)
	if !present {
		t.Fatalf("descendant %d missing before cleanup", descendant)
	}
	failObservation()

	if err := process.Kill(100 * time.Millisecond); !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("kill lost the watcher diagnostic: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exit, err := process.Wait(ctx)
	if !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("wait lost the watcher diagnostic: %v", err)
	}
	if !exit.Signaled || exit.Signal != "terminated" {
		t.Fatalf("leader exit = %+v, want the real termination status", exit)
	}
	requireStopped(t, descendant, startTicks)

	process.owner.mu.Lock()
	fd := process.owner.fd
	process.owner.mu.Unlock()
	if err := process.Close(); !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("close lost the watcher diagnostic: %v", err)
	}
	select {
	case <-process.outputDone:
	default:
		t.Fatal("output drain did not stop")
	}
	select {
	case <-process.watcher.done:
	default:
		t.Fatal("exit watcher was not joined")
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatalf("identity descriptor %d still open after close: %v", fd, err)
	}
}

// TestManagerWatcherFailureReclaimsRealGroup requires the manager to consume the recovered
// cleanup instead of vetoing the watcher error, then to release the slot it no longer owns.
func TestManagerWatcherFailureReclaimsRealGroup(t *testing.T) {
	failObservation := injectWatchFailure(t)
	manager, err := NewManager(ManagerOptions{MaxActive: 1, Grace: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	id, err := manager.Start(context.Background(), Spec{Command: []string{resistantGroup, dir}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	descendant, err := strconv.Atoi(awaitFile(t, filepath.Join(dir, "pid")))
	if err != nil {
		t.Fatalf("invalid descendant pid: %v", err)
	}
	_, startTicks, present := procState(descendant)
	if !present {
		t.Fatalf("descendant %d missing before cleanup", descendant)
	}
	failObservation()

	if err := manager.Cancel(id); !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("cancel lost the watcher diagnostic: %v", err)
	}
	state := managerWait(t, manager, id)
	if state.Running || state.Exit == nil || !state.Exit.Signaled || !strings.Contains(state.Error, errFixtureWatcher.Error()) {
		t.Fatalf("state = %+v, want a terminal recovered job with the watcher diagnostic", state)
	}
	requireStopped(t, descendant, startTicks)
	if _, err := manager.Start(context.Background(), Spec{Command: []string{"true"}}); err != nil {
		t.Fatalf("recovered cleanup did not release its slot: %v", err)
	}
	if err := manager.Close(); !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("close hid the recorded failure: %v", err)
	}
}

// TestExternalReapBeforeCleanup reaps the leader outside this package. The group identity must
// still reclaim the descendant, and no exit status may be invented for the leader.
func TestExternalReapBeforeCleanup(t *testing.T) {
	process, descendant := startGroupFixture(t)
	_, startTicks, _ := procState(descendant)

	if err := syscall.Kill(process.PID(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	var status syscall.WaitStatus
	if pid, err := syscall.Wait4(process.PID(), &status, 0, nil); err != nil || pid != process.PID() {
		t.Fatalf("external reap: pid=%d status=%v err=%v", pid, status, err)
	}
	if err := process.Kill(0); !errors.Is(err, ErrExitUnavailable) {
		t.Fatalf("kill error = %v, want an unavailable exit status", err)
	}
	requireStopped(t, descendant, startTicks)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exit, err := process.Wait(ctx)
	if !errors.Is(err, ErrExitUnavailable) {
		t.Fatalf("wait error = %v, want an unavailable exit status", err)
	}
	if exit != (Exit{}) {
		t.Fatalf("invented exit %+v for a leader reaped elsewhere", exit)
	}
}

// TestExternalReapDuringGrace reaps the leader while escalation waits. The group must still be
// reclaimed through its identity, and the leader status stays unavailable.
func TestExternalReapDuringGrace(t *testing.T) {
	process, descendant := startGroupFixture(t)
	_, startTicks, _ := procState(descendant)

	killDone := make(chan error, 1)
	go func() { killDone <- process.Kill(2 * time.Second) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		state, _, present := procState(process.PID())
		if present && state == 'Z' {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("leader did not terminate during the grace period")
		}
		time.Sleep(2 * time.Millisecond)
	}
	var status syscall.WaitStatus
	if pid, err := syscall.Wait4(process.PID(), &status, 0, nil); err != nil || pid != process.PID() {
		t.Fatalf("external reap: pid=%d status=%v err=%v", pid, status, err)
	}
	select {
	case err := <-killDone:
		if !errors.Is(err, ErrExitUnavailable) {
			t.Fatalf("kill error = %v, want an unavailable exit status", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("kill did not finish after the external reap")
	}
	requireStopped(t, descendant, startTicks)
}

// TestUnverifiedIdentityNeverSignals covers a missing capability or handle: nothing may reach
// a numeric group id, so the leader of a real child survives every attempt.
func TestUnverifiedIdentityNeverSignals(t *testing.T) {
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	unverified := &groupOwner{fd: -1, process: process.cmd.Process}

	if err := unverified.signal(syscall.SIGTERM); !errors.Is(err, ErrProcessIdentityUnsupported) {
		t.Fatalf("signal on an unverified identity = %v, want unsupported", err)
	}
	if err := unverified.gone(); !errors.Is(err, ErrProcessIdentityUnsupported) {
		t.Fatalf("group check without identity = %v, want unsupported", err)
	}
	if err := unverified.terminate(0); !errors.Is(err, ErrProcessIdentityUnsupported) {
		t.Fatalf("terminate without identity = %v, want an unsupported report", err)
	}
	if err := syscall.Kill(process.PID(), 0); err != nil {
		t.Fatalf("unverified cleanup signalled a numeric group id: %v", err)
	}
}

// TestCapabilityUnavailableRefusesStartup requires the capability probe to run before the user
// command: an unsupported kernel refuses startup without executing anything.
func TestCapabilityUnavailableRefusesStartup(t *testing.T) {
	previous := probeIdentity
	probeIdentity = func() error {
		return fmt.Errorf("%w: fixture without pidfd support", ErrProcessIdentityUnsupported)
	}
	t.Cleanup(func() { probeIdentity = previous })

	marker := filepath.Join(t.TempDir(), "ran")
	_, err := StartProcess(Spec{Command: []string{"touch", marker}})
	if !errors.Is(err, ErrProcessIdentityUnsupported) {
		t.Fatalf("start error = %v, want unsupported identity", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("user command ran without the required capability: %v", err)
	}
}

// TestStartupObservationFailureReclaimsGroup covers initialization failing after the child
// started: the group must be cleaned up instead of leaking an unsupervised child.
func TestStartupObservationFailureReclaimsGroup(t *testing.T) {
	dir := t.TempDir()
	previous := watchProcess
	watchProcess = func(*groupOwner, *os.Process) (*exitWatcher, error) {
		awaitFile(t, filepath.Join(dir, "pid"))
		return nil, errFixtureWatcher
	}
	t.Cleanup(func() { watchProcess = previous })

	_, err := StartProcess(Spec{Command: []string{resistantGroup, dir}, Shell: true})
	if !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("start error = %v, want the observation failure", err)
	}
	descendant, convErr := strconv.Atoi(awaitFile(t, filepath.Join(dir, "pid")))
	if convErr != nil {
		t.Fatalf("invalid descendant pid: %v", convErr)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, present := procState(descendant)
		if !present {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant %d survived failed initialization", descendant)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestConcurrentCancelAndCloseWatchedFailure runs both terminal paths against a broken
// observation and requires them to share one cleanup.
func TestConcurrentCancelAndCloseWatchedFailure(t *testing.T) {
	failObservation := injectWatchFailure(t)
	manager, err := NewManager(ManagerOptions{MaxActive: 1, Grace: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	id, err := manager.Start(context.Background(), Spec{Command: []string{resistantGroup, dir}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	descendant, err := strconv.Atoi(awaitFile(t, filepath.Join(dir, "pid")))
	if err != nil {
		t.Fatalf("invalid descendant pid: %v", err)
	}
	_, startTicks, _ := procState(descendant)
	failObservation()

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- manager.Cancel(id) }()
	go func() { <-start; results <- manager.Close() }()
	close(start)
	for range 2 {
		select {
		case err := <-results:
			if !errors.Is(err, errFixtureWatcher) {
				t.Fatalf("concurrent terminal error = %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("concurrent cancel and close did not finish")
		}
	}
	requireStopped(t, descendant, startTicks)
}

// TestFailedObservationLeavesWaitBounded requires Wait to stay out of a live child's way: no
// reaper may hold the process until the child happens to exit, so cleanup after a timed-out
// Wait must still terminate and reap it.
func TestFailedObservationLeavesWaitBounded(t *testing.T) {
	failObservation := injectWatchFailure(t)
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	failObservation()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := process.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("wait error = %v, want the watcher diagnostic and the deadline", err)
	}
	if err := syscall.Kill(process.PID(), 0); err != nil {
		t.Fatalf("wait terminated a child it could not observe: %v", err)
	}

	closed := make(chan error, 1)
	go func() { closed <- process.Close() }()
	select {
	case err := <-closed:
		if !errors.Is(err, errFixtureWatcher) {
			t.Fatalf("close error = %v, want the watcher diagnostic", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("close blocked behind a wait that could not observe exit")
	}
	exit, known := process.terminalExit()
	if !known || !exit.Signaled || exit.Signal == "" {
		t.Fatalf("exit = %+v known=%v, want the reaped termination status", exit, known)
	}
	if err := syscall.Kill(process.PID(), 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("leader survived cleanup: %v", err)
	}
}

// TestEscalationGateExcludesReap requires an admitted escalation to finish signalling before our
// own reap can complete: the leader stays a zombie through the grace and refuses only afterwards.
func TestEscalationGateExcludesReap(t *testing.T) {
	dir := t.TempDir()
	release := filepath.Join(dir, "release")
	// The leader ignores TERM and exits on its own once released, so the exit watcher fires
	// while the escalation is still inside its grace window.
	process, err := StartProcess(Spec{Command: []string{`trap "" TERM; until [ -e "$1" ]; do sleep 0.01; done; exit 0`, release}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })

	killDone := make(chan error, 1)
	go func() { killDone <- process.Kill(2 * time.Second) }()

	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitForState(t, process.PID(), 'Z')

	// The leader exited, but the escalation owns the gate: no reap may complete yet.
	waitReturned := make(chan error, 1)
	go func() {
		_, err := process.Wait(context.Background())
		waitReturned <- err
	}()
	// Deterministic barrier: the escalation owns the gate, the leader is still unreaped, and
	// a concurrent Wait therefore cannot have completed.
	if process.gate.TryLock() {
		process.gate.Unlock()
		t.Fatal("escalation does not hold the signal/reap gate")
	}
	if _, _, present := procState(process.PID()); !present {
		t.Fatal("leader was reaped while the escalation was still signalling")
	}
	// Signal joins the gate, so it is admitted only after the escalation finished and then
	// refuses: our reap completed first.
	signalled := make(chan error, 1)
	go func() { signalled <- process.Signal(syscall.SIGTERM) }()

	select {
	case err := <-killDone:
		if err != nil && !errors.Is(err, ErrGroupNotGone) {
			t.Fatalf("kill error = %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("escalation did not finish")
	}
	if err := <-signalled; !errors.Is(err, ErrReaped) {
		t.Fatalf("signal after our reap = %v, want ErrReaped", err)
	}
	select {
	case err := <-waitReturned:
		if err != nil {
			t.Fatalf("wait error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not return after the escalation")
	}
}

// TestWaitCancellationWhileGateHeld requires Wait to answer its context while an admitted
// escalation owns the gate: a reap needs the gate, so neither admitting nor joining it may
// hold the caller.
func TestWaitCancellationWhileGateHeld(t *testing.T) {
	process, err := StartProcess(Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	select {
	case <-process.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("leader did not exit")
	}

	process.gate.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	waited := make(chan error, 1)
	go func() {
		_, err := process.Wait(ctx)
		waited <- err
	}()
	select {
	case err := <-waited:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("wait error = %v, want its context deadline", err)
		}
	case <-time.After(10 * time.Second):
		process.gate.Unlock()
		t.Fatal("Wait blocked on the escalation gate instead of its context")
	}
	process.gate.Unlock()
}

// TestCachedWaitAfterFailedWatcherCleanup requires a Wait after Close to report the recorded
// reap: observing the identity again would only probe a descriptor Close released.
func TestCachedWaitAfterFailedWatcherCleanup(t *testing.T) {
	failObservation := injectWatchFailure(t)
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	failObservation()
	if err := process.Close(); !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("close error = %v, want the watcher diagnostic", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := process.Wait(ctx)
	if !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("wait lost the watcher diagnostic: %v", err)
	}
	if errors.Is(err, ErrProcessIdentityUnsupported) {
		t.Fatalf("wait probed the released identity: %v", err)
	}
	if !exit.Signaled || exit.Signal == "" {
		t.Fatalf("wait exit = %+v, want the recorded termination status", exit)
	}
}

// TestSignalHoldsGateAcrossDelivery requires Signal to hold the signal/reap gate across its
// refusal check and its delivery, so our own reap cannot complete in between.
func TestSignalHoldsGateAcrossDelivery(t *testing.T) {
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })

	delivering := make(chan struct{})
	releaseDelivery := make(chan struct{})
	var delivered atomic.Int32
	var releasing sync.Once
	previous := deliverGroupSignal
	deliverGroupSignal = func(owner *groupOwner, sig syscall.Signal) error {
		if delivered.Add(1) == 1 {
			close(delivering)
			<-releaseDelivery
		}
		return previous(owner, sig)
	}
	t.Cleanup(func() {
		releasing.Do(func() { close(releaseDelivery) })
		deliverGroupSignal = previous
	})

	signalled := make(chan error, 1)
	go func() { signalled <- process.Signal(syscall.SIGTERM) }()
	<-delivering
	// Deterministic barrier: the delivery is in flight, so Signal owns the gate and no reap
	// can publish while a delivery could still reach the group.
	if process.gate.TryLock() {
		process.gate.Unlock()
		t.Fatal("Signal does not hold the signal/reap gate across delivery")
	}
	if _, _, done := process.reapedResult(); done {
		t.Fatal("leader was reaped while a signal delivery was in flight")
	}
	releasing.Do(func() { close(releaseDelivery) })
	if err := <-signalled; err != nil {
		t.Fatalf("signal before our reap = %v", err)
	}

	if err := process.Kill(0); err != nil {
		t.Fatalf("kill = %v", err)
	}
	if err := process.Signal(syscall.SIGTERM); !errors.Is(err, ErrReaped) {
		t.Fatalf("signal after our reap = %v, want ErrReaped", err)
	}
	if got := delivered.Load(); got != 1 {
		t.Fatalf("signal reached the group %d times, want one delivery before our reap", got)
	}
}

// countPidfds reports how many pidfd descriptors the test process holds.
func countPidfds(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("list own descriptors: %v", err)
	}
	count := 0
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err == nil && strings.Contains(link, "pidfd") {
			count++
		}
	}
	return count
}

// TestObservationClosesItsDescriptor requires the private descriptor of a passive observation to
// be closed on cancellation and on success, so repeated waits cannot accumulate pidfds.
func TestObservationClosesItsDescriptor(t *testing.T) {
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	before := countPidfds(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = process.owner.observeExit(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled observation = %v, want the deadline", err)
	}
	if after := countPidfds(t); after != before {
		t.Fatalf("cancelled observation left %d pidfds open, want %d", after, before)
	}

	if err := process.owner.sendGroup(syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill leader: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	err = process.owner.observeExit(ctx)
	cancel()
	if err != nil {
		t.Fatalf("observation after exit = %v, want exit evidence", err)
	}
	if after := countPidfds(t); after != before {
		t.Fatalf("completed observation left %d pidfds open, want %d", after, before)
	}
}

// TestReleasedIdentityDoesNotRedirectObservation releases the identity while a passive
// observation is pending. The observer polls a private copy, so the released descriptor is
// neither reported as invalid nor replaced by whatever reuses its number.
func TestReleasedIdentityDoesNotRedirectObservation(t *testing.T) {
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	// The package reap is unavailable once the identity was released by hand.
	t.Cleanup(func() { _ = process.cmd.Wait() })
	before := countPidfds(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	observed := make(chan error, 1)
	go func() { observed <- process.owner.observeExit(ctx) }()
	// Barrier: the observer holds its private descriptor before it waits.
	deadline := time.Now().Add(10 * time.Second)
	for countPidfds(t) == before {
		if time.Now().After(deadline) {
			t.Fatal("observation did not copy the identity descriptor")
		}
		runtime.Gosched()
	}
	process.owner.mu.Lock()
	released := process.owner.fd
	process.owner.mu.Unlock()
	if err := process.owner.release(); err != nil {
		t.Fatalf("release identity: %v", err)
	}
	// Reuse the released number for a file that is always readable: an observation on the
	// shared descriptor would invent an exit.
	devnull, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer unix.Close(devnull)
	if err := unix.Dup3(devnull, released, unix.O_CLOEXEC); err != nil {
		t.Fatalf("reuse released descriptor %d: %v", released, err)
	}
	defer unix.Close(released)
	if err := process.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("terminate leader: %v", err)
	}
	select {
	case err := <-observed:
		if err != nil {
			t.Fatalf("observation after release = %v, want exit evidence", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("observation lost the identity after release")
	}
}

// waitForState waits until pid reports state, without polling faster than needed.
func waitForState(t *testing.T, pid int, want byte) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if state, _, present := procState(pid); present && state == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d did not reach state %q", pid, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
