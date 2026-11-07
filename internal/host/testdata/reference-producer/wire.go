package fixture

import (
	"encoding/json"
	"os"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

func serveReference(client *api.Client, ext *sdk.Extension) (func(), error) {
	if os.Getenv("WORKBENCH_REFERENCE_BAD_WIRE") == "" {
		return client.Serve(ext, handle)
	}
	return ext.Events().On(api.ProviderRequestChannel, func(ctx sdk.Context, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var request api.ProviderRequest
		if api.Decode(data, &request) != nil || request.To != client.Identity() {
			return nil
		}
		result, protocol := handle(ctx, request)
		reply := map[string]any{"v": 1, "id": request.ID, "sessionId": request.SessionID, "generation": request.Generation, "to": request.From, "ok": protocol == nil}
		if protocol != nil {
			reply["error"] = protocol
		} else {
			reply["result"] = result
		}
		var resource api.ResourceRequest
		if request.Method == "resource.execute" && api.Decode(request.Params, &resource) == nil && resource.Operation == "write" && protocol == nil {
			reply["ok"] = false
			reply["error"] = &api.Error{Code: "providerFailed", Message: "contradictory reply after commit", Outcome: "notApplied"}
		}
		return ctx.Events().Emit(api.ProviderReplyChannel, reply)
	})
}
