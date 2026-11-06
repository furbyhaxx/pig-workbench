//go:build integration

package host

import (
	"bufio"
	"encoding/json"
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
	return startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write,ls"), recorder
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
