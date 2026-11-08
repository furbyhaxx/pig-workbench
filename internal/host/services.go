package host

import (
	"context"
	"errors"
	"slices"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
)

func (a *app) service(ctx sdk.Context, value any) error {
	request, decodeErr := api.DecodeRequest(value)
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil
	}
	defer release()
	if request.SessionID != s.id {
		return nil
	}
	if request.From.Owner == "" || request.From.Instance == "" || request.ID == "" {
		return nil
	}
	if request.Method == "cancel" {
		if decodeErr == nil {
			return a.cancelService(s, request)
		}
		return nil
	}
	reply := api.Reply{Version: 1, ID: request.ID, SessionID: s.id, Generation: s.generation, To: request.From}
	if decodeErr != nil {
		reply.Error = protocolError(decodeErr)
	} else if request.Method != "discover" && request.Generation != s.generation {
		reply.Error = &api.Error{Code: "staleGeneration", Message: "rediscover Workbench before making service requests", Retryable: true, Outcome: "notApplied"}
	} else {
		timeout := request.TimeoutMS
		if timeout == 0 {
			timeout = 30000
		}
		bounded, cancel := context.WithTimeout(work, time.Duration(timeout)*time.Millisecond)
		defer cancel()
		key := request.From.Owner + "\x00" + request.From.Instance + "\x00" + request.ID
		s.requestsMu.Lock()
		ownerCount := 0
		for _, r := range s.requests {
			if r.owner == request.From.Owner {
				ownerCount++
			}
		}
		if _, exists := s.requests[key]; exists {
			reply.Error = &api.Error{Code: "conflict", Message: "request ID is already in flight"}
		} else if ownerCount >= 8 {
			reply.Error = &api.Error{Code: "busy", Message: "owner has 8 in-flight requests", Retryable: true}
		} else {
			s.requests[key] = activeRequest{owner: request.From.Owner, cancel: cancel}
		}
		s.requestsMu.Unlock()
		if reply.Error == nil {
			defer func() { s.requestsMu.Lock(); delete(s.requests, key); s.requestsMu.Unlock() }()
			reply.Result, reply.Error = a.dispatchService(bounded, ctx, s, request)
		}
	}
	reply.OK = reply.Error == nil
	return ctx.Events().Emit(api.ReplyChannel, reply)
}

// advertisedCapabilities lists exactly the wired discover capabilities.
var advertisedCapabilities = []string{"location", "presentation", "resources", "fetch", "status", "docs", "frames", "snapshots"}

