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

func TestLiveBackgroundJobStdinAndCompletion(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	requests := 0
	latest := ""
	jobID := ""
	idPattern := regexp.MustCompile(`Job ([0-9a-f]{32}):`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		mu.Lock()
		index := requests
		requests++
		encoded, _ := json.Marshal(request["messages"])
		latest = string(encoded)
		if match := idPattern.FindStringSubmatch(latest); len(match) > 1 {
			jobID = match[1]
		}
		id := jobID
		mu.Unlock()
		name := "jobs"
		args := map[string]any{"id": id}
		switch index {
		case 0:
			name = "bash"
			args = map[string]any{"command": "cat", "interactive": true, "background": true, "timeout_seconds": 10}
		case 1:
			args["action"] = "write"
			args["text"] = "live-job-roundtrip\n"
		case 2:
			args["action"] = "eof"
		case 3:
			args["action"] = "wait"
			args["timeout_seconds"] = 5
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": "jobs-fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < 4 {
			arguments, _ := json.Marshal(args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("job-call-%d", index), "type": "function", "function": map[string]any{"name": name, "arguments": string(arguments)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "finished"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "pig", "--offline", "--no-session", "-e", repo, "--mode", "json", "--model", "workbench-fixture/mock", "--tools", "bash,jobs", "Run the scripted job fixture.")
	command.Dir = root
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PI_") && !strings.HasPrefix(value, "PIG_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("host: %v\n%s\n%s", err, stderr.String(), output)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 5 || jobID == "" || !strings.Contains(latest, "live-job-roundtrip") || !strings.Contains(latest, "exited") {
		t.Fatalf("job workflow did not complete: requests=%d id=%q messages=%s", requests, jobID, latest)
	}
}
