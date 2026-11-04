package prompt

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/nikolalohinski/gonja/v2/exec"
)

const renderStateKey = "\x00prompt-render-state"

var errMissingRenderState = errors.New("prompt execution has no owned render lifetime")

// renderState carries one render's cancellation context and owns the range
// producers it starts. Producers must be joined before the render returns.
type renderState struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	closing bool
	running sync.WaitGroup
	// live counts unfinished producers, total counts registered ones.
	live  atomic.Int64
	total atomic.Int64
}

func newRenderState(ctx context.Context) *renderState {
	child, cancel := context.WithCancel(ctx)
	return &renderState{ctx: child, cancel: cancel}
}

func renderStateOf(environment *exec.Environment) *renderState {
	if environment == nil {
		return nil
	}
	value, found := environment.Context.Get(renderStateKey)
	if !found {
		return nil
	}
	state, _ := value.(*renderState)
	return state
}

func (s *renderState) context() context.Context { return s.ctx }

func (s *renderState) err() error {
	if s == nil {
		return errMissingRenderState
	}
	return s.ctx.Err()
}

// start runs one range producer under this render. It reports false when the
// render is already joining, so no producer is started after join began.
func (s *renderState) start(run func()) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return false
	}
	s.running.Add(1)
	s.live.Add(1)
	s.total.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.running.Done()
		defer s.live.Add(-1)
		run()
	}()
	return true
}

// join cancels the render context so blocked producers can return, then waits
// for every producer to finish.
func (s *renderState) join() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	s.cancel()
	s.running.Wait()
}
