package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

// errProviderUnavailable means no registered owner answered within the call.
var errProviderUnavailable = errors.New("contribution provider did not answer; it may have unloaded")

// providerError is a contribution's protocol failure, kept distinct from transport failure.
type providerError struct{ protocol *api.Error }

func (e *providerError) Error() string {
	return "provider failed: " + e.protocol.Code + ": " + e.protocol.Message
}

// maxProviderInFlight is SPEC-0001 §4's per-owner bound on concurrent provider operations.
const maxProviderInFlight = 8

// providerDrainBound caps how long a withdrawal waits for cooperative operations to stop.
const providerDrainBound = 5 * time.Second

type providerPending struct {
	to             api.Identity
	registrationID string
	replies        []api.Reply
	// progress is the newest accepted note; older ones are overwritten, so state per request stays one value.
	progress *api.ProviderProgress
}

// providerBus is Workbench's synchronous caller for registered contributions.
// It installs correlation before emitting and only copies replies in the listener.
type providerBus struct {
	identity   api.Identity
	sessionID  string
	generation string
	events     func() (sdk.Context, bool)
	// cancelEvents is the extension-level bus: unlike a request-bound one it stays usable after the caller aborted.
	cancelEvents sdk.EventBus
	cancelReady  bool

	mu       sync.Mutex
	pending  map[string]*providerPending
	inFlight map[string]int
	regs     map[string]*registrationOps
}

// registrationOps tracks the operations running against one registration so a withdrawal can stop and drain them.
type registrationOps struct {
	closed bool
	active map[string]api.Identity
	idle   chan struct{}
}

// begin admits one operation against a registration unless it was withdrawn; the caller runs end exactly once.
func (b *providerBus) begin(registrationID, requestID string, to api.Identity) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ops := b.regs[registrationID]
	if ops == nil {
		ops = &registrationOps{active: make(map[string]api.Identity)}
		b.regs[registrationID] = ops
	}
	if ops.closed {
		return nil, &providerError{protocol: &api.Error{Code: "unavailable", Message: "the provider registration was withdrawn", Outcome: "notApplied"}}
	}
	ops.active[requestID] = to
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			delete(ops.active, requestID)
			if len(ops.active) == 0 && ops.idle != nil {
				close(ops.idle)
				ops.idle = nil
			}
		})
	}, nil
}

// withdrawn reports whether the registration was closed, so an unanswered call can say why.
func (b *providerBus) withdrawn(registrationID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	ops := b.regs[registrationID]
	return ops != nil && ops.closed
}

// withdraw closes admission for a registration, tells running operations to stop and waits up to
// wait for them to end. It reports whether everything drained; it never holds b.mu while waiting.
func (b *providerBus) withdraw(registrationID string, wait time.Duration) bool {
	b.mu.Lock()
	ops := b.regs[registrationID]
	if ops == nil {
		ops = &registrationOps{active: make(map[string]api.Identity)}
		b.regs[registrationID] = ops
	}
	ops.closed = true
	running := make(map[string]api.Identity, len(ops.active))
	for id, to := range ops.active {
		running[id] = to
	}
	var idle chan struct{}
	if len(running) > 0 {
		if ops.idle == nil {
			ops.idle = make(chan struct{})
		}
		idle = ops.idle
	}
	b.mu.Unlock()
	for id, to := range running {
		b.cancelRemote(to, id)
	}
	if idle == nil {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-idle:
		return true
	case <-timer.C:
		return false
	}
}

// reopen admits operations again after a replacement installed a usable provider.
func (b *providerBus) reopen(registrationID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ops := b.regs[registrationID]; ops != nil {
		ops.closed = false
	}
}

// admit reserves one in-flight slot for owner; the caller must run the returned release exactly once.
func (b *providerBus) admit(owner string) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inFlight[owner] >= maxProviderInFlight {
		return nil, &providerError{protocol: &api.Error{Code: "busy", Message: fmt.Sprintf("provider %q has %d operations in flight", owner, maxProviderInFlight), Retryable: true, Outcome: "notApplied"}}
	}
	b.inFlight[owner]++
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.inFlight[owner]--
			if b.inFlight[owner] <= 0 {
				delete(b.inFlight, owner)
			}
			b.mu.Unlock()
		})
	}, nil
}

func newProviderBus(sessionID, generation string, events func() (sdk.Context, bool)) *providerBus {
	return &providerBus{identity: api.Identity{Owner: "pig-workbench", Instance: generation}, sessionID: sessionID, generation: generation, events: events, pending: make(map[string]*providerPending), inFlight: make(map[string]int), regs: make(map[string]*registrationOps)}
}

