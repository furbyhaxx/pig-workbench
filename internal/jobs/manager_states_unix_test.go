//go:build unix

package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestManagerRetainsACommandThatFailedToStart pins the failedToStart state: the reserved ID
// answers status, list and output, and frees its slot.
func TestManagerRetainsACommandThatFailedToStart(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{MaxActive: 1})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"true"}, Dir: filepath.Join(t.TempDir(), "gone")})
	if !errors.Is(err, ErrFailedToStart) || id == "" {
		t.Fatalf("start = %q, %v", id, err)
	}
	listed := manager.List()
	if len(listed) != 1 || !listed[0].FailedToStart || listed[0].Running || listed[0].Exit != nil || listed[0].Error == "" {
		t.Fatalf("list = %+v", listed)
	}
	if state := managerWait(t, manager, id); !state.FailedToStart {
		t.Fatalf("wait = %+v", state)
	}
	if data, _, _, _, err := manager.Output(id, 0, 100); err != nil || len(data) != 0 {
		t.Fatalf("output = %q, %v", data, err)
	}
	for name, err := range map[string]error{"cancel": manager.Cancel(id), "eof": manager.EOF(id), "signal": manager.Signal(id, syscall.SIGTERM)} {
		if !errors.Is(err, ErrFailedToStart) {
			t.Errorf("%s on a job that never ran = %v", name, err)
		}
	}
	next, err := manager.Start(context.Background(), Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatalf("the failed start kept its slot: %v", err)
	}
	managerWait(t, manager, next)
}

// TestManagerReportsStoppingUntilTheExitIsObserved pins the stopping state.
func TestManagerReportsStoppingUntilTheExitIsObserved(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{Grace: 400 * time.Millisecond})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"sh", "-c", "trap '' TERM; echo ready; while :; do sleep 0.05; done"}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, _, _, _, _ := manager.Output(id, 0, 100); len(data) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the job never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state, _ := manager.Status(id); state.Stopping || !state.Running {
		t.Fatalf("before the stop request: %+v", state)
	}
	cancelled := make(chan error, 1)
	go func() { cancelled <- manager.Cancel(id) }()
	seen := false
	for !seen && time.Now().Before(deadline) {
		state, err := manager.Status(id)
		if err != nil {
			t.Fatal(err)
		}
		seen = state.Stopping && state.Running
		if !state.Running {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !seen {
		t.Fatal("a job ignoring TERM never reported stopping during its grace period")
	}
	if err := <-cancelled; err != nil {
		t.Fatal(err)
	}
	if state, _ := manager.Status(id); state.Stopping || state.Running || !state.Cancelled {
		t.Fatalf("after the kill: %+v", state)
	}
}
