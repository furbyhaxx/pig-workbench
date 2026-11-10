//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/config"
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

func TestLiveSessionEndpointSettingsAtStartup(t *testing.T) {
	for _, tc := range []struct {
		name             string
		settings         string
		wantEndpoint     bool
		wantDiscoverable bool
	}{
		{"disabled", "enabled = false", false, false},
		{"hidden", "discoverable = false", true, false},
		{"default", "enabled = true", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := shortRuntime(t)
			t.Setenv("XDG_RUNTIME_DIR", runtime)
			root := t.TempDir()
			agent := filepath.Join(root, "agent")
			if err := os.MkdirAll(agent, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[sessions]\n"+tc.settings+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			fixture := startRPCWithExtensions(t, root, true, nil)
			fixture.command("ready", "/workbench show")
			records := endpointRecords(t, runtime)
			if (len(records) == 1) != tc.wantEndpoint {
				t.Fatalf("got %d endpoints, want endpoint=%v", len(records), tc.wantEndpoint)
			}
			for _, record := range records {
				if record["discoverable"] != tc.wantDiscoverable {
					t.Fatalf("discoverability = %v, want %v", record["discoverable"], tc.wantDiscoverable)
				}
			}
		})
	}
}

func TestLiveTwoSessionsDiscoverEachOtherAndAnIdleReceiverWakes(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, []scriptedCall{{"read", map[string]any{"path": "session://self/messages"}}})
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	var receiverRecord map[string]any
	for id, record := range records {
		receiverID, receiverRecord = id, record
	}
	short := shortID(receiverID)
	sender := startLiveSession(t, runtime, []scriptedCall{
		{"ls", map[string]any{"path": "agent://"}},
		{"write", map[string]any{"path": "agent://" + short + "/inbox", "content": "WAKE-TEXT-FROM-A: please look at the parser."}},
		{"read", map[string]any{"path": "session://self/messages"}},
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
	if !strings.Contains(listing, short) || !strings.Contains(listing, "  idle") {
		t.Fatalf("ls agent:// must list the other live session with its state: %v", toolResults(t, listing))
	}
	if sent := waitForRequest(t, sender, " to "+short); !strings.Contains(sent, "WAKE-TEXT-FROM-A") {
		t.Fatalf("session://self/messages lacks the sent message: %v", toolResults(t, sent))
	}
	if got := waitForRequest(t, receiver, " from "); !strings.Contains(got, "WAKE-TEXT-FROM-A") {
		t.Fatalf("session://self/messages lacks the received message: %v", toolResults(t, got))
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

func TestLiveAgentReplyIncrementsHopAndStopsWake(t *testing.T) {
	runtime := shortRuntime(t)
	initiator := startLiveSession(t, runtime, nil)
	initial := waitForRecords(t, runtime, 1)
	var initiatorID string
	for id := range initial {
		initiatorID = id
	}
	settingsNotice(t, initiator.fixture, "hop-limit", "set runtime sessions.max-hops 1")
	form := map[string]any{"title": "Continue", "questions": []any{map[string]any{
		"id": "proceed", "label": "Proceed", "prompt": "Send the reply?", "type": "single",
		"options": []any{map[string]any{"value": "yes", "label": "Yes"}},
	}}}
	replier := startLiveSession(t, runtime, []scriptedCall{
		{"ask_user", form},
		{"write", map[string]any{"path": "agent://" + shortID(initiatorID) + "/inbox", "content": "REAL-AGENT-REPLY"}},
	}, "--tools", "read,write,ls,ask_user")
	records := waitForRecords(t, runtime, 2)
	var replierID string
	for id := range records {
		if id != initiatorID {
			replierID = id
		}
	}
	if replierID == "" {
		t.Fatal("replying session was not discovered")
	}
	initiator.recorder.calls = []scriptedCall{{"write", map[string]any{"path": "agent://" + shortID(replierID) + "/inbox", "content": "START-CONVERSATION"}}}
	initiator.fixture.send("start", "prompt", "Start the conversation.")
	initiator.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if endpointRecords(t, runtime)[initiatorID]["state"] == "idle" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initiator did not return to idle before the reply")
		}
		time.Sleep(50 * time.Millisecond)
	}
	initialReceipt := waitForRequest(t, initiator, "accepted by")
	if !strings.Contains(initialReceipt, "(woken)") {
		t.Fatalf("initial message did not wake the replier: %v", toolResults(t, initialReceipt))
	}
	beforeReply := len(initiator.requests())
	dialog := replier.fixture.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "select"
	})
	options, ok := dialog["options"].([]any)
	if !ok || len(options) == 0 {
		t.Fatalf("reply dialog had no options: %+v", dialog)
	}
	response, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": dialog["id"], "value": options[0]})
	if _, err := fmt.Fprintf(replier.fixture.stdin, "%s\n", response); err != nil {
		t.Fatal(err)
	}
	replier.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	body := waitForRequest(t, replier, "accepted by")
	if !strings.Contains(body, "(queued)") {
		t.Fatalf("reply at hop limit must be queued without waking: %v", toolResults(t, body))
	}
	conversation := regexp.MustCompile(`conversation ([0-9a-f]{8})`)
	first := conversation.FindStringSubmatch(initialReceipt)
	reply := conversation.FindStringSubmatch(body)
	if len(first) != 2 || len(reply) != 2 || first[1] != reply[1] {
		t.Fatalf("reply lost its conversation: initial=%v reply=%v", first, reply)
	}
	time.Sleep(500 * time.Millisecond)
	if got := len(initiator.requests()); got != beforeReply {
		t.Fatalf("hop-limited reply woke the initiator: before=%d after=%d", beforeReply, got)
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
	short := shortID(receiverID)
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
	short := shortID(receiverID)
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
	settingsNotice(t, receiver.fixture, "hop-limit", "set runtime sessions.max-hops 1")
	envelope = messagingEnvelope(receiverID, 1)
	envelope.ID = "m2"
	if reply := callEndpoint(t, runtime, data, envelope); reply["delivery"] != "queued" {
		t.Fatalf("configured hop limit must prevent automatic wake, got %v", reply)
	}
	envelope = messagingEnvelope(receiverID, 0)
	envelope.ID = "m3"
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
	short := shortID(receiverID)
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
	sender.send("send", "prompt", "/fixture-send "+shortID(receiverID))
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

func notifyContaining(f *rpcFixture, want string) map[string]any {
	return f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), want)
	})
}

