// Package lifecycle owns session work and shutdown draining.
package lifecycle

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed means the session no longer admits work.
var ErrClosed = errors.New("session is closing; retry in the active session")

// Supervisor cancels and joins every admitted operation.
type Supervisor struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	wg     sync.WaitGroup
}

// New creates an independent session lifetime.
func New() *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{ctx: ctx, cancel: cancel}
}

// Admit binds a request to the session lifetime and returns its idempotent release.
func (s *Supervisor) Admit(parent context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, ErrClosed
	}
	s.wg.Add(1)
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(s.ctx, cancel)
	release := sync.OnceFunc(func() { stop(); cancel(); s.wg.Done() })
	return ctx, release, nil
}

// Start runs owned background work until it completes or the session stops.
func (s *Supervisor) Start(work func(context.Context)) error {
	ctx, release, err := s.Admit(s.ctx)
	if err != nil {
		return err
	}
	go func() { defer release(); work(ctx) }()
	return nil
}

// Stop closes admission, cancels work and waits for every operation to release.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}
