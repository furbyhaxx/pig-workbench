package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

var (
	// ErrUnavailable means discovery found no live Workbench responder.
	ErrUnavailable = errors.New("Workbench is unavailable; use the extension's own fallback or retry after a ready event")
	// ErrAmbiguous means more than one responder answered a single request.
	ErrAmbiguous = errors.New("multiple Workbench responders answered; resolve the duplicate extensions before retrying")
	// ErrClosed means the client has unsubscribed and cannot make requests.
	ErrClosed = errors.New("Workbench client is closed")
)

// TransportError retains a request ID for explicit reconciliation after a failed call.
type TransportError struct {
	RequestID string
	Outcome   string
	cause     error
}

// Error omits transport payloads, which may contain another extension's private data.
func (e *TransportError) Error() string {
	return "Workbench request " + e.RequestID + " failed; outcome " + e.Outcome + "; inspect the cause and reconcile by request ID"
}

// Unwrap exposes the original transport failure for errors.Is/errors.As.
func (e *TransportError) Unwrap() error { return e.cause }

// CallOptions controls an execution budget and an optional contribution binding.
type CallOptions struct {
	TimeoutMS      int
	RegistrationID string
}

type pendingCall struct {
	session    string
	generation string
	discover   bool
	replies    []Reply
}

// Client is an optional, synchronous v1 bus consumer with no owned goroutines.
type Client struct {
	mu          sync.Mutex
	identity    Identity
	pending     map[string]*pendingCall
	closed      bool
	unsubscribe func()
}

// NewClient subscribes before any request can emit; the extension owns Close.
func NewClient(ext *sdk.Extension) (*Client, error) {
	owner := ext.Name()
	if owner == "" || len(owner) > 128 || strings.IndexFunc(owner, unicode.IsControl) >= 0 {
		return nil, errors.New("extension name is not a valid Workbench owner")
	}
	instance, err := randomID()
	if err != nil {
		return nil, err
	}
	client := &Client{identity: Identity{Owner: owner, Instance: instance}, pending: make(map[string]*pendingCall)}
	unsubscribe, err := ext.Events().On(ReplyChannel, client.onReply)
	if err != nil {
		return nil, err
	}
	client.unsubscribe = unsubscribe
	return client, nil
}

// Close unsubscribes once; already-running cooperative Emit calls are not preempted.
func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.pending = nil
	unsubscribe := c.unsubscribe
	c.mu.Unlock()
	unsubscribe()
}

// Discover requires exactly one compatible responder and never silently grants a capability.
func (c *Client) Discover(ctx sdk.Context, capabilities ...string) (Discovery, error) {
	request, err := c.request(ctx, "", "discover", DiscoveryRequest{Majors: []int{1}, Capabilities: capabilities}, nil)
	if err != nil {
		return Discovery{}, err
	}
	replies, _, err := c.exchange(ctx, request)
	if err != nil {
		return Discovery{}, err
	}
	if len(replies) == 0 {
		return Discovery{}, ErrUnavailable
	}
	if len(replies) != 1 {
		return Discovery{}, ErrAmbiguous
	}
	reply := replies[0]
	if !reply.OK {
		return Discovery{}, reply.Error
	}
	encoded, err := json.Marshal(reply.Result)
	if err != nil {
		return Discovery{}, errors.New("invalid discovery response")
	}
	var endpoint Discovery
	if err := json.Unmarshal(encoded, &endpoint); err != nil {
		return Discovery{}, errors.New("invalid discovery response")
	}
	if endpoint.Major != 1 || endpoint.SessionID != request.SessionID || endpoint.Generation != reply.Generation || endpoint.Generation == "" || endpoint.Instance == "" || endpoint.Limits.ControlBytes <= 0 {
		return Discovery{}, errors.New("incompatible or incorrectly bound Workbench discovery response")
	}
	for _, capability := range capabilities {
		if !slices.Contains(endpoint.Capabilities, capability) {
			return Discovery{}, fmt.Errorf("Workbench did not provide required capability %q", capability)
		}
	}
	return endpoint, nil
}

