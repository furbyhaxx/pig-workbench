//go:build integration && liveeval

package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// The live evaluation spends real provider tokens; run it with `just live-eval`.
// The key arrives only through the environment and never reaches retained evidence.

const liveModel = "deepseek/deepseek-flash"

type evalSession struct {
	t          *testing.T
	f          *rpcFixture
	transcript []map[string]any
	tools      map[string]int
	starts     []string
	input      int
	output     int
	cacheRead  int
}

func requireLiveKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		t.Skip("DEEPSEEK_API_KEY is not set; use `just live-eval`")
	}
	return key
}

func startEvalSession(t *testing.T, root, work string, delivery string, extra ...string) *evalSession {
	t.Helper()
	requireLiveKey(t)
	repo := repoRoot(t)
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0700); err != nil {
		t.Fatal(err)
	}
	settings := "[prompt]\ndelivery = '" + delivery + "'\n"
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	args := []string{"--offline", "--session-dir", filepath.Join(root, "sessions"), "-e", repo, "--mode", "rpc", "--model", liveModel, "--thinking", "off"}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, "pig", args...)
	cmd.Dir = work
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PIG_") && !strings.HasPrefix(v, "PI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	f := newRPCFixture(t, cmd, ctx, cancel)
	f.root = root
	return &evalSession{t: t, f: f, tools: map[string]int{}}
}

type turnResult struct {
	text      string
	tools     []string
	dialogs   int
	startArgs string
	calls     []toolCall
}

// toolCall is one executed tool call with its observed outcome.
type toolCall struct {
	name    string
	args    string
	isError bool
	text    string
}

// succeeded reports whether some call to the tool named every fragment and did not fail.
func (r turnResult) succeeded(tool string, fragments ...string) bool {
	for _, call := range r.calls {
		if call.name != tool || call.isError {
			continue
		}
		match := true
		for _, fragment := range fragments {
			if !strings.Contains(call.args, fragment) {
				match = false
			}
		}
		if match {
			return true
		}
	}
	return false
}

// turn sends one prompt and answers dialogs through answer until the agent settles.
func (s *evalSession) turn(id, message string, answer func(event map[string]any) string) turnResult {
	s.t.Helper()
	s.f.send(id, "prompt", message)
	var result turnResult
	var last bytes.Buffer
	pending := map[string]int{}
	for {
		select {
		case record := <-s.f.records:
			s.keep(record)
			switch record["type"] {
			case "extension_error":
				s.t.Fatalf("extension error: %+v", record)
			case "response":
				if record["success"] == false {
					s.t.Fatalf("RPC error: %+v", record)
				}
			case "tool_execution_start":
				name := fmt.Sprint(record["toolName"])
				s.tools[name]++
				result.tools = append(result.tools, name)
				encoded, _ := json.Marshal(record["args"])
				result.startArgs += name + " " + string(encoded) + "\n"
				result.calls = append(result.calls, toolCall{name: name, args: string(encoded)})
				pending[fmt.Sprint(record["toolCallId"])] = len(result.calls) - 1
			case "tool_execution_end":
				if index, ok := pending[fmt.Sprint(record["toolCallId"])]; ok {
					result.calls[index].isError, _ = record["isError"].(bool)
					encoded, _ := json.Marshal(record["result"])
					result.calls[index].text = string(encoded)
				}
			case "extension_ui_request":
				method := fmt.Sprint(record["method"])
				if method == "select" || method == "input" || method == "confirm" || method == "editor" {
					result.dialogs++
					value := answer(record)
					response := map[string]any{"type": "extension_ui_response", "id": record["id"], "value": value}
					encoded, _ := json.Marshal(response)
					fmt.Fprintf(s.f.stdin, "%s\n", encoded)
				}
			case "message_end":
				if message, ok := record["message"].(map[string]any); ok && message["role"] == "assistant" {
					last.Reset()
					if content, ok := message["content"].([]any); ok {
						for _, block := range content {
							if b, ok := block.(map[string]any); ok && b["type"] == "text" {
								last.WriteString(fmt.Sprint(b["text"]))
							}
						}
					}
					if usage, ok := message["usage"].(map[string]any); ok {
						s.input += int(number(usage["input"]))
						s.output += int(number(usage["output"]))
						s.cacheRead += int(number(usage["cacheRead"]))
					}
				}
			case "agent_settled":
				result.text = last.String()
				for _, call := range result.calls {
					if call.isError {
						s.t.Logf("turn %s: %s failed (recovered or not, see checks): %.200s", id, call.name, call.text)
					}
				}
				return result
			}
		case <-s.f.done:
			s.t.Fatalf("pig exited early: %v stderr: %s", s.f.exitErr, s.f.stderr.String())
		case <-s.f.ctx.Done():
			s.t.Fatal("live turn deadline exceeded")
		}
	}
}

