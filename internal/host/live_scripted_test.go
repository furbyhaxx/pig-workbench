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
	"strconv"
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
	results, _ := runScriptedRequests(t, root, tools, config, fixtures, steps...)
	return results
}

// runScriptedRequests is runScriptedWith that also returns each provider request's messages as JSON.
func runScriptedRequests(t *testing.T, root, tools, config string, fixtures []string, steps ...scriptStep) ([]string, []string) {
	t.Helper()
	var mu sync.Mutex
	var latest, requests []string
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
		encoded, _ := json.Marshal(request.Messages)
		mu.Lock()
		latest = results
		requests = append(requests, string(encoded))
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
	return latest, requests
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
		{"auto", true, "auto", true},
		{"sections", true, "sections", false},
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

func TestLiveSchemeWriteInsideAComposedCommandIsRefusedBeforeTheShellRuns(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "shell-ran")
	results := runScripted(t, root, "bash", func([]string) scriptedCall {
		return scriptedCall{"bash", map[string]any{"command": "touch " + marker + `; echo kill > "job://example/ctl"`}}
	})
	if len(results) != 1 || !strings.Contains(results[0], "standalone") {
		t.Fatalf("composed scheme write was not refused: %v", results)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell executed despite refusal: %v", err)
	}
}

func TestLiveBashRunsCommandsThatMentionASchemeAndPassesSchemePathsAsFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("see agent://peer/inbox\nplain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "fetched-by-curl") }))
	defer web.Close()
	cases := []struct{ command, want string }{
		{`cat 'sess'ion://self/title`, "session://self/title"},
		{`curl -s ` + web.URL + `/page`, "fetched-by-curl"},
		{`grep -c "agent://" notes.txt`, "1"},
		{"cat <<'EOF'\nagent://peer/inbox and docs://workbench/\nEOF", "agent://peer/inbox and docs://workbench/"},
		{`sed -e 's#agent://#x://#' notes.txt`, "see x://peer/inbox"},
		{`P=agent; grep -c "$P://" notes.txt`, "1"},
		{`cat cfg://workbench/status | wc -c | grep -vq '^0$' && test -r cfg://workbench/status && echo piped-ok`, "piped-ok"},
		{`cat cfg://workbench/status | tee cfg://workbench/copy`, "standalone"},
	}
	steps := make([]scriptStep, len(cases))
	for i, c := range cases {
		steps[i] = func([]string) scriptedCall { return scriptedCall{"bash", map[string]any{"command": c.command}} }
	}
	results := runScripted(t, root, "bash", steps...)
	if len(results) != len(cases) {
		t.Fatalf("got %d results: %v", len(results), results)
	}
	for i, c := range cases {
		if !strings.Contains(results[i], c.want) || c.want != "standalone" && strings.Contains(results[i], "standalone") {
			t.Errorf("%q: %s", c.command, results[i])
		}
	}
}