func (b *providerBus) onReply(_ sdk.Context, value any) error {
	reply, err := api.DecodeReply(value)
	if err != nil {
		return nil
	}
	if reply.To != b.identity || reply.SessionID != b.sessionID || reply.Generation != b.generation {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.pending[reply.ID]
	if pending == nil {
		return nil
	}
	if len(pending.replies) < 2 {
		pending.replies = append(pending.replies, reply)
	}
	return nil
}

// onProgress records the newest note for the request it names. It only copies into the pending slot and
// returns: it never emits, never waits and never calls the provider, so it cannot re-enter a running handler.
func (b *providerBus) onProgress(_ sdk.Context, value any) error {
	note, err := api.DecodeProgress(value)
	if err != nil {
		return nil
	}
	if note.To != b.identity || note.SessionID != b.sessionID || note.Generation != b.generation {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.pending[note.RequestID]
	if pending == nil || pending.to != note.From {
		return nil
	}
	if pending.progress != nil && note.Seq <= pending.progress.Seq {
		return nil
	}
	pending.progress = &note
	return nil
}

// activity reports the operations running against one registration and the newest progress note among them.
func (b *providerBus) activity(registrationID string) (running int, latest *api.ProviderProgress) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, pending := range b.pending {
		if pending.registrationID != registrationID {
			continue
		}
		running++
		if pending.progress != nil && (latest == nil || pending.progress.Seq > latest.Seq) {
			note := *pending.progress
			latest = &note
		}
	}
	return running, latest
}

// call emits one provider request and decodes the single reply into result.
func (b *providerBus) call(ctx context.Context, to api.Identity, registrationID, method string, params, result any, budget time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	release, err := b.admit(to.Owner)
	if err != nil {
		return err
	}
	defer release()
	encoded, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode provider request: %w", err)
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	if budget <= 0 {
		budget = 30 * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		budget = min(budget, time.Until(deadline))
	}
	request := api.ProviderRequest{Version: 1, ID: hex.EncodeToString(token[:]), SessionID: b.sessionID, Generation: b.generation, From: b.identity, To: to, RegistrationID: registrationID, Method: method, Params: encoded, TimeoutMS: max(1, int(budget/time.Millisecond))}
	if frame, err := json.Marshal(request); err != nil || len(frame) > api.ControlBytes {
		return &providerError{protocol: &api.Error{Code: "limitExceeded", Message: fmt.Sprintf("provider request exceeds %d bytes", api.ControlBytes), Outcome: "notApplied"}}
	}
	end, err := b.begin(registrationID, request.ID, to)
	if err != nil {
		return err
	}
	defer end()
	emitter, ok := b.events()
	if !ok {
		return errProviderUnavailable
	}
	if bound, ok := ctx.Value(hostRequestKey{}).(sdk.Context); ok {
		emitter = bound
	}
	pending := &providerPending{to: to, registrationID: registrationID}
	b.mu.Lock()
	b.pending[request.ID] = pending
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.pending, request.ID); b.mu.Unlock() }()
	emitErr := emitter.Events().Emit(api.ProviderRequestChannel, request)
	b.mu.Lock()
	replies := append([]api.Reply(nil), pending.replies...)
	b.mu.Unlock()
	switch len(replies) {
	case 0:
		if emitErr != nil || ctx.Err() != nil {
			// The host releases only the caller (P3); the listener keeps running until told to stop.
			b.cancelRemote(to, request.ID)
		}
		if emitErr != nil {
			return fmt.Errorf("emit provider request: %w", emitErr)
		}
		if b.withdrawn(registrationID) {
			return &providerError{protocol: &api.Error{Code: "unavailable", Message: "the provider registration was withdrawn during the operation", Outcome: "unknown"}}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return errProviderUnavailable
	case 1:
	default:
		return errors.New("more than one provider answered one request")
	}
	reply := replies[0]
	if !reply.OK {
		return &providerError{protocol: reply.Error}
	}
	if result == nil {
		return nil
	}
	data, err := json.Marshal(reply.Result)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("provider result shape is invalid: %w", err)
	}
	return nil
}

// cancelRemote tells a provider that stopped being awaited to stop; it needs no reply and is best effort.
func (b *providerBus) cancelRemote(to api.Identity, requestID string) {
	if !b.cancelReady {
		return
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return
	}
	params, _ := json.Marshal(map[string]string{"requestId": requestID})
	_ = b.cancelEvents.Emit(api.ProviderCancelChannel, api.ProviderRequest{Version: 1, ID: hex.EncodeToString(token[:]), SessionID: b.sessionID, Generation: b.generation, From: b.identity, To: to, Method: "cancel", Params: params})
}

// activityText describes running operations for an inspector line; it is empty while the provider is idle.
func (b *providerBus) activityText(registrationID string) string {
	running, latest := b.activity(registrationID)
	if running == 0 {
		return ""
	}
	text := fmt.Sprintf("  [running %d", running)
	if latest != nil {
		if latest.Message != "" {
			text += ": " + noticeText(latest.Message, api.MaxProgressMessage)
		}
		if latest.Total > 0 {
			text += fmt.Sprintf(" (%d/%d)", latest.Completed, latest.Total)
		} else if latest.Completed > 0 {
			text += fmt.Sprintf(" (%d)", latest.Completed)
		}
	}
	return text + "]"
}