func number(value any) float64 {
	n, _ := value.(float64)
	return n
}

// keep retains the records that evidence behavior and drops streaming noise.
func (s *evalSession) keep(record map[string]any) {
	switch record["type"] {
	case "message_update", "entry_appended", "turn_start", "turn_end", "message_start":
		return
	}
	s.transcript = append(s.transcript, record)
}

var keyShape = regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`)

// save writes the transcript with the API key and key-shaped tokens removed.
func (s *evalSession) save(name string) {
	s.t.Helper()
	out := os.Getenv("LIVE_EVAL_OUT")
	if out == "" {
		return
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		s.t.Fatal(err)
	}
	var buf bytes.Buffer
	for _, record := range s.transcript {
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		buf.Write(encoded)
		buf.WriteByte('\n')
	}
	text := buf.String()
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		text = strings.ReplaceAll(text, key, "<redacted>")
	}
	text = keyShape.ReplaceAllString(text, "<redacted>")
	text = strings.ReplaceAll(text, s.f.root, "$ROOT")
	if err := os.WriteFile(filepath.Join(out, name), []byte(text), 0644); err != nil {
		s.t.Fatal(err)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func hasAll(have []string, want ...string) []string {
	var missing []string
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
			}
		}
		if !found {
			missing = append(missing, w)
		}
	}
	return missing
}

// providerTap records every request body that pig sends to the real provider. It sits in front of
// the builtin deepseek provider through a models.json baseUrl override; headers (the key) are never kept.
type providerTap struct {
	mu     sync.Mutex
	bodies []string
}

func startProviderTap(t *testing.T, root string) *providerTap {
	t.Helper()
	upstream, _ := url.Parse("https://api.deepseek.com")
	tap := &providerTap{}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.FlushInterval = -1
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = upstream.Host
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if strings.Contains(r.URL.Path, "chat/completions") {
			tap.mu.Lock()
			tap.bodies = append(tap.bodies, string(body))
			tap.mu.Unlock()
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0700); err != nil {
		t.Fatal(err)
	}
	models, _ := json.Marshal(map[string]any{"providers": map[string]any{"deepseek": map[string]any{"baseUrl": server.URL}}})
	if err := os.WriteFile(filepath.Join(agent, "models.json"), models, 0600); err != nil {
		t.Fatal(err)
	}
	return tap
}

func (p *providerTap) requests() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.bodies...)
}

// count is how many times marker appears in the request at index (JSON-escaped text is unescaped first).
func (p *providerTap) count(index int, marker string) int {
	requests := p.requests()
	if index >= len(requests) {
		return -1
	}
	return strings.Count(requests[index], marker)
}

// save writes the recorded requests with the key and key-shaped tokens removed.
func (p *providerTap) save(t *testing.T, name string) {
	t.Helper()
	out := os.Getenv("LIVE_EVAL_OUT")
	if out == "" {
		return
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(p.requests(), "\n")
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		text = strings.ReplaceAll(text, key, "<redacted>")
	}
	text = keyShape.ReplaceAllString(text, "<redacted>")
	if err := os.WriteFile(filepath.Join(out, name), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}
