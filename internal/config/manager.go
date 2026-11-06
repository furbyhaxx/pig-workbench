package config

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"
)

// Paths are host-resolved settings files, anchored independently of mutable cd.
type Paths struct {
	User    string
	Project string
}

// Reconcile applies live consumers before a candidate snapshot is published.
type Reconcile func(ctx context.Context, previous, next *Snapshot) error

// Status identifies whether a valid snapshot is active or awaiting repair.
type Status struct {
	Ready    bool   `json:"ready"`
	Stale    bool   `json:"stale"`
	Error    string `json:"error,omitempty"`
	Revision uint64 `json:"configRevision"`
}

// Manager serializes configuration transitions and publishes immutable snapshots.
type Manager struct {
	gate        chan struct{}
	mu          sync.RWMutex
	paths       Paths
	definitions map[string]Definition
	current     *Snapshot
	runtime     map[string]any
	reconcile   Reconcile
	lastErr     error
}

// New constructs a loader without reading files or assuming project trust.
func New(paths Paths, reconcile Reconcile) (*Manager, error) {
	if !filepath.IsAbs(paths.User) || !filepath.IsAbs(paths.Project) {
		return nil, errors.New("configuration paths must be absolute host-resolved paths")
	}
	return &Manager{gate: make(chan struct{}, 1), paths: paths, definitions: definitions(), runtime: make(map[string]any), reconcile: reconcile}, nil
}

func (m *Manager) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case m.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-m.gate
			return nil, err
		}
		return func() { <-m.gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Load reads all permitted layers; a failure retains the previous snapshot as stale.
func (m *Manager) Load(ctx context.Context, trusted bool) error {
	release, err := m.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	documents, err := m.readLayers(trusted)
	if err == nil {
		_, err = m.publish(ctx, documents)
	}
	if err != nil {
		m.recordError(err)
	}
	return err
}

// Current returns the active snapshot, or an actionable admission error when stale.
func (m *Manager) Current() (*Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastErr != nil {
		return m.current, fmt.Errorf("configuration unavailable or stale: %w", m.lastErr)
	}
	if m.current == nil {
		return nil, errors.New("configuration has not loaded")
	}
	return m.current, nil
}

// Inspect exposes last-valid configuration for diagnostics without permitting work.
func (m *Manager) Inspect() (*Snapshot, Status) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := Status{Ready: m.current != nil, Stale: m.lastErr != nil}
	if m.current != nil {
		status.Revision = m.current.revision
	}
	if m.lastErr != nil {
		status.Error = m.lastErr.Error()
	}
	return m.current, status
}

func (m *Manager) recordError(err error) { m.mu.Lock(); m.lastErr = err; m.mu.Unlock() }

func (m *Manager) readLayers(trusted bool) (map[Scope]*document, error) {
	user, err := readDocument(m.paths.User, m.definitions)
	if err != nil {
		return nil, err
	}
	layers := map[Scope]*document{User: user, Runtime: {values: copyValues(m.runtime)}}
	if trusted {
		project, err := readDocument(m.paths.Project, m.definitions)
		if err != nil {
			return nil, err
		}
		layers[Project] = project
	}
	return layers, nil
}

func (m *Manager) publish(ctx context.Context, documents map[Scope]*document) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	previous := m.current
	revision := uint64(1)
	if previous != nil {
		revision = previous.revision + 1
	}
	m.mu.RUnlock()
	if revision >= 1<<53 {
		return nil, errors.New("configuration revision exhausted")
	}
	candidate, err := compose(documents, revision)
	if err != nil {
		return nil, err
	}
	if m.reconcile != nil {
		if err := m.reconcile(ctx, previous, candidate); err != nil {
			return nil, fmt.Errorf("settings saved but consumer reconciliation failed: %w", err)
		}
	}
	m.mu.Lock()
	m.current = candidate
	m.lastErr = nil
	m.mu.Unlock()
	return candidate, nil
}

// Edit sets one known value or removes its override in the selected scope.
type Edit struct {
	Key   string
	Value any
	Reset bool
}

// SaveResult distinguishes durable writes, snapshot publication and pending features.
type SaveResult struct {
	Scope     Scope    `json:"scope"`
	Saved     bool     `json:"saved"`
	Published bool     `json:"published"`
	Revision  uint64   `json:"configRevision"`
	Changed   []string `json:"changed"`
	Pending   []string `json:"pending"`
}

