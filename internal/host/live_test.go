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

func TestLiveReadLocationAndCodemode(t *testing.T) {
	if _, err := exec.LookPath("pig"); err != nil {
		t.Fatal("integration requires pig on PATH")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	next := filepath.Join(work, "next")
	agent := filepath.Join(root, "agent")
	for _, dir := range []string{next, agent} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, text := range map[string]string{filepath.Join(work, "source.txt"): "one\nβ\nthree\n" + strings.Repeat("overflow\n", 100), filepath.Join(next, "source.txt"): "nested-location\n"} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct {
		name string
		args map[string]any
	}{
		{"read", map[string]any{"path": "source.txt:2-3:raw"}},
		{"cd", map[string]any{"path": "next"}},
		{"read", map[string]any{"path": "source.txt:raw"}},
		{"codemode", map[string]any{"code": "const r = await Promise.allSettled([tools.read({path:'source.txt:raw'}), tools.read({path:'missing.txt'})]); for (const v of r) text(v.status === 'fulfilled' ? v.value : v.reason.message);"}},
		{"cd", map[string]any{"reset": true}},
		{"read", map[string]any{"path": "source.txt:raw"}},
	}
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		mu.Lock()
		index := len(requests)
		requests = append(requests, request)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("fixture-%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < len(steps) {
			step := steps[index]
			args, _ := json.Marshal(step.args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("call-%d", index), "type": "function", "function": map[string]any{"name": step.name, "arguments": string(args)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "fixture finished"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[read]\nmax-lines = 8\nmax-bytes = 4096\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pig", "--offline", "--no-session", "-e", repo, "--mode", "json", "--model", "workbench-fixture/mock", "--tools", "read,cd,codemode", "Exercise the scripted fixture.")
	cmd.Dir = work
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PIG_") && !strings.HasPrefix(v, "PI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("live pig: %v\nstderr: %s\nstdout: %s", err, stderr.String(), output)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != len(steps)+1 {
		t.Fatalf("got %d provider requests, want %d\n%s\n%s", len(requests), len(steps)+1, output, stderr.String())
	}
	requestText := func(i int) string {
		data, err := json.Marshal(requests[i]["messages"])
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if !strings.Contains(requestText(1), "β\\nthree\\n") {
		t.Fatalf("direct raw selector lost exact content: %s", requestText(1))
	}
	if !strings.Contains(requestText(3), "nested-location\\n") {
		t.Fatalf("cd did not affect subsequent relative read: %s", requestText(3))
	}
	if !strings.Contains(requestText(4), "nested-location") || !strings.Contains(requestText(4), "Script completed") {
		t.Fatalf("codemode lost nested result: %s", requestText(4))
	}
	var nestedVerified, budgetVerified bool
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		var event struct {
			Type    string `json:"type"`
			Message struct {
				ToolName   string          `json:"toolName"`
				ToolCallID string          `json:"toolCallId"`
				Content    json.RawMessage `json:"content"`
				Details    struct {
					ConfigRevision uint64 `json:"configRevision"`
					Resources      []struct {
						SelectionComplete bool `json:"selectionComplete"`
					} `json:"resources"`
				} `json:"details"`
				NestedCalls struct {
					Complete bool `json:"complete"`
					Calls    []struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					} `json:"calls"`
				} `json:"nestedCalls"`
			} `json:"message"`
		}
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "message_end" && event.Message.ToolName == "codemode" {
			calls := event.Message.NestedCalls.Calls
			if !event.Message.NestedCalls.Complete || len(calls) != 2 || calls[0].ID != "call-3/1" || calls[0].Status != "ok" || calls[1].Status != "error" {
				t.Fatalf("nested outcomes lost: %+v", event.Message.NestedCalls)
			}
			nestedVerified = true
		}
		if event.Type == "message_end" && event.Message.ToolCallID == "call-5" {
			details := event.Message.Details
			if details.ConfigRevision == 0 || len(details.Resources) != 1 || details.Resources[0].SelectionComplete {
				t.Fatalf("configured read ceiling or provenance lost: %+v", details)
			}
			var blocks []struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Message.Content, &blocks); err != nil {
				t.Fatal(err)
			}
			var text strings.Builder
			for _, block := range blocks {
				text.WriteString(block.Text)
			}
			if strings.Count(text.String(), "\n")+1 > 8 || len(text.String()) > 4096 {
				t.Fatal("read ignored configured result budget")
			}
			budgetVerified = true
		}
	}
	if !budgetVerified {
		t.Fatal("no configured-budget read result was observed")
	}
	if !nestedVerified {
		t.Fatal("no retained codemode call tree")
	}

	if !strings.Contains(requestText(5), work) {
		t.Fatalf("reset did not return to origin: %s", requestText(5))
	}
}
