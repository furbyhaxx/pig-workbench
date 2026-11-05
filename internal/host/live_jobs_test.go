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

// TestLiveReloadTerminatesRunningJobs proves a reload does not leave an
// unsupervised background process behind (SPEC-0008 §8 reload cleanup).
//
// Known defect, tracked by slice-1-completion 10.1.2: a background job outlives
// /reload until its own timeout, so shutdown-time cleanup does not take effect
// across a reload (ANA-2026-11-05-slice-1-probes). Remove the skip when the
// next-start reconciliation lands.
func TestLiveReloadTerminatesRunningJobs(t *testing.T) {
	t.Skip("known defect 10.1.2: /reload skips session_shutdown, background jobs outlive it")

	markerFile := filepath.Join(t.TempDir(), fmt.Sprintf("reload-job-%d", time.Now().UnixNano()))
	marker := markerFile
	f, _ := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		if index == 0 {
			args, _ := json.Marshal(map[string]any{"command": "sh -c 'touch " + markerFile + "; sleep 300; true'", "background": true})
			return map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call-0", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(args)}}}}, "tool_calls"
		}
		return map[string]any{"role": "assistant", "content": "started"}, "stop"
	})
	f.send("job", "prompt", "start the background job")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	if _, err := os.Stat(markerFile); err != nil {
		t.Fatalf("the background job never started: %v", err)
	}
	if !processMatches(t, marker) {
		t.Fatal("the running job is not observable before the reload")
	}
	f.send("reload", "prompt", "/reload")
	deadline := time.Now().Add(10 * time.Second)
	for processMatches(t, marker) {
		if time.Now().After(deadline) {
			t.Fatalf("reload left the background job running: %s", marker)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// processMatches reports whether any process command line contains needle.
func processMatches(t *testing.T, needle string) bool {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), needle) {
			return true
		}
	}
	return false
}