func TestLiveBashSchemeArgumentsThatCannotBecomeFiles(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "ran")
	bash := func(command string) scriptStep {
		return func([]string) scriptedCall { return scriptedCall{"bash", map[string]any{"command": command}} }
	}
	results := runScripted(t, root, "bash",
		bash(`cat cfg://workbench/no-such-entry | cat; echo passed-through`),
		bash(`touch `+marker+`; ls -la cfg://workbench/integrations/`),
		bash(`head -c 120000 /dev/zero | tr '\0' a`),
		func(previous []string) scriptedCall {
			id := strings.TrimSuffix(strings.Fields(previous[len(previous)-1])[1], ":")
			return scriptedCall{"bash", map[string]any{"command": `touch ` + marker + `; wc -c job://` + id + `/log`}}
		},
	)
	if len(results) != 4 {
		t.Fatalf("got %d results: %v", len(results), results)
	}
	if !strings.Contains(results[0], "passed-through") || !strings.Contains(results[0], "cfg://workbench/no-such-entry") {
		t.Errorf("unresolvable word was not left to the shell: %s", results[0])
	}
	if !strings.Contains(results[1], "is a directory") || !strings.Contains(results[1], "standalone `ls cfg://workbench/integrations/`") {
		t.Errorf("directory argument: %s", results[1])
	}
	if !strings.Contains(results[3], "too large") {
		t.Errorf("over-limit argument: %.300s", results[3])
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("a refused command ran: %v", err)
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

func TestLiveFindWalksScratchWithoutFollowingLinks(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	t.Setenv("XDG_CACHE_HOME", cache)
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "outside.md"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "find",
		func([]string) scriptedCall {
			dir := filepath.Join(cache, "pig-workbench", "scratch")
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Errorf("scratch session missing: entries=%v err=%v", entries, err)
			} else {
				scratch := filepath.Join(dir, entries[0].Name())
				if err := os.Mkdir(filepath.Join(scratch, "nested"), 0700); err != nil {
					t.Errorf("create scratch directory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(scratch, "nested", "two words.md"), []byte("inside"), 0600); err != nil {
					t.Errorf("create scratch file: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(scratch, "escape")); err != nil {
					t.Errorf("create symlink: %v", err)
				}
			}
			return scriptedCall{"find", map[string]any{"path": "local:///", "pattern": "*.md"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"find", map[string]any{"path": "local:///escape", "pattern": "*.md"}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[0], "local:///nested/two%20words.md") || strings.Contains(strings.Join(results, ""), "outside.md") || !strings.Contains(results[1], "not a directory") {
		t.Fatalf("scratch find escaped or failed to return its URI: %v", results)
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

func TestLiveGrepKeepsMatchWhenContextIsOversized(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "context.txt"), []byte(strings.Repeat("x", 100000)+"\nneedle\nafter\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "grep", func([]string) scriptedCall {
		return scriptedCall{"grep", map[string]any{"pattern": "needle", "path": "context.txt", "context": 1}}
	})
	if len(results) != 1 || len(results[0]) > 50<<10 || !strings.Contains(results[0], ":2:1: needle") || !strings.Contains(results[0], "context truncated") {
		t.Fatalf("oversized context hid the match or its truncation: %v", results)
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
	if len(results) != 1 || len(results[0]) > 50<<10 || !strings.Contains(results[0], "results incomplete") {
		t.Fatalf("context exceeded the budget or hid truncation: %v bytes, %v results", len(strings.Join(results, "")), len(results))
	}
}

func TestLiveGrepReportsWindowAroundLongLineMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "long.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 50000)+"needle"+strings.Repeat("y", 50000)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "grep", func([]string) scriptedCall {
		return scriptedCall{"grep", map[string]any{"path": path, "pattern": "needle"}}
	})
	if len(results) != 1 || !strings.Contains(results[0], ":1:50001:") || !strings.Contains(results[0], "[line window from byte ") || !strings.Contains(results[0], "needle") || !strings.Contains(results[0], "results incomplete") || len(results[0]) > 48<<10 {
		t.Fatalf("long-line match was lost or misreported: %v", results)
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

func TestLiveTaggedBatchRequiresEveryTargetsInstructions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{"a.txt": "first\n", "nested/b.txt": "second\n", "nested/AGENTS.md": strings.Repeat("nested instruction\n", 250)} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	patch := func(results []string) scriptedCall {
		return scriptedCall{"edit", map[string]any{"hashline": "[a.txt#" + tagOf(t, results[0]) + "]\nPUT 1.=1:\n+changed first\n[nested/b.txt#" + tagOf(t, results[1]) + "]\nPUT 1.=1:\n+changed second\n"}}
	}
	results := runScripted(t, root, "read,edit",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "a.txt"}} },
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "nested/b.txt"}} },
		patch,
		func([]string) scriptedCall {
			for path, want := range map[string]string{"a.txt": "first\n", "nested/b.txt": "second\n"} {
				got, err := os.ReadFile(filepath.Join(root, path))
				if err != nil || string(got) != want {
					t.Errorf("blocked batch changed %s: %q %v", path, got, err)
				}
			}
			return scriptedCall{"read", map[string]any{"path": "nested/AGENTS.md"}}
		},
		patch,
	)
	if len(results) != 5 || !strings.Contains(results[2], "incompleteInstruction") || !strings.Contains(results[4], "changed") {
		t.Fatalf("tagged batch did not enforce nested instruction prerequisite: %v", results)
	}
	for path, want := range map[string]string{"a.txt": "changed first\n", "nested/b.txt": "changed second\n"} {
		got, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", path, got, err)
		}
	}
}

