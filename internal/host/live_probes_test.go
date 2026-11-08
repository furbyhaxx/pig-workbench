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

// probeSession starts Workbench with the probe fixture and a scripted provider,
// returning the RPC fixture plus the recorded provider requests.
type probeRecorder struct {
	mu       sync.Mutex
	requests []map[string]any
	tools    []map[string]any
}

func (r *probeRecorder) record(body map[string]any) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, body)
	return len(r.requests) - 1
}

func (r *probeRecorder) messages(index int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index >= len(r.requests) {
		return ""
	}
	encoded, _ := json.Marshal(r.requests[index]["messages"])
	return string(encoded)
}

func (r *probeRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func startProbeSession(t *testing.T, script func(index int, body map[string]any) (delta map[string]any, finish any)) (*rpcFixture, *probeRecorder) {
	t.Helper()
	root := t.TempDir()
	recorder := &probeRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		index := recorder.record(body)
		delta, finish := script(index, body)
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(payload any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("r%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": payload, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		send(delta, nil)
		send(map[string]any{}, finish)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	f := startRPCWithExtensions(t, root, true, []string{"probe-agent"}, "--model", "workbench-fixture/mock")
	return f, recorder
}

func probeNotify(f *rpcFixture, probe string) map[string]any {
	f.t.Helper()
	record := f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"probe":"`+probe+`"`)
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(record["message"].(string)), &result); err != nil {
		f.t.Fatal(err)
	}
	return result
}

// TestProbeIdleWakeStartsTurn observes a followUp wake with TriggerTurn from a
// goroutine using the context retained at session_start, while the session idles.
func TestProbeIdleWakeStartsTurn(t *testing.T) {
	f, recorder := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		return map[string]any{"role": "assistant", "content": fmt.Sprintf("reply %d", index)}, "stop"
	})
	f.send("first", "prompt", "start")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	before := recorder.count()
	f.send("wake", "prompt", "/probe wake PROBE-WAKE-IDLE")
	probeNotify(f, "wake")
	waitForRequests(t, f, recorder, before+1)
	if !strings.Contains(recorder.messages(before), "PROBE-WAKE-IDLE") {
		t.Fatalf("woken request lacks the wake content: %s", recorder.messages(before))
	}
}

func waitForRequests(t *testing.T, f *rpcFixture, recorder *probeRecorder, want int) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if recorder.count() >= want {
			return
		}
		select {
		case <-f.ctx.Done():
			t.Fatalf("provider received %d of %d expected requests", recorder.count(), want)
		case <-ticker.C:
		}
	}
}

// TestProbeBusyWakeSteers observes a wake scheduled by a tool during a running
// turn: it must reach the model at the next boundary of that same turn.
func TestProbeBusyWakeSteers(t *testing.T) {
	f, recorder := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		if index == 0 {
			args, _ := json.Marshal(map[string]any{"text": "PROBE-WAKE-BUSY"})
			return map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call-0", "type": "function", "function": map[string]any{"name": "probe_wake", "arguments": string(args)}}}}, "tool_calls"
		}
		return map[string]any{"role": "assistant", "content": "done"}, "stop"
	})
	f.send("busy", "prompt", "run the probe")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	if recorder.count() < 2 {
		t.Fatalf("expected the tool result to continue the turn, got %d requests", recorder.count())
	}
	if !strings.Contains(recorder.messages(1), "PROBE-WAKE-BUSY") {
		t.Fatalf("busy turn did not receive the steered wake: %s", recorder.messages(1))
	}
}

// TestProbeSessionTitleT1 observes SetSessionName visibility without reload and
// whether an empty name clears the title.
func TestProbeSessionTitleT1(t *testing.T) {
	f, _ := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		return map[string]any{"role": "assistant", "content": "ok"}, "stop"
	})
	f.send("name", "prompt", "/probe name parser refactor")
	result := probeNotify(f, "name")
	if result["read"] != "parser refactor" || result["replicated"] != "parser refactor" {
		t.Fatalf("title not visible after set: %+v", result)
	}
	f.send("clear", "prompt", "/probe clear")
	cleared := probeNotify(f, "clear")
	if cleared["read"] != "<nil>" {
		t.Fatalf("empty name did not clear the title: %+v", cleared)
	}
}

// TestProbeSessionInfoT2 observes that the title is persisted as a session_info
// entry and survives a resume of the same session.
func TestProbeSessionInfoT2(t *testing.T) {
	f, _ := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		return map[string]any{"role": "assistant", "content": "ok"}, "stop"
	})
	f.send("name", "prompt", "/probe name persisted title")
	probeNotify(f, "name")
	f.send("turn", "prompt", "record the title")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	f.send("title", "prompt", "/probe title")
	title := probeNotify(f, "title")
	path, _ := title["file"].(string)
	if path == "" || path == "<nil>" {
		t.Fatalf("session file missing: %+v", title)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "session_info") || !strings.Contains(string(raw), "persisted title") {
		t.Fatalf("title was not persisted as session_info:\n%s", raw)
	}
	if title["sessionInfoEntries"] == float64(0) {
		t.Fatalf("no session_info entry recorded: %+v", title)
	}
}

// TestProbeSessionInfoBranchT3 observes which session_info wins when the entry
// is not on the active branch: fork from a point before the rename.
func TestProbeSessionInfoBranchT3(t *testing.T) {
	f, _ := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		return map[string]any{"role": "assistant", "content": "ok"}, "stop"
	})
	f.send("first", "prompt", "first user message")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	f.send("name", "prompt", "/probe name branch-A")
	probeNotify(f, "name")
	f.send("ids", "prompt", "/probe ids")
	ids := probeNotify(f, "ids")
	rows, _ := ids["entries"].([]any)
	forkPoint := ""
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["type"] == "message" && row["role"] == "user" && forkPoint == "" {
			forkPoint, _ = row["id"].(string)
		}
	}
	if forkPoint == "" {
		t.Fatalf("no user entry to fork before: %+v", ids)
	}
	f.sendRaw("fork", "fork", map[string]any{"entryId": forkPoint})
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "fork" })
	f.send("title", "prompt", "/probe title")
	title := probeNotify(f, "title")
	t.Logf("title after fork before the rename: %+v", title)
	if title["read"] == "branch-A" {
		t.Log("session_info is session-wide: the inactive-branch rename still names the session")
	} else {
		t.Logf("session_info is branch-scoped: after forking before the rename the title reads %v", title["read"])
	}
}

// TestProbeResumedCwdAcrossSymlinkedBuckets observes the cwd a resumed session
// reports when --continue picks a session recorded in another checkout of the
// same project (SPEC-0012 §3.5 open follow-up).
func TestProbeResumedCwdAcrossSymlinkedBuckets(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "work", "repo")
	cloneDir := filepath.Join(root, "work", "clone")
	for _, dir := range []string{repoDir, cloneDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	shared := filepath.Join(root, "projects", "prj-test", "sessions")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{repoDir, cloneDir} {
		bucket := filepath.Join(root, "agent", "sessions", bucketName(dir))
		if err := os.MkdirAll(filepath.Dir(bucket), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, bucket); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"r\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"r\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)

	start := func(dir string, args ...string) *rpcFixture {
		t.Helper()
		repo := repoRoot(t)
		options := []string{"--offline", "-e", repo, "-e", filepath.Join(repo, "internal/host/testdata/probe-agent"), "--mode", "rpc", "--model", "workbench-fixture/mock"}
		options = append(options, args...)
		var env []string
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "PIG_") && !strings.HasPrefix(value, "PI_") {
				env = append(env, value)
			}
		}
		env = append(env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+filepath.Join(root, "agent"), "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		cmd := exec.CommandContext(ctx, "pig", options...)
		cmd.Dir = dir
		cmd.Env = env
		return newRPCFixture(t, cmd, ctx, cancel)
	}
	first := start(repoDir)
	first.send("name", "prompt", "/probe name shared session")
	probeNotify(first, "name")
	first.send("turn", "prompt", "record")
	first.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	first.close()

	resumed := start(cloneDir, "--continue")
	resumed.send("cwd", "prompt", "/probe cwd")
	result := probeNotify(resumed, "cwd")
	t.Logf("resumed in %s: %+v", cloneDir, result)
	if result["sessionName"] != "shared session" {
		t.Fatalf("--continue did not pick the shared session: %+v", result)
	}
	resumed.close()
}

func bucketName(dir string) string {
	encoded := strings.TrimPrefix(dir, "/")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	return "--" + encoded + "--"
}

// TestProbeNativeCredentialExpansion observes how models.json apiKey forms reach
// the wire: literal, $ENV expansion and a leading !command (SPEC-0013 §2 M4).
func TestProbeNativeCredentialExpansion(t *testing.T) {
	root := t.TempDir()
	type observed struct {
		auth   string
		custom string
	}
	seen := map[string]observed{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		path := r.URL.Path
		seen[path] = observed{auth: r.Header.Get("Authorization"), custom: r.Header.Get("X-Probe")}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"r\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"r\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	models := map[string]any{"providers": map[string]any{
		"probe-literal": map[string]any{"baseUrl": server.URL + "/literal/v1", "api": "openai-completions", "apiKey": "sk-literal-value", "models": []any{map[string]any{"id": "mock", "name": "literal", "input": []string{"text"}, "contextWindow": 32000, "maxTokens": 1000}}},
		"probe-env":     map[string]any{"baseUrl": server.URL + "/env/v1", "api": "openai-completions", "apiKey": "$PROBE_API_KEY", "models": []any{map[string]any{"id": "mock", "name": "env", "input": []string{"text"}, "contextWindow": 32000, "maxTokens": 1000}}},
		"probe-command": map[string]any{"baseUrl": server.URL + "/command/v1", "api": "openai-completions", "apiKey": "!printf sk-from-command", "models": []any{map[string]any{"id": "mock", "name": "command", "input": []string{"text"}, "contextWindow": 32000, "maxTokens": 1000}}},
	}}
	data, err := json.Marshal(models)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "models.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"probe-literal", "probe-env", "probe-command"} {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		cmd := exec.CommandContext(ctx, "pig", "--offline", "--no-session", "--mode", "json", "--model", provider+"/mock", "hi")
		cmd.Dir = root
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "PROBE_API_KEY=sk-from-env", "PIG_HOME=" + root, "PIG_CODING_AGENT_DIR=" + agent, "XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_STATE_HOME=" + filepath.Join(root, "state")}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		cancel()
		if err != nil {
			t.Logf("%s: %v (stderr %s; stdout %s)", provider, err, stderr.String(), output)
			continue
		}
	}
	for path, value := range seen {
		t.Logf("%s Authorization=%q X-Probe=%q", path, value.auth, value.custom)
	}
	if len(seen) == 0 {
		t.Skip("no provider reached the local server; credential expansion unobserved")
	}
}

// TestProbeIdleStateTransitions observes ctx.IsIdle across a running turn, an
// abort, and the settled state (SPEC-0011 §10 state tracking).
func TestProbeIdleStateTransitions(t *testing.T) {
	release := make(chan struct{})
	f, _ := startProbeSession(t, func(index int, _ map[string]any) (map[string]any, any) {
		<-release
		return map[string]any{"role": "assistant", "content": "ok"}, "stop"
	})
	f.send("turn", "prompt", "hold the turn")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_start" })
	waitForState(t, f, false)
	f.sendRaw("abort", "abort", nil)
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "abort" })
	close(release)
	f.send("settled", "prompt", "/probe state")
	settled := probeNotify(f, "state")
	if settled["idle"] != true {
		t.Fatalf("session did not return to idle after an aborted turn: %+v", settled)
	}
}

func waitForState(t *testing.T, f *rpcFixture, wantIdle bool) {
	t.Helper()
	f.send("state", "prompt", "/probe state")
	record := f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"probe":"state"`)
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(record["message"].(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result["idle"] != wantIdle {
		t.Fatalf("idle = %v, want %v: %+v", result["idle"], wantIdle, result)
	}
}

// TestLiveForkWithAContributedStatusSegmentDoesNotDeadlock: the session replacement closes the old
// status board while the host holds the fork request open, so the clear must run on the live context.
func TestLiveForkWithAContributedStatusSegmentDoesNotDeadlock(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		data, _ := json.Marshal(map[string]any{"id": "r", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	f := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock")
	f.send("first", "prompt", "first")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	f.sendRaw("forks", "get_fork_messages", nil)
	listed := f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "forks" })
	messages, _ := listed["data"].(map[string]any)["messages"].([]any)
	if len(messages) == 0 {
		t.Fatalf("no fork point: %+v", listed)
	}
	entry, _ := messages[0].(map[string]any)["entryId"].(string)
	f.sendRaw("fork", "fork", map[string]any{"entryId": entry})
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "fork" })
}