// Call invokes one service without retries and decodes a successful result when requested.
// TimeoutMS is a cooperative execution budget, not preemption of a hung shared-bus listener.
func (c *Client) Call(ctx sdk.Context, endpoint Discovery, method string, params, result any, options *CallOptions) error {
	if result != nil {
		target := reflect.ValueOf(result)
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return errors.New("Workbench result destination must be a non-nil pointer")
		}
	}
	request, err := c.request(ctx, endpoint.Generation, method, params, options)
	if err != nil {
		return err
	}
	if endpoint.Major != 1 || endpoint.SessionID != request.SessionID {
		return errors.New("rediscover Workbench in the active session before calling services")
	}
	replies, emitted, err := c.exchange(ctx, request)
	if err != nil {
		outcome := "notApplied"
		if emitted {
			outcome = "unknown"
		}
		return &TransportError{RequestID: request.ID, Outcome: outcome, cause: err}
	}
	if len(replies) == 0 {
		return &TransportError{RequestID: request.ID, Outcome: "unknown", cause: ErrUnavailable}
	}
	if len(replies) != 1 {
		return &TransportError{RequestID: request.ID, Outcome: "unknown", cause: ErrAmbiguous}
	}
	reply := replies[0]
	if !reply.OK {
		return reply.Error
	}
	if result == nil {
		return nil
	}
	encoded, err := json.Marshal(reply.Result)
	if err != nil {
		return &TransportError{RequestID: request.ID, Outcome: "unknown", cause: errors.New("Workbench result could not be decoded")}
	}
	if err := json.Unmarshal(encoded, result); err != nil {
		return &TransportError{RequestID: request.ID, Outcome: "unknown", cause: errors.New("Workbench result shape is incompatible")}
	}
	return nil
}

func (c *Client) request(ctx sdk.Context, generation, method string, params any, options *CallOptions) (Request, error) {
	if err := ctx.Err(); err != nil {
		return Request{}, err
	}
	id, err := randomID()
	if err != nil {
		return Request{}, err
	}
	session, err := ctx.GetSessionID()
	if err != nil {
		return Request{}, err
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return Request{}, errors.New("Workbench params must be JSON-serializable")
	}
	request := Request{Version: 1, ID: id, SessionID: session, Generation: generation, From: c.identity, Method: method, Params: encoded, TimeoutMS: 30000}
	if options != nil {
		if options.TimeoutMS != 0 {
			request.TimeoutMS = options.TimeoutMS
		}
		request.RegistrationID = options.RegistrationID
	}
	if _, err := DecodeRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (c *Client) exchange(ctx sdk.Context, request Request) ([]Reply, bool, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, false, ErrClosed
	}
	if len(c.pending) >= 8 {
		c.mu.Unlock()
		return nil, false, &Error{Code: "busy", Message: "client has 8 in-flight requests", Retryable: true, Outcome: "notApplied"}
	}
	pending := &pendingCall{session: request.SessionID, generation: request.Generation, discover: request.Method == "discover"}
	c.pending[request.ID] = pending
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, request.ID); c.mu.Unlock() }()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	emitErr := ctx.Events().Emit(RequestChannel, request)
	c.mu.Lock()
	replies := slices.Clone(pending.replies)
	closed := c.closed
	c.mu.Unlock()
	// A correlated reply is a known outcome even when another listener failed the emit.
	if len(replies) > 0 {
		return replies, true, nil
	}
	if !request.discover() {
		c.cancel(ctx, request)
	}
	if emitErr != nil {
		return nil, true, emitErr
	}
	if closed {
		return nil, true, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	return nil, true, nil
}

// cancel asks Workbench to stop one of this client's requests; it is best effort and expects no reply.
func (r Request) discover() bool { return r.Method == "discover" }

func (c *Client) cancel(ctx sdk.Context, request Request) {
	params, err := json.Marshal(map[string]string{"requestId": request.ID})
	if err != nil {
		return
	}
	id, err := randomID()
	if err != nil {
		return
	}
	_ = ctx.Events().Emit(CancelChannel, Request{Version: 1, ID: id, SessionID: request.SessionID, Generation: request.Generation, From: c.identity, Method: "cancel", Params: params})
}

func (c *Client) onReply(_ sdk.Context, value any) error {
	reply, err := DecodeReply(value)
	if err != nil {
		return nil
	}
	if reply.To != c.identity {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending[reply.ID]
	if pending == nil || reply.SessionID != pending.session {
		return nil
	}
	if !pending.discover && reply.Generation != pending.generation && (reply.Error == nil || reply.Error.Code != "staleGeneration") {
		return nil
	}
	if len(pending.replies) < 2 {
		pending.replies = append(pending.replies, reply)
	}
	return nil
}

func randomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create Workbench correlation ID: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
