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
	"slices"
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
	return runScriptedWith(t, root, tools, "", nil, steps...)
}

// runScriptedConfig is runScripted with extra Workbench.toml text appended.
func runScriptedConfig(t *testing.T, root, tools, config string, steps ...scriptStep) []string {
	t.Helper()
	return runScriptedWith(t, root, tools, config, nil, steps...)
}

// runScriptedWith also co-loads the named testdata extensions.
func runScriptedWith(t *testing.T, root, tools, config string, fixtures []string, steps ...scriptStep) []string {
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
	if !strings.HasPrefix(config, "[edit]\n") {
		config = "[edit]\nmode = 'auto'\n" + config
	}
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, fixtures, "--model", "workbench-fixture/mock", "--tools", tools)
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

func TestLivePromptSettingsControlTakeover(t *testing.T) {
	for _, tc := range []struct {
		name          string
		enabled       bool
		delivery      string
		wantWorkbench bool
	}{
		{"force", true, "force", true},
		{"disabled", false, "force", false},
		{"auto", true, "auto", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			recorder := &requestRecorder{}
			server := recorder.serve(t)
			defer server.Close()
			agent := filepath.Join(root, "agent")
			writeFixtureProvider(t, agent, server.URL)
			settings := fmt.Sprintf("[prompt]\nenabled = %t\ndelivery = '%s'\n", tc.enabled, tc.delivery)
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte(settings), 0600); err != nil {
				t.Fatal(err)
			}
			fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
			fixture.send("go", "prompt", "Test prompt delivery.")
			fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			if len(recorder.bodies) != 1 {
				t.Fatalf("got %d provider requests", len(recorder.bodies))
			}
			got := strings.Contains(recorder.bodies[0], "Workbench resources are reachable by URI")
			if got != tc.wantWorkbench {
				t.Fatalf("workbench prompt present=%v, want %v", got, tc.wantWorkbench)
			}
		})
	}
}

func TestLiveSummaryRespectsReadBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		maxLines int
		maxBytes int
	}{
		{"lines", 8, 51200},
		{"bytes", 2000, 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			var source strings.Builder
			source.WriteString("package fixture\n\n")
			for i := 0; i < 24; i++ {
				fmt.Fprintf(&source, "func f%d() {\n", i)
				for j := 0; j < 20; j++ {
					source.WriteString("println(1)\n")
				}
				source.WriteString("}\n")
			}
			if err := os.WriteFile(filepath.Join(root, "fixture.go"), []byte(source.String()), 0o600); err != nil {
				t.Fatal(err)
			}
			settings := fmt.Sprintf("[read]\nmax-lines = %d\nmax-bytes = %d\n", tc.maxLines, tc.maxBytes)
			results := runScriptedConfig(t, root, "read", settings, func([]string) scriptedCall {
				return scriptedCall{"read", map[string]any{"path": "fixture.go"}}
			})
			if got := results[0]; len(got) > tc.maxBytes || strings.Count(got, "\n")+1 > tc.maxLines {
				t.Fatalf("read exceeded configured budget: %d bytes, %d lines\n%s", len(got), strings.Count(got, "\n")+1, got)
			}
		})
	}
}

