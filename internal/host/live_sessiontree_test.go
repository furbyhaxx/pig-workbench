//go:build integration

package host

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func startInRoot(t *testing.T, root string, calls []scriptedCall) (*rpcFixture, *requestRecorder) {
	t.Helper()
	recorder := &requestRecorder{calls: calls}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	return startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write,ls,bash"), recorder
}

func storedSessionID(t *testing.T, root string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*.jsonl"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	}
	if len(files) != 1 {
		t.Fatalf("want one stored session, have %v", files)
	}
	file, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var header struct {
		ID string `json:"id"`
	}
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		_ = json.Unmarshal(scanner.Bytes(), &header)
	}
	if header.ID == "" {
		t.Fatal("stored session has no id")
	}
	return header.ID
}

func TestLiveStoredSessionTitleRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	first, _ := startInRoot(t, root, []scriptedCall{{"write", map[string]any{"path": "session://self/title", "content": "Original title"}}})
	first.send("initial", "prompt", "Set the original title.")
	first.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	first.close()
	id := storedSessionID(t, root)
	uri := "session://" + id + "/title"
	second, recorder := startInRoot(t, root, []scriptedCall{
		{"write", map[string]any{"path": uri, "content": "Declined title"}},
		{"read", map[string]any{"path": uri}},
		{"write", map[string]any{"path": uri, "content": "Approved title"}},
		{"read", map[string]any{"path": uri}},
		{"bash", map[string]any{"command": "DeclinedRedirect > " + uri}},
		{"read", map[string]any{"path": uri}},
		{"bash", map[string]any{"command": "ApprovedRedirect > " + uri}},
		{"read", map[string]any{"path": uri}},
	})
	second.send("rename", "prompt", "Change the stored session title.")
	for _, decision := range []bool{false, true, false, true} {
		event := second.wait(func(event map[string]any) bool {
			return event["type"] == "extension_ui_request" && event["method"] == "confirm"
		})
		if !strings.Contains(fmt.Sprint(event["title"]), "Change session title") {
			t.Fatalf("unexpected confirmation: %+v", event)
		}
		response, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": event["id"], "confirmed": decision})
		if _, err := fmt.Fprintf(second.stdin, "%s\n", response); err != nil {
			t.Fatal(err)
		}
	}
	second.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 8 || !strings.Contains(results[0], "declined") || !strings.Contains(results[1], "Original title") || !strings.Contains(results[2], "title set") || !strings.Contains(results[3], "Approved title") || !strings.Contains(results[4], "declined") || !strings.Contains(results[5], "Approved title") || !strings.Contains(results[6], "title set") || !strings.Contains(results[7], "ApprovedRedirect") {
		t.Fatalf("confirmation did not guard title changes: %v", results)
	}
}

func TestLiveStoredSessionsAreListedReadAndTitled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	first, _ := startInRoot(t, root, []scriptedCall{{"write", map[string]any{"path": "session://self/title", "content": "Alpha topic\nsecond line"}}})
	first.send("a", "prompt", "ALPHA-PROMPT remember this")
	first.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	first.close()
	id := storedSessionID(t, root)

	second, recorder := startInRoot(t, root, []scriptedCall{
		{"ls", map[string]any{"path": "session://"}},
		{"read", map[string]any{"path": "session://" + shortID(id) + "/title"}},
		{"read", map[string]any{"path": "session://by-id/" + id + "/transcript"}},
		{"ls", map[string]any{"path": "session://by-id/"}},
	})
	second.send("b", "prompt", "inspect the stored one")
	second.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := strings.ReplaceAll(recorder.bodies[len(recorder.bodies)-1], `\"`, `"`)
	for _, want := range []string{id, "Alpha topic second line", "ALPHA-PROMPT remember this", "session://" + id + "/"} {
		if !strings.Contains(last, want) {
			t.Fatalf("the final request lacks %q: %v", want, toolResults(t, recorder.bodies[len(recorder.bodies)-1]))
		}
	}
}

func TestLiveSessionSelfFramesAreListedAndRead(t *testing.T) {
	root := t.TempDir()
	fixture, recorder := startInRoot(t, root, []scriptedCall{
		{"ls", map[string]any{"path": "session://self/"}},
		{"ls", map[string]any{"path": "session://self/frames/"}},
		{"read", map[string]any{"path": "session://self/frames"}},
	})
	fixture.send("p", "prompt", "inspect the frames")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 3 || !strings.Contains(results[0], "frames") {
		t.Fatalf("self listing lacks frames: %v", results)
	}
	for _, result := range results[1:] {
		if !strings.Contains(result, "session.date") || !strings.Contains(result, "delivered") {
			t.Errorf("frame receipts lack the delivered date frame:\n%s", result)
		}
	}
}

func TestLiveSessionListingsPageFiftyStoredSessions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	first, _ := startInRoot(t, root, nil)
	first.send("a", "prompt", "remember this")
	first.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	first.close()
	id := storedSessionID(t, root)
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*.jsonl"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{id}
	for i := 1; i < 50; i++ {
		clone := fmt.Sprintf("%s%04x", id[:len(id)-4], i)
		if clone == id {
			continue
		}
		ids = append(ids, clone)
		name := strings.ReplaceAll(filepath.Base(files[0]), id, clone)
		if name == filepath.Base(files[0]) {
			name = clone + ".jsonl"
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(files[0]), name), []byte(strings.ReplaceAll(string(content), id, clone)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	second, recorder := startInRoot(t, root, []scriptedCall{
		{"ls", map[string]any{"path": "session://"}},
		{"ls", map[string]any{"path": "session://by-id/"}},
	})
	second.send("b", "prompt", "list them")
	second.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 2 {
		t.Fatalf("want two listings: %v", results)
	}
	for index, result := range results {
		for _, want := range ids {
			if !strings.Contains(result, want) {
				t.Fatalf("listing %d lacks session %s: %.600s", index, want, result)
			}
		}
	}
}
