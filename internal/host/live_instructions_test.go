//go:build integration

package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolResults returns every tool message of one recorded provider request, in order.
func toolResults(t *testing.T, body string) []string {
	t.Helper()
	var request struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatal(err)
	}
	var results []string
	for _, message := range request.Messages {
		if message["role"] == "tool" {
			results = append(results, toolMessageText(message))
		}
	}
	return results
}

func TestLiveChangedInstructionsArriveAsADiffAgainstTheDeliveredView(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	var steering strings.Builder
	for i := 1; i <= 40; i++ {
		steering.WriteString("Steering rule " + strings.Repeat("r", i%9) + string(rune('A'+i%26)) + " must hold.\n")
	}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte(steering.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a.txt"}},
		{"bash", map[string]any{"command": "sed -i 's/^Steering rule rB must hold./Rule CHANGED-BY-TEST./' AGENTS.md"}},
		{"read", map[string]any{"path": "a.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 4 {
		t.Fatalf("got %d requests, want 4", len(recorder.bodies))
	}
	if !strings.Contains(recorder.bodies[0], "Steering rule") {
		t.Fatal("the first request must carry the full steering view")
	}
	if after, _ := os.ReadFile(agents); !strings.Contains(string(after), "CHANGED-BY-TEST") {
		t.Fatal("the scripted edit did not change AGENTS.md")
	}
	last := recorder.bodies[3]
	if !strings.Contains(last, "Instruction update for") || !strings.Contains(last, `+Rule CHANGED-BY-TEST.`) {
		t.Fatalf("the change must arrive as an instruction update with a diff hunk")
	}
	// Request 0 carries the full 40-rule view once; the update adds only its hunk, never the whole view again.
	if got := strings.Count(last, "Steering rule "); got > 40+10 {
		t.Fatalf("the update repeated the view (%d rule mentions)", got)
	}
}
