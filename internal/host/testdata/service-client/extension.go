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