func TestLiveSessionsCommandSendsRedactsAndHonoursBlock(t *testing.T) {
	runtime := shortRuntime(t)
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	receiver := startLiveSession(t, runtime, nil)
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	for id := range records {
		receiverID = id
	}
	sender := startLiveSession(t, runtime, nil)
	all := waitForRecords(t, runtime, 2)
	var senderID string
	for id := range all {
		if id != receiverID {
			senderID = id
		}
	}
	secret := "ghp_" + strings.Repeat("Zy8xW7", 5)
	sender.fixture.send("ls", "prompt", "/workbench sessions ls")
	notifyContaining(sender.fixture, shortID(receiverID))
	sender.fixture.send("send", "prompt", "/workbench sessions send "+shortID(receiverID)+" CMD-CARRIER token "+secret)
	notifyContaining(sender.fixture, "accepted by")
	body := waitForRequest(t, receiver, "CMD-CARRIER")
	if strings.Contains(body, secret) || !regexp.MustCompile(`red-sec-[a-z2-7]{16}`).MatchString(body) {
		t.Fatal("a secret typed into /workbench sessions send must reach the receiver only as a placeholder")
	}
	receiver.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })

	receiver.fixture.send("block", "prompt", "/workbench sessions block "+senderID)
	notifyContaining(receiver.fixture, "block:")
	sender.fixture.send("again", "prompt", "/workbench sessions send "+shortID(receiverID)+" AFTER-BLOCK")
	notifyContaining(sender.fixture, "policyDenied")
	for _, request := range receiver.requests() {
		if strings.Contains(request, "AFTER-BLOCK") {
			t.Fatal("a blocked peer's message reached the receiver")
		}
	}
}

