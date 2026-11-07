//go:build integration

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func TestLiveReferenceProducerReload(t *testing.T) {
	repo := repoRoot(t)
	t.Setenv("WORKBENCH_REFERENCE_DOCS", filepath.Join(repo, "internal/host/testdata/reference-producer/docs"))
	f := startRPC(t, t.TempDir(), "-e", filepath.Join(repo, "internal/host/testdata/reference-producer"))
	status := func(id string) map[string]any {
		f.send(id, "prompt", "/reference status")
		record := f.wait(func(record map[string]any) bool {
			return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"reference":"registered"`)
		})
		var result map[string]any
		if err := json.Unmarshal([]byte(record["message"].(string)), &result); err != nil {
			t.Fatal(err)
		}
		if result["lockHeld"] != false {
			t.Fatal("producer state lock held during a synchronous service request")
		}
		if result["kinds"] != float64(7) {
			t.Fatalf("unexpected registration count: %+v", result)
		}
		return result
	}
	first := status("before")
	for i := range 2 {
		f.command(fmt.Sprintf("reload-%d", i), "/reference reload")
		second := status(fmt.Sprintf("after-%d", i))
		if consumer := f.probe(fmt.Sprintf("consumer-%d", i)); consumer["generation"] != second["generation"] {
			t.Fatal("independent consumer discovered a different generation after reload")
		}
		if first["generation"] == second["generation"] {
			t.Fatal("reload reused a service generation")
		}
		oldReceipts := first["receipts"].(map[string]any)
		for kind, value := range second["receipts"].(map[string]any) {
			if value.(map[string]any)["registrationId"] == oldReceipts[kind].(map[string]any)["registrationId"] {
				t.Errorf("reload retained %s registration handle", kind)
			}
		}
		first = second
	}
}

// TestLiveReferenceProducerSeams co-loads a pig-web-shaped extension and drives every
// contribution seam through the real agent loop with a scripted provider.
func TestLiveReferenceProducerSeams(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKBENCH_REFERENCE_DOCS", filepath.Join(repo, "internal/host/testdata/reference-producer/docs"))
	steps := []struct {
		name string
		args map[string]any
	}{
		{"read", map[string]any{"path": "kb://sources/a.md:2-3"}},
		{"ls", map[string]any{"path": "kb://sources/"}},
		{"edit", map[string]any{"hashline": "[kb://sources/a.md#AAAA]\nPUT 2.=2:\n+changed"}},
		{"read", map[string]any{"path": "https://fixture.test/page"}},
		{"read", map[string]any{"path": "https://fixture.test/located"}},
		{"read", map[string]any{"path": "https://fixture.test/missing"}},
		{"read", map[string]any{"path": "docs://reference-producer/README.md"}},
		{"read", map[string]any{"path": "docs://workbench/schemes/kb"}},
		{"web_search", map[string]any{"query": "fixture"}},
		{"read", map[string]any{"path": "git://fixture/main.go:120-180"}},
		{"edit", map[string]any{"hashline": "[git://fixture/main.go#AAAA]\nPUT 120.=120:\n+changed"}},
	}
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		index := len(requests)
		requests = append(requests, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("r%d", index), "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index < len(steps) {
			args, _ := json.Marshal(steps[index].args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("r%d", index), "type": "function", "function": map[string]any{"name": steps[index].name, "arguments": string(args)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := runPigJSON(t, ctx, repo, root, work, []string{"-e", filepath.Join(repo, "internal/host/testdata/reference-producer")}, "exercise the seams")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != len(steps)+1 {
		t.Fatalf("expected %d provider calls, got %d\n%s", len(steps)+1, len(requests), output[max(0, len(output)-3000):])
	}
	toolResult := func(index int) string {
		messages, _ := requests[index]["messages"].([]any)
		last, _ := json.Marshal(messages[len(messages)-1])
		return string(last)
	}
	first, _ := json.Marshal(requests[0]["messages"])
	for _, want := range []string{"REFERENCE-NOTICE", "2025-06-01", "reference fixture"} {
		if !strings.Contains(string(first), want) {
			t.Errorf("first request lacks %q (frame notice or contributed model field):\n%s", want, first)
		}
	}
	claim := regexp.MustCompile(`\\u003csystem-info src=\\"reference\.research\\"[^>]*trust=\\"([a-z]+)\\" authority=\\"([a-z]+)\\"\\u003e\\n(REFERENCE-TRUSTED-CLAIM)`).FindStringSubmatch(string(first))
	if claim == nil {
		t.Errorf("trusted-claim frame did not reach the first request:\n%s", first)
	} else if claim[1] != "untrusted" || claim[2] != "data" {
		t.Errorf("producer raised its own trust: trust=%s authority=%s", claim[1], claim[2])
	}
	checks := []struct {
		index   int
		want    []string
		without []string
	}{
		{1, []string{"2:alpha two", "3:alpha three", "kb://sources/a.md"}, nil},
		{2, []string{"a.md", "nested", "page.html"}, nil},
		{3, []string{"kb://", "nothing was sent"}, nil},
		{4, []string{"Backend page", "Delivered by the reference fetch backend"}, []string{"<article>"}},
		{5, []string{"Located page", "knowledge base snapshot"}, nil},
		{6, []string{"404"}, []string{"Backend page"}},
		{7, []string{"registered through the Workbench docs contribution"}, nil},
		{8, []string{"Stored web sources", "browse stored sources", "reference-producer"}, nil},
		{9, []string{"Fixture result"}, nil},
		{10, []string{"120:git line 120", "180:git line 180", "git://fixture/main.go"}, []string{"git line 119", "git line 181"}},
		{11, []string{"git://", "nothing was sent"}, nil},
	}
	for _, check := range checks {
		result := toolResult(check.index)
		for _, want := range check.want {
			if !strings.Contains(result, want) {
				t.Errorf("step %d (%s) lacks %q:\n%s", check.index-1, steps[check.index-1].name, want, result)
			}
		}
		for _, unwanted := range check.without {
			if strings.Contains(result, unwanted) {
				t.Errorf("step %d (%s) unexpectedly contains %q:\n%s", check.index-1, steps[check.index-1].name, unwanted, result)
			}
		}
	}
	if t.Failed() && len(output) > 0 {
		t.Logf("pig output tail:\n%s", output[max(0, len(output)-3000):])
	}
}

// TestLiveReferenceProducerFrameRules drives the SPEC-0002 §8 publication rows
// through the service bus from an independent producer.
func TestLiveReferenceProducerFrameRules(t *testing.T) {
	repo := repoRoot(t)
	f := startRPC(t, t.TempDir(), "-e", filepath.Join(repo, "internal/host/testdata/reference-producer"))
	f.send("frames", "prompt", "/reference frames")
	record := f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"frames":"probed"`)
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(record["message"].(string)), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"duplicateSameFrame": true,
		"changedDuplicate":   "conflict",
		"followUp":           "permissionDenied",
		"undeclaredKind":     "permissionDenied",
		"statusState":        "pending",
		"statusErr":          true,
		"cancelState":        "cancelled",
		"cancelErr":          true,
		"cancelAgain":        "conflict",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %v, want %v (all: %v)", key, got[key], value, got)
		}
	}
	if code, _ := got["foreignSource"].(string); code == "" {
		t.Errorf("a source outside the producer prefix was accepted: %v", got)
	}
}

