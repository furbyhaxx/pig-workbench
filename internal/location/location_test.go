package location

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransitionPersistsBeforeActivation(t *testing.T) {
	root := t.TempDir()
	next := filepath.Join(root, "next")
	if err := os.Mkdir(next, 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	original := m.Current()
	failure := errors.New("disk full")
	if _, err := m.Transition(context.Background(), next, false, func(Change) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if m.Current() != original {
		t.Fatal("failed persistence activated location")
	}
	captured, release, err := m.Acquire("read", false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	process, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	change, err := m.Transition(context.Background(), "next", false, func(c Change) error {
		if m.Current() != original {
			t.Fatal("activated before persistence")
		}
		if c.Current.Revision != original.Revision+1 {
			t.Fatal("revision not incremented")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Current() != change.Current {
		t.Fatal("transition not active")
	}
	got, err := captured.Resolve("file")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "file") {
		t.Fatalf("read lease moved: %s", got)
	}
	now, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if now != process {
		t.Fatal("process cwd changed")
	}
}

func TestMutationLeaseAndTransitionTimeout(t *testing.T) {
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, release, err := m.Acquire("write-1", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = m.Transition(ctx, m.Current().Cwd, false, func(Change) error { t.Fatal("persisted while mutation active"); return nil })
	var busy *BusyError
	if !errors.As(err, &busy) || len(busy.Operations) != 1 || busy.Operations[0] != "write-1" {
		t.Fatalf("missing lease conflict: %v", err)
	}
	release()
	release()
	if _, err := m.Transition(context.Background(), m.Current().Cwd, true, func(Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestRestoringEarlierFactKeepsRevisionMonotonic(t *testing.T) {
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := m.Current()
	if _, err := m.Transition(context.Background(), old.Cwd, false, func(Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
	previous := m.Current().Revision
	if err := m.Restore(old); err != nil {
		t.Fatal(err)
	}
	if m.Current().Revision <= previous {
		t.Fatal("restore reused an old location revision")
	}
}

func TestRestoreMissingLocationDoesNotFallback(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Current()
	s.Cwd = filepath.Join(root, "gone")
	s.Revision = 8
	if err := m.Restore(s); err != nil {
		t.Fatal(err)
	}
	if m.Current().Available {
		t.Fatal("missing cwd is available")
	}
	if _, _, err := m.Acquire("write", true); err == nil {
		t.Fatal("relative execution silently fell back")
	}
	if _, err := m.Transition(context.Background(), ".", false, func(Change) error { return nil }); err == nil {
		t.Fatal("relative cd silently fell back")
	}
	if _, err := m.Transition(context.Background(), "", true, func(Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if m.Current().Cwd != root || !m.Current().Available {
		t.Fatal("reset failed")
	}
}