// TestLiveEveryCompletedCommandIsAcceptedByItsHandler keeps completion and dispatch from drifting apart.
func TestLiveEveryCompletedCommandIsAcceptedByItsHandler(t *testing.T) {
	items, err := completeWorkbench("", nil, config.Catalog())
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, item := range items {
		value := item.Value
		if strings.HasPrefix(value, "settings set") || strings.HasPrefix(value, "settings reset") {
			continue
		}
		switch value {
		case "sessions send", "sessions trust", "sessions untrust", "sessions block", "sessions unblock", "sessions hide", "sessions show", "trust add allow", "trust add deny", "trust rm", "instructions pin", "instructions unpin", "ask resume", "ask discard", "projects create", "projects join", "projects join file", "projects join marker":
			continue
		}
		commands = append(commands, value)
	}
	if len(commands) < 15 {
		t.Fatalf("too few completions to check: %v", commands)
	}
	runtime := shortRuntime(t)
	l := startLiveSession(t, runtime, nil)
	for i, command := range commands {
		id := fmt.Sprintf("c%d", i)
		l.fixture.send(id, "prompt", "/workbench "+command)
		var messages []string
		l.fixture.wait(func(record map[string]any) bool {
			if record["type"] == "extension_ui_request" && record["method"] == "notify" {
				messages = append(messages, fmt.Sprint(record["message"]))
			}
			return record["type"] == "response" && record["id"] == id
		})
		for _, message := range messages {
			if strings.Contains(message, "usage:") {
				t.Errorf("/workbench %s is completed but its handler refuses it: %s", command, message)
			}
		}
	}
}

func TestLiveActivitySettingsControlWidget(t *testing.T) {
	for _, tc := range []struct {
		name      string
		enabled   bool
		placement string
		want      string
	}{
		{"above", true, "above-editor", "aboveEditor"},
		{"below", true, "below-editor", "belowEditor"},
		{"disabled", false, "above-editor", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := shortRuntime(t)
			t.Setenv("XDG_RUNTIME_DIR", runtime)
			root := t.TempDir()
			recorder := &requestRecorder{calls: []scriptedCall{
				{"bash", map[string]any{"command": "sleep 5", "background": true}},
			}}
			server := recorder.serve(t)
			defer server.Close()
			agent := filepath.Join(root, "agent")
			writeFixtureProvider(t, agent, server.URL)
			settings := fmt.Sprintf("[activity]\nenabled = %t\nplacement = '%s'\n", tc.enabled, tc.placement)
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte(settings), 0600); err != nil {
				t.Fatal(err)
			}
			socket := filepath.Join(runtime, "tmux.sock")
			tmux := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("tmux", append([]string{"-S", socket}, args...)...)
				cmd.Env = append(os.Environ(), "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("tmux %v: %v: %s", args, err, out)
				}
				return string(out)
			}
			tmux("new-session", "-d", "-s", "workbench", "-x", "80", "-y", "24", "-c", root, "pig", "--offline", "--no-session", "-e", repoRoot(t), "--model", "workbench-fixture/mock", "--tools", "bash")
			stopTUI(t, socket, "workbench")
			boundaries := func(capture string) []int {
				var lines []int
				for i, line := range strings.Split(capture, "\n") {
					if strings.HasPrefix(strings.TrimSpace(line), "────────") {
						lines = append(lines, i)
					}
				}
				return lines
			}
			deadline := time.Now().Add(15 * time.Second)
			capture := ""
			for time.Now().Before(deadline) {
				capture = tmux("capture-pane", "-p", "-t", "workbench")
				if len(boundaries(capture)) >= 2 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if len(boundaries(capture)) < 2 {
				t.Fatalf("editor did not start:\n%s", capture)
			}
			tmux("send-keys", "-t", "workbench", "Start a background job.", "Enter")
			deadline = time.Now().Add(15 * time.Second)
			finished := false
			for time.Now().Before(deadline) {
				capture = tmux("capture-pane", "-p", "-t", "workbench")
				recorder.mu.Lock()
				finished = len(recorder.bodies) >= 2
				recorder.mu.Unlock()
				if finished && (tc.want == "" || strings.Contains(capture, "1 jobs")) {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if !finished {
				t.Fatalf("script did not finish:\n%s", capture)
			}
			if tc.want == "" {
				time.Sleep(2 * activityPollInterval)
				capture = tmux("capture-pane", "-p", "-t", "workbench")
			}
			line := -1
			for i, text := range strings.Split(capture, "\n") {
				if strings.Contains(text, "1 jobs") {
					line = i
				}
			}
			if tc.want == "" && line >= 0 {
				t.Fatalf("disabled activity produced a line:\n%s", capture)
			}
			if tc.want != "" {
				borders := boundaries(capture)
				if line < 0 || len(borders) < 2 || tc.want == "aboveEditor" && line >= borders[len(borders)-2] || tc.want == "belowEditor" && line <= borders[len(borders)-1] {
					t.Fatalf("wrong %s activity placement:\n%s", tc.want, capture)
				}
			}
		})
	}
}

