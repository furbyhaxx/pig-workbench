package fixture

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

// Extension creates a cooperative service consumer for the integration suite.
func Extension() *sdk.Extension {
	ext := sdk.New("service-client")
	ext.Command("fixture", "Probe the real Workbench service bus.", func(ctx sdk.Context, _ string) error {
		id, err := ctx.GetSessionID()
		if err != nil {
			return err
		}
		var mu sync.Mutex
		var replies []api.Reply
		off, err := ctx.Events().On(api.ReplyChannel, func(_ sdk.Context, value any) error {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var reply api.Reply
			if err := json.Unmarshal(data, &reply); err != nil {
				return err
			}
			if reply.SessionID == id && reply.To.Owner == "service-client" && reply.To.Instance == "fixture" {
				mu.Lock()
				replies = append(replies, reply)
				mu.Unlock()
			}
			return nil
		})
		if err != nil {
			return err
		}
		defer off()
		call := func(method, generation string, params any) (api.Reply, error) {
			mu.Lock()
			replies = nil
			mu.Unlock()
			encoded, err := json.Marshal(params)
			if err != nil {
				return api.Reply{}, err
			}
			request := api.Request{Version: 1, ID: method, SessionID: id, Generation: generation, From: api.Identity{Owner: "service-client", Instance: "fixture"}, Method: method, Params: encoded}
			if err := ctx.Events().Emit(api.RequestChannel, request); err != nil {
				return api.Reply{}, err
			}
			mu.Lock()
			defer mu.Unlock()
			if len(replies) != 1 {
				return api.Reply{}, fmt.Errorf("expected one reply to %s, got %d", method, len(replies))
			}
			return replies[0], nil
		}
		discovered, err := call("discover", "", api.DiscoveryRequest{Majors: []int{1}, Capabilities: []string{"location"}})
		if err != nil {
			return err
		}
		if !discovered.OK {
			return discovered.Error
		}
		location, err := call("location.get", discovered.Generation, map[string]any{})
		if err != nil {
			return err
		}
		if !location.OK {
			return location.Error
		}
		stale, err := call("location.get", "old-generation", map[string]any{})
		if err != nil {
			return err
		}
		if stale.OK || stale.Error == nil || stale.Error.Code != "staleGeneration" {
			return fmt.Errorf("stale generation was not rejected")
		}
		oversized, err := call("location.get", discovered.Generation, map[string]any{"text": strings.Repeat("x", api.ControlBytes)})
		if err != nil {
			return err
		}
		if oversized.OK || oversized.Error == nil || oversized.Error.Code != "limitExceeded" {
			return fmt.Errorf("oversized request was not rejected with a correlated error")
		}
		output, err := json.Marshal(map[string]any{"fixture": "passed", "generation": discovered.Generation, "sessionId": id, "location": location.Result, "stale": stale.Error.Code})
		if err != nil {
			return err
		}
		ctx.Notify(string(output), "info")
		return nil
	})
	return ext
}
