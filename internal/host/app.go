// Package host adapts Workbench domain services to the running PiG SDK.
package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	readHistory         readHistory
	listCursors         listingCursorStore
	resourceEvents      atomic.Uint64
	editState           editState
	presentations       *presentationCatalog
	presentationData    presentationCache
	presentationLayouts presentationLayouts
	contributions       *contribution.Registry
	contextFrames       *contextFrameState
	jobs                *jobs.Manager
	jobMeta             jobMetadataStore
	snapshots           *hashline.Store
	viewsMu             sync.Mutex
	pendingViews        map[string][]pendingView
	runningCalls        map[string]string
	callRoots           map[string]string
	guideInjected       map[string]string
	jobRegistry         *jobs.Registry
	jobOwner            *jobs.OwnerHandle
	requestsMu          sync.Mutex
	requests            map[string]activeRequest
	unsubscribe         func()
	id                  string
	generation          string
	supervisor          *lifecycle.Supervisor
	location            *location.Manager
	settings            *config.Manager
	secrets             *secretsBoundary
	router              *resource.Router
	skills              *skillState
	prompt              promptState
	scratch             string
	scratchClaim        io.Closer
	materialized        atomic.Uint64
	projects            *project.Registry
	host                sdk.Context
	bus                 *providerBus
	fetch               *fetchRouter
	status              *statusBoard
	docs                *docsCatalog
	producers           *producerRegistry
	children            *childCalls
	endpoint            *sessionEndpoint
	activity            *activityLine
	installedMu         sync.Mutex
	installed           map[string]func()
	projectMu           sync.Mutex
	projectCache        projectIdentityCache
}

type app struct {
	extEvents sdk.EventBus
	// editTextMode is the edit.mode the registered edit description was built for.
	editTextMu   sync.Mutex
	editTextMode string
	// forcePanels > 0 while a hub entry runs, so every result shows in a panel instead of a vanishing toast.
	forcePanels atomic.Int32
	hubTitle    atomic.Value
	mu          sync.RWMutex
	session     *session
	initErr     error
	syntax      *syntax.Service
	// shortcutKey is the activity shortcut registered at load; it changes only on the next load.
	shortcutKey, shortcutNote string
}

type requestContext struct{ sdk.Context }

