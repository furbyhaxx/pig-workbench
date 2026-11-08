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

type providerPending struct {
	to      api.Identity
	replies []api.Reply
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

	mu       sync.Mutex
	pending  map[string]*providerPending
	inFlight map[string]int
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
	return &providerBus{identity: api.Identity{Owner: "pig-workbench", Instance: generation}, sessionID: sessionID, generation: generation, events: events, pending: make(map[string]*providerPending), inFlight: make(map[string]int)}
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
	emitter, ok := b.events()
	if !ok {
		return errProviderUnavailable
	}
	if bound, ok := ctx.Value(hostRequestKey{}).(sdk.Context); ok {
		emitter = bound
	}
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
	pending := &providerPending{to: to}
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
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return
	}
	params, _ := json.Marshal(map[string]string{"requestId": requestID})
	_ = b.cancelEvents.Emit(api.ProviderCancelChannel, api.ProviderRequest{Version: 1, ID: hex.EncodeToString(token[:]), SessionID: b.sessionID, Generation: b.generation, From: b.identity, To: to, Method: "cancel", Params: params})
}