func TestLiveSchemeRedirectUsesControlWrite(t *testing.T) {
	results := runScripted(t, t.TempDir(), "bash,read",
		func([]string) scriptedCall {
			return scriptedCall{"bash", map[string]any{"command": "Updated > session://self/title"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"path": "session://self/title"}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[1], "Updated") {
		t.Fatalf("control redirect did not update the title: %v", results)
	}
}

func TestLiveControlWriteRejectsRevisionBeforeSideEffect(t *testing.T) {
	root := t.TempDir()
	jobID := ""
	results := runScripted(t, root, "bash,write,jobs",
		func([]string) scriptedCall {
			return scriptedCall{"bash", map[string]any{"command": "sleep 30", "background": true}}
		},
		func(prior []string) scriptedCall {
			match := regexp.MustCompile(`Job ([0-9a-f]{32}): running`).FindStringSubmatch(prior[0])
			if len(match) != 2 {
				t.Fatalf("background job did not start: %s", prior[0])
			}
			jobID = match[1]
			return scriptedCall{"write", map[string]any{"path": "job://" + jobID + "/ctl", "content": "kill", "expectedRevision": "stale"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"jobs", map[string]any{"action": "status", "id": jobID}}
		},
	)
	if !strings.Contains(results[1], "expectedRevision") || !strings.Contains(results[2], ": running") {
		t.Fatalf("precondition was ignored or the job was changed: %v", results)
	}
}

func TestLiveLocalReadCannotEscapeScratch(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	t.Setenv("XDG_CACHE_HOME", cache)
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("must-not-escape\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read",
		func([]string) scriptedCall {
			dir := filepath.Join(cache, "pig-workbench", "scratch")
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Errorf("scratch session missing: entries=%v err=%v", entries, err)
			} else {
				scratch := filepath.Join(dir, entries[0].Name())
				if err := os.Symlink(outside, filepath.Join(scratch, "escape")); err != nil {
					t.Errorf("create symlink: %v", err)
				}
				if err := os.WriteFile(filepath.Join(scratch, "inside"), []byte("safe scratch data\n"), 0600); err != nil {
					t.Errorf("create scratch file: %v", err)
				}
			}
			return scriptedCall{"read", map[string]any{"path": "local://escape"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"path": "local://inside"}}
		},
	)
	if len(results) != 2 || strings.Contains(results[0], "must-not-escape") || !strings.Contains(results[1], "safe scratch data") || strings.Contains(results[1], cache) {
		t.Fatalf("scratch confinement failed: %v", results)
	}
}

func TestLiveGrepEmitsRequestedContext(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "example.txt"), []byte("before\nneedle\nafter\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "grep",
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"pattern": "needle", "path": project, "context": 1}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"pattern": "needle", "path": project}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[0], "-1- before") || !strings.Contains(results[0], "-3- after") || !strings.Contains(results[0], ":2:1: needle") {
		t.Fatalf("requested context missing or confused with matches: %v", results)
	}
	if strings.Contains(results[1], "before") || strings.Contains(results[1], "after") {
		t.Fatalf("unrequested context was emitted: %s", results[1])
	}
}

func TestLiveGrepContextHonoursOutputBudget(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "many.txt"), []byte(strings.Repeat("needle "+strings.Repeat("x", 100)+"\n", 1000)), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "grep", func([]string) scriptedCall {
		return scriptedCall{"grep", map[string]any{"pattern": "needle", "path": project, "context": 2}}
	})
	if len(results) != 1 || len(results[0]) > 50<<10 || !strings.Contains(results[0], "results truncated") {
		t.Fatalf("context exceeded the budget or hid truncation: %v bytes, %v results", len(strings.Join(results, "")), len(results))
	}
}

