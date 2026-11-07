package host

import (
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

func TestProviderBusRejectsMalformedReplyBranches(t *testing.T) {
	for name, reply := range map[string]api.Reply{
		"empty success":              {OK: true},
		"error with result":          {Error: &api.Error{Code: "providerFailed", Message: "failed"}, Result: map[string]any{"committed": true}},
		"empty error":                {Error: &api.Error{}},
		"unsafe unknown retry":       {Error: &api.Error{Code: "outcomeUnknown", Message: "unknown", Outcome: "unknown", Retryable: true}},
		"contradictory unknown code": {Error: &api.Error{Code: "outcomeUnknown", Message: "unknown", Outcome: "notApplied"}},
		"invalid outcome":            {Error: &api.Error{Code: "providerFailed", Message: "failed", Outcome: "probably-applied"}},
	} {
		t.Run(name, func(t *testing.T) {
			bus := newProviderBus("session", "generation", nil)
			bus.pending["request"] = &providerPending{to: api.Identity{Owner: "producer", Instance: "instance"}}
			reply.Version, reply.ID, reply.SessionID, reply.Generation, reply.To = 1, "request", "session", "generation", bus.identity
			if err := bus.onReply(sdk.Context{}, reply); err != nil {
				t.Fatal(err)
			}
			if len(bus.pending["request"].replies) != 0 {
				t.Fatal("malformed provider reply became an apparent result")
			}
		})
	}
}
