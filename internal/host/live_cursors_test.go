//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

var listingCursorPattern = regexp.MustCompile(`cursor "([^"]+)"`)

func cursorOf(t *testing.T, text string) string {
	t.Helper()
	match := listingCursorPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		t.Fatalf("listing has no cursor: %s", text)
	}
	return match[1]
}

func TestLiveListingCursorRejectsOtherRootsAndChangedRevisions(t *testing.T) {
	revision := filepath.Join(t.TempDir(), "revision")
	if err := os.WriteFile(revision, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKBENCH_REFERENCE_CATALOG_REVISION", revision)
	results := runScriptedWith(t, t.TempDir(), "ls", "", []string{"reference-producer"},
		func([]string) scriptedCall { return scriptedCall{"ls", map[string]any{"path": "kb://paged/"}} },
		func(results []string) scriptedCall {
			return scriptedCall{"ls", map[string]any{"path": "kb://sources/", "cursor": cursorOf(t, results[0])}}
		},
		func(results []string) scriptedCall {
			return scriptedCall{"ls", map[string]any{"path": "kb://paged/", "cursor": cursorOf(t, results[0])}}
		},
		func(results []string) scriptedCall {
			if err := os.WriteFile(revision, []byte("second"), 0600); err != nil {
				t.Error(err)
			}
			return scriptedCall{"ls", map[string]any{"path": "kb://paged/", "cursor": cursorOf(t, results[0])}}
		},
	)
	if len(results) != 4 || !strings.Contains(results[1], "staleRevision") || !strings.Contains(results[2], "a.md") || !strings.Contains(results[3], "staleRevision") {
		t.Fatalf("cursor binding failed: %v", results)
	}
}

func TestLiveListingCursorExpiresAfterReload(t *testing.T) {
	root := t.TempDir()
	var saved atomic.Value
	recorder := &requestRecorder{next: func(index int) *scriptedCall {
		if index == 0 {
			return &scriptedCall{"ls", map[string]any{"path": "kb://paged/"}}
		}
		if index == 2 {
			return &scriptedCall{"ls", map[string]any{"path": "kb://paged/", "cursor": saved.Load().(string)}}
		}
		return nil
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock", "--tools", "ls")
	status := func(id string) string {
		fixture.send(id, "prompt", "/reference status")
		event := fixture.wait(func(e map[string]any) bool {
			return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), `"reference":"registered"`)
		})
		var state map[string]any
		if err := json.Unmarshal([]byte(event["message"].(string)), &state); err != nil {
			t.Fatal(err)
		}
		return state["generation"].(string)
	}
	before := status("before")
	fixture.send("first", "prompt", "List the first page.")
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
	recorder.mu.Lock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	recorder.mu.Unlock()
	saved.Store(cursorOf(t, results[len(results)-1]))
	fixture.command("reload", "/reference reload")
	if after := status("after"); after == before {
		t.Fatal("reload did not replace the generation")
	}
	fixture.send("second", "prompt", "Continue with the retained cursor.")
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results = toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if !strings.Contains(results[len(results)-1], "staleRevision") {
		t.Fatalf("cursor survived reload: %v", results)
	}
}