// TestLiveReferenceProducerLoadsBeforeWorkbench covers the SPEC-0002 §8 row that
// extension load order must not matter.
func TestLiveGrepWalksPaginatedProvider(t *testing.T) {
	results := runScriptedWith(t, t.TempDir(), "grep", "", []string{"reference-producer"},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "kb://paged/", "pattern": "paged", "literal": true, "glob": "*.md"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "kb://sources/", "pattern": "alpha two", "literal": true, "context": 1}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[0], "kb://paged/a.md:1:1: paged alpha") || !strings.Contains(results[0], "kb://paged/nested/b.md:1:1: paged bravo") || strings.Contains(results[0], "control") || strings.Contains(results[0], "incomplete") {
		t.Fatalf("provider grep missed paginated results or followed a noWalk entry: %v", results)
	}
	for _, want := range []string{"kb://sources/a.md-1- alpha one", "kb://sources/a.md:2:1: alpha two", "kb://sources/a.md-3- alpha three"} {
		if !strings.Contains(results[1], want) {
			t.Fatalf("provider grep omitted %q: %s", want, results[1])
		}
	}
}

func TestLiveFindWalksPaginatedProvider(t *testing.T) {
	results := runScriptedWith(t, t.TempDir(), "find", "", []string{"reference-producer"},
		func([]string) scriptedCall {
			return scriptedCall{"find", map[string]any{"path": "kb://paged/", "pattern": "*.md"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"find", map[string]any{"path": "kb://paged/", "pattern": "*.md", "limit": 1}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[0], "kb://paged/a.md") || !strings.Contains(results[0], "kb://paged/nested/b.md") || strings.Contains(results[0], "control") {
		t.Fatalf("provider walk failed: %v", results)
	}
	if !strings.Contains(results[1], "truncated") {
		t.Fatalf("provider result limit was not reported: %s", results[1])
	}
}

func TestLiveReferenceProducerLoadsBeforeWorkbench(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	cmd := exec.CommandContext(ctx, "pig", "--offline", "--session-dir", filepath.Join(root, "sessions"), "-e", filepath.Join(repo, "internal/host/testdata/reference-producer"), "-e", repo, "--mode", "rpc")
	cmd.Dir = root
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PIG_") && !strings.HasPrefix(v, "PI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+filepath.Join(root, "agent"), "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	f := newRPCFixture(t, cmd, ctx, cancel)
	f.send("status", "prompt", "/reference status")
	record := f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"reference":"registered"`)
	})
	if !strings.Contains(fmt.Sprint(record["message"]), `"kinds":6`) {
		t.Fatalf("producer loaded first did not register every kind: %v", record["message"])
	}
}

func TestLiveIntegrationsAndResourcesCommandsListTheContributions(t *testing.T) {
	repo := repoRoot(t)
	t.Setenv("WORKBENCH_REFERENCE_DOCS", filepath.Join(repo, "internal/host/testdata/reference-producer/docs"))
	f := startRPC(t, t.TempDir(), "-e", filepath.Join(repo, "internal/host/testdata/reference-producer"))
	notice := func(id, command, want string) string {
		f.send(id, "prompt", command)
		record := f.wait(func(record map[string]any) bool {
			return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), want)
		})
		return fmt.Sprint(record["message"])
	}
	all := notice("all", "/workbench integrations", "reference-producer")
	for _, kind := range []string{"frameProducer", "toolPresentation", "resourceProvider", "fetchBackend", "statusSegment", "docs"} {
		if !strings.Contains(all, kind) {
			t.Errorf("/workbench integrations lacks the %s registration:\n%s", kind, all)
		}
	}
	one := notice("one", "/workbench integrations reference-producer", `"schemes"`)
	if !strings.Contains(one, "kb") {
		t.Errorf("an owner view must show full descriptors:\n%s", one)
	}
	schemes := notice("schemes", "/workbench resources", "kb://")
	if !strings.Contains(schemes, "kb://  contributed by reference-producer") || !strings.Contains(schemes, "file://  builtin") {
		t.Errorf("/workbench resources must separate builtin and contributed schemes:\n%s", schemes)
	}
}

// TestLiveStoredSearchKeyIsRedactedInJobLogs covers the pig-web seam row: a search key held in the
// environment must not reach the provider through a job log, whichever tool reads it.
func TestLiveStoredSearchKeyIsRedactedInJobLogs(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()
	key := "sk-search-" + strings.Repeat("Qw4eR5tY", 4)
	t.Setenv("SEARCH_API_KEY", key)
	var id string
	var mu sync.Mutex
	var bodies []string
	steps := func(index int) (string, map[string]any) {
		switch index {
		case 0:
			return "bash", map[string]any{"command": "echo \"search key is $SEARCH_API_KEY\"; sleep 20", "background": true}
		case 1:
			return "jobs", map[string]any{"action": "output", "id": id}
		case 2:
			return "read", map[string]any{"path": "job://" + id + "/log"}
		}
		return "", nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"mock","object":"model"}]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		index := len(bodies)
		bodies = append(bodies, string(raw))
		if match := regexp.MustCompile(`Job ([0-9a-f]{32}): running`).FindStringSubmatch(string(raw)); match != nil {
			id = match[1]
		}
		name, args := steps(index)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, finish any) {
			data, _ := json.Marshal(map[string]any{"id": "r", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if name != "" {
			encoded, _ := json.Marshal(args)
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("c%d", index), "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "done"}, nil)
			send(map[string]any{}, "stop")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := runPigJSON(t, ctx, repo, root, root, []string{"-e", filepath.Join(repo, "internal/host/testdata/reference-producer"), "--tools", "bash,jobs,read"}, "run the search job")
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 4 {
		t.Fatalf("expected 4 provider requests, got %d\n%s", len(bodies), output[max(0, len(output)-2000):])
	}
	for i, body := range bodies {
		if strings.Contains(body, key) {
			t.Fatalf("request %d carries the stored search key", i)
		}
	}
	if !regexp.MustCompile(`search key is red-sec-[a-z2-7]{16}`).MatchString(strings.ReplaceAll(bodies[3], `\"`, `"`)) {
		t.Fatalf("the job log must show a placeholder where the key was:\n%.1500s", bodies[3])
	}
}
