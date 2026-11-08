package host

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestProviderBusAdmissionIsBoundedPerOwnerAndReleasedOnce(t *testing.T) {
	bus := newProviderBus("session", "generation", nil)
	var releases []func()
	for range maxProviderInFlight {
		release, err := bus.admit("alpha")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	_, err := bus.admit("alpha")
	var provider *providerError
	if !errors.As(err, &provider) || provider.protocol.Code != "busy" || provider.protocol.Outcome != "notApplied" || !provider.protocol.Retryable {
		t.Fatalf("ninth operation was not refused as busy/notApplied: %v", err)
	}
	if release, err := bus.admit("beta"); err != nil {
		t.Fatalf("another owner was throttled by alpha: %v", err)
	} else {
		release()
	}
	releases[0]()
	releases[0]()
	if _, err := bus.admit("alpha"); err != nil {
		t.Fatalf("slot was not freed: %v", err)
	}
	if _, err := bus.admit("alpha"); err == nil {
		t.Fatal("double release freed a second slot")
	}
	for _, release := range releases[1:] {
		release()
	}
}

func TestProviderCallFailsBeforeEmitWhenOwnerIsAtItsLimit(t *testing.T) {
	bus := newProviderBus("session", "generation", func() (sdk.Context, bool) { t.Fatal("emit reached despite the limit"); return sdk.Context{}, false })
	for range maxProviderInFlight {
		if _, err := bus.admit("alpha"); err != nil {
			t.Fatal(err)
		}
	}
	err := bus.call(t.Context(), api.Identity{Owner: "alpha", Instance: "i"}, "reg", "resource.execute", map[string]any{}, nil, 0)
	var provider *providerError
	if !errors.As(err, &provider) || provider.protocol.Code != "busy" {
		t.Fatalf("limit was not enforced before dispatch: %v", err)
	}
}

func TestWithdrawnRegistrationRefusesNewOperationsBeforeDispatch(t *testing.T) {
	bus := newProviderBus("session", "generation", func() (sdk.Context, bool) {
		t.Fatal("emit reached a withdrawn registration")
		return sdk.Context{}, false
	})
	if !bus.withdraw("reg", time.Second) {
		t.Fatal("an idle registration did not drain")
	}
	err := bus.call(t.Context(), api.Identity{Owner: "alpha", Instance: "i"}, "reg", "resource.execute", map[string]any{}, nil, 0)
	var provider *providerError
	if !errors.As(err, &provider) || provider.protocol.Code != "unavailable" || provider.protocol.Outcome != "notApplied" {
		t.Fatalf("withdrawn registration was dispatched or mislabelled: %v", err)
	}
}

func TestWithdrawWaitsForActiveOperationsAndReopenRestoresTheProvider(t *testing.T) {
	bus := newProviderBus("session", "generation", nil)
	to := api.Identity{Owner: "alpha", Instance: "i"}
	end, err := bus.begin("reg", "req-1", to)
	if err != nil {
		t.Fatal(err)
	}
	drained := make(chan bool, 1)
	go func() { drained <- bus.withdraw("reg", 5*time.Second) }()
	select {
	case <-drained:
		t.Fatal("withdraw returned while an operation was still running")
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := bus.begin("reg", "req-2", to); err == nil {
		t.Fatal("admission stayed open while draining")
	}
	end()
	select {
	case ok := <-drained:
		if !ok {
			t.Fatal("withdraw reported an undrained registration after the operation ended")
		}
	case <-time.After(time.Second):
		t.Fatal("withdraw did not return after the last operation ended")
	}
	if _, err := bus.begin("reg", "req-3", to); err == nil {
		t.Fatal("registration reopened without an install")
	}
	bus.reopen("reg")
	if end, err := bus.begin("reg", "req-4", to); err != nil {
		t.Fatalf("rollback reinstall did not restore the provider: %v", err)
	} else {
		end()
	}
}

func TestWithdrawGivesUpAfterTheBoundAndReportsIt(t *testing.T) {
	bus := newProviderBus("session", "generation", nil)
	if _, err := bus.begin("reg", "stuck", api.Identity{Owner: "alpha", Instance: "i"}); err != nil {
		t.Fatal(err)
	}
	if bus.withdraw("reg", 30*time.Millisecond) {
		t.Fatal("a hung operation was reported as drained")
	}
}

func TestProviderProgressIsBoundedCorrelatedAndNewestWins(t *testing.T) {
	bus := newProviderBus("session", "generation", nil)
	owner := api.Identity{Owner: "build", Instance: "i-1"}
	other := api.Identity{Owner: "other", Instance: "i-9"}
	bus.pending["req-1"] = &providerPending{to: owner, registrationID: "reg-1"}
	bus.pending["req-2"] = &providerPending{to: other, registrationID: "reg-2"}
	note := func(request string, from api.Identity, seq int, message string) map[string]any {
		return map[string]any{"v": 1, "sessionId": "session", "generation": "generation", "from": from, "to": bus.identity, "requestId": request, "seq": seq, "message": message, "completed": seq, "total": 10}
	}
	deliver := func(value any) {
		t.Helper()
		if err := bus.onProgress(sdk.Context{}, value); err != nil {
			t.Fatal(err)
		}
	}

	deliver(note("req-1", owner, 2, "second"))
	deliver(note("req-1", owner, 1, "stale"))
	if got := bus.pending["req-1"].progress; got == nil || got.Message != "second" {
		t.Fatalf("an older note replaced a newer one: %+v", got)
	}
	deliver(note("req-1", other, 3, "spoofed"))
	deliver(note("req-gone", owner, 4, "late"))
	deliver(note("req-1", owner, 3, "bad\x1b[31m"))
	deliver(map[string]any{"v": 1, "seq": 9})
	if got := bus.pending["req-1"].progress; got.Message != "second" {
		t.Fatalf("invalid or foreign progress updated retained state: %+v", got)
	}
	if bus.pending["req-2"].progress != nil {
		t.Fatal("progress leaked into another request")
	}
	deliver(note("req-1", owner, 4, strings.Repeat("x", api.MaxProgressMessage+1)))
	if got := bus.pending["req-1"].progress; got.Seq != 2 {
		t.Fatalf("oversized message accepted: %+v", got)
	}
	running, latest := bus.activity("reg-1")
	if running != 1 || latest == nil || latest.Message != "second" {
		t.Fatalf("activity = %d %+v", running, latest)
	}
	if text := bus.activityText("reg-1"); !strings.Contains(text, "second") || !strings.Contains(text, "2/10") {
		t.Fatalf("activity text = %q", text)
	}
	if bus.activityText("reg-unknown") != "" {
		t.Fatal("idle registration reported activity")
	}
}
