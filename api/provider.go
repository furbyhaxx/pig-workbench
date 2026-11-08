package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// ProviderRequestChannel carries Workbench requests to registered contributions.
const ProviderRequestChannel = "workbench:v1:provider-request"

// ProviderCancelChannel carries cancellation of one in-flight provider request.
// It is separate because the host serializes a listener per channel, so a busy handler cannot see a cancel queued behind it.
const ProviderCancelChannel = "workbench:v1:provider-cancel"

// ProviderReplyChannel carries contribution replies back to Workbench.
const ProviderReplyChannel = "workbench:v1:provider-reply"

// ProviderRequest is one Workbench call addressed to a registered contribution owner.
type ProviderRequest struct {
	Version        int             `json:"v"`
	ID             string          `json:"id"`
	SessionID      string          `json:"sessionId"`
	Generation     string          `json:"generation"`
	From           Identity        `json:"from"`
	To             Identity        `json:"to"`
	RegistrationID string          `json:"registrationId"`
	Method         string          `json:"method"`
	Params         json.RawMessage `json:"params"`
	TimeoutMS      int             `json:"timeoutMs,omitempty"`

	ctx context.Context
}

// Context is cancelled when the request's budget elapses or Workbench sends a cancel for it.
// A handler doing more than bounded validation must poll it and stop with a "cancelled" error.
func (r ProviderRequest) Context() context.Context {
	if r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}

// Receipt acknowledges one accepted registration.
type Receipt struct {
	RegistrationID     string `json:"registrationId"`
	DescriptorRevision string `json:"descriptorRevision"`
	Kind               string `json:"kind"`
	Key                string `json:"key"`
	Idempotent         bool   `json:"idempotent"`
	AppliesAfterReload bool   `json:"appliesAfterReload"`
}

// Project is the calling session's project identity; absent in the global project.
type Project struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Component string `json:"component,omitempty"`
}

// LineRange is an inclusive one-based line span; End 0 means through EOF.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end,omitempty"`
}

// Selector is a Workbench line selection forwarded to a lines-mode provider.
type Selector struct {
	Kind   string      `json:"kind"`
	Ranges []LineRange `json:"ranges,omitempty"`
	Tails  []int       `json:"tails,omitempty"`
	Raw    bool        `json:"raw"`
}

// PageLimits bound one provider page.
type PageLimits struct {
	Bytes int `json:"bytes"`
	Lines int `json:"lines"`
}

// ResourceRequest is the `resource.execute` payload (SPEC-0004 §4).
type ResourceRequest struct {
	OperationID      string            `json:"operationId"`
	Operation        string            `json:"operation"`
	RequestedURI     string            `json:"requestedUri"`
	URI              string            `json:"uri"`
	Selector         *Selector         `json:"selector,omitempty"`
	Cursor           string            `json:"cursor,omitempty"`
	ExpectedRevision string            `json:"expectedRevision,omitempty"`
	Limits           PageLimits        `json:"limits"`
	Project          *Project          `json:"project,omitempty"`
	Mutation         *ResourceMutation `json:"mutation,omitempty"`
}

// ResourceEntry is one typed listing row.
type ResourceEntry struct {
	Name     string `json:"name"`
	URI      string `json:"uri"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size,omitempty"`
	Revision string `json:"revision,omitempty"`
	Target   string `json:"target,omitempty"`
	NoWalk   bool   `json:"noWalk,omitempty"`
}

// TextPage is a bounded text page with exact coverage facts.
type TextPage struct {
	Text              string      `json:"text"`
	ShownRanges       []LineRange `json:"shownRanges"`
	TotalLines        *int        `json:"totalLines,omitempty"`
	SelectionComplete bool        `json:"selectionComplete"`
	ResourceComplete  bool        `json:"resourceComplete"`
}

// ResourceResult is a provider's answer to stat, read or list.
type ResourceResult struct {
	CanonicalURI string           `json:"canonicalUri"`
	Kind         string           `json:"kind"`
	Revision     string           `json:"revision,omitempty"`
	Size         int64            `json:"size,omitempty"`
	Lines        int              `json:"lines,omitempty"`
	Page         *TextPage        `json:"page,omitempty"`
	Entries      []ResourceEntry  `json:"entries,omitempty"`
	NextCursor   string           `json:"nextCursor,omitempty"`
	Complete     *bool            `json:"complete,omitempty"`
	Receipt      *MutationReceipt `json:"receipt,omitempty"`
}

// FetchRequest asks a fetch backend for one URL (SPEC-0004 §11).
type FetchRequest struct {
	URL       string   `json:"url"`
	Refresh   bool     `json:"refresh"`
	Offline   bool     `json:"offline"`
	Accept    string   `json:"accept,omitempty"`
	TimeoutMS int      `json:"timeoutMs"`
	MaxBytes  int      `json:"maxBytes"`
	Project   *Project `json:"project,omitempty"`
}

