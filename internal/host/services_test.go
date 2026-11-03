package host

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/location"
)

func TestDiscoveryAndLocation(t *testing.T) {
	manager, err := location.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: "session", generation: "generation", location: manager}
	a := &app{}
	params, _ := json.Marshal(api.DiscoveryRequest{Majors: []int{1}, Capabilities: []string{"location"}})
	result, failure := a.dispatchService(context.Background(), s, api.Request{Method: "discover", Params: params})
	if failure != nil {
		t.Fatal(failure)
	}
	discovery := result.(api.Discovery)
	if discovery.Generation != "generation" || discovery.SessionID != "session" || discovery.Location.(location.Snapshot).Cwd != manager.Current().Cwd {
		t.Fatalf("wrong binding: %+v", discovery)
	}
	if len(discovery.Capabilities) != 1 || discovery.Capabilities[0] != "location" {
		t.Fatalf("unwired capability advertised: %+v", discovery)
	}
	result, failure = a.dispatchService(context.Background(), s, api.Request{Method: "location.get", Params: json.RawMessage(`{}`)})
	if failure != nil || result.(location.Snapshot) != manager.Current() {
		t.Fatalf("location service differs: %v %v", result, failure)
	}
	params, _ = json.Marshal(api.DiscoveryRequest{Majors: []int{1}, Capabilities: []string{"resources"}})
	_, failure = a.dispatchService(context.Background(), s, api.Request{Method: "discover", Params: params})
	if failure == nil || failure.Code != "unsupported" {
		t.Fatal("unavailable capability succeeded")
	}
}

func TestCancellationIsOwnerScoped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &session{generation: "generation", requests: map[string]activeRequest{"owner\x00instance\x00pending": {owner: "owner", cancel: cancel}}}
	request := api.Request{Generation: "generation", From: api.Identity{Owner: "other", Instance: "instance"}, Params: json.RawMessage(`{"requestId":"pending"}`)}
	a := &app{}
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("cross-owner cancel applied")
	}
	request.From.Owner = "owner"
	request.Generation = "stale"
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("stale generation cancel applied")
	}
	request.Generation = "generation"
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("owner cancellation lost")
	}
}