func TestLiveTaggedMoveRequiresDestinationInstructions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "AGENTS.md"), []byte(strings.Repeat("nested instruction\n", 250)), 0600); err != nil {
		t.Fatal(err)
	}
	patch := func(results []string) scriptedCall {
		return scriptedCall{"edit", map[string]any{"hashline": "[a.txt#" + tagOf(t, results[0]) + "]\nMV nested/new.txt\n"}}
	}
	results := runScripted(t, root, "read,edit",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "a.txt"}} },
		patch,
		func([]string) scriptedCall {
			if _, err := os.Stat(filepath.Join(root, "nested", "new.txt")); !os.IsNotExist(err) {
				t.Errorf("blocked move created destination: %v", err)
			}
			return scriptedCall{"read", map[string]any{"path": "nested/AGENTS.md"}}
		},
		patch,
	)
	if len(results) != 4 || !strings.Contains(results[1], "incompleteInstruction") || !strings.Contains(results[3], "nested/new.txt") {
		t.Fatalf("tagged move did not enforce destination instructions: %v", results)
	}
	got, err := os.ReadFile(filepath.Join(root, "nested", "new.txt"))
	if err != nil || string(got) != "first\n" {
		t.Fatalf("move did not complete after instructions were read: %q %v", got, err)
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

// TestLiveTreeRootsListAndScratchIsWritable pins the tree roots a real model reads first.
func TestLiveTreeRootsListAndScratchIsWritable(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, ".agents", "skills", "alpha")
	if err := os.MkdirAll(skill, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: alpha\ndescription: first skill\n---\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := []scriptedCall{
		{"read", map[string]any{"path": "session://self"}},
		{"read", map[string]any{"path": "session://"}},
		{"read", map[string]any{"path": "docs://workbench/"}},
		{"read", map[string]any{"path": "docs://"}},
		{"read", map[string]any{"path": "skill://"}},
		{"write", map[string]any{"path": "local://note.txt", "content": "scratch-ok"}},
		{"read", map[string]any{"path": "local://note.txt"}},
		{"read", map[string]any{"path": "local://"}},
		{"write", map[string]any{"path": "local://../escape.txt", "content": "x"}},
		{"read", map[string]any{"path": "job://"}},
		{"read", map[string]any{"path": "agent://"}},
		{"read", map[string]any{"path": "cfg://"}},
	}
	steps := make([]scriptStep, len(calls))
	for i, call := range calls {
		steps[i] = func([]string) scriptedCall { return call }
	}
	results := runScripted(t, root, "read,write", steps...)
	for i, want := range map[int]string{0: "transcript", 1: "by-id", 2: "schemes", 3: "workbench", 4: "alpha", 5: "local://note.txt", 6: "scratch-ok", 7: "note.txt"} {
		if !strings.Contains(results[i], want) || strings.Contains(results[i], "unsupported") || strings.Contains(results[i], "not found") {
			t.Errorf("step %d (%s) lacks %q: %s", i, calls[i].args["path"], want, results[i])
		}
	}
	if strings.Contains(results[8], "Wrote") {
		t.Errorf("scratch write escaped its root: %s", results[8])
	}
	for i := 9; i < len(results); i++ {
		if strings.Contains(results[i], "requires a") || strings.Contains(results[i], "unsupported") || strings.Contains(results[i], "not found") {
			t.Errorf("bare root %s failed: %s", calls[i].args["path"], results[i])
		}
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); err == nil {
		t.Error("scratch write created a file outside the scratch")
	}
}

// TestLiveForcedPromptCarriesHostInputs pins that a forced prompt is built from the host's real
// inputs; the first A/B run found it served an empty skeleton.
func TestLiveForcedPromptCarriesHostInputs(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, ".agents", "skills", "alpha")
	if err := os.MkdirAll(skill, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: alpha\ndescription: first skill\n---\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[prompt]\ndelivery = 'force'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--approve", "--append-system-prompt", "APPEND-MARKER")
	fixture.send("go", "prompt", "hello")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	body := recorder.bodies[0]
	for _, want := range []string{"Workbench resources are reachable by URI", "- read: ", "- edit: ", "tagged {hashline} batches", "APPEND-MARKER", "alpha", "first skill", "Be concise"} {
		if !strings.Contains(body, want) {
			t.Errorf("forced prompt lacks %q", want)
		}
	}
}

// TestLiveCompactionInvalidatesSeenSnapshots proves compaction cannot resurrect seen state:
// a tag delivered before /compact no longer authorizes an edit, and the file stays untouched.
func TestLiveCompactionInvalidatesSeenSnapshots(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte(lines(20)), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var tag, editResult string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			http.Error(w, "bad JSON", 400)
			return
		}
		last := request.Messages[len(request.Messages)-1]
		for i := len(request.Messages) - 1; i >= 0; i-- {
			last = request.Messages[i]
			if last["role"] != "user" || !strings.HasPrefix(strings.TrimSpace(toolMessageText(last)), "<system-info") {
				break
			}
		}
		text := strings.TrimSpace(toolMessageText(last))
		mu.Lock()
		call := scriptedCall{}
		switch {
		case last["role"] == "tool" && fileTagPattern.MatchString(text):
			tag = fileTagPattern.FindStringSubmatch(text)[1]
		case last["role"] == "tool":
			editResult = text
		case strings.HasSuffix(text, "read it"):
			call = scriptedCall{"read", map[string]any{"path": "src.txt"}}
		case strings.HasSuffix(text, "edit now"):
			call = scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tag + "]\nPUT 3.=3:\n+resurrected\n"}}
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": "compaction", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if call.tool != "" {
			encoded, _ := json.Marshal(call.args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "c-" + call.tool, "type": "function", "function": map[string]any{"name": call.tool, "arguments": string(encoded)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "ok"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,edit")
	for i, prompt := range []string{"hello", "read it", "again"} {
		f.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	}
	mu.Lock()
	if tag == "" {
		mu.Unlock()
		t.Fatal("the read never delivered a tag")
	}
	mu.Unlock()
	f.sendRaw("compact", "compact", nil)
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "compact" })
	f.send("edit", "prompt", "edit now")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	mu.Lock()
	defer mu.Unlock()
	if got := readFile(t, path); got != lines(20) {
		t.Fatalf("compaction resurrected seen state; file now:\n%s", got)
	}
	if editResult == "" || strings.Contains(editResult, "resurrected") {
		t.Fatalf("the edit after compaction was not refused: %q", editResult)
	}
}

