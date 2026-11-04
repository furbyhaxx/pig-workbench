// Package location resolves paths against immutable, revisioned working locations.
package location

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Snapshot is the captured local working location of an operation.
type Snapshot struct {
	Authority   string `json:"authority"`
	Cwd         string `json:"cwd"`
	DisplayCwd  string `json:"displayCwd"`
	ProjectRoot string `json:"projectRoot"`
	Origin      string `json:"origin"`
	Revision    uint64 `json:"revision"`
	Available   bool   `json:"available"`
}

// Change is persisted before its current snapshot becomes active.
type Change struct {
	SchemaVersion int      `json:"schemaVersion"`
	Previous      Snapshot `json:"previous"`
	Current       Snapshot `json:"current"`
	Reason        string   `json:"reason"`
}

// BusyError reports operations holding mutation leases.
type BusyError struct{ Operations []string }

// Error describes why a location transition could not acquire its lease.
func (e *BusyError) Error() string {
	return "locationBusy: mutations in progress: " + strings.Join(e.Operations, ", ")
}

// ProjectLookup resolves the project root containing a canonical directory.
// An empty root means no project was identified and the directory is its own root.
type ProjectLookup func(context.Context, string) (string, error)

// Manager owns one session's local location and active mutation leases.
type Manager struct {
	mu        sync.Mutex
	current   Snapshot
	changing  bool
	mutations map[string]struct{}
	changed   chan struct{}
	lookup    ProjectLookup // immutable after construction
}

// New resolves the host origin and its project root without changing process cwd.
// A nil lookup keeps the location's own directory as its project root.
func New(ctx context.Context, origin string, lookup ProjectLookup) (*Manager, error) {
	cwd, err := canonicalDir(origin)
	if err != nil {
		return nil, err
	}
	root, err := resolveProjectRoot(ctx, lookup, cwd)
	if err != nil {
		return nil, err
	}
	return &Manager{current: Snapshot{Authority: "local", Cwd: cwd, DisplayCwd: cwd, ProjectRoot: root, Origin: cwd, Revision: 1, Available: true}, mutations: make(map[string]struct{}), changed: make(chan struct{}), lookup: lookup}, nil
}

// Current returns a value copy of the active location.
func (m *Manager) Current() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.current }

// Resolve resolves a local path against this snapshot, never the process cwd.
func (s Snapshot) Resolve(path string) (string, error) {
	if !s.Available {
		return "", fmt.Errorf("stored location %q is unavailable; use cd with an absolute path or reset", s.Cwd)
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("path contains NUL")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.Cwd, path)
	}
	return filepath.Clean(path), nil
}

// Acquire captures a location and optionally holds a mutation lease until release.
func (m *Manager) Acquire(id string, mutation bool) (Snapshot, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.current.Available {
		return Snapshot{}, nil, fmt.Errorf("location %q is unavailable; use cd reset", m.current.Cwd)
	}
	if mutation {
		if id == "" {
			return Snapshot{}, nil, errors.New("mutation operation ID is required")
		}
		if m.changing {
			return Snapshot{}, nil, &BusyError{Operations: []string{"location transition"}}
		}
		if _, ok := m.mutations[id]; ok {
			return Snapshot{}, nil, fmt.Errorf("duplicate mutation operation %q", id)
		}
		m.mutations[id] = struct{}{}
	}
	release := sync.OnceFunc(func() {
		if !mutation {
			return
		}
		m.mu.Lock()
		delete(m.mutations, id)
		m.signal()
		m.mu.Unlock()
	})
	return m.current, release, nil
}

