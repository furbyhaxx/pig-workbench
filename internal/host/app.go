// Package host adapts Workbench domain services to the running PiG SDK.
package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
	"github.com/furbyhaxx/pig-workbench/internal/lifecycle"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/syntax"
)

type activeRequest struct {
	owner  string
	cancel context.CancelFunc
}

type session struct {
	editState     editState
	presentations *presentationCatalog
	contributions *contribution.Registry
	contextFrames *contextFrameState
	jobs          *jobs.Manager
	jobMeta       jobMetadataStore
	snapshots     *hashline.Store
	viewsMu       sync.Mutex
	pendingViews  map[string][]pendingView
	requestsMu    sync.Mutex
	requests      map[string]activeRequest
	unsubscribe   func()
	id            string
	generation    string
	supervisor    *lifecycle.Supervisor
	location      *location.Manager
	settings      *config.Manager
	router        *resource.Router
	scratch       string
	projects      *project.Registry
	host          sdk.Context
	bus           *providerBus
	fetch         *fetchRouter
	status        *statusBoard
	docs          *docsCatalog
	producers     *producerRegistry
	installedMu   sync.Mutex
	installed     map[string]func()
	projectMu     sync.Mutex
	projectCache  projectIdentityCache
}

type app struct {
	mu      sync.RWMutex
	session *session
	initErr error
	syntax  *syntax.Service
}

type requestContext struct{ sdk.Context }

func (requestContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (requestContext) Value(any) any               { return nil }

// Register wires session lifecycle and the implemented tools and commands.
func Register(ext *sdk.Extension) {
	a := &app{syntax: syntax.New()}
	ext.OnSessionStart(a.start)
	ext.OnSessionShutdown(a.shutdown)
	ext.OnEvent(sdk.EventSessionTree, a.restore)
	ext.OnEvent(sdk.EventSessionTree, a.invalidateSeen)
	ext.OnEvent("session_compact", a.invalidateSeen)
	ext.OnToolResult(a.confirmRead)
	ext.OnEvent("before_agent_start", a.contextFrames)
	ext.OnEvent("context", a.confirmContextFrames)
	ext.OnEvent("tool_call", a.toolInstructions)
	a.registerTools(ext)
	a.registerAsk(ext)
	a.registerJobs(ext)
	a.registerEdit(ext)
	a.registerWriteAndList(ext)
	a.registerSearchTools(ext)
	a.registerCommands(ext)
	ext.ToolRenderer(a.resolveToolRenderer)
}

func (a *app) start(ctx sdk.Context, _ map[string]any) (any, error) {
	if _, err := a.shutdown(ctx, nil); err != nil {
		return nil, err
	}
	id, err := ctx.GetSessionID()
	if err != nil {
		return nil, a.fail(err)
	}
	agentDirectory, err := sdk.AgentDir()
	if err != nil {
		return nil, a.fail(err)
	}
	projects, err := project.Open(agentDirectory)
	if err != nil {
		return nil, a.fail(err)
	}
	manager, err := location.New(requestContext{ctx}, ctx.Cwd(), func(work context.Context, cwd string) (string, error) {
		detected, err := projects.Detect(work, cwd)
		if err != nil {
			return "", err
		}
		return detected.Root, nil
	})
	if err != nil {
		return nil, a.fail(err)
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, a.fail(fmt.Errorf("create session generation: %w", err))
	}
	generation := hex.EncodeToString(token[:])
	contributions, err := contribution.New(generation, contribution.Options{Validator: validateContributionDescriptor})
	if err != nil {
		return nil, a.fail(fmt.Errorf("create contribution registry: %w", err))
	}
	settings, err := loadSettings(ctx)
	if err != nil {
		return nil, a.fail(err)
	}
	host := ctx
	hostContext := func() (sdk.Context, bool) { return host, true }
	bus := newProviderBus(id, generation, hostContext)
	fetch := &fetchRouter{bus: bus}
	providers, scratch, err := buildRouter(ctx, settings, manager, fetch)
	if err != nil {
		return nil, a.fail(err)
	}
	fetch.router = func() *resource.Router { return providers }
	docsCache := ""
	if cache, cacheErr := os.UserCacheDir(); cacheErr == nil {
		docsCache = filepath.Join(cache, "pig-workbench", "docs-sources")
	}
	docs := &docsCatalog{sets: make(map[string]*docsSet), cache: docsCache}
	snapshots, err := hashline.NewStore(hashline.StoreLimits{}, generation)
	if err != nil {
		return nil, a.fail(err)
	}
	jobManager, err := jobs.NewManager(jobs.ManagerOptions{})
	if err != nil {
		return nil, a.fail(err)
	}
	contextState, err := restoreFrameLedger(ctx, id)
	if err != nil {
		_ = jobManager.Close()
		return nil, a.fail(err)
	}
	s := &session{
		editState:     editState{mode: "", registers: make(map[string]hashline.Register), noops: make(map[string]int)},
		presentations: newPresentationCatalog(),
		contributions: contributions,
		contextFrames: contextState,
		jobs:          jobManager,
		jobMeta:       jobMetadataStore{values: make(map[string]jobMetadata)},
		snapshots:     snapshots,
		pendingViews:  make(map[string][]pendingView),
		id:            id,
		generation:    generation,
		supervisor:    lifecycle.New(),
		location:      manager,
		settings:      settings,
		router:        providers,
		scratch:       scratch,
		requests:      make(map[string]activeRequest),
		projects:      projects,
		host:          host,
		bus:           bus,
		fetch:         fetch,
		status:        newStatusBoard(hostContext),
		docs:          docs,
		producers:     newProducerRegistry(),
	}
	fetch.project = s.projectIdentity
	docs.guide = s.schemeGuide
	if err := providers.RegisterBuiltin(docsProvider{catalog: docs, schemes: providers.Schemes}); err != nil {
		return nil, a.fail(err)
	}
	if err := restoreLocation(ctx, manager); err != nil {
		s.supervisor.Stop()
		return nil, a.fail(err)
	}
	unsubscribe, err := ctx.Events().On(api.RequestChannel, a.service)
	if err != nil {
		s.supervisor.Stop()
		return nil, a.fail(fmt.Errorf("subscribe Workbench services: %w", err))
	}
	unsubscribeReplies, err := ctx.Events().On(api.ProviderReplyChannel, bus.onReply)
	if err != nil {
		unsubscribe()
		s.supervisor.Stop()
		return nil, a.fail(fmt.Errorf("subscribe Workbench provider replies: %w", err))
	}
	s.unsubscribe = func() { unsubscribe(); unsubscribeReplies() }
	a.mu.Lock()
	a.session = s
	a.initErr = nil
	a.mu.Unlock()
	if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: s.generation + ":ready", Type: "ready", Data: map[string]any{"capabilities": advertisedCapabilities}}); err != nil {
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
		old.status.close()
		old.supervisor.Stop()
		if old.jobs != nil {
			if err := old.jobs.Close(); err != nil {
				ctx.Notify("Workbench job cleanup failed: "+err.Error(), "error")
			}
		}
		if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: old.id, Generation: old.generation, EventID: old.generation + ":closing", Type: "closing", Data: map[string]any{}}); err != nil {
			ctx.Notify("Workbench closing notification failed.", "warning")
		}
		if old.scratch != "" {
			if err := os.RemoveAll(old.scratch); err != nil {
				ctx.Notify("Workbench scratch cleanup failed: "+err.Error(), "warning")
			}
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