// TestLiveCodemodeEditRefusesUnseenLinesLikeTheDirectTool proves direct and codemode edits share one safety path.
func TestLiveCodemodeEditRefusesUnseenLinesLikeTheDirectTool(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte(lines(60)), 0600); err != nil {
		t.Fatal(err)
	}
	editCode := func(tag string) string {
		patch := "[src.txt#" + tag + "]\nPUT 40.=40:\n+replaced\n"
		return "try { text(await tools.edit({hashline: " + strconv.Quote(patch) + "})); } catch (e) { text('REFUSED: ' + e.message); }"
	}
	results := runScripted(t, root, "read,edit,codemode",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "src.txt:1-5"}} },
		func(prior []string) scriptedCall {
			return scriptedCall{"edit", map[string]any{"hashline": "[src.txt#" + tagOf(t, prior[0]) + "]\nPUT 40.=40:\n+replaced\n"}}
		},
		func(prior []string) scriptedCall {
			return scriptedCall{"codemode", map[string]any{"code": editCode(tagOf(t, prior[0]))}}
		},
	)
	if got := readFile(t, path); got != lines(60) {
		t.Fatalf("an unseen line was edited:\n%s", got[:200])
	}
	for name, result := range map[string]string{"direct": results[1], "codemode": results[2]} {
		if !strings.Contains(result, "not seen") || !strings.Contains(result, "read them before editing") {
			t.Errorf("%s refusal differs: %s", name, result)
		}
	}
}

// TestLiveWriteNeverInstallsATruncatedReadCopy proves a copied partial read cannot replace the file.
func TestLiveWriteNeverInstallsATruncatedReadCopy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src.txt")
	if err := os.WriteFile(path, []byte(lines(60)), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read,write",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "src.txt:1-5"}} },
		func(prior []string) scriptedCall {
			return scriptedCall{"write", map[string]any{"path": "src.txt", "content": prior[0]}}
		},
	)
	if got := readFile(t, path); got != lines(60) {
		t.Fatalf("a truncated read copy replaced the file:\n%s", got)
	}
	if !strings.Contains(results[1], "complete") {
		t.Fatalf("refusal does not name the complete-source requirement: %s", results[1])
	}
}