func (a *app) dispatchService(ctx context.Context, host sdk.Context, s *session, request api.Request) (any, *api.Error) {
	if err := ctx.Err(); err != nil {
		return nil, protocolError(err)
	}
	switch request.Method {
	case "discover":
		var params api.DiscoveryRequest
		if err := api.Decode(request.Params, &params); err != nil {
			return nil, &api.Error{Code: "invalidRequest", Message: "discover expects majors and capabilities"}
		}
		if !slices.Contains(params.Majors, 1) {
			return nil, &api.Error{Code: "incompatibleVersion", Message: "Workbench supports protocol major 1"}
		}
		capabilities := slices.Clone(advertisedCapabilities)
		if s.endpoint != nil {
			capabilities = append(capabilities, "sessions")
		}
		if s.secrets.granted(request.From.Owner) {
			capabilities = append(capabilities, "secrets")
		}
		for _, capability := range params.Capabilities {
			if !slices.Contains(capabilities, capability) {
				return nil, &api.Error{Code: "unsupported", Field: "params.capabilities", Message: "one or more requested capabilities are unavailable"}
			}
		}
		result := api.Discovery{Major: 1, Minor: 0, WorkbenchVersion: "development", Instance: s.generation, SessionID: s.id, Generation: s.generation, Capabilities: capabilities, Limits: api.Limits{ControlBytes: api.ControlBytes, PageBytes: 51200, PageLines: 2000, InFlightPerOwner: 8}}
		if slices.Contains(params.Capabilities, "location") {
			result.Location = s.location.Current()
		}
		return result, nil
	case "register":
		var params registrationParams
		if api.Decode(request.Params, &params) == nil && params.Kind == string(contribution.KindToolPresentation) {
			tools, err := host.GetAllTools()
			if err != nil {
				return nil, &api.Error{Code: "permissionDenied", Message: "verificationRequired: host tool ownership metadata is unavailable", Outcome: "notApplied"}
			}
			ctx = context.WithValue(ctx, presentationToolsKey{}, tools)
			trusted, err := host.IsProjectTrusted()
			if err != nil {
				return nil, protocolError(err)
			}
			ctx = context.WithValue(ctx, displayTrustKey{}, trusted)
		}
		result, failure := a.registerContribution(ctx, s, request)
		if failure == nil && params.Kind == string(contribution.KindToolPresentation) {
			if err := a.retainDisplaySchema(host, s, request, result); err != nil {
				host.Notify("Display schema registered but not retained for replay: "+noticeText(err.Error(), 0), "warning")
			}
		}
		return result, failure
	case "unregister":
		if current, err := s.contributions.Get(request.From, request.RegistrationID); err == nil && current.Kind == contribution.KindToolPresentation {
			trusted, err := host.IsProjectTrusted()
			if err != nil {
				return nil, protocolError(err)
			}
			ctx = context.WithValue(ctx, displayTrustKey{}, trusted)
		}
		return a.unregisterContribution(ctx, s, request)
	case "describe":
		return a.describeContributions(ctx, s, request)
	case "settings.get":
		return a.displaySettingsGet(s, request)
	case "resources.changed":
		return a.resourcesChanged(s, request)
	case "status.update":
		return a.statusUpdate(host, s, request)
	case "frames.publish":
		return a.framesPublish(host, s, request)
	case "frames.status", "frames.get", "frames.cancel", "frames.list":
		return a.framesQuery(s, request)
	case "context.fields.set":
		return a.contextFieldsSet(host, s, request)
	case "snapshots.view", "snapshots.status", "snapshots.touched":
		return a.snapshotsService(ctx, host, s, request)
	case "sessions.list", "sessions.send":
		return a.sessionsService(ctx, s, request)
	case "secrets.redact":
		return a.secretsRedact(s, request)
	case "location.get":
		var params struct{}
		if err := api.Decode(request.Params, &params); err != nil {
			return nil, &api.Error{Code: "invalidRequest", Message: "location.get takes empty params"}
		}
		return s.location.Current(), nil
	default:
		return nil, &api.Error{Code: "unsupported", Message: "service method is not available: " + request.Method}
	}
}

// cancelRequest serves cancel envelopes that arrive on their own channel.
func (a *app) cancelRequest(ctx sdk.Context, value any) error {
	request, err := api.DecodeRequest(value)
	if err != nil || request.Method != "cancel" {
		return nil
	}
	s, _, release, admitErr := a.admit(ctx)
	if admitErr != nil {
		return nil
	}
	defer release()
	if request.SessionID != s.id {
		return nil
	}
	return a.cancelService(s, request)
}

func (a *app) cancelService(s *session, request api.Request) error {
	if request.Generation != s.generation {
		return nil
	}
	var params struct {
		RequestID string `json:"requestId"`
	}
	if err := api.Decode(request.Params, &params); err != nil {
		return nil
	}
	key := request.From.Owner + "\x00" + request.From.Instance + "\x00" + params.RequestID
	s.requestsMu.Lock()
	r, ok := s.requests[key]
	s.requestsMu.Unlock()
	if ok {
		r.cancel()
	}
	return nil
}

func protocolError(err error) *api.Error {
	var protocol *api.Error
	if errors.As(err, &protocol) {
		return protocol
	}
	switch {
	case errors.Is(err, context.Canceled):
		return &api.Error{Code: "cancelled", Message: "request cancelled", Retryable: true}
	case errors.Is(err, context.DeadlineExceeded):
		return &api.Error{Code: "deadlineExceeded", Message: "request deadline exceeded", Retryable: true}
	default:
		return &api.Error{Code: "internal", Message: "Workbench service failed"}
	}
}
