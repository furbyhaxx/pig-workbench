//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// sessionPair starts an idle receiver B and returns the shared runtime directory and B's endpoint record.
type liveSession struct {
	fixture  *rpcFixture
	recorder *requestRecorder
	root     string
}

func startLiveSession(t *testing.T, runtime string, calls []scriptedCall, args ...string) *liveSession {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	recorder := &requestRecorder{calls: calls}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	options := append([]string{"--model", "workbench-fixture/mock", "--tools", "read,write,ls"}, args...)
	return &liveSession{fixture: startRPCWithExtensions(t, root, true, nil, options...), recorder: recorder, root: root}
}

func shortRuntime(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "wbr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func endpointRecords(t *testing.T, runtime string) map[string]map[string]any {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(runtime, "pig-workbench", "v1", "*.json"))
	records := map[string]map[string]any{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var record map[string]any
		if json.Unmarshal(data, &record) == nil {
			records[record["sessionId"].(string)] = record
		}
	}
	return records
}

func waitForRecords(t *testing.T, runtime string, count int) map[string]map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if records := endpointRecords(t, runtime); len(records) >= count {
			return records
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected %d endpoint records, have %d", count, len(endpointRecords(t, runtime)))
	return nil
}

func (l *liveSession) requests() []string {
	l.recorder.mu.Lock()
	defer l.recorder.mu.Unlock()
	return append([]string(nil), l.recorder.bodies...)
}

