//go:build integration

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLiveWriteRevisionAndLs(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		index := len(requests)
		requests = append(requests, body)
		mu.Unlock()
		steps := []struct {
			name string
			args map[string]any
		}{
			{"write", map[string]any{"path": "created.txt", "content": "hello\nworld\n"}},
			{"read", map[string]any{"path": "created.txt:1-1:raw"}},
			{"write", map[string]any{"path": "created.txt", "content": "stale\n", "expectedRevision": strings.Repeat("ab", 32)}},
			{"write", map[string]any{"path": "created.txt", "content": "stale\n", "expectedRevision": "4a1e"}},
			{"ls", map[string]any{"path": "."}},
			{"write", map[string]any{"path": "created.txt", "content": "fresh\n"}},
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("r%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < len(steps) {
			args, _ := json.Marshal(steps[index].args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("c%d", index), "type": "function", "function": map[string]any{"name": steps[index].name, "arguments": string(args)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		usage, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("r%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", usage)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := runPigJSON(t, ctx, repo, root, work, []string{"--tools", "read,write,ls"}, "exercise")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 7 {
		t.Fatalf("expected 7 provider calls, got %d\n%s", len(requests), output)
	}
	text := func(i int) string {
		data, _ := json.Marshal(requests[i]["messages"])
		return string(data)
	}
	if !strings.Contains(text(2), "hello") {
		t.Fatalf("written content not readable: %s", text(2))
	}
	if !strings.Contains(text(3), "staleRevision") {
		t.Fatalf("wrong revision was not refused: %s", text(3))
	}
	if !strings.Contains(text(4), "64-hex Revision") {
		t.Fatalf("a malformed revision must be refused with the receipt hint: %s", text(4))
	}
	if !strings.Contains(text(5), "created.txt") || !strings.Contains(text(5), "work") {
		t.Fatalf("listing did not use the captured location: %s", text(5))
	}
	data, err := os.ReadFile(filepath.Join(work, "created.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fresh\n" {
		t.Fatalf("final write content wrong: %q", data)
	}
	info, err := os.Stat(filepath.Join(work, "created.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("new file mode %o, want 0600", info.Mode().Perm())
	}
}

func TestLiveSessionResources(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		index := len(requests)
		requests = append(requests, body)
		mu.Unlock()
		steps := []struct {
			name string
			args map[string]any
		}{
			{"read", map[string]any{"path": "session://self/meta"}},
			{"read", map[string]any{"path": "session://self/transcript:1-6"}},
			{"read", map[string]any{"path": "session://self/entries:1-2"}},
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("r%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < len(steps) {
			args, _ := json.Marshal(steps[index].args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("c%d", index), "type": "function", "function": map[string]any{"name": steps[index].name, "arguments": string(args)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := runPigJSON(t, ctx, repo, root, work, []string{"--tools", "read"}, "exercise session resources")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 4 {
		t.Fatalf("expected 4 provider calls, got %d\n%s", len(requests), output)
	}
	text := func(i int) string {
		data, _ := json.Marshal(requests[i]["messages"])
		return string(data)
	}
	if !strings.Contains(text(1), `\"sessionId\"`) || !strings.Contains(text(1), `\"cwd\"`) {
		t.Fatalf("session meta missing: %s", text(1))
	}
	if !strings.Contains(text(3), "## user") {
		t.Fatalf("transcript projection missing: %s", text(3))
	}
}
