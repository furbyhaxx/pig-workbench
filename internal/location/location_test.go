package location

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransitionPersistsBeforeActivation(t *testing.T) {
	root := t.TempDir()
	next := filepath.Join(root, "next")
	if err := os.Mkdir(next, 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), root, nil)
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
	if change.Current.ProjectRoot != change.Current.Cwd {
		t.Fatalf("pure location root did not follow cwd: %s", change.Current.ProjectRoot)
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
	m, err := New(context.Background(), t.TempDir(), nil)
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
	m, err := New(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	old := m.Current()
	if _, err := m.Transition(context.Background(), old.Cwd, false, func(Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
	previous := m.Current().Revision
	if err := m.RestoreContext(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if m.Current().Revision <= previous {
		t.Fatal("restore reused an old location revision")
	}
}

func TestRestoreMissingLocationDoesNotFallback(t *testing.T) {
	root := t.TempDir()
	m, err := New(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Current()
	s.Cwd = filepath.Join(root, "gone")
	s.Revision = 8
	if err := m.RestoreContext(context.Background(), s); err != nil {
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

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func nestedLookup(roots ...string) ProjectLookup {
	return func(_ context.Context, dir string) (string, error) {
		for _, root := range roots {
			if containsDir(root, dir) {
				return root, nil
			}
		}
		return "", nil
	}
}

func TestProjectRootFollowsProjectAcrossTransition(t *testing.T) {
	first, second := tempDir(t), tempDir(t)
	start := filepath.Join(first, "src", "deep")
	next := filepath.Join(second, "sub")
	for _, dir := range []string{start, next} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(context.Background(), start, nestedLookup(first, second))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Current().ProjectRoot; got != first {
		t.Fatalf("initial root %q does not bind %q", got, first)
	}
	change, err := m.Transition(context.Background(), next, false, func(c Change) error {
		if c.Current.ProjectRoot != second {
			t.Fatalf("persisted root %q, want %q", c.Current.ProjectRoot, second)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Current() != change.Current {
		t.Fatal("activated snapshot differs from persisted change")
	}
	if m.Current().Cwd != next || m.Current().ProjectRoot != second || m.Current().Origin != start {
		t.Fatalf("transition bound %+v", m.Current())
	}
	if _, err := m.Transition(context.Background(), "", true, func(Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if m.Current().Cwd != start || m.Current().ProjectRoot != first {
		t.Fatalf("reset bound %+v", m.Current())
	}
}

func TestProjectRootValidation(t *testing.T) {
	base, outside := tempDir(t), tempDir(t)
	inner := filepath.Join(base, "child")
	if err := os.Mkdir(inner, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), inner, func(context.Context, string) (string, error) { return outside, nil }); err == nil {
		t.Fatal("root outside the location was accepted")
	}
	if _, err := New(context.Background(), inner, func(context.Context, string) (string, error) { return "base", nil }); err == nil {
		t.Fatal("relative root was accepted")
	}
	lookupErr := errors.New("registry unavailable")
	if _, err := New(context.Background(), inner, func(context.Context, string) (string, error) { return "", lookupErr }); !errors.Is(err, lookupErr) {
		t.Fatalf("lookup failure not reported: %v", err)
	}
}

func TestTransitionKeepsSnapshotWhenRootLookupFails(t *testing.T) {
	root, target := tempDir(t), tempDir(t)
	var fail atomic.Bool
	lookup := func(_ context.Context, dir string) (string, error) {
		if fail.Load() {
			return "", errors.New("registry offline")
		}
		return root, nil
	}
	m, err := New(context.Background(), root, lookup)
	if err != nil {
		t.Fatal(err)
	}
	before := m.Current()
	fail.Store(true)
	if _, err := m.Transition(context.Background(), target, false, func(Change) error { t.Fatal("persisted without a root"); return nil }); err == nil {
		t.Fatal("transition ignored the failed root lookup")
	}
	if m.Current() != before {
		t.Fatal("failed root lookup changed the location")
	}
}

func TestBlockedProjectLookupDoesNotHoldMutex(t *testing.T) {
	root, target := tempDir(t), tempDir(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	lookup := func(ctx context.Context, dir string) (string, error) {
		if calls.Add(1) == 1 {
			return root, nil
		}
		close(entered)
		select {
		case <-release:
			return root, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	m, err := New(context.Background(), root, lookup)
	if err != nil {
		t.Fatal(err)
	}
	before := m.Current()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := m.Transition(ctx, target, false, func(Change) error { t.Error("persisted while the lookup was blocked"); return nil })
		done <- err
	}()
	<-entered
	read := make(chan Snapshot, 1)
	go func() { read <- m.Current() }()
	select {
	case <-read:
	case <-time.After(2 * time.Second):
		t.Fatal("manager mutex was held during the project lookup")
	}
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lookup returned %v", err)
	}
	if m.Current() != before {
		t.Fatal("cancelled lookup activated the location")
	}
}

func TestRestoreContextRefreshesStaleRoot(t *testing.T) {
	first, second := tempDir(t), tempDir(t)
	start, target := filepath.Join(first, "src"), filepath.Join(second, "pkg")
	for _, dir := range []string{start, target} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(context.Background(), start, nestedLookup(first, second))
	if err != nil {
		t.Fatal(err)
	}
	stale := m.Current()
	stale.Cwd = target
	stale.DisplayCwd = target
	stale.ProjectRoot = first
	stale.Revision = 9
	if err := m.RestoreContext(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if m.Current().ProjectRoot != second {
		t.Fatalf("restored stale root %q", m.Current().ProjectRoot)
	}
	if m.Current().Revision != 9 || m.Current().Origin != start {
		t.Fatalf("restore changed revision or origin: %+v", m.Current())
	}
}

func TestRestoreMissingCwdSkipsProjectLookup(t *testing.T) {
	root := tempDir(t)
	var consulted atomic.Int64
	lookup := func(context.Context, string) (string, error) {
		consulted.Add(1)
		return root, nil
	}
	m, err := New(context.Background(), root, lookup)
	if err != nil {
		t.Fatal(err)
	}
	consulted.Store(0)
	s := m.Current()
	s.Cwd = filepath.Join(root, "gone")
	s.ProjectRoot = tempDir(t)
	s.Revision = 8
	if err := m.RestoreContext(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if consulted.Load() != 0 {
		t.Fatal("project lookup ran for a missing cwd")
	}
	if m.Current().Available {
		t.Fatal("missing cwd is available")
	}
	if m.Current().ProjectRoot != s.ProjectRoot {
		t.Fatal("unavailable location substituted its project root")
	}
}