// TestLiveDisabledFetchBackendFallsBackToTheBuiltinReader covers fetch.disabled-backends:
// the same read is served by the contributed backend unless the user disabled it.
func TestLiveDisabledFetchBackendFallsBackToTheBuiltinReader(t *testing.T) {
	read := func(results []string) scriptedCall {
		return scriptedCall{tool: "read", args: map[string]any{"path": "https://fixture.test/page"}}
	}
	const marker = "Delivered by the reference fetch backend"
	on := runScriptedWith(t, t.TempDir(), "read", "", []string{"reference-producer"}, read)
	if !strings.Contains(on[0], marker) {
		t.Fatalf("backend did not serve the URL while enabled: %s", on[0])
	}
	off := runScriptedWith(t, t.TempDir(), "read", "[fetch]\ndisabled-backends = ['reference-producer/web']\n", []string{"reference-producer"}, read)
	if strings.Contains(off[0], marker) {
		t.Fatalf("disabled backend still served the URL: %s", off[0])
	}
}

// TestLiveAutoLoadSkillIsDeliveredOnceAndStaysDeclinedAfterUnload covers SPEC-0014 §8 auto_load.
func TestLiveAutoLoadSkillIsDeliveredOnceAndStaysDeclinedAfterUnload(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: alpha\ndescription: sql helper\nauto_load:\n  touched: [\"*.sql\"]\n---\nAUTO-SKILL-BODY\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.sql"), []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readSQL := func([]string) scriptedCall { return scriptedCall{tool: "read", args: map[string]any{"path": "a.sql"}} }
	unload := func([]string) scriptedCall {
		return scriptedCall{tool: "write", args: map[string]any{"path": "skill://alpha/ctl", "content": "unload"}}
	}
	_, requests := runScriptedRequests(t, root, "read,write", "", nil, readSQL, readSQL, unload, readSQL)
	const marker = "skill alpha auto-loaded"
	if strings.Contains(requests[0], marker) {
		t.Fatal("skill was delivered before any file was touched")
	}
	if !strings.Contains(requests[1], marker) || !strings.Contains(requests[1], "AUTO-SKILL-BODY") {
		t.Fatalf("touching a matching file did not deliver the skill:\n%s", requests[1])
	}
	if got := strings.Count(requests[len(requests)-1], marker); got != 1 {
		t.Fatalf("skill delivered %d times across repeated touches and an unload; want exactly once", got)
	}
}

// TestLiveSkillsListingFrameMovesTheListingOutOfThePrompt covers skills.listing: with frame the
// Workbench prompt has no <skills> section and the listing rides the capabilities frame.
func TestLiveSkillsListingFrameMovesTheListingOutOfThePrompt(t *testing.T) {
	setup := func(t *testing.T) string {
		root := t.TempDir()
		dir := filepath.Join(root, ".agents", "skills", "alpha")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: alpha\ndescription: routing text\n---\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}
	read := func([]string) scriptedCall {
		return scriptedCall{tool: "read", args: map[string]any{"path": "skill://alpha"}}
	}
	run := func(listing string) string {
		_, requests := runScriptedRequests(t, setup(t), "read", "[prompt]\ndelivery = 'force'\n[skills]\nlisting = '"+listing+"'\n", nil, read)
		return strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\"`, `"`).Replace(requests[0])
	}
	prompt := run("prompt")
	if !strings.Contains(prompt, "<skills>") || strings.Contains(prompt, `"location": "skill://alpha"`) {
		t.Fatalf("prompt listing must keep <skills> and leave the frame without a listing:\n%s", prompt)
	}
	frame := run("frame")
	if strings.Contains(frame, "<skills>") || !strings.Contains(frame, `"location": "skill://alpha"`) || !strings.Contains(frame, "routing text") {
		t.Fatalf("frame listing must drop <skills> and carry the skill in session.capabilities:\n%s", frame)
	}
}
