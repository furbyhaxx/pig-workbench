//go:build integration

package host

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/messaging"
)

func messagingEnvelope(to string, hop int) messaging.Envelope {
	return messaging.Envelope{ID: "m1", ConversationID: "conversation-1", Hop: hop, ToSessionID: to, From: messaging.Peer{SessionID: "peer-session-0001", SessionName: "probe"}, Body: "HOP-PROBE", ExpiresAt: time.Now().Add(time.Hour)}
}

func callEndpoint(t *testing.T, runtime string, record map[string]any, envelope messaging.Envelope) map[string]any {
	t.Helper()
	path := filepath.Join(runtime, "pig-workbench", "v1", record["endpoint"].(string)+".sock")
	reply, err := messaging.Call(path, record["token"].(string), "probe", messaging.Request{Type: "message", Message: &envelope})
	if err != nil || reply.Receipt == nil {
		t.Fatalf("call failed: %+v %v", reply, err)
	}
	return map[string]any{"status": reply.Receipt.Status, "delivery": reply.Receipt.Delivery, "reason": reply.Receipt.Reason}
}
