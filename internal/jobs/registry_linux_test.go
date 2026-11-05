//go:build linux

package jobs

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func liveGroup(t *testing.T) (int, string) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 300; true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	identity, err := processIdentity(pid)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return remnantAlive(Record{PID: pid, Identity: identity}) })
	return pid, identity
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func groupRunning(pid int) bool {
	return syscall.Kill(-pid, 0) == nil
}

// TestSweepReclaimsAnotherGeneration proves a job started by a runtime that is
// gone is terminated by the next start.
func TestSweepReclaimsAnotherGeneration(t *testing.T) {
	registry, err := NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pid, identity := liveGroup(t)
	stale := Record{JobID: "job-a", SessionID: "session", Generation: "gen-1", PID: pid, Identity: identity}
	if err := registry.Add(stale); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Killed) != 1 || result.Killed[0].JobID != "job-a" {
		t.Fatalf("stale job was not reclaimed: %+v", result)
	}
	waitFor(t, func() bool { return !remnantAlive(stale) })
	if _, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"}); err != nil {
		t.Fatal(err)
	}
}

// TestSweepKeepsTheCurrentGeneration proves a live runtime's own jobs survive.
func TestSweepKeepsTheCurrentGeneration(t *testing.T) {
	registry, err := NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pid, identity := liveGroup(t)
	owner, err := registry.HoldOwner("gen-2")
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := registry.Add(Record{JobID: "job-b", SessionID: "session", Generation: "gen-2", PID: pid, Identity: identity}); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Retained) != 1 || len(result.Killed) != 0 {
		t.Fatalf("own generation was reclaimed: %+v", result)
	}
	if !groupRunning(pid) {
		t.Fatal("a retained job was terminated")
	}
}

// TestSweepKeepsAnotherLiveSession proves liveness, not session identity, decides
// reclamation: a live runtime's jobs are never signalled, even for another session.
func TestSweepKeepsAnotherLiveSession(t *testing.T) {
	registry, err := NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pid, identity := liveGroup(t)
	other, err := registry.HoldOwner("gen-other")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := registry.Add(Record{JobID: "job-c", SessionID: "other-session", Generation: "gen-other", PID: pid, Identity: identity}); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Killed) != 0 || len(result.Retained) != 1 {
		t.Fatalf("a live runtime's job was touched: %+v", result)
	}
	if !groupRunning(pid) {
		t.Fatal("a live runtime's job was terminated")
	}
}

// TestSweepDropsDeadAndUnreadableRecords proves records do not accumulate.
func TestSweepDropsDeadAndUnreadableRecords(t *testing.T) {
	directory := t.TempDir()
	registry, err := NewRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(Record{JobID: "job-d", SessionID: "session", Generation: "gen-1", PID: 999999, Identity: "0"}); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 1 || len(result.Killed) != 0 {
		t.Fatalf("a dead job was not dropped cleanly: %+v", result)
	}
	entries, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"})
	if err != nil || len(entries.Killed)+len(entries.Removed)+len(entries.Retained) != 0 {
		t.Fatalf("records survived a reclaim: %+v %v", entries, err)
	}
}

// TestRemoveDropsOneRecord proves a finished job leaves nothing behind.
func TestRemoveDropsOneRecord(t *testing.T) {
	registry, err := NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(Record{JobID: "job-e", SessionID: "session", Generation: "gen-1", PID: 4242, Identity: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove("session", "job-e"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove("session", "job-e"); err != nil {
		t.Fatalf("removing a missing record must be idempotent: %v", err)
	}
	result, err := registry.Sweep(Owner{SessionID: "session", Generation: "gen-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Retained) != 0 || len(result.Removed) != 0 || len(result.Killed) != 0 {
		t.Fatalf("a removed record was still swept: %+v", result)
	}
}
