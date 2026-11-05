//go:build integration

package host

import (
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

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestLiveTaggedAndExactEditUseObservedSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("\ufeffone\r\ntwo\r\nthree\r\n"), 0640); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	requests := 0
	latest := ""
	tagPattern := regexp.MustCompile(`#([A-F0-9]{4})\]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad JSON", 400)
			return
		}
		mu.Lock()
		index := requests
		requests++
		body, _ := json.Marshal(request["messages"])
		latest = string(body)
		text := latest
		mu.Unlock()
		name := "read"
		args := map[string]any{"path": "source.txt"}
		switch index {
		case 1:
			tag := "UNKNOWN"
			if match := tagPattern.FindStringSubmatch(text); len(match) > 1 {
				tag = match[1]
			}
			name = "edit"
			args = map[string]any{"hashline": "[" + resource.FileURI(path) + "#" + tag + "]\nPUT 2.=2:\n+changed\n"}
		case 2:
			name = "edit"
			args = map[string]any{"path": "source.txt", "edits": []any{map[string]any{"oldText": "changed", "newText": "final"}}}
		case 3:
			args["path"] = "source.txt:raw"
		case 4:
			copied := ""
			if messages, ok := request["messages"].([]any); ok {
				for _, raw := range messages {
					if message, ok := raw.(map[string]any); ok && message["tool_call_id"] == "edit-call-0" {
						copied, _ = message["content"].(string)
					}
				}
			}
			name = "write"
			args = map[string]any{"path": "copy.txt", "content": copied}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": "edit-fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < 5 {
			encoded, _ := json.Marshal(args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("edit-call-%d", index), "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[edit]\nmode = 'auto'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "read,edit,write")
	fixture.send("edit", "prompt", "Run the scripted editing fixture.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if string(actual) != "\ufeffone\r\nfinal\r\nthree\r\n" {
		t.Fatalf("edits did not land: %q\n%s", actual, latest)
	}
	if !strings.Contains(latest, `one\r\nfinal\r\nthree\r\n`) {
		t.Fatalf("edited bytes did not reach the model: %s", latest)
	}
	copied, err := os.ReadFile(filepath.Join(root, "copy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != "\ufeffone\r\ntwo\r\nthree\r\n" {
		t.Fatalf("copied tagged view lost original byte conventions: %q", copied)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatal("edit changed file mode")
	}
}

func TestLiveTaggedEditRelocatesAcrossExternalChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("\ufeffone\r\ntwo\r\nthree\r\n"), 0640); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	requests := 0
	latest := ""
	tagPattern := regexp.MustCompile(`#([A-F0-9]{4})\]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad JSON", 400)
			return
		}
		mu.Lock()
		index := requests
		requests++
		body, _ := json.Marshal(request["messages"])
		latest = string(body)
		text := latest
		mu.Unlock()
		name := "read"
		args := map[string]any{"path": "source.txt"}
		switch index {
		case 1:
			tag := "UNKNOWN"
			if match := tagPattern.FindStringSubmatch(text); len(match) > 1 {
				tag = match[1]
			}
			if err := os.WriteFile(path, []byte("\ufeffzero\r\none\r\ntwo\r\nthree\r\n"), 0640); err != nil {
				t.Errorf("external change: %v", err)
			}
			name = "edit"
			args = map[string]any{"hashline": "[" + resource.FileURI(path) + "#" + tag + "]\nPUT 2.=2:\n+changed\n"}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": "edit-drift-fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < 2 {
			encoded, _ := json.Marshal(args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("drift-call-%d", index), "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[edit]\nmode = 'auto'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "read,edit")
	fixture.send("edit", "prompt", "Run the scripted external-change edit fixture.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != "\ufeffzero\r\none\r\nchanged\r\nthree\r\n" {
		t.Fatalf("relocated edit did not preserve the external change: %q\n%s", actual, latest)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(latest, "changed") || !strings.Contains(latest, "zero") {
		t.Fatalf("post-edit source did not reach the model: %s", latest)
	}
}