// FetchResult is a fetch backend's body or locate answer.
type FetchResult struct {
	Body           *string  `json:"body,omitempty"`
	Locate         string   `json:"locate,omitempty"`
	FinalURL       string   `json:"finalUrl"`
	Redirects      []string `json:"redirects,omitempty"`
	Status         int      `json:"status"`
	ContentType    string   `json:"contentType,omitempty"`
	Charset        string   `json:"charset,omitempty"`
	Revision       string   `json:"revision,omitempty"`
	Handler        string   `json:"handler,omitempty"`
	UsedThirdParty bool     `json:"usedThirdParty"`
	Notices        []string `json:"notices,omitempty"`
}

// ProviderHandler answers one Workbench provider request with a result or a protocol error.
type ProviderHandler func(ctx sdk.Context, request ProviderRequest) (any, *Error)

// Identity returns this client's owner and instance, which Workbench addresses provider calls to.
func (c *Client) Identity() Identity { return c.identity }

// Register admits one contribution and returns its receipt.
func (c *Client) Register(ctx sdk.Context, endpoint Discovery, kind, key string, descriptor any) (Receipt, error) {
	var receipt Receipt
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return receipt, errors.New("descriptor must be JSON-serializable")
	}
	params := map[string]any{"kind": kind, "key": key, "descriptor": json.RawMessage(encoded)}
	err = c.Call(ctx, endpoint, "register", params, &receipt, nil)
	return receipt, err
}

// Unregister withdraws one registration; repeating it is a no-op.
func (c *Client) Unregister(ctx sdk.Context, endpoint Discovery, registrationID string) error {
	return c.Call(ctx, endpoint, "unregister", map[string]any{}, nil, &CallOptions{RegistrationID: registrationID})
}

// Serve answers Workbench provider requests addressed to this client until the returned stop runs.
// The handler runs inside the listener and must stay bounded; it never sees requests for other owners.
func (c *Client) Serve(ext *sdk.Extension, handler ProviderHandler) (func(), error) {
	if handler == nil {
		return nil, errors.New("provider handler must not be nil")
	}
	var activeMu sync.Mutex
	active := make(map[string]context.CancelFunc)
	stopCancel, err := ext.Events().On(ProviderCancelChannel, func(_ sdk.Context, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) > ControlBytes {
			return nil
		}
		var request ProviderRequest
		if err := Decode(encoded, &request); err != nil || request.Version != 1 || request.To != c.identity || request.Method != "cancel" {
			return nil
		}
		var params struct {
			RequestID string `json:"requestId"`
		}
		if Decode(request.Params, &params) != nil {
			return nil
		}
		activeMu.Lock()
		cancel := active[params.RequestID]
		activeMu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	stopRequests, err := ext.Events().On(ProviderRequestChannel, func(ctx sdk.Context, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) > ControlBytes {
			return nil
		}
		var request ProviderRequest
		if err := Decode(encoded, &request); err != nil {
			return nil
		}
		if request.Version != 1 || request.To != c.identity || !validWireID(request.ID) || !validWireID(request.From.Owner) || !validWireID(request.From.Instance) {
			return nil
		}
		budget := time.Duration(request.TimeoutMS) * time.Millisecond
		if budget <= 0 {
			budget = 30 * time.Second
		}
		work, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()
		request.ctx = work
		activeMu.Lock()
		active[request.ID] = cancel
		activeMu.Unlock()
		defer func() { activeMu.Lock(); delete(active, request.ID); activeMu.Unlock() }()
		reply := Reply{Version: 1, ID: request.ID, SessionID: request.SessionID, Generation: request.Generation, To: request.From}
		result, protocol := handler(ctx, request)
		switch {
		case protocol != nil:
			reply.Error = protocol
		case result == nil:
			reply.Error = &Error{Code: "providerFailed", Message: "provider returned no result"}
		default:
			reply.OK, reply.Result = true, result
		}
		if encodedReply, err := json.Marshal(reply); err != nil || len(encodedReply) > ControlBytes {
			reply = Reply{Version: 1, ID: request.ID, SessionID: request.SessionID, Generation: request.Generation, To: request.From, Error: &Error{Code: "limitExceeded", Message: fmt.Sprintf("provider reply exceeds %d bytes", ControlBytes)}}
		}
		return ctx.Events().Emit(ProviderReplyChannel, reply)
	})
	if err != nil {
		stopCancel()
		return nil, err
	}
	return func() { stopRequests(); stopCancel() }, nil
}

func validWireID(value string) bool {
	return value != "" && len(value) <= 128 && strings.IndexFunc(value, unicode.IsControl) < 0
}
