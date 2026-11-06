//go:build integration

package host

import (
	"encoding/json"
	"fmt"
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
}

func (r *secretRecorder) serve(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		index := len(r.bodies)
		r.bodies = append(r.bodies, string(body))
		r.mu.Unlock()
		if index < len(r.calls) {
			writeScriptedTurn(w, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("secret-%d", index), "type": "function", "function": map[string]any{"name": r.calls[index].tool, "arguments": mustJSON(r.calls[index].args)}}}}, "tool_calls")
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
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "My token is "+promptToken+", use it carefully.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != len(recorder.calls)+1 {
		t.Fatalf("got %d provider requests, want %d", len(recorder.bodies), len(recorder.calls)+1)
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
	if got := len(placeholder.FindAllString(last, -1)); got < 3 {
		t.Fatalf("the final request should carry placeholders for all three secrets, found %d", got)
	}
}
