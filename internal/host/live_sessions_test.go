//go:build integration

package host

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
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
