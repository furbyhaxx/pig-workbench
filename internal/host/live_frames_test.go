//go:build integration

package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveResumeDoesNotResendDeliveredContextFrames resumes a persisted session
// and asserts the unchanged context facts are not delivered a second time.
func TestLiveResumeDoesNotResendDeliveredContextFrames(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	agent := filepath.Join(root, "agent")
	for _, dir := range []string{work, agent} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		messages, _ := json.Marshal(request["messages"])
		mu.Lock()
		index := len(requests)
		requests = append(requests, string(messages))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("fixture-%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		send(map[string]any{"role": "assistant", "content": "fixture finished"}, nil)
		send(map[string]any{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	run := func(extra string, prompt string) {
		args := []string{"--offline", "--session-dir", filepath.Join(root, "sessions"), "-e", repo, "--mode", "json", "--model", "workbench-fixture/mock", "--tools", "read"}
		if extra != "" {
			args = append(args, extra)
		}
		cmd := exec.CommandContext(ctx, "pig", append(args, prompt)...)
		cmd.Dir = work
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "PIG_") && !strings.HasPrefix(value, "PI_") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if output, err := cmd.Output(); err != nil {
			t.Fatalf("live pig: %v\nstderr: %s\nstdout: %s", err, stderr.String(), output)
		}
	}
	run("", "first")
	run("--continue", "second")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("got %d provider requests, want 2", len(requests))
	}
	for _, source := range []string{"session.date", "session.environment", "session.project", "session.model", "session.capabilities"} {
		if got := strings.Count(requests[1], source); got != 1 {
			t.Errorf("%s appears %d times after resume, want 1", source, got)
		}
	}
}