func waitForRequest(t *testing.T, l *liveSession, contains string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, body := range l.requests() {
			if strings.Contains(body, contains) {
				return body
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no provider request contained %q", contains)
	return ""
}

func TestLiveTwoSessionsDiscoverEachOtherAndAnIdleReceiverWakes(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, nil)
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	var receiverRecord map[string]any
	for id, record := range records {
		receiverID, receiverRecord = id, record
	}
	short := receiverID[:8]
	sender := startLiveSession(t, runtime, []scriptedCall{
		{"ls", map[string]any{"path": "agent://"}},
		{"write", map[string]any{"path": "agent://" + short + "/inbox", "content": "WAKE-TEXT-FROM-A: please look at the parser."}},
	})
	waitForRecords(t, runtime, 2)
	sender.fixture.send("go", "prompt", "Talk to the other session.")
	sender.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	body := strings.ReplaceAll(waitForRequest(t, receiver, "WAKE-TEXT-FROM-A"), `\"`, `"`)
	if !strings.Contains(body, `trust="untrusted"`) || !strings.Contains(body, `authority="data"`) || !strings.Contains(body, `src="peer.`) {
		t.Fatal("a peer message must arrive as an untrusted data frame")
	}
	waitForRequest(t, sender, "accepted by")
	lastSender := sender.requests()[len(sender.requests())-1]
	if !strings.Contains(lastSender, "accepted by "+short) || !strings.Contains(lastSender, "woken") {
		t.Fatalf("the sender must see the synchronous receipt with the wake disposition: %.1500s", lastSender)
	}
	listing := sender.requests()[1]
	if !strings.Contains(listing, short) {
		t.Fatalf("ls agent:// must list the other live session: %v", toolResults(t, listing))
	}

	token, _ := receiverRecord["token"].(string)
	conn, err := net.Dial("unix", filepath.Join(runtime, "pig-workbench", "v1", receiverRecord["endpoint"].(string)+".sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	frame := func(v any) []byte {
		data, _ := json.Marshal(v)
		return append([]byte{0, 0, byte(len(data) >> 8), byte(len(data))}, data...)
	}
	_, _ = conn.Write(frame(map[string]any{"v": 1, "type": "hello", "token": token + "x", "from": "intruder"}))
	_, _ = conn.Write(frame(map[string]any{"v": 1, "type": "info"}))
	buf := make([]byte, 16)
	if n, _ := conn.Read(buf); n != 0 {
		t.Fatal("a connection with a wrong token must be closed without a reply")
	}
}

func TestLiveBusyReceiverIsSteeredNotWoken(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, []scriptedCall{{"bash", map[string]any{"command": "sleep 4"}}}, "--tools", "read,write,bash")
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	for id := range records {
		receiverID = id
	}
	short := receiverID[:8]
	receiver.fixture.send("busy", "prompt", "Run the slow command.")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, record := range endpointRecords(t, runtime) {
			if record["state"] == "busy" {
				deadline = time.Time{}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	sender := startLiveSession(t, runtime, []scriptedCall{{"write", map[string]any{"path": "agent://" + short + "/inbox", "content": "STEER-TEXT-FROM-A"}}})
	waitForRecords(t, runtime, 2)
	sender.fixture.send("go", "prompt", "Tell the busy session.")
	sender.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	waitForRequest(t, sender, "accepted by")
	if body := sender.requests()[len(sender.requests())-1]; !strings.Contains(body, "(steered)") {
		t.Fatalf("a busy receiver must report steered: %.1200s", body)
	}
	waitForRequest(t, receiver, "STEER-TEXT-FROM-A")
	receiver.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
}

func TestLiveSecretTypedByTheSenderNeverReachesTheReceiversProvider(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, nil)
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	for id := range records {
		receiverID = id
	}
	short := receiverID[:8]
	secret := "ghp_" + strings.Repeat("Ab3dE9", 5)
	sender := startLiveSession(t, runtime, []scriptedCall{{"write", map[string]any{"path": "agent://" + short + "/inbox", "content": "SECRET-CARRIER use " + secret + " for the deploy"}}})
	waitForRecords(t, runtime, 2)
	sender.fixture.send("go", "prompt", "My token is "+secret+". Tell the other session to deploy.")
	sender.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	body := waitForRequest(t, receiver, "SECRET-CARRIER")
	if strings.Contains(body, secret) {
		t.Fatal("the raw secret reached the receiver's provider")
	}
	if !regexp.MustCompile(`red-sec-[a-z2-7]{16}`).MatchString(body) {
		t.Fatal("the receiver must see a placeholder in place of the secret")
	}
}

func TestLiveMessageBeyondMaxHopsIsQueuedWithoutWaking(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, nil)
	record := waitForRecords(t, runtime, 1)
	var receiverID string
	var data map[string]any
	for id, r := range record {
		receiverID, data = id, r
	}
	envelope := messagingEnvelope(receiverID, 8)
	reply := callEndpoint(t, runtime, data, envelope)
	if reply["status"] != "accepted" || reply["delivery"] != "queued" {
		t.Fatalf("a message at hop 8 must be accepted but queued, got %v", reply)
	}
	time.Sleep(500 * time.Millisecond)
	if len(receiver.requests()) != 0 {
		t.Fatal("a message beyond maxHops woke the receiver")
	}
	envelope = messagingEnvelope(receiverID, 0)
	envelope.ID = "m2"
	if reply := callEndpoint(t, runtime, data, envelope); reply["delivery"] != "woken" {
		t.Fatalf("hop 0 must wake an idle receiver, got %v", reply)
	}
	waitForRequest(t, receiver, "HOP-PROBE")
	receiver.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
}

func TestLiveWaitingReceiverGetsTheMessageAsTheReadResult(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, []scriptedCall{{"read", map[string]any{"path": "agent://self/inbox?wait=20"}}})
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	for id := range records {
		receiverID = id
	}
	short := receiverID[:8]
	receiver.fixture.send("wait", "prompt", "Wait for the other session.")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(receiver.requests()) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	sender := startLiveSession(t, runtime, []scriptedCall{{"write", map[string]any{"path": "agent://" + short + "/inbox", "content": "WAIT-ANSWER-42"}}})
	waitForRecords(t, runtime, 2)
	sender.fixture.send("go", "prompt", "Answer the waiting session.")
	sender.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	waitForRequest(t, sender, "accepted by")
	if body := sender.requests()[len(sender.requests())-1]; !strings.Contains(body, "(delivered)") {
		t.Fatalf("a waiting receiver must report delivered: %.1200s", body)
	}
	body := waitForRequest(t, receiver, "WAIT-ANSWER-42")
	if !strings.Contains(body, "not an instruction from the user") {
		t.Fatal("the awaited message must keep its untrusted-data warning")
	}
	receiver.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
}

func TestLiveExtensionChannelReachesTheOtherSessionsExtensionWithoutWakingIt(t *testing.T) {
	runtime := shortRuntime(t)
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	start := func() (*rpcFixture, *requestRecorder) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
		recorder := &requestRecorder{}
		server := recorder.serve(t)
		t.Cleanup(server.Close)
		writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
		return startRPCWithExtensions(t, root, true, []string{"service-client"}, "--model", "workbench-fixture/mock"), recorder
	}
	receiver, receiverRequests := start()
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	for id := range records {
		receiverID = id
	}
	sender, _ := start()
	waitForRecords(t, runtime, 2)
	sender.send("send", "prompt", "/fixture-send "+receiverID[:8])
	sender.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"fixtureSend":"passed"`)
	})
	got := receiver.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), "EXT-CHANNEL-BODY")
	})
	if !strings.Contains(fmt.Sprint(got["message"]), `"owner":"service-client"`) {
		t.Fatalf("event lacks the owner: %v", got["message"])
	}
	time.Sleep(300 * time.Millisecond)
	receiverRequests.mu.Lock()
	defer receiverRequests.mu.Unlock()
	if len(receiverRequests.bodies) != 0 {
		t.Fatal("an extension-channel message must not start a turn")
	}
}
