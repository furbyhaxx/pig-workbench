package host

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/endpoint"
	"github.com/furbyhaxx/pig-workbench/internal/messaging"
)

func TestSessionMessageSettingsAffectDelivery(t *testing.T) {
	root := t.TempDir()
	settings, err := config.New(config.Paths{User: filepath.Join(root, "user.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	set := func(key string, value any) {
		t.Helper()
		current, err := settings.Current()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := settings.Save(t.Context(), config.Runtime, current, []config.Edit{{Key: key, Value: value}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	e := &sessionEndpoint{
		s:       &session{id: "receiver", settings: settings},
		record:  endpoint.Record{SessionID: "receiver", ProjectID: "project"},
		limiter: messaging.NewLimiter(),
		trusted: map[string]bool{}, blocked: map[string]bool{}, known: map[string]bool{},
		waiters: map[*inboxWaiter]struct{}{},
	}
	env := messaging.Envelope{ID: "m1", ToSessionID: "receiver", From: messaging.Peer{SessionID: "sender", ProjectID: "other"}}
	set("sessions.inbound", "off")
	if got := e.Message(env); got.Reason != messaging.PolicyDenied {
		t.Fatalf("inbound off: %+v", got)
	}
	set("sessions.inbound", "sameProject")
	if got := e.Message(env); got.Reason != messaging.PolicyDenied {
		t.Fatalf("different project: %+v", got)
	}
	set("sessions.inbound", "allowlist")
	if got := e.Message(env); got.Reason != messaging.PolicyDenied {
		t.Fatalf("untrusted peer: %+v", got)
	}
	e.trusted["sender"] = true
	set("sessions.rate-limit", int64(1))
	waiter := &inboxWaiter{ch: make(chan messaging.Envelope, 1)}
	e.waiters[waiter] = struct{}{}
	if got := e.Message(env); got.Status != messaging.Accepted {
		t.Fatalf("trusted peer: %+v", got)
	}
	if got := e.Message(env); got.Reason != messaging.RateLimited {
		t.Fatalf("rate limit: %+v", got)
	}
	set("sessions.wake", "never")
	if e.wakeAllowed(env, true, 8) {
		t.Fatal("wake=never woke the receiver")
	}
	set("sessions.wake", "trustedPeers")
	if e.wakeAllowed(env, false, 8) {
		t.Fatal("untrusted peer woke receiver")
	}
	set("sessions.wake-budget", int64(1))
	if !e.wakeAllowed(env, true, 8) || e.wakeAllowed(env, true, 8) {
		t.Fatal("wake budget did not admit exactly one wake")
	}
}
func TestAgentColumnsShowIdleTimeModelAndTrustAndOmitUnknownFacts(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	full := endpoint.Record{SessionName: "parser", ProjectSlug: "wb", Cwd: "/w", State: "idle", StateSince: now.Add(-90 * time.Second), Model: "p/m"}
	if got := agentColumns(full, "trusted", now); got != "parser  wb  /w  idle 1m30s  p/m  trusted" {
		t.Fatalf("columns = %q", got)
	}
	if got := agentColumns(endpoint.Record{Cwd: "/w", State: "busy", StateSince: now.Add(-time.Minute)}, "", now); got != "/w  busy" {
		t.Fatalf("columns = %q", got)
	}
}