func TestLiveActivityLineCountsMatchTheRegistries(t *testing.T) {
	runtime := shortRuntime(t)
	l := startLiveSession(t, runtime, []scriptedCall{
		{"bash", map[string]any{"command": "sleep 30", "background": true}},
		{"bash", map[string]any{"command": "exit 3"}},
		{"read", map[string]any{"path": "cfg://workbench/status"}},
	}, "--tools", "read,write,bash")
	var widgets []string
	l.fixture.send("work", "prompt", "start a job")
	l.fixture.wait(func(record map[string]any) bool {
		if record["type"] == "extension_ui_request" && record["method"] == "setWidget" {
			widgets = append(widgets, fmt.Sprint(record["widgetLines"]))
		}
		return record["type"] == "agent_end"
	})
	l.fixture.send("show", "prompt", "/workbench show")
	shown := fmt.Sprint(notifyContaining(l.fixture, "Activity:")["message"])
	if !strings.Contains(shown, "1 jobs") {
		t.Fatalf("/workbench show must carry the running job count: %s", shown)
	}
	status := ""
	for _, request := range l.requests() {
		if strings.Contains(request, `\"activity\"`) {
			status = request
		}
	}
	if !strings.Contains(status, `\"key\": \"jobs\"`) || !strings.Contains(status, `\"count\": 1`) {
		t.Fatalf("cfg://workbench/status must carry the same activity items: %.1500s", status)
	}
	if len(widgets) == 0 {
		t.Log("no setWidget record was observed in RPC mode; the line is verified through the shared items")
	}
}

func TestLiveACrashedSessionsRecordIsPrunedByTheNextStart(t *testing.T) {
	runtime := shortRuntime(t)
	crashed := startLiveSession(t, runtime, nil)
	records := waitForRecords(t, runtime, 1)
	var deadID string
	for id := range records {
		deadID = id
	}
	if err := crashed.fixture.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-crashed.fixture.done
	crashed.fixture.closeOnce.Do(crashed.fixture.cancel)
	dir := filepath.Join(runtime, "pig-workbench", "v1")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, deadID+".json"), old, old); err != nil {
		t.Fatal(err)
	}
	rewriteHeartbeat(t, filepath.Join(dir, deadID+".json"), old)
	startLiveSession(t, runtime, nil)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := endpointRecords(t, runtime)[deadID]; !ok {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the dead session's record must be removed once its PID is gone and its socket is silent")
}

func rewriteHeartbeat(t *testing.T, path string, at time.Time) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record["heartbeatAt"] = at.UTC().Format(time.RFC3339Nano)
	out, _ := json.Marshal(record)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLiveStaleAddressesAreRefusedWithTheCurrentSession covers SPEC-0011 §10:
// a message for a replaced session answers sessionChanged and an expired one
// answers expired; neither reaches the model.
func TestLiveStaleAddressesAreRefusedWithTheCurrentSession(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, nil)
	records := waitForRecords(t, runtime, 1)
	var receiverID string
	var data map[string]any
	for id, r := range records {
		receiverID, data = id, r
	}
	stale := messagingEnvelope("another-session-id-0001", 0)
	if reply := callEndpoint(t, runtime, data, stale); reply["status"] != "rejected" || reply["reason"] != "sessionChanged" {
		t.Fatalf("a message for another session must answer sessionChanged, got %v", reply)
	}
	expired := messagingEnvelope(receiverID, 0)
	expired.ID = "m-expired"
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	if reply := callEndpoint(t, runtime, data, expired); reply["status"] != "rejected" || reply["reason"] != "expired" {
		t.Fatalf("an expired message must answer expired, got %v", reply)
	}
	time.Sleep(300 * time.Millisecond)
	if len(receiver.requests()) != 0 {
		t.Fatal("a refused message woke the receiver")
	}
}

