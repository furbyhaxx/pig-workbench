package fixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

var (
	clientMu sync.Mutex
	client   *api.Client
)

// Extension creates a cooperative service consumer for the integration suite.
func Extension() *sdk.Extension {
	ext := sdk.New("service-client")
	ext.OnLoaded(func(ctx sdk.Context) {
		created, err := api.NewClient(ext)
		if err != nil {
			ctx.Notify("cannot initialize Workbench client: "+err.Error(), "error")
			return
		}
		clientMu.Lock()
		old := client
		client = created
		clientMu.Unlock()
		if old != nil {
			old.Close()
		}
	})
	ext.OnLoaded(func(ctx sdk.Context) {
		_, _ = ext.Events().On(api.EventChannel, func(ctx sdk.Context, value any) error {
			encoded, err := json.Marshal(value)
			if err != nil || !strings.Contains(string(encoded), `"sessions.message"`) {
				return nil
			}
			ctx.Notify(`{"sessionsMessage":`+string(encoded)+`}`, "info")
			return nil
		})
	})
	ext.Command("fixture-send", "Send an extension-channel message to another live session.", func(ctx sdk.Context, args string) error {
		clientMu.Lock()
		client := client
		clientMu.Unlock()
		if client == nil {
			return errors.New("client not initialized")
		}
		endpoint, err := client.Discover(ctx, "sessions")
		if err != nil {
			return err
		}
		var listed api.SessionsListResult
		if err := client.Call(ctx, endpoint, "sessions.list", map[string]any{}, &listed, nil); err != nil {
			return err
		}
		var denied api.SessionsSendResult
		err = client.Call(ctx, endpoint, "sessions.send", api.SessionsSendParams{Target: strings.TrimSpace(args), Channel: "ext:someone-else", Body: "must be refused"}, &denied, nil)
		var protocol *api.Error
		if !errors.As(err, &protocol) || protocol.Code != "permissionDenied" {
			return fmt.Errorf("a foreign ext: channel was not refused: %v", err)
		}
		var sent api.SessionsSendResult
		if err := client.Call(ctx, endpoint, "sessions.send", api.SessionsSendParams{Target: strings.TrimSpace(args), Channel: "ext:service-client", Body: "EXT-CHANNEL-BODY"}, &sent, nil); err != nil {
			return err
		}
		output, _ := json.Marshal(map[string]any{"fixtureSend": "passed", "listed": len(listed.Sessions), "receipt": sent})
		ctx.Notify(string(output), "info")
		return nil
	})
	ext.OnSessionShutdown(func(sdk.Context, map[string]any) (any, error) {
		clientMu.Lock()
		old := client
		client = nil
		clientMu.Unlock()
		if old != nil {
			old.Close()
		}
		return nil, nil
	})
	ext.Command("fixture-errors", "Probe optional-service failures without Workbench.", func(ctx sdk.Context, _ string) error {
		clientMu.Lock()
		client := client
		clientMu.Unlock()
		if client == nil {
			return errors.New("client not initialized")
		}
		if _, err := client.Discover(ctx); !errors.Is(err, api.ErrUnavailable) {
			return fmt.Errorf("absent service: %v", err)
		}
		var modeMu sync.Mutex
		mode := "incompatible"
		unsubscribe, err := ext.Events().On(api.RequestChannel, func(ctx sdk.Context, value any) error {
			request, err := api.DecodeRequest(value)
			if err != nil {
				return err
			}
			if request.Method != "discover" {
				modeMu.Lock()
				currentMode := mode
				modeMu.Unlock()
				switch currentMode {
				case "reply-then-fail":
					reply := api.Reply{Version: 1, ID: request.ID, SessionID: request.SessionID, Generation: "fixture-generation", To: request.From, OK: true, Result: map[string]any{"applied": true}}
					if err := ctx.Events().Emit(api.ReplyChannel, reply); err != nil {
						return err
					}
					return errors.New("a sibling listener failed after the reply")
				case "fail-silently":
					return errors.New("listener failed without replying")
				}
				return nil
			}
			reply := api.Reply{Version: 1, ID: request.ID, SessionID: request.SessionID, Generation: "fixture-generation", To: request.From}
			modeMu.Lock()
			currentMode := mode
			modeMu.Unlock()
			if currentMode == "incompatible" {
				reply.Error = &api.Error{Code: "incompatibleVersion", Message: "fixture major mismatch"}
				return ctx.Events().Emit(api.ReplyChannel, reply)
			}
			reply.OK = true
			reply.Result = api.Discovery{Major: 1, SessionID: request.SessionID, Generation: reply.Generation, Instance: "fixture", Limits: api.Limits{ControlBytes: api.ControlBytes}}
			if err := ctx.Events().Emit(api.ReplyChannel, reply); err != nil {
				return err
			}
			if currentMode == "ambiguous" {
				reply.Generation = "second-generation"
				return ctx.Events().Emit(api.ReplyChannel, reply)
			}
			return nil
		})
		if err != nil {
			return err
		}
		defer unsubscribe()
		var cancelMu sync.Mutex
		var cancelled []string
		stopCancel, err := ext.Events().On(api.CancelChannel, func(_ sdk.Context, value any) error {
			request, err := api.DecodeRequest(value)
			if err != nil || request.Method != "cancel" {
				return nil
			}
			var params struct {
				RequestID string `json:"requestId"`
			}
			if err := api.Decode(request.Params, &params); err != nil {
				return nil
			}
			cancelMu.Lock()
			cancelled = append(cancelled, params.RequestID)
			cancelMu.Unlock()
			return nil
		})
		if err != nil {
			return err
		}
		defer stopCancel()
		_, err = client.Discover(ctx)
		var protocol *api.Error
		if !errors.As(err, &protocol) || protocol.Code != "incompatibleVersion" {
			return fmt.Errorf("incompatible service: %v", err)
		}
		modeMu.Lock()
		mode = "ambiguous"
		modeMu.Unlock()
		if _, err := client.Discover(ctx); !errors.Is(err, api.ErrAmbiguous) {
			return fmt.Errorf("duplicate service: %v", err)
		}
		modeMu.Lock()
		mode = "single"
		modeMu.Unlock()
		endpoint, err := client.Discover(ctx)
		if err != nil {
			return err
		}
		err = client.Call(ctx, endpoint, "register", map[string]any{}, nil, nil)
		var transport *api.TransportError
		if !errors.As(err, &transport) || transport.Outcome != "unknown" || transport.RequestID == "" || !errors.Is(err, api.ErrUnavailable) {
			return fmt.Errorf("missing mutation receipt was not unknown: %v", err)
		}
		modeMu.Lock()
		mode = "reply-then-fail"
		modeMu.Unlock()
		var known map[string]any
		if err := client.Call(ctx, endpoint, "location.get", map[string]any{}, &known, nil); err != nil || known["applied"] != true {
			return fmt.Errorf("a correlated reply was discarded because another listener failed: %v", err)
		}
		modeMu.Lock()
		mode = "fail-silently"
		modeMu.Unlock()
		err = client.Call(ctx, endpoint, "location.get", map[string]any{}, &known, nil)
		if !errors.As(err, &transport) || transport.Outcome != "unknown" || transport.RequestID == "" {
			return fmt.Errorf("a failed emit without a reply was not unknown: %v", err)
		}
		cancelMu.Lock()
		sent := slices.Clone(cancelled)
		cancelMu.Unlock()
		if len(sent) == 0 || sent[len(sent)-1] != transport.RequestID {
			return fmt.Errorf("cancel did not carry the original request identity: sent=%v request=%s", sent, transport.RequestID)
		}
		ctx.Notify(`{"fixtureErrors":"passed"}`, "info")
		return nil
	})
	ext.Command("fixture", "Probe the real Workbench service bus.", func(ctx sdk.Context, _ string) error {
		clientMu.Lock()
		client := client
		clientMu.Unlock()
		if client == nil {
			return errors.New("client not initialized")
		}
		endpoint, err := client.Discover(ctx, "location")
		if err != nil {
			return err
		}
		var location map[string]any
		if err := client.Call(ctx, endpoint, "location.get", map[string]any{}, &location, nil); err != nil {
			return err
		}
		stale := endpoint
		stale.Generation = "old-generation"
		var unused map[string]any
		err = client.Call(ctx, stale, "location.get", map[string]any{}, &unused, nil)
		var protocol *api.Error
		if !errors.As(err, &protocol) || protocol.Code != "staleGeneration" {
			return fmt.Errorf("stale generation was not rejected: %v", err)
		}
		tooLarge := map[string]any{"text": strings.Repeat("x", api.ControlBytes)}
		err = client.Call(ctx, endpoint, "location.get", tooLarge, &unused, nil)
		if !errors.As(err, &protocol) || protocol.Code != "limitExceeded" || protocol.Outcome != "" && protocol.Outcome != "notApplied" {
			return fmt.Errorf("oversized request was not rejected locally and safely: %v", err)
		}
		session, err := ctx.GetSessionID()
		if err != nil {
			return err
		}
		output, err := json.Marshal(map[string]any{"fixture": "passed", "generation": endpoint.Generation, "sessionId": session, "location": location, "stale": protocol.Code, "limits": endpoint.Limits})
		if err != nil {
			return err
		}
		ctx.Notify(string(output), "info")
		return nil
	})
	return ext
}
