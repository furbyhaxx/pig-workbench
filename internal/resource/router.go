// Package resource resolves canonical resources through declared providers.
package resource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// EntryKind classifies one listing entry.
type EntryKind string

const (
	EntryFile    EntryKind = "file"
	EntryDir     EntryKind = "dir"
	EntryLink    EntryKind = "link"
	EntryControl EntryKind = "ctl"
)

// Entry is one typed provider listing row.
type Entry struct {
	Name     string    `json:"name"`
	URI      string    `json:"uri"`
	Kind     EntryKind `json:"kind"`
	Size     int64     `json:"size,omitempty"`
	Revision string    `json:"revision,omitempty"`
	Target   string    `json:"target,omitempty"`
	NoWalk   bool      `json:"noWalk,omitempty"`
	// Detail is a one-line description shown after the name in listings.
	Detail string `json:"detail,omitempty"`
}

// Listing is a bounded directory page.
type Listing struct {
	Revision   string  `json:"revision,omitempty"`
	Entries    []Entry `json:"entries"`
	NextCursor string  `json:"nextCursor,omitempty"`
	Complete   bool    `json:"complete"`
}

// StatResult describes an addressable resource.
type StatResult struct {
	URI      string    `json:"uri"`
	Kind     EntryKind `json:"kind"`
	Size     int64     `json:"size,omitempty"`
	Lines    int       `json:"lines,omitempty"`
	Revision string    `json:"revision,omitempty"`
}

// ReadRequest is one bounded read through the router.
type ReadRequest struct {
	URI       string
	Requested string
	Selector  Selection
	Budget    Budget
}

// ReadResult carries the rendered page and its facts.
type ReadResult struct {
	Content string
	Facts   Page
}

// Provider is implemented by every scheme backend, local or contributed.
type Provider interface {
	Scheme() string
	Stat(ctx context.Context, uri string) (StatResult, error)
	List(ctx context.Context, uri string, cursor string, limit int) (Listing, error)
	Read(ctx context.Context, request ReadRequest) (ReadResult, error)
}

// ErrUnsupported reports an operation the provider does not advertise.
var ErrUnsupported = errors.New("operation is unsupported for this resource scheme")

// ErrNotFound reports an absent resource without implying an empty success.
var ErrNotFound = errors.New("resource not found")

// Router dispatches a parsed target to exactly one provider.
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRouter creates an empty router; builtin providers register at startup.
func NewRouter() *Router { return &Router{providers: make(map[string]Provider)} }

// Register adds or replaces one canonical scheme provider.
func (r *Router) Register(provider Provider) error {
	if provider == nil {
		return errors.New("provider must not be nil")
	}
	scheme := provider.Scheme()
	if scheme == "" || scheme != strings.ToLower(scheme) || strings.ContainsAny(scheme, ":/?#") {
		return fmt.Errorf("invalid provider scheme %q", scheme)
	}
	if reserved[scheme] {
		return fmt.Errorf("builtin scheme %q is owned by Workbench and cannot be replaced", scheme)
	}
	if _, retired := retiredSchemes[scheme]; retired {
		return fmt.Errorf("retired scheme %q must not be registered", scheme)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[scheme] = provider
	return nil
}

// RegisterBuiltin installs a Workbench-owned scheme; extension registration cannot replace it.
func (r *Router) RegisterBuiltin(provider Provider) error {
	if provider == nil {
		return errors.New("provider must not be nil")
	}
	scheme := provider.Scheme()
	if !reserved[scheme] {
		return fmt.Errorf("%q is not a reserved builtin scheme", scheme)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[scheme] = provider
	return nil
}

// RegisterContributed installs one external provider under every scheme atomically.
// A scheme already held by any provider is a conflict; nothing is installed then.
func (r *Router) RegisterContributed(providers []Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, provider := range providers {
		scheme := provider.Scheme()
		if reserved[scheme] {
			return fmt.Errorf("builtin scheme %q is owned by Workbench and cannot be replaced", scheme)
		}
		if _, retired := retiredSchemes[scheme]; retired {
			return fmt.Errorf("retired scheme %q must not be registered", scheme)
		}
		if _, held := r.providers[scheme]; held {
			return fmt.Errorf("scheme %q is already registered", scheme)
		}
	}
	for _, provider := range providers {
		r.providers[provider.Scheme()] = provider
	}
	return nil
}

// UnregisterContributed removes external schemes; builtin schemes are never removed.
func (r *Router) UnregisterContributed(schemes []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, scheme := range schemes {
		if !reserved[scheme] {
			delete(r.providers, scheme)
		}
	}
}

// Schemes returns registered canonical schemes in lexical order.
// Locatable reports whether a scheme resolves to a local filesystem path. Scheme commands
// (G8.20) only intercept non-locatable schemes; file:// stays ordinary shell.
func (r *Router) Locatable(scheme string) bool { return scheme == "file" || scheme == "local" }

func (r *Router) Schemes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Provider resolves one target's scheme, refusing an unregistered one.
func (r *Router) Provider(target Target) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[target.Scheme]
	if ok {
		return provider, nil
	}
	if replacement, retired := retiredSchemes[target.Scheme]; retired {
		return nil, fmt.Errorf("unknown scheme %q; use %q", target.Scheme, replacement+"://")
	}
	return nil, ErrUnsupported
}

// Read resolves and reads one target, preserving its requested spelling in errors.
func (r *Router) Read(ctx context.Context, target Target, budget Budget) (ReadResult, error) {
	provider, err := r.Provider(target)
	if err != nil {
		return ReadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReadResult{}, err
	}
	result, err := provider.Read(ctx, ReadRequest{URI: target.URI, Requested: target.Requested, Selector: target.Selection, Budget: budget})
	if err != nil {
		return ReadResult{}, fmt.Errorf("read %s: %w", target.Requested, err)
	}
	return result, nil
}

var reserved = map[string]bool{
	"file": true, "ssh": true, "http": true, "https": true, "local": true, "out": true, "conflict": true, "artifact": true,
	"project": true, "session": true, "agent": true, "job": true, "cfg": true, "skill": true, "docs": true, "secret": true,
}

var retiredSchemes = map[string]string{
	"history": "session", "branch": "session", "proc": "job", "skills": "skill", "jobs": "job",
	"harness": "cfg", "self": "session", "workbench": "cfg", "projects": "project", "secrets": "secret",
}
