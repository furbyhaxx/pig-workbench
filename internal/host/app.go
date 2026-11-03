// Package host adapts Workbench domain services to the running PiG SDK.
package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/lifecycle"
	"github.com/furbyhaxx/pig-workbench/internal/location"
)

type activeRequest struct {
	owner  string
	cancel context.CancelFunc
}

type session struct {
	requestsMu  sync.Mutex
	requests    map[string]activeRequest
	unsubscribe func()
	id          string
	generation  string
	supervisor  *lifecycle.Supervisor
	location    *location.Manager
}

type app struct {
	mu      sync.RWMutex
	session *session
	initErr error
}

type requestContext struct{ sdk.Context }

func (requestContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (requestContext) Value(any) any               { return nil }

// Register wires session lifecycle and the implemented tools and commands.
func Register(ext *sdk.Extension) {
	a := &app{}
	ext.OnSessionStart(a.start)
	ext.OnSessionShutdown(a.shutdown)
	ext.OnEvent(sdk.EventSessionTree, a.restore)
	a.registerTools(ext)
	a.registerCommands(ext)
}

func (a *app) start(ctx sdk.Context, _ map[string]any) (any, error) {
	if _, err := a.shutdown(ctx, nil); err != nil {
		return nil, err
	}
	id, err := ctx.GetSessionID()
	if err != nil {
		return nil, a.fail(err)
	}
	manager, err := location.New(ctx.Cwd())
	if err != nil {
		return nil, a.fail(err)
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, a.fail(fmt.Errorf("create session generation: %w", err))
	}
	s := &session{id: id, generation: hex.EncodeToString(token[:]), supervisor: lifecycle.New(), location: manager, requests: make(map[string]activeRequest)}
	if err := restoreLocation(ctx, manager); err != nil {
		s.supervisor.Stop()
		return nil, a.fail(err)
	}
	unsubscribe, err := ctx.Events().On(api.RequestChannel, a.service)
	if err != nil {
		s.supervisor.Stop()
		return nil, a.fail(fmt.Errorf("subscribe Workbench services: %w", err))
	}
	s.unsubscribe = unsubscribe
	a.mu.Lock()
	a.session = s
	a.initErr = nil
	a.mu.Unlock()
	if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: s.generation + ":ready", Type: "ready", Data: map[string]any{"capabilities": []string{"location"}}}); err != nil {
		ctx.Notify("Workbench ready event failed; clients can discover on demand.", "warning")
	}
	return nil, nil
}

func (a *app) fail(err error) error { a.mu.Lock(); a.initErr = err; a.mu.Unlock(); return err }

func (a *app) shutdown(ctx sdk.Context, _ map[string]any) (any, error) {
	a.mu.Lock()
	old := a.session
	a.session = nil
	a.mu.Unlock()
	if old != nil {
		old.supervisor.Stop()
		if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: old.id, Generation: old.generation, EventID: old.generation + ":closing", Type: "closing", Data: map[string]any{}}); err != nil {
			ctx.Notify("Workbench closing notification failed.", "warning")
		}
		old.unsubscribe()
	}
	return nil, nil
}

func (a *app) current() (*session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.initErr != nil {
		return nil, fmt.Errorf("Workbench initialization failed: %w", a.initErr)
	}
	if a.session == nil {
		return nil, errors.New("Workbench session is not active")
	}
	return a.session, nil
}

func (a *app) admit(ctx sdk.Context) (*session, context.Context, func(), error) {
	s, err := a.current()
	if err != nil {
		return nil, nil, nil, err
	}
	work, release, err := s.supervisor.Admit(requestContext{ctx})
	if err != nil {
		return nil, nil, nil, err
	}
	return s, work, release, nil
}

func (a *app) restore(ctx sdk.Context, _ map[string]any) (any, error) {
	s, _, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return nil, restoreLocation(ctx, s.location)
}