// Transition validates, waits for mutating leases, binds the project root, persists and activates a location.
// persist must atomically record the change or return an error without recording it.
func (m *Manager) Transition(ctx context.Context, path string, reset bool, persist func(Change) error) (Change, error) {
	m.mu.Lock()
	if m.changing {
		m.mu.Unlock()
		return Change{}, &BusyError{Operations: []string{"location transition"}}
	}
	m.changing = true
	previous := m.current
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.changing = false; m.signal(); m.mu.Unlock() }()
	if reset {
		path = previous.Origin
	}
	if !filepath.IsAbs(path) {
		resolved, err := previous.Resolve(path)
		if err != nil {
			return Change{}, err
		}
		path = resolved
	}
	cwd, err := canonicalDir(path)
	if err != nil {
		return Change{}, err
	}
	if err := ctx.Err(); err != nil {
		return Change{}, err
	}
	for {
		m.mu.Lock()
		if len(m.mutations) == 0 {
			m.mu.Unlock()
			break
		}
		wait := m.changed
		m.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			m.mu.Lock()
			ids := make([]string, 0, len(m.mutations))
			for id := range m.mutations {
				ids = append(ids, id)
			}
			m.mu.Unlock()
			sort.Strings(ids)
			return Change{}, errors.Join(&BusyError{Operations: ids}, ctx.Err())
		}
	}
	root, err := resolveProjectRoot(ctx, m.lookup, cwd)
	if err != nil {
		return Change{}, err
	}
	if previous.Revision >= 1<<53-1 {
		return Change{}, errors.New("location revision exhausted")
	}
	current := previous
	current.Cwd = cwd
	current.DisplayCwd = cwd
	current.ProjectRoot = root
	current.Revision++
	current.Available = true
	change := Change{SchemaVersion: 1, Previous: previous, Current: current, Reason: "cd"}
	if reset {
		change.Reason = "reset"
	}
	if err := ctx.Err(); err != nil {
		return Change{}, err
	}
	if err := persist(change); err != nil {
		return Change{}, fmt.Errorf("persist location: %w", err)
	}
	m.mu.Lock()
	m.current = current
	m.mu.Unlock()
	return change, nil
}

// RestoreContext replays a persisted local snapshot without silently replacing an unavailable cwd.
// A project lookup refreshes a stale stored root; a failed or cancelled lookup leaves the location unchanged.
func (m *Manager) RestoreContext(ctx context.Context, s Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Authority != "local" || !filepath.IsAbs(s.Cwd) || !filepath.IsAbs(s.Origin) || s.Revision == 0 || s.Revision >= 1<<53 {
		return errors.New("invalid stored location")
	}
	cwd, err := canonicalDir(s.Cwd)
	s.Available = err == nil
	if s.Available {
		root, err := resolveProjectRoot(ctx, m.lookup, cwd)
		if err != nil {
			return err
		}
		s.ProjectRoot = root
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.changing || len(m.mutations) != 0 {
		return &BusyError{Operations: []string{"active operations"}}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == m.current {
		return nil
	}
	if s.Revision <= m.current.Revision {
		if m.current.Revision >= 1<<53-1 {
			return errors.New("location revision exhausted")
		}
		s.Revision = m.current.Revision + 1
	}
	m.current = s
	m.signal()
	return nil
}

func (m *Manager) signal() { close(m.changed); m.changed = make(chan struct{}) }

func resolveProjectRoot(ctx context.Context, lookup ProjectLookup, cwd string) (string, error) {
	if lookup == nil {
		return cwd, nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := lookup(ctx, cwd)
	if err != nil {
		return "", fmt.Errorf("resolve project root for %q: %w", cwd, err)
	}
	if root == "" {
		return "", errors.New("project lookup returned no absolute root")
	}
	root, err = canonicalDir(root)
	if err != nil {
		return "", fmt.Errorf("project root for %q: %w", cwd, err)
	}
	if !containsDir(root, cwd) {
		return "", fmt.Errorf("project root %q does not contain location %q", root, cwd)
	}
	return root, nil
}

// containsDir reports whether the canonical dir is root or lies below it.
func containsDir(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalDir(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("location must be absolute: %q", path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve location: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat location: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("location %q is not a directory", path)
	}
	return filepath.Clean(path), nil
}
