package fileedit

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSweepLocksRemovesOnlyOldLocksNothingHolds(t *testing.T) {
	dir := lockDir(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour)
	age := func(identity string) string {
		release, err := lockAll(ctx, dir, []target{{source: identity}})
		if err != nil {
			t.Fatal(err)
		}
		release()
		path := lockPath(dir, identity)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		return path
	}
	idle := age("/idle")
	held := age("/held")
	releaseFresh, err := lockAll(ctx, dir, []target{{source: "/fresh"}})
	if err != nil {
		t.Fatal(err)
	}
	releaseFresh()
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, old, old); err != nil {
		t.Fatal(err)
	}
	releaseHeld, err := lockAll(ctx, dir, []target{{source: "/held"}})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHeld()
	if err := os.Chtimes(held, old, old); err != nil {
		t.Fatal(err)
	}

	if removed := SweepLocks(ctx, dir, 24*time.Hour, time.Now()); removed != 1 {
		t.Fatalf("removed %d lock files, want 1", removed)
	}
	if _, err := os.Stat(idle); !os.IsNotExist(err) {
		t.Fatalf("idle lock survived: %v", err)
	}
	for _, path := range []string{held, lockPath(dir, "/fresh"), other} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was removed: %v", filepath.Base(path), err)
		}
	}
}

func TestLockingMarksTheLockFileAsUsed(t *testing.T) {
	dir := lockDir(t)
	release, err := lockAll(context.Background(), dir, []target{{source: "/a"}})
	if err != nil {
		t.Fatal(err)
	}
	release()
	path := lockPath(dir, "/a")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	release, err = lockAll(context.Background(), dir, []target{{source: "/a"}})
	if err != nil {
		t.Fatal(err)
	}
	release()
	if removed := SweepLocks(context.Background(), dir, 24*time.Hour, time.Now()); removed != 0 {
		t.Fatalf("a lock used a moment ago was swept (%d)", removed)
	}
}

func TestEditsAndSweepsNeverShareALock(t *testing.T) {
	dir := lockDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stop := make(chan struct{})
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		for {
			select {
			case <-stop:
				return
			default:
				// A future "now" makes every unheld lock old enough.
				SweepLocks(ctx, dir, time.Hour, time.Now().Add(2*time.Hour))
			}
		}
	}()
	var inside atomic.Int32
	done := make(chan error, 4)
	for range 4 {
		go func() {
			for range 150 {
				release, err := lockAll(ctx, dir, []target{{source: "/contended"}})
				if err != nil {
					done <- err
					return
				}
				if inside.Add(1) != 1 {
					release()
					done <- os.ErrExist
					return
				}
				time.Sleep(200 * time.Microsecond)
				inside.Add(-1)
				release()
			}
			done <- nil
		}()
	}
	for range 4 {
		if err := <-done; err != nil {
			t.Fatalf("lock was not exclusive or failed: %v", err)
		}
	}
	close(stop)
	<-swept
}
