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

func TestLiveHTTPReadUsesBuiltinStaticReader(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>Live reader</title></head><body>
<nav>chrome navigation</nav>
<article><h1>Live reader</h1><p>This article is served over HTTP and must reach the model as readable Markdown through the resource router.</p></article>
<footer>page footer</footer></body></html>`)
	}))
	defer page.Close()
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
			{"read", map[string]any{"path": page.URL + "/article"}},
			{"read", map[string]any{"path": page.URL + "/article:raw"}},
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("h%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < len(steps) {
			args, _ := json.Marshal(steps[index].args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("h%d", index), "type": "function", "function": map[string]any{"name": steps[index].name, "arguments": string(args)}}}}, nil)
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
	output := runPigJSON(t, ctx, repo, root, work, []string{"--tools", "read"}, "exercise")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 3 {
		t.Fatalf("expected 3 provider calls, got %d\n%s", len(requests), output)
	}
	readable, _ := json.Marshal(requests[1]["messages"])
	if !strings.Contains(string(readable), "Live reader") || !strings.Contains(string(readable), "readable Markdown") || strings.Contains(string(readable), "chrome navigation") {
		t.Fatalf("readable HTTP result did not reach the model:\n%s", readable)
	}
	raw, _ := json.Marshal(requests[2]["messages"])
	if !strings.Contains(string(raw), "doctype") || !strings.Contains(string(raw), "page footer") {
		t.Fatalf("raw HTTP result did not reach the model:\n%s", raw)
	}
}
