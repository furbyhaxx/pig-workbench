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
)

type scriptedCall struct {
	tool string
	args map[string]any
}

// scriptStep chooses the next tool call from the tool results seen so far.
type scriptStep func(results []string) scriptedCall

var fileTagPattern = regexp.MustCompile(`#([A-F0-9]{4})\]`)

// tagOf extracts the first tagged-read header tag from a tool result.
func tagOf(t *testing.T, result string) string {
	t.Helper()
	match := fileTagPattern.FindStringSubmatch(result)
	if match == nil {
		t.Fatalf("result carries no tag:\n%s", result)
	}
	return match[1]
}

func toolMessageText(message map[string]any) string {
	switch content := message["content"].(type) {
	case string:
		return content
	case []any:
		var b strings.Builder
		for _, part := range content {
			if object, ok := part.(map[string]any); ok {
				if text, ok := object["text"].(string); ok {
					b.WriteString(text)
				}
			}
		}
		return b.String()
	}
	return ""
}

// runScripted drives the real agent loop against a scripted provider and returns every tool result.
func runScripted(t *testing.T, root, tools string, steps ...scriptStep) []string {
	t.Helper()
	return runScriptedConfig(t, root, tools, "", steps...)
}

// runScriptedConfig is runScripted with extra Workbench.toml text appended.
func runScriptedConfig(t *testing.T, root, tools, config string, steps ...scriptStep) []string {
	t.Helper()
	var mu sync.Mutex
	var latest []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad JSON", 400)
			return
		}
		var results []string
		for _, message := range request.Messages {
			if message["role"] == "tool" {
				results = append(results, toolMessageText(message))
			}
		}
		mu.Lock()
		latest = results
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": "scripted", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index := len(results); index < len(steps) {
			call := steps[index](results)
			encoded, _ := json.Marshal(call.args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("scripted-%d", index), "type": "function", "function": map[string]any{"name": call.tool, "arguments": string(encoded)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[edit]\nmode = 'auto'\n"+config), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", tools)
	fixture.send("script", "prompt", "Run the scripted fixture.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	mu.Lock()
	defer mu.Unlock()
	if len(latest) != len(steps) {
		t.Fatalf("script ran %d of %d steps; results: %q", len(latest), len(steps), latest)
	}
	return latest
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func lines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func TestLiveEditRefusesLinesTheModelNeverSaw(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte(lines(60)), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read,edit",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "src.txt:1-5"}} },
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tagOf(t, prior[0]) + "]\nPUT 40.=40:\n+replaced\n"}}
		},
	)
	if got := readFile(t, path); got != lines(60) {
		t.Fatalf("an unseen line was edited:\n%s", got[:200])
	}
	if !strings.Contains(results[1], "not seen") || !strings.Contains(results[1], "read them before editing") {
		t.Fatalf("refusal does not name the unseen lines and the next action: %s", results[1])
	}
}

func TestLiveEditRelocatesOnlyWhenTheTargetIsUnambiguous(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0600); err != nil {
		t.Fatal(err)
	}
	external := "alpha\nBETA CHANGED OUTSIDE\ngamma\n"
	runScripted(t, root, "read,edit",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "src.txt"}} },
		func(prior []string) scriptedCall {
			if err := os.WriteFile(path, []byte(external), 0600); err != nil {
				t.Errorf("external write: %v", err)
			}
			return scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tagOf(t, prior[0]) + "]\nPUT 2.=2:\n+mine\n"}}
		},
	)
	if got := readFile(t, path); got != external {
		t.Fatalf("an edit whose target changed underneath it was applied anyway: %q", got)
	}
}

func TestLiveEditAnOlderTagRecoversAgainstTheNewestSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runScripted(t, root, "read,edit",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "src.txt"}} },
		func([]string) scriptedCall {
			if err := os.WriteFile(path, []byte("zero\none\ntwo\nthree\n"), 0600); err != nil {
				t.Errorf("external write: %v", err)
			}
			return scriptedCall{"read", map[string]any{"path": "src.txt"}}
		},
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tagOf(t, prior[0]) + "]\nPUT 3.=3:\n+THREE\n"}}
		},
	)
	if got := readFile(t, path); got != "zero\none\ntwo\nTHREE\n" {
		t.Fatalf("older tag did not recover onto the newest source: %q", got)
	}
}

func TestLiveBlockCutPastesIntoAnotherFileThroughARegister(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.go")
	b := filepath.Join(root, "b.go")
	if err := os.WriteFile(a, []byte("package a\n\nfunc helper() {\n\tx()\n}\n\nfunc other() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("package b\n\nfunc use() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read,edit",
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"paths": []any{"a.go", "b.go"}}}
		},
		func(prior []string) scriptedCall {
			tags := fileTagPattern.FindAllStringSubmatch(prior[0], -1)
			if len(tags) < 2 {
				t.Fatalf("multi-read returned %d tagged files:\n%s", len(tags), prior[0])
			}
			return scriptedCall{"edit", map[string]any{"hashline": "[a.go#" + tags[0][1] + "]\nCUT 3* @helper\n[b.go#" + tags[1][1] + "]\nPUT <3 @helper\n"}}
		},
	)
	if got := readFile(t, a); got != "package a\n\n\nfunc other() {}\n" {
		t.Errorf("a.go = %q\nresult: %s", got, results[1])
	}
	if got := readFile(t, b); got != "package b\n\nfunc helper() {\n\tx()\n}\nfunc use() {}\n" {
		t.Errorf("b.go = %q\nresult: %s", got, results[1])
	}
}

func TestLiveEditRefusesALineTheReadBudgetCutInHalf(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	original := "short\n" + strings.Repeat("x", 2000) + "\nlast\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScriptedConfig(t, root, "read,edit", "\n[read]\nmax-bytes = 1024\n",
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"path": "src.txt"}}
		},
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tagOf(t, prior[0]) + "]\nPUT 2.=2:\n+replaced\n"}}
		},
	)
	if got := readFile(t, path); got != original {
		t.Fatalf("a partially delivered line was edited: %q\n%s", got[:40], results[1])
	}
	if !strings.Contains(results[1], "not seen") {
		t.Fatalf("refusal does not name the unseen line: %s", results[1])
	}
}

func TestLiveBlockReplaceUsesSyntaxBoundariesAndRefusesAnUnanchoredLine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	original := "package a\n\nfunc helper() {\n\tx()\n}\n\nfunc other() {}\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read,edit",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "a.go"}} },
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[a.go#" + tagOf(t, prior[0]) + "]\nPUT 3*:\n+func helper() { y() }\n"}}
		},
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[a.go#" + tagOf(t, prior[0]) + "]\nPUT 1*:\n+package nope\n"}}
		},
	)
	if got := readFile(t, path); got != "package a\n\nfunc helper() { y() }\n\nfunc other() {}\n" {
		t.Fatalf("block replace did not cover exactly the function: %q\nfirst: %s\nsecond: %s", got, results[1], results[2])
	}
	if !strings.Contains(strings.ToLower(results[2]), "block") {
		t.Fatalf("a non-block anchor was not refused with a block explanation: %s", results[2])
	}
}