// Save merges selected changes against a captured baseline and rechecks project trust.
// trusted must read the host's current trust state and is not retained after return.
func (m *Manager) Save(ctx context.Context, scope Scope, base *Snapshot, edits []Edit, trusted func() (bool, error)) (SaveResult, error) {
	result := SaveResult{Scope: scope}
	if scope != User && scope != Project && scope != Runtime {
		return result, errors.New("save scope must be user, project or runtime")
	}
	if base == nil {
		return result, errors.New("capture a valid configuration baseline before saving")
	}
	if len(edits) == 0 {
		return result, errors.New("no setting changes to save")
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	release, err := m.acquire(bounded)
	if err != nil {
		return result, err
	}
	defer release()
	isTrusted := func() (bool, error) {
		if trusted == nil {
			return false, nil
		}
		return trusted()
	}
	projectTrusted, err := isTrusted()
	if err != nil {
		return result, err
	}
	if scope == Project && !projectTrusted {
		return result, errors.New("project configuration save requires current host trust")
	}
	normalized, err := m.validateEdits(edits)
	if err != nil {
		return result, err
	}
	var unlock func() error
	var target string
	if scope != Runtime {
		path := m.paths.User
		if scope == Project {
			path = m.paths.Project
		}
		target, unlock, err = lockTarget(bounded, path)
		if err != nil {
			return result, err
		}
		defer unlock()
	}
	layers, err := m.readLayers(projectTrusted)
	if err != nil {
		return result, err
	}
	current := layers[scope]
	if current == nil {
		current = &document{values: make(map[string]any)}
	}
	baseline := base.documents[scope]
	if scope != Runtime && baseline != nil && baseline.canonical != current.canonical {
		return result, errors.New("configuration target changed since editing began; reload and reauthorize")
	}
	for _, edit := range normalized {
		old, oldExists := lookup(baseline, edit.Key)
		now, nowExists := lookup(current, edit.Key)
		if oldExists != nowExists || !reflect.DeepEqual(old, now) {
			return result, &Error{Path: current.path, Key: edit.Key, Reason: "setting changed since this edit began; reload and reconcile the conflicting value"}
		}
	}
	if scope == Runtime {
		values := copyValues(current.values)
		for _, edit := range normalized {
			if edit.Reset {
				delete(values, edit.Key)
			} else {
				values[edit.Key] = edit.Value
			}
		}
		layers[Runtime] = &document{values: values}
	} else {
		bytes, err := editDocument(current, normalized)
		if err != nil {
			return result, err
		}
		candidate, err := parseDocument(current.path, bytes, m.definitions)
		if err != nil {
			return result, err
		}
		if err := verifyRoundTrip(current, candidate, normalized); err != nil {
			return result, err
		}
		candidate.canonical = current.canonical
		layers[scope] = candidate
	}
	if _, err := compose(layers, base.revision); err != nil {
		return result, err
	}
	stillTrusted, err := isTrusted()
	if err != nil {
		return result, err
	}
	if stillTrusted != projectTrusted {
		return result, errors.New("host project trust changed before save; reload before retrying")
	}
	if err := bounded.Err(); err != nil {
		return result, err
	}
	if scope != Runtime {
		path := m.paths.User
		if scope == Project {
			path = m.paths.Project
		}
		resolved, err := resolveTarget(path)
		if err != nil {
			return result, err
		}
		if resolved != target {
			return result, errors.New("configuration symlink target changed while saving; retry after inspection")
		}
		committed, err := atomicWrite(bounded, target, layers[scope].raw)
		result.Saved = committed
		if err != nil {
			if committed {
				m.recordError(err)
			}
			return result, err
		}
	} else {
		m.runtime = copyValues(layers[Runtime].values)
	}
	result.Saved = true
	for _, edit := range normalized {
		result.Changed = append(result.Changed, edit.Key)
		if m.definitions[edit.Key].Application != "live" {
			result.Pending = append(result.Pending, edit.Key)
		}
	}
	reconcileContext, reconcileCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer reconcileCancel()
	snapshot, err := m.publish(reconcileContext, layers)
	if err != nil {
		m.recordError(err)
		return result, err
	}
	result.Published = true
	result.Revision = snapshot.revision
	return result, nil
}

func (m *Manager) validateEdits(edits []Edit) ([]Edit, error) {
	normalized := make([]Edit, len(edits))
	seen := make(map[string]bool)
	for i, edit := range edits {
		definition, ok := m.definitions[edit.Key]
		if !ok {
			return nil, &Error{Key: edit.Key, Reason: "unknown or unavailable setting key; inspect the catalog"}
		}
		if seen[edit.Key] {
			return nil, &Error{Key: edit.Key, Reason: "setting appears twice in one save"}
		}
		seen[edit.Key] = true
		if edit.Reset {
			if edit.Value != nil {
				return nil, errors.New("reset must not also supply a value")
			}
		} else {
			value, err := definition.validate(edit.Value)
			if err != nil {
				return nil, &Error{Key: edit.Key, Reason: err.Error()}
			}
			edit.Value = value
		}
		normalized[i] = edit
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Key < normalized[j].Key })
	return normalized, nil
}

func lookup(doc *document, key string) (any, bool) {
	if doc == nil {
		return nil, false
	}
	value, ok := doc.values[key]
	return value, ok
}
func copyValues(values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = cloneValue(value)
	}
	return result
}