func TestLiveGrepAcceptsLiteralPatternList(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("apple\npear\nplum\n"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"grep", map[string]any{"patterns": []any{"apple", "plum"}, "literal": true}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "grep")
	fixture.send("go", "prompt", "Search for the terms.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 2 {
		t.Fatalf("got %d provider requests, want 2", len(recorder.bodies))
	}
	results := toolResults(t, recorder.bodies[1])
	if len(results) != 1 || !strings.Contains(results[0], "apple") || !strings.Contains(results[0], "plum") || strings.Contains(results[0], "pear") {
		t.Fatalf("literal OR search returned wrong results: %v", results)
	}
	var request struct {
		Tools []struct {
			Function struct {
				Name       string `json:"name"`
				Parameters struct {
					Required []string `json:"required"`
					OneOf    []struct {
						Required []string `json:"required"`
						Not      struct {
							Required []string `json:"required"`
						} `json:"not"`
						Properties struct {
							Literal struct {
								Const *bool `json:"const"`
							} `json:"literal"`
						} `json:"properties"`
					} `json:"oneOf"`
				} `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(recorder.bodies[0]), &request); err != nil {
		t.Fatal(err)
	}
	for _, tool := range request.Tools {
		if tool.Function.Name != "grep" {
			continue
		}
		p := tool.Function.Parameters
		if slices.Contains(p.Required, "pattern") || len(p.OneOf) != 2 || !slices.Contains(p.OneOf[0].Required, "pattern") || !slices.Contains(p.OneOf[0].Not.Required, "patterns") || !slices.Contains(p.OneOf[1].Required, "patterns") || !slices.Contains(p.OneOf[1].Required, "literal") || !slices.Contains(p.OneOf[1].Not.Required, "pattern") || p.OneOf[1].Properties.Literal.Const == nil || !*p.OneOf[1].Properties.Literal.Const {
			t.Fatalf("the published schema does not offer both search forms: %+v", p)
		}
		return
	}
	t.Fatal("grep was not published to the provider")
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

func TestLiveEditModeRestrictsAcceptedSyntax(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		params map[string]any
	}{
		{"hashline", map[string]any{"path": "src.txt", "edits": []any{map[string]any{"oldText": "old", "newText": "new"}}}},
		{"replace", map[string]any{"hashline": "[src.txt#1234]\nPUT 1.=1:\n+new\n"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "src.txt")
			if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
				t.Fatal(err)
			}
			results := runScriptedConfig(t, root, "edit", "[edit]\nmode = '"+tc.mode+"'\n", func([]string) scriptedCall {
				return scriptedCall{"edit", tc.params}
			})
			if !strings.Contains(results[0], "edit.mode is "+tc.mode) || readFile(t, path) != "old\n" {
				t.Fatalf("mode restriction was not enforced: %s", results[0])
			}
		})
	}
}

func TestLiveEditSeenGuardCanBeDisabledByUser(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte(lines(60)), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScriptedConfig(t, root, "read,edit", "enforce-seen-lines = false\n",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "src.txt:1-5"}} },
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tagOf(t, prior[0]) + "]\nPUT 40.=40:\n+replaced\n"}}
		},
	)
	if got := readFile(t, path); !strings.Contains(got, "\nreplaced\n") {
		t.Fatalf("disabled seen guard did not allow the edit: %s", results[1])
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

func TestLiveSnapshotsViewLetsAnotherToolsOutputBeEditedWithoutARead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0600); err != nil {
		t.Fatal(err)
	}
	view := func(mode string) scriptStep {
		return func([]string) scriptedCall {
			return scriptedCall{"web_view", map[string]any{"uri": path, "mode": mode}}
		}
	}
	edit := func(index int) scriptStep {
		return func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[notes.txt#" + tagOf(t, prior[index]) + "]\nPUT 2.=2:\n+BETA\n"}}
		}
	}
	results := runScriptedWith(t, root, "web_view,edit", "", []string{"reference-producer"},
		view("verbatim"), edit(0),
		view("foreign"),
	)
	if got := readFile(t, path); got != "alpha\nBETA\ngamma\n" {
		t.Fatalf("anchors from a viewed block did not edit the file: %q\n%s", got, results[1])
	}
	if !strings.Contains(results[0], "retained=true") || !strings.Contains(results[0], "touchedStale=false") {
		t.Fatalf("status did not report the retained snapshot: %s", results[0])
	}
	if !strings.Contains(results[2], "permissionDenied") {
		t.Fatalf("a call the caller does not own was served: %s", results[2])
	}
}

func TestLiveSnapshotsViewGrantsNothingWhenTheBlockIsCut(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	original := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	runScriptedWith(t, root, "web_view,edit", "", []string{"reference-producer"},
		func([]string) scriptedCall {
			return scriptedCall{"web_view", map[string]any{"uri": path, "mode": "cut"}}
		},
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[notes.txt#" + tagOf(t, prior[0]) + "]\nPUT 2.=2:\n+BETA\n"}}
		},
	)
	if got := readFile(t, path); got != original {
		t.Fatalf("a cut block granted edit authority: %q", got)
	}
}

// requestRecorder serves a scripted provider and keeps every raw request body; calls are consumed
// in order, then next (if set) chooses further calls until it returns nil.
type requestRecorder struct {
	mu     sync.Mutex
	bodies []string
	calls  []scriptedCall
	next   func(index int) *scriptedCall
}

func (r *requestRecorder) serve(t *testing.T) *httptest.Server {
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
