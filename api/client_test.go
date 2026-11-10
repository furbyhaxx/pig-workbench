package api

import (
	"encoding/json"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func TestClientIgnoresUnboundReplies(t *testing.T) {
	client := &Client{identity: Identity{Owner: "build", Instance: "one"}, pending: map[string]*pendingCall{"request": {session: "session", generation: "generation"}}}
	base := Reply{Version: 1, ID: "request", SessionID: "session", Generation: "generation", To: client.identity, OK: true, Result: map[string]any{}}
	for _, mutate := range []func(*Reply){
		func(r *Reply) { r.SessionID = "other" },
		func(r *Reply) { r.To = Identity{Owner: "build", Instance: "two"} },
		func(r *Reply) { r.Generation = "" },
		func(r *Reply) { r.Generation = "old-generation" },
		func(r *Reply) { r.ID = "expired-request" },
		func(r *Reply) { r.Error = &Error{Code: "internal", Message: "both"} },
		func(r *Reply) { r.Result = nil },
		func(r *Reply) { r.Version = 2 },
	} {
		reply := base
		mutate(&reply)
		value, _ := json.Marshal(reply)
		if err := client.onReply(sdk.Context{}, json.RawMessage(value)); err != nil {
			t.Fatal(err)
		}
		if len(client.pending["request"].replies) != 0 {
			t.Fatalf("foreign or malformed reply accepted: %+v", reply)
		}
		client.pending["request"].replies = nil
	}
	value, _ := json.Marshal(base)
	if err := client.onReply(sdk.Context{}, json.RawMessage(value)); err != nil {
		t.Fatal(err)
	}
	if len(client.pending["request"].replies) != 1 {
		t.Fatal("correlated reply dropped")
	}
}

func TestClientDiscardsLateReplies(t *testing.T) {
	client := &Client{identity: Identity{Owner: "build", Instance: "one"}, pending: make(map[string]*pendingCall)}
	reply := Reply{Version: 1, ID: "expired", SessionID: "session", Generation: "generation", To: client.identity, OK: true, Result: map[string]any{"applied": true}}
	if err := client.onReply(sdk.Context{}, reply); err != nil {
		t.Fatal(err)
	}
	client.pending["next"] = &pendingCall{session: "session", generation: "generation"}
	if err := client.onReply(sdk.Context{}, reply); err != nil {
		t.Fatal(err)
	}
	if len(client.pending["next"].replies) != 0 {
		t.Fatal("late mutation reply completed a different request")
	}
}

func TestUnknownParamsAndTransportErrorRedaction(t *testing.T) {
	var request DiscoveryRequest
	if err := Decode([]byte(`{"majors":[1],"unknown":true}`), &request); err == nil {
		t.Fatal("unknown params accepted")
	}
	transport := &TransportError{RequestID: "request", Outcome: "unknown"}
	if text := transport.Error(); contains(text, "result") || contains(text, "secret") {
		t.Fatal("transport error leaked detail")
	}
	if transport.Unwrap() != nil {
		t.Fatal("expected no cause")
	}
}

func contains(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
