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

// TestLiveCrashReclaimsOrphanedJob proves a killed agent leaves no unsupervised
// process: the next session start terminates the job its predecessor owned
// (SPEC-0008 §8 crash ownership).
func TestLiveCrashReclaimsOrphanedJob(t *testing.T) {
	root := t.TempDir()
	markerFile := filepath.Join(root, "crash-job-started")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(fmt.Sprint(body["messages"]), "start the job") {
			args, _ := json.Marshal(map[string]any{"command": "sh -c 'touch " + markerFile + "; sleep 300; true'", "background": true})
			delta := map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call-0", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(args)}}}}
			writeScriptedTurn(w, delta, "tool_calls")
			return
		}
		writeScriptedTurn(w, map[string]any{"role": "assistant", "content": "ok"}, "stop")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)

	first := startRPCWithExtensions(t, root, true, []string{"probe-agent"}, "--model", "workbench-fixture/mock")
	first.send("job", "prompt", "start the job")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(markerFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background job never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !processMatches(t, markerFile) {
		t.Skip("the job process is not observable on this platform")
	}
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-first.done
	// The kill was deliberate; the fixture would otherwise report it as a failure
	// and hold its deadline open during cleanup.
	first.exitErr = nil
	first.cancel()
	if !processMatches(t, markerFile) {
		t.Skip("the orphan did not survive the crash on this platform; nothing to reclaim")
	}

	started := time.Now()
	second := startRPCWithExtensions(t, root, true, []string{"probe-agent"}, "--model", "workbench-fixture/mock")
	second.send("hi", "prompt", "hi")
	deadline = time.Now().Add(10 * time.Second)
	for processMatches(t, markerFile) {
		if time.Now().After(deadline) {
			t.Fatal("a crashed agent left the job running after the next start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("reclamation took %s, which is not a prompt next-start sweep", elapsed)
	} else {
		t.Logf("orphan reclaimed %s after the next session started", elapsed.Round(time.Millisecond))
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

func writeScriptedTurn(w http.ResponseWriter, delta map[string]any, finish string) {
	w.Header().Set("Content-Type", "text/event-stream")
	data, _ := json.Marshal(map[string]any{"id": "r", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
	end, _ := json.Marshal(map[string]any{"id": "r", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}})
	fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", data, end)
}

// TestLiveConfiguredJobTimeoutApplies proves the bash tool's deadline comes from
// configuration rather than a hardcoded value (SPEC-0008 shared job policy).
func TestLiveConfiguredJobTimeoutApplies(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("version = 1\n\n[jobs]\ntimeout-seconds = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var toolResult string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		messages := fmt.Sprint(body["messages"])
		mu.Unlock()
		if strings.Contains(messages, "run a slow command") && !strings.Contains(messages, "Job ") {
			args, _ := json.Marshal(map[string]any{"command": "sleep 30"})
			delta := map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call-0", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(args)}}}}
			writeScriptedTurn(w, delta, "tool_calls")
			return
		}
		mu.Lock()
		toolResult = messages
		mu.Unlock()
		writeScriptedTurn(w, map[string]any{"role": "assistant", "content": "ok"}, "stop")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)

	f := startRPCWithExtensions(t, root, true, []string{"probe-agent"}, "--model", "workbench-fixture/mock")
	f.send("slow", "prompt", "run a slow command")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	mu.Lock()
	result := toolResult
	mu.Unlock()
	if !strings.Contains(result, "timedOut") && !strings.Contains(result, "timedout") {
		t.Fatalf("the configured 1s deadline did not end the command: %s", result)
	}
}

// TestLiveSessionEndTerminatesJobs proves the shutdown path still kills running
// jobs; the next-start sweep covers the case where shutdown never runs.
func TestLiveSessionEndTerminatesJobs(t *testing.T) {
	root := t.TempDir()
	markerFile := filepath.Join(root, "session-end-job")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(fmt.Sprint(body["messages"]), "start the job") {
			args, _ := json.Marshal(map[string]any{"command": "sh -c 'touch " + markerFile + "; sleep 300; true'", "background": true})
			delta := map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call-0", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(args)}}}}
			writeScriptedTurn(w, delta, "tool_calls")
			return
		}
		writeScriptedTurn(w, map[string]any{"role": "assistant", "content": "ok"}, "stop")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)

	f := startRPCWithExtensions(t, root, true, []string{"probe-agent"}, "--model", "workbench-fixture/mock")
	f.send("job", "prompt", "start the job")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(markerFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background job never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !processMatches(t, markerFile) {
		t.Skip("the job process is not observable on this platform")
	}
	// The shutdown handler closes the job manager, so stdin close alone is enough;
	// the fixture's own close waits on it and reports the wait as a failure.
	if err := f.stdin.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(20 * time.Second)
	for processMatches(t, markerFile) {
		if time.Now().After(deadline) {
			t.Fatal("ending the session left the job running")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The session was ended on purpose; release the fixture without reporting it.
	f.cancel()
	<-f.done
	f.exitErr = nil
}