// TestLiveARealSessionReplacementRefusesTheOldAddress drives /new on a running session: the old
// session ID is refused with the current one, the new ID is reachable and wakes the model.
func TestLiveARealSessionReplacementRefusesTheOldAddress(t *testing.T) {
	runtime := shortRuntime(t)
	receiver := startLiveSession(t, runtime, nil)
	before := waitForRecords(t, runtime, 1)
	var oldID string
	for id := range before {
		oldID = id
	}
	receiver.fixture.send("replace", "new_session", "")
	receiver.fixture.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "replace" })
	var current string
	var record map[string]any
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && current == "" {
		for id, r := range endpointRecords(t, runtime) {
			if id != oldID {
				current, record = id, r
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if current == "" {
		t.Fatalf("no endpoint record for the replacement session; records: %v", endpointRecords(t, runtime))
	}
	if reply := callEndpoint(t, runtime, record, messagingEnvelope(oldID, 0)); reply["status"] != "rejected" || reply["reason"] != "sessionChanged" {
		t.Fatalf("a message for the replaced session must answer sessionChanged, got %v", reply)
	}
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(strings.Join(receiver.requests(), "\n"), "HOP-PROBE") {
		t.Fatal("a message for the replaced session reached the model")
	}
	if reply := callEndpoint(t, runtime, record, messagingEnvelope(current, 0)); reply["status"] != "accepted" {
		t.Fatalf("the replacement session must accept messages addressed to it, got %v", reply)
	}
	waitForRequest(t, receiver, "HOP-PROBE")
}

func TestLiveActivityShortcutSettingBindsTheConfiguredKey(t *testing.T) {
	for _, tc := range []struct {
		name, setting, press, idle string
	}{
		{"configured", "alt+j", "M-j", "M-w"},
		{"invalid-keeps-default", "Alt W", "M-w", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := shortRuntime(t)
			t.Setenv("XDG_RUNTIME_DIR", runtime)
			root := t.TempDir()
			recorder := &requestRecorder{}
			server := recorder.serve(t)
			defer server.Close()
			agent := filepath.Join(root, "agent")
			writeFixtureProvider(t, agent, server.URL)
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[activity]\nshortcut = '"+tc.setting+"'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			socket := filepath.Join(runtime, "tmux.sock")
			tmux := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("tmux", append([]string{"-S", socket}, args...)...)
				cmd.Env = append(os.Environ(), "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("tmux %v: %v: %s", args, err, out)
				}
				return string(out)
			}
			tmux("new-session", "-d", "-s", "workbench", "-x", "100", "-y", "24", "-c", root, "pig", "--offline", "--no-session", "-e", repoRoot(t), "--model", "workbench-fixture/mock", "--tools", "read")
			stopTUI(t, socket, "workbench")
			waitFor := func(text string, within time.Duration) (string, bool) {
				capture := ""
				for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
					capture = tmux("capture-pane", "-p", "-t", "workbench")
					if strings.Contains(capture, text) {
						return capture, true
					}
				}
				return capture, false
			}
			if capture, ok := waitFor("────────", 15*time.Second); !ok {
				t.Fatalf("editor did not start:\n%s", capture)
			}
			if tc.idle != "" {
				tmux("send-keys", "-t", "workbench", tc.idle)
				if capture, ok := waitFor("Workbench activity", 2*time.Second); ok {
					t.Fatalf("the default key still opened the inspector after it was rebound:\n%s", capture)
				}
			}
			// The editor can draw before the extension's shortcut table is live, so a key sent in that gap is lost.
			opened, capture := false, ""
			for attempt := 0; attempt < 6 && !opened; attempt++ {
				tmux("send-keys", "-t", "workbench", tc.press)
				capture, opened = waitFor("Workbench activity: idle", 4*time.Second)
			}
			if !opened {
				t.Fatalf("%s did not open the inspector:\n%s", tc.press, capture)
			}
		})
	}
}

func TestLiveAgentListingPagesFiftyPeers(t *testing.T) {
	runtime := shortRuntime(t)
	startLiveSession(t, runtime, nil)
	var receiverID string
	for id := range waitForRecords(t, runtime, 1) {
		receiverID = id
	}
	dir := filepath.Join(runtime, "pig-workbench", "v1")
	template, err := os.ReadFile(filepath.Join(dir, receiverID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{shortID(receiverID)}
	for i := 1; i < 50; i++ {
		clone := fmt.Sprintf("%s%04x", receiverID[:len(receiverID)-4], i)
		if clone == receiverID {
			continue
		}
		want = append(want, shortID(clone))
		if err := os.WriteFile(filepath.Join(dir, clone+".json"), []byte(strings.ReplaceAll(string(template), receiverID, clone)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sender := startLiveSession(t, runtime, []scriptedCall{{"ls", map[string]any{"path": "agent://"}}})
	sender.fixture.send("list", "prompt", "List the peers.")
	sender.fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	requests := sender.requests()
	results := toolResults(t, requests[len(requests)-1])
	if len(results) != 1 {
		t.Fatalf("want one listing: %v", results)
	}
	for _, name := range want {
		if !strings.Contains(results[0], name) {
			t.Fatalf("listing lacks peer %s: %.800s", name, results[0])
		}
	}
}
