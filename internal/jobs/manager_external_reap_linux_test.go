package jobs

import (
	"context"
	"errors"
	"syscall"
	"testing"
)

// TestManagerUnadmittedExternalReapRemainsOwned covers a leader reaped outside this package
// before admission finished: no status exists, so the slot and the failure must be retained.
func TestManagerUnadmittedExternalReapRemainsOwned(t *testing.T) {
	managerTestUnix(t)
	manager, err := NewManager(ManagerOptions{MaxActive: 1})
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.active = 1
	manager.mu.Unlock()
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(process.PID(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	var status syscall.WaitStatus
	if pid, err := syscall.Wait4(process.PID(), &status, 0, nil); err != nil || pid != process.PID() {
		t.Fatalf("external reap: pid=%d status=%v err=%v", pid, status, err)
	}
	cleanupErr, unresolved := manager.cleanupAdmission(process, "unadmitted", context.Canceled)
	if !unresolved || !errors.Is(cleanupErr, context.Canceled) || !errors.Is(cleanupErr, ErrExitUnavailable) || !errors.Is(cleanupErr, ErrJobCleanupIncomplete) {
		t.Fatalf("unadmitted cleanup = %v, unresolved=%v", cleanupErr, unresolved)
	}
	manager.mu.Lock()
	retained := len(manager.unresolved) == 1 && manager.unresolved[0] == process && manager.active == 1
	manager.mu.Unlock()
	if !retained {
		t.Fatal("unresolved process or active slot was forgotten")
	}
	if _, err := manager.Start(context.Background(), Spec{Command: []string{"true"}}); !errors.Is(err, ErrJobLimit) {
		t.Fatalf("unresolved admission released capacity: %v", err)
	}
	if err := manager.Close(); !errors.Is(err, ErrJobCleanupIncomplete) {
		t.Fatalf("close hid unresolved cleanup: %v", err)
	}
}

// TestManagerReportsUnknownWhenTheLeaderStatusWasLost pins the unknown state: a leader reaped
// outside this package leaves no status, so the job is neither running for certain nor exited.
func TestManagerReportsUnknownWhenTheLeaderStatusWasLost(t *testing.T) {
	managerTestUnix(t)
	manager, err := NewManager(ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Start(context.Background(), Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.job(id)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the supervisor before its reap so the outside reap wins deterministically.
	job.selectionMu.Lock()
	pid := job.process.PID()
	killErr := syscall.Kill(pid, syscall.SIGKILL)
	var status syscall.WaitStatus
	reaped, waitErr := syscall.Wait4(pid, &status, 0, nil)
	job.selectionMu.Unlock()
	if killErr != nil || waitErr != nil || reaped != pid {
		t.Fatalf("external reap: kill=%v pid=%d err=%v", killErr, reaped, waitErr)
	}
	state := managerWait(t, manager, id)
	if !state.Unknown || !state.Running || state.Stopping || state.Exit != nil || state.Error == "" {
		t.Fatalf("state = %+v", state)
	}
	if listed := manager.List(); len(listed) != 1 || !listed[0].Unknown {
		t.Fatalf("list = %+v", listed)
	}
	if err := manager.Close(); !errors.Is(err, ErrJobCleanupIncomplete) {
		t.Fatalf("close hid the unresolved job: %v", err)
	}
}
