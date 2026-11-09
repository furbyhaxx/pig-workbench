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
	"regexp"
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
	if got := strings.Count(requests[0], "training_data_notice"); got != 1 {
		t.Errorf("the model frame carries the stale-training-data guidance %d times, want once", got)
	}
	if got := strings.Count(requests[1], "training_data_notice"); got != 1 {
		t.Errorf("the guidance appears %d times after resume, want once", got)
	}
	nonces := make(map[string]bool)
	for _, request := range requests {
		for _, match := range regexp.MustCompile(`n=\\"([0-9A-F]{8})\\"`).FindAllStringSubmatch(request, -1) {
			nonces[match[1]] = true
		}
	}
	if len(nonces) != 1 {
		t.Errorf("frames carry %d distinct nonces across a resume, want one per session: %v", len(nonces), nonces)
	}
}

// TestLiveCompactionResendsContextFactsOnce asserts that after compaction
// summarizes the earlier deliveries, each unchanged context fact reaches the
// model again exactly once.
func TestLiveCompactionResendsContextFactsOnce(t *testing.T) {
	root := t.TempDir()
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
		requests = append(requests, string(messages))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		send(map[string]any{"role": "assistant", "content": "fixture finished"}, nil)
		send(map[string]any{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	for i, prompt := range []string{"first", "second"} {
		f.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	}
	f.sendRaw("compact", "compact", nil)
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "compact" })
	mu.Lock()
	before := len(requests)
	mu.Unlock()
	f.send("p2", "prompt", "third")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	mu.Lock()
	defer mu.Unlock()
	if len(requests) <= before {
		t.Fatal("no provider request after compaction")
	}
	last := requests[len(requests)-1]
	for _, source := range []string{"session.date", "session.environment", "session.model", "session.capabilities"} {
		if got := strings.Count(last, source); got != 1 {
			t.Errorf("%s appears %d times after compaction, want 1", source, got)
		}
	}
}

// TestLiveCompactionReplaysLoadedSkillsAsPointers asserts that a skill read before compaction comes back
// as a pointer, never as its body, and that a re-read answers with a pointer instead of a fresh instruction.
func TestLiveCompactionReplaysLoadedSkillsAsPointers(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: alpha\ndescription: d\n---\nSKILL-BODY-MARKER\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		messages, _ := json.Marshal(request["messages"])
		mu.Lock()
		requests = append(requests, string(messages))
		calls++
		first := calls == 1
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if first {
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "load-1", "type": "function", "function": map[string]any{"name": "read", "arguments": `{"path":"skill://alpha"}`}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "fixture finished"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	f.send("p0", "prompt", "load the skill")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	f.send("p1", "prompt", "second")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	f.sendRaw("compact", "compact", nil)
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "compact" })
	mu.Lock()
	before := len(requests)
	mu.Unlock()
	f.send("p2", "prompt", "third")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	mu.Lock()
	defer mu.Unlock()
	if len(requests) <= before {
		t.Fatal("no provider request after compaction")
	}
	last := requests[len(requests)-1]
	if !strings.Contains(last, "skills loaded earlier") || !strings.Contains(last, "skill://alpha") {
		t.Fatalf("loaded skill pointer missing after compaction: %.600s", last)
	}
	if strings.Contains(last, "SKILL-BODY-MARKER") {
		t.Fatal("skill body was presented again after compaction")
	}
}

// TestLiveContextFramesAreSeparateSessionEntries asserts every context frame
// of the first prompt is its own session entry rather than one entry holding
// several frames.
func TestLiveContextFramesAreSeparateSessionEntries(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	agent := filepath.Join(root, "agent")
	for _, dir := range []string{work, agent} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
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
	cmd := exec.CommandContext(ctx, "pig", "--offline", "--session-dir", filepath.Join(root, "sessions"), "-e", repo, "--mode", "json", "--model", "workbench-fixture/mock", "--tools", "read", "first")
	cmd.Dir = work
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PIG_") && !strings.HasPrefix(value, "PI_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("live pig: %v\n%s", err, output)
	}
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("got %d session files, want 1", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	opening := regexp.MustCompile(`<system-(?:info|instruction|notice) src=`)
	var entries []string
	for _, line := range strings.Split(string(data), "\n") {
		var entry struct {
			CustomType string `json:"customType"`
			Content    any    `json:"content"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil || entry.CustomType != "pig-workbench.context" {
			continue
		}
		encoded, _ := json.Marshal(entry.Content)
		entries = append(entries, string(encoded))
	}
	if len(entries) < 5 {
		t.Fatalf("got %d context entries, want one per context frame (at least 5)", len(entries))
	}
	for index, entry := range entries {
		if count := len(opening.FindAllString(strings.ReplaceAll(entry, `\u003c`, "<"), -1)); count != 1 {
			t.Errorf("context entry %d holds %d frames, want 1: %.200s", index, count, entry)
		}
	}
}
