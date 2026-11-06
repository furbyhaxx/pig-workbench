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
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestLiveReferenceProducerReload(t *testing.T) {
	repo := repoRoot(t)
	t.Setenv("WORKBENCH_REFERENCE_DOCS", filepath.Join(repo, "internal/host/testdata/reference-producer/docs"))
	f := startRPC(t, t.TempDir(), "-e", filepath.Join(repo, "internal/host/testdata/reference-producer"))
	status := func(id string) map[string]any {
		f.send(id, "prompt", "/reference status")
		record := f.wait(func(record map[string]any) bool {
			return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"reference":"registered"`)
		})
		var result map[string]any
		if err := json.Unmarshal([]byte(record["message"].(string)), &result); err != nil {
			t.Fatal(err)
		}
		if result["lockHeld"] != false {
			t.Fatal("producer state lock held during a synchronous service request")
		}
		if result["kinds"] != float64(6) {
			t.Fatalf("unexpected registration count: %+v", result)
		}
		return result
	}
	first := status("before")
	for i := range 2 {
		f.command(fmt.Sprintf("reload-%d", i), "/reference reload")
		second := status(fmt.Sprintf("after-%d", i))
		if consumer := f.probe(fmt.Sprintf("consumer-%d", i)); consumer["generation"] != second["generation"] {
			t.Fatal("independent consumer discovered a different generation after reload")
		}
		if first["generation"] == second["generation"] {
			t.Fatal("reload reused a service generation")
		}
		oldReceipts := first["receipts"].(map[string]any)
		for kind, value := range second["receipts"].(map[string]any) {
			if value.(map[string]any)["registrationId"] == oldReceipts[kind].(map[string]any)["registrationId"] {
				t.Errorf("reload retained %s registration handle", kind)
			}
		}
		first = second
	}
}

// TestLiveReferenceProducerSeams co-loads a pig-web-shaped extension and drives every
// contribution seam through the real agent loop with a scripted provider.
func TestLiveReferenceProducerSeams(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKBENCH_REFERENCE_DOCS", filepath.Join(repo, "internal/host/testdata/reference-producer/docs"))
	steps := []struct {
		name string
		args map[string]any
	}{
		{"read", map[string]any{"path": "kb://sources/a.md:2-3"}},
		{"ls", map[string]any{"path": "kb://sources/"}},
		{"edit", map[string]any{"hashline": "[kb://sources/a.md#AAAA]\nPUT 2.=2:\n+changed"}},
		{"read", map[string]any{"path": "https://fixture.test/page"}},
		{"read", map[string]any{"path": "https://fixture.test/located"}},
		{"read", map[string]any{"path": "https://fixture.test/missing"}},
		{"read", map[string]any{"path": "docs://reference-producer/README.md"}},
		{"read", map[string]any{"path": "docs://workbench/schemes/kb"}},
		{"web_search", map[string]any{"query": "fixture"}},
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
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("r%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < len(steps) {
			args, _ := json.Marshal(steps[index].args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("r%d", index), "type": "function", "function": map[string]any{"name": steps[index].name, "arguments": string(args)}}}}, nil)
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
	output := runPigJSON(t, ctx, repo, root, work, []string{"-e", filepath.Join(repo, "internal/host/testdata/reference-producer")}, "exercise the seams")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != len(steps)+1 {
		t.Fatalf("expected %d provider calls, got %d\n%s", len(steps)+1, len(requests), output[max(0, len(output)-3000):])
	}
	toolResult := func(index int) string {
		messages, _ := requests[index]["messages"].([]any)
		last, _ := json.Marshal(messages[len(messages)-1])
		return string(last)
	}
	first, _ := json.Marshal(requests[0]["messages"])
	for _, want := range []string{"REFERENCE-NOTICE", "2025-06-01", "reference fixture"} {
		if !strings.Contains(string(first), want) {
			t.Errorf("first request lacks %q (frame notice or contributed model field):\n%s", want, first)
		}
	}
	claim := regexp.MustCompile(`\\u003csystem-info src=\\"reference\.research\\"[^>]*trust=\\"([a-z]+)\\" authority=\\"([a-z]+)\\"\\u003e\\n(REFERENCE-TRUSTED-CLAIM)`).FindStringSubmatch(string(first))
	if claim == nil {
		t.Errorf("trusted-claim frame did not reach the first request:\n%s", first)
	} else if claim[1] != "untrusted" || claim[2] != "data" {
		t.Errorf("producer raised its own trust: trust=%s authority=%s", claim[1], claim[2])
	}
	checks := []struct {
		index   int
		want    []string
		without []string
	}{
		{1, []string{"2:alpha two", "3:alpha three", "kb://sources/a.md"}, nil},
		{2, []string{"a.md", "nested", "page.html"}, nil},
		{3, []string{"kb://", "nothing was sent"}, nil},
		{4, []string{"Backend page", "Delivered by the reference fetch backend"}, []string{"<article>"}},
		{5, []string{"Located page", "knowledge base snapshot"}, nil},
		{6, []string{"404"}, []string{"Backend page"}},
		{7, []string{"registered through the Workbench docs contribution"}, nil},
		{8, []string{"Stored web sources", "browse stored sources", "reference-producer"}, nil},
		{9, []string{"Fixture result"}, nil},
	}
	for _, check := range checks {
		result := toolResult(check.index)
		for _, want := range check.want {
			if !strings.Contains(result, want) {
				t.Errorf("step %d (%s) lacks %q:\n%s", check.index-1, steps[check.index-1].name, want, result)
			}
		}
		for _, unwanted := range check.without {
			if strings.Contains(result, unwanted) {
				t.Errorf("step %d (%s) unexpectedly contains %q:\n%s", check.index-1, steps[check.index-1].name, unwanted, result)
			}
		}
	}
	if t.Failed() && len(output) > 0 {
		t.Logf("pig output tail:\n%s", output[max(0, len(output)-3000):])
	}
}