func (requestContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (r requestContext) Value(key any) any {
	if key == (hostRequestKey{}) {
		return r.Context
	}
	return nil
}

// hostRequestKey carries the SDK request context so a provider emit is released when its tool call is aborted (P3).
type hostRequestKey struct{}

// Register wires session lifecycle and the implemented tools and commands.
func Register(ext *sdk.Extension) {
	a := &app{syntax: syntax.New(), extEvents: ext.Events(), editTextMode: editModeDefault}
	a.registerMessageRenderers(ext)
	ext.OnSessionStart(a.start)
	ext.OnSessionShutdown(a.shutdown)
	ext.OnEvent(sdk.EventSessionTree, a.restore)
	ext.OnEvent(sdk.EventSessionTree, a.invalidateSeen)
	ext.OnEvent("session_compact", a.invalidateSeen)
	ext.OnEvent("session_compact", a.replaySkills)
	ext.OnEvent(sdk.EventAgentStart, a.onAgentStart)
	ext.OnEvent(sdk.EventAgentEnd, a.onAgentEnd)
	ext.OnEvent("input", a.redactInput)
	ext.OnEvent(sdk.EventContextWithSystem, a.redactContext)
	ext.OnEvent("user_bash", a.userBash)
	ext.OnToolResult(a.redactResult)
	ext.OnToolResult(a.capturePresentationFinal)
	ext.OnToolResult(a.confirmRead)
	ext.OnEvent(sdk.EventToolExecutionUpdate, a.capturePresentationUpdate)
	ext.OnEvent(sdk.EventToolExecutionStart, a.captureChildStart)
	ext.OnEvent(sdk.EventToolExecutionEnd, a.captureChildEnd)
	ext.OnEvent("before_agent_start", a.contextFrames)
	ext.OnEvent("before_provider_request", a.spliceLatePrompt)
	ext.OnEvent("before_provider_request", a.redactProviderRequest)
	ext.OnEvent("context", a.confirmContextFrames)
	ext.OnEvent("context", a.projectReadContext)
	ext.OnEvent("context", a.preloadPresentationHistory)
	ext.OnEvent("tool_call", a.substituteCall)
	ext.OnEvent("tool_call", a.toolInstructions)
	ext.OnEvent("tool_call", a.trackCall)
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
		if detected.How == "project-file" {
			// Keep the registry copy and locations current (SPEC-0012 §3.4); recognition already succeeded without it.
			_ = projects.Observe(work, cwd)
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
	bus.cancelEvents, bus.cancelReady = a.extEvents, true
	fetch := &fetchRouter{bus: bus}
	var current *session
	providers, scratch, err := buildRouter(ctx, settings, manager, fetch, func() (*session, bool) { return current, current != nil })
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
		runningCalls:  make(map[string]string),
		callRoots:     make(map[string]string),
		guideInjected: make(map[string]string),
		id:            id,
		generation:    generation,
		supervisor:    lifecycle.New(),
		location:      manager,
		settings:      settings,
		secrets:       newHostSecretsBoundary(ctx, settings, agentDirectory, manager),
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
		children:      newChildCalls(),
	}
	fetch.project = s.projectIdentity
	fetch.disabled = s.fetchBackendDisabled
	docs.guide = s.guideFor
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
	unsubscribeCancel, err := ctx.Events().On(api.CancelChannel, a.cancelRequest)
	if err != nil {
		unsubscribe()
		unsubscribeReplies()
		s.supervisor.Stop()
		return nil, a.fail(fmt.Errorf("subscribe Workbench cancellation: %w", err))
	}
	unsubscribeProgress, err := ctx.Events().On(api.ProviderProgressChannel, bus.onProgress)
	if err != nil {
		unsubscribe()
		unsubscribeReplies()
		unsubscribeCancel()
		s.supervisor.Stop()
		return nil, a.fail(fmt.Errorf("subscribe Workbench provider progress: %w", err))
	}
	s.unsubscribe = func() { unsubscribe(); unsubscribeReplies(); unsubscribeCancel(); unsubscribeProgress() }
	s.retainScratch(ctx)
	// A reload or a crash skips shutdown cleanup, and the host delivers no
	// session_shutdown to a reloaded runtime, so the next start of this session
	// reclaims the job process groups its predecessor left behind.
	if dir, err := jobRegistryBase(); err == nil {
		if registry, registryErr := jobs.NewRegistry(dir); registryErr == nil {
			s.jobRegistry = registry
			owner, holdErr := registry.HoldOwner(s.generation)
			if holdErr != nil {
				ctx.Notify(noticeText("Job liveness claim failed: "+holdErr.Error(), 0), "warning")
			} else {
				s.jobOwner = owner
			}
			result, sweepErr := registry.Sweep(jobs.Owner{SessionID: s.id, Generation: s.generation})
			if sweepErr != nil {
				ctx.Notify(noticeText("Job remnant cleanup incomplete: "+sweepErr.Error(), 0), "warning")
			} else if len(result.Killed) > 0 {
				ctx.Notify(noticeText(fmt.Sprintf("Reclaimed %d job process group(s) left by a previous runtime.", len(result.Killed)), 0), "info")
			}
		} else {
			ctx.Notify(noticeText("Job registry unavailable: "+registryErr.Error(), 0), "warning")
		}
	}
	s.skills = &skillState{loaded: map[string]bool{}, declined: map[string]bool{}, replayed: map[string]bool{}, roots: func() []string {
		cwd := s.location.Current().Cwd
		return []string{cwd + "/.agents/skills", cwd + "/.pig/skills"}
	}, persist: func(loaded, declined []string) error {
		return ctx.AppendEntry("pig-workbench.skills", map[string]any{"schemaVersion": 1, "loaded": loaded, "declined": declined})
	}}
	if options, err := ctx.GetSystemPromptOptions(); err == nil {
		s.skills.setHost(options.Skills)
	}
	restoreSkills(ctx, s.skills)
	if err := restoreDisplaySchemas(ctx, s); err != nil {
		ctx.Notify(noticeText("Retained display schemas were not restored: "+err.Error(), 0), "warning")
	}
	current = s
	a.mu.Lock()
	a.session = s
	a.initErr = nil
	a.mu.Unlock()
	a.startEndpoint(ctx, s)
	a.adoptSessionStorage(ctx, s)
	a.startActivity(ctx, s)
	a.offerProject(ctx, s)
	announcePendingAsk(ctx)
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
		old.status.close(ctx)
		if old.activity != nil {
			old.activity.close(ctx)
		}
		if old.endpoint != nil {
			old.endpoint.close()
		}
		old.supervisor.Stop()
		if old.jobOwner != nil {
			_ = old.jobOwner.Close()
		}
		if old.jobs != nil {
			if err := old.jobs.Close(); err != nil {
				ctx.Notify("Workbench job cleanup failed: "+err.Error(), "error")
			}
		}
		if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: old.id, Generation: old.generation, EventID: old.generation + ":closing", Type: "closing", Data: map[string]any{}}); err != nil {
			ctx.Notify("Workbench closing notification failed.", "warning")
		}
		if old.scratchClaim != nil {
			_ = old.scratchClaim.Close()
		}
		if old.scratch != "" {
			_ = os.RemoveAll(materializedRoot(old.scratch))
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
