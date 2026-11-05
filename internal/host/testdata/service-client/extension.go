package fixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

var (
	initializeOnce sync.Once
	initializeErr  error
	client         *api.Client
)

// Extension creates a cooperative service consumer for the integration suite.
func Extension() *sdk.Extension {
	ext := sdk.New("service-client")
	ext.OnLoaded(func(ctx sdk.Context) {
		initializeOnce.Do(func() {
			client, initializeErr = api.NewClient(ext)
		})
		if initializeErr != nil {
			ctx.Notify("cannot initialize Workbench client: "+initializeErr.Error(), "error")
		}
	})
	ext.Command("fixture-errors", "Probe optional-service failures without Workbench.", func(ctx sdk.Context, _ string) error {
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
		ctx.Notify(`{"fixtureErrors":"passed"}`, "info")
		return nil
	})
	ext.Command("fixture", "Probe the real Workbench service bus.", func(ctx sdk.Context, _ string) error {
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
