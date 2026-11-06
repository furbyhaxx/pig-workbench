//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// secretRecorder serves a scripted provider and keeps every raw request body.
type secretRecorder struct {
	mu     sync.Mutex
	bodies []string
	calls  []scriptedCall
	next   func(index int) *scriptedCall
}

func (r *secretRecorder) serve(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		index := len(r.bodies)
		r.bodies = append(r.bodies, string(body))
		r.mu.Unlock()
		var call *scriptedCall
		if index < len(r.calls) {
			call = &r.calls[index]
		} else if r.next != nil {
			call = r.next(index)
		}
		if call != nil {
			writeScriptedTurn(w, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("secret-%d", index), "type": "function", "function": map[string]any{"name": call.tool, "arguments": mustJSON(call.args)}}}}, "tool_calls")
			return
		}
		writeScriptedTurn(w, map[string]any{"role": "assistant", "content": "done"}, "stop")
	}))
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestLiveProviderRequestsNeverCarryAKnownSecret(t *testing.T) {
	root := t.TempDir()
	fileToken := "ghp_" + strings.Repeat("Ab3dE9", 5)
	promptToken := "ghp_" + strings.Repeat("Zy8xW7", 5)
	commandToken := "ghp_" + strings.Repeat("Qw4eR5", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("deploy with "+fileToken+"\nthen stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "emit.sh"), []byte("echo "+commandToken+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	recorder := &secretRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "notes.txt"}},
		{"bash", map[string]any{"command": "sh emit.sh"}},
		{"bash", map[string]any{"command": "sh emit.sh; sleep 30", "background": true}},
	}}
	recorder.next = func(index int) *scriptedCall {
		recorder.mu.Lock()
		last := recorder.bodies[len(recorder.bodies)-1]
		recorder.mu.Unlock()
		id := regexp.MustCompile(`Job ([0-9a-f]{32}): running`).FindStringSubmatch(last)
		switch {
		case index == 3 && id != nil:
			return &scriptedCall{"jobs", map[string]any{"action": "output", "id": id[1]}}
		case index == 4 && id != nil:
			return &scriptedCall{"read", map[string]any{"path": "job://" + id[1] + "/log"}}
		}
		return nil
	}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash,jobs")
	fixture.send("script", "prompt", "My token is "+promptToken+", use it carefully.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 6 {
		t.Fatalf("got %d provider requests, want 6", len(recorder.bodies))
	}
	placeholder := regexp.MustCompile(`red-sec-[a-z2-7]{16}`)
	for i, body := range recorder.bodies {
		for name, secret := range map[string]string{"file": fileToken, "prompt": promptToken, "command output": commandToken} {
			if strings.Contains(body, secret) {
				t.Fatalf("request %d carries the %s secret", i, name)
			}
		}
	}
	last := recorder.bodies[len(recorder.bodies)-1]
	var request struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(last), &request); err != nil {
		t.Fatal(err)
	}
	var results []string
	for _, message := range request.Messages {
		if message["role"] == "tool" {
			results = append(results, toolMessageText(message))
		}
	}
	if len(results) != 5 {
		t.Fatalf("got %d tool results, want 5: %q", len(results), results)
	}
	for _, index := range []int{3, 4} {
		if !placeholder.MatchString(results[index]) {
			t.Fatalf("result %d did not carry the job log as a placeholder: %s", index, results[index])
		}
	}
	if got := len(placeholder.FindAllString(last, -1)); got < 3 {
		t.Fatalf("the final request should carry placeholders for all three secrets, found %d", got)
	}
}

func TestLiveToolReceivesTheRealValueWhileTheTranscriptKeepsThePlaceholder(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Mn6pL2", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("key="+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &secretRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	placeholderOf := func() string {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		match := regexp.MustCompile(`red-sec-[a-z2-7]{16}`).FindString(recorder.bodies[len(recorder.bodies)-1])
		return match
	}
	recorder.calls = []scriptedCall{{"read", map[string]any{"path": "notes.txt"}}}
	recorder.next = func(index int) *scriptedCall {
		if index == 1 {
			return &scriptedCall{"bash", map[string]any{"command": "printf %s " + placeholderOf() + " > copied.txt"}}
		}
		return nil
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "Copy the key.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	copied, err := os.ReadFile(filepath.Join(root, "copied.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != token {
		t.Fatalf("the shell received %q instead of the real value", copied)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, token) {
			t.Fatalf("request %d carries the real token", i)
		}
	}
}

func TestLiveBashReceivesAnInjectedEnvSecretAndItsOutputStaysRedacted(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	writeFixtureProvider(t, agent, "http://127.0.0.1:1")
	settings, err := config.New(config.Paths{User: filepath.Join(agent, "Workbench.toml"), Project: filepath.Join(root, "project", "Workbench.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	boundary := newSecretsBoundary(settings, agent, func() string { return "" }, func() bool { return false }, func() string { return root })
	guard, _, err := boundary.Guard()
	if err != nil {
		t.Fatal(err)
	}
	value := "injected-service-token-9f3a"
	if err := guard.Learn([]byte(value), "token", "user"); err != nil {
		t.Fatal(err)
	}
	placeholder, _ := guard.Redact(value)
	if err := boundary.vault.SetMetadata(secrets.Record{Placeholder: placeholder, Kind: "token", Source: "user", Scope: "global", Env: "SERVICE_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	recorder := &secretRecorder{calls: []scriptedCall{{"bash", map[string]any{"command": `printf %s "$SERVICE_TOKEN" > got.txt; echo "token=$SERVICE_TOKEN"`}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "bash")
	fixture.send("script", "prompt", "Print the service token variable.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	got, err := os.ReadFile(filepath.Join(root, "got.txt"))
	if err != nil || string(got) != value {
		t.Fatalf("the command saw %q (%v), want the injected value", got, err)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, value) {
			t.Fatalf("request %d carries the injected secret", i)
		}
	}
	if !strings.Contains(recorder.bodies[len(recorder.bodies)-1], placeholder) {
		t.Fatal("the command output should reach the model as the placeholder")
	}
}
