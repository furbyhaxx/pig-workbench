//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furbyhaxx/pig-workbench/api"
)

func TestLiveIncompleteSingleReadNamesItsSelectorAndNeverSaysBatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(lines(5000)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "short.txt"), []byte(lines(3)), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "long.txt"}} },
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"paths": []any{"short.txt", "missing.txt"}}}
		},
	)
	if len(results) != 2 || strings.Contains(results[0], "Batch incomplete") || !strings.Contains(results[0], "continue with selector :") {
		t.Fatalf("single incomplete read: %.300q", lastChars(results, 0))
	}
	if !strings.Contains(results[1], "Batch incomplete") {
		t.Fatalf("an incomplete batch lost its notice: %.300q", lastChars(results, 1))
	}
}

func lastChars(results []string, i int) string {
	if i >= len(results) {
		return "(no result)"
	}
	return results[i][max(0, len(results[i])-300):]
}

func TestLiveCompleteReadProjectsOlderPartialWithoutChangingHistory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte(lines(20)), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "source.txt:1-2"}} },
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "source.txt"}} },
	)
	if len(results) != 2 || !strings.Contains(results[0], "superseded by read scripted-1") || strings.Contains(results[0], "1:line 1") || !strings.Contains(results[1], "20:line 20") {
		t.Fatalf("projection did not reach actual provider context: %v", results)
	}
	files, err := filepath.Glob(filepath.Join(root, "sessions", "**", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	}
	if len(files) == 0 {
		t.Fatal("no persisted history to verify")
	}
	var history strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		history.Write(data)
	}
	if strings.Contains(history.String(), "superseded by read") || !strings.Contains(history.String(), "1:line 1") {
		t.Fatal("context projection changed persisted history")
	}
}

func TestLiveBatchedProviderReadsKeepCapturedProjectAcrossCd(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".project"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".project", "Project.toml"), []byte("version=1\nid='prj-capture1'\nslug='capture'\nname='Capture'\n[component]\nslug='capture'\nprimary=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gate := filepath.Join(t.TempDir(), "provider")
	t.Setenv("WORKBENCH_REFERENCE_PROJECT_BARRIER", gate)
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"paths": []string{"kb://sources/a.md", "kb://sources/nested/b.md"}}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock", "--tools", "read")
	defer func() { _ = os.WriteFile(gate+".release", []byte("done"), 0600) }()
	fixture.send("read", "prompt", "Read both resources.")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(gate + ".entered"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("read never reached provider barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fixture.send("move", "prompt", "/cd "+t.TempDir())
	response := fixture.wait(func(e map[string]any) bool { return e["type"] == "response" && e["id"] == "move" })
	if response["success"] != true {
		t.Fatalf("concurrent cd failed: %+v", response)
	}
	if err := os.WriteFile(gate+".release", []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
	encoded, err := os.ReadFile(gate + ".requests")
	if err != nil {
		t.Fatal(err)
	}
	requests := strings.Split(strings.TrimSpace(string(encoded)), "\n")
	if len(requests) != 2 {
		t.Fatalf("want two provider reads: %s", encoded)
	}
	for _, line := range requests {
		var request api.ResourceRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatal(err)
		}
		if request.Project == nil || request.Project.ID != "prj-capture1" {
			t.Fatalf("request drifted to the new project during cd: %+v", request)
		}
	}
}

func TestLiveGrepFilesOnlyEmitsPathsWithoutSourceText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("needle private source text\nneedle repeated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScriptedWith(t, root, "grep", "", []string{"reference-producer"},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "source.txt", "pattern": "needle", "filesOnly": true, "context": 2}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "kb://sources/a.md", "pattern": "alpha", "filesOnly": true}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[0], "source.txt") || strings.Contains(results[0], "private source text") || strings.Contains(results[0], ":1:") || strings.Count(results[1], "kb://sources/a.md") != 1 || strings.Contains(results[1], "alpha") {
		t.Fatalf("filesOnly leaked source or coordinates: %v", results)
	}
}

func TestLiveCaseInsensitiveLiteralGrepKeepsSourceByteColumn(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "unicode.txt"), []byte("İ NEEDLE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "grep", func([]string) scriptedCall {
		return scriptedCall{"grep", map[string]any{"path": "unicode.txt", "pattern": "needle", "literal": true, "ignoreCase": true}}
	})
	if len(results) != 1 || !strings.Contains(results[0], ":1:4: İ NEEDLE") {
		t.Fatalf("literal grep did not retain original byte column: %v", results)
	}
}

func TestLiveSearchGlobDialectAcrossLocalAndProviderRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"src/a.go": "needle go\n", "src/nested/b.md": "needle markdown\n", "src/nested/c.txt": "needle excluded\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	results := runScriptedWith(t, root, "grep,find", "", []string{"reference-producer"},
		func([]string) scriptedCall {
			return scriptedCall{"find", map[string]any{"path": "src", "pattern": "**/*.{go,md}"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "src", "pattern": "needle", "glob": "**/[ab].{go,md}"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"find", map[string]any{"path": "kb://sources/", "pattern": "**/*.{go,md}"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "kb://sources/", "pattern": "alpha|bravo", "glob": "**/*.{go,md}"}}
		},
	)
	if len(results) != 4 || !strings.Contains(results[0], "a.go") || !strings.Contains(results[0], "b.md") || strings.Contains(results[0], "c.txt") || !strings.Contains(results[1], "needle go") || !strings.Contains(results[1], "needle markdown") || strings.Contains(results[1], "needle excluded") || !strings.Contains(results[2], "kb://sources/nested/b.md") || !strings.Contains(results[3], "bravo") || !strings.Contains(results[3], "alpha") {
		t.Fatalf("glob dialect mismatch: %v", results)
	}
}

func TestLiveSearchHonoursGitignoreAndExplicitFileTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "ignored"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{".gitignore": "ignored/\n*.log\n!keep.log\n", "keep.log": "needle kept\n", "hidden.log": "needle hidden\n", "ignored/secret.txt": "needle ignored\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	results := runScripted(t, root, "grep,find",
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": ".", "pattern": "needle"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"find", map[string]any{"path": ".", "pattern": "*.log"}}
		},
		func([]string) scriptedCall {
			return scriptedCall{"grep", map[string]any{"path": "hidden.log", "pattern": "needle"}}
		},
	)
	if len(results) != 3 || !strings.Contains(results[0], "needle kept") || strings.Contains(results[0], "needle hidden") || strings.Contains(results[0], "needle ignored") || !strings.Contains(results[1], "keep.log") || strings.Contains(results[1], "hidden.log") || !strings.Contains(results[2], "needle hidden") {
		t.Fatalf("live ignore/explicit semantics: %v", results)
	}
}

func TestLiveProviderGuidesAndContinuationRespectReadBudgets(t *testing.T) {
	for _, lines := range []int{5, 20} {
		t.Run(fmt.Sprint(lines), func(t *testing.T) {
			t.Setenv("WORKBENCH_REFERENCE_LONG_GUIDE", "1")
			t.Setenv("WORKBENCH_REFERENCE_INCOMPLETE_PAGE", "1")
			results := runScriptedWith(t, t.TempDir(), "read", fmt.Sprintf("[read]\nmax-bytes=1024\nmax-lines=%d\n", lines), []string{"reference-producer"}, func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "kb://sources/a.md"}} })
			if len(results) != 1 || len(results[0]) > 1024 || strings.Count(results[0], "\n")+1 > lines || !strings.Contains(results[0], "alpha one") || !strings.Contains(results[0], "docs://workbench/schemes/kb") || !strings.Contains(results[0], "continue") {
				t.Fatalf("unbounded guide or lost source/continuation: %v", results)
			}
		})
	}
}

func TestLiveReadRepetitionThresholdControlsHints(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("retained source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var steps []scriptStep
	for range 2 {
		steps = append(steps, func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "source.txt"}} })
	}
	results := runScriptedConfig(t, root, "read", "[read]\nloop-hint-after=2\n", steps...)
	if len(results) != 2 || strings.Contains(results[0], "Repeated read") || !strings.Contains(results[1], "Repeated read") || !strings.Contains(results[1], "retained source") {
		t.Fatalf("configured repetition threshold not applied: %v", results)
	}
}

func TestLiveRepetitionHintStaysWithinReadBudget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(strings.Repeat("source line with enough text to consume the page\n", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	var steps []scriptStep
	for range 3 {
		steps = append(steps, func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "large.txt"}} })
	}
	results := runScriptedConfig(t, root, "read", "[read]\nmax-bytes=1024\nmax-lines=20\n", steps...)
	if len(results) != 3 || !strings.Contains(results[2], "Repeated read") {
		t.Fatalf("hint missing: %v", results)
	}
	for _, result := range results {
		if len(result) > 1024 || strings.Count(result, "\n") > 20 || !strings.Contains(result, "source line") {
			t.Fatalf("hint lost source or exceeded its budget: %q", result)
		}
	}
}

func TestLiveRepeatedReadHintPreservesSourceAndResetsOnRevision(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("original source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	results := runScripted(t, root, "read",
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "source.txt"}} },
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": path}} },
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "source.txt:1-"}} },
		func([]string) scriptedCall {
			if err := os.WriteFile(path, []byte("changed source\n"), 0600); err != nil {
				t.Error(err)
			}
			return scriptedCall{"read", map[string]any{"path": "source.txt"}}
		},
	)
	if len(results) != 4 {
		t.Fatalf("results: %v", results)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(results[i], "original source") || strings.Contains(results[i], "Repeated read") != (i == 2) {
			t.Fatalf("read %d: %q", i, results[i])
		}
	}
	if !strings.Contains(results[3], "changed source") || strings.Contains(results[3], "Repeated read") {
		t.Fatalf("changed revision inherited hint: %q", results[3])
	}
}

func startSlowProviderRead(t *testing.T, env map[string]string) (*rpcFixture, string) {
	t.Helper()
	gate := filepath.Join(t.TempDir(), "slow")
	t.Setenv("WORKBENCH_REFERENCE_CANCEL_PROBE", gate)
	for key, value := range env {
		t.Setenv(key, value)
	}
	root := t.TempDir()
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "kb://slow/probe"}}}}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("read", "prompt", "Read the slow resource.")
	return fixture, gate
}

func waitForFile(t *testing.T, path string, within time.Duration, what string) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within %s", what, within)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLiveProviderHandlerObservesTheRequestDeadline(t *testing.T) {
	fixture, gate := startSlowProviderRead(t, map[string]string{"WORKBENCH_REFERENCE_QUERY_TIMEOUT_MS": "700"})
	waitForFile(t, gate+".entered", 10*time.Second, "provider entry")
	if got := waitForFile(t, gate+".stopped", 8*time.Second, "cooperative stop"); !strings.Contains(got, "deadline") {
		t.Fatalf("handler stopped for the wrong reason: %q", got)
	}
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
}

func TestLiveAbortedReadCancelsTheCooperativeProvider(t *testing.T) {
	fixture, gate := startSlowProviderRead(t, nil)
	waitForFile(t, gate+".entered", 10*time.Second, "provider entry")
	fixture.sendRaw("abort", "abort", nil)
	if got := waitForFile(t, gate+".stopped", 8*time.Second, "cooperative stop"); !strings.Contains(got, "cancel") {
		t.Fatalf("handler stopped for the wrong reason: %q", got)
	}
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
}

func TestLiveUnregisterStopsAndDrainsARunningProviderRead(t *testing.T) {
	fixture, gate := startSlowProviderRead(t, nil)
	waitForFile(t, gate+".entered", 10*time.Second, "provider entry")
	started := time.Now()
	fixture.send("withdraw", "prompt", "/reference withdraw-kb")
	fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), "kbFixture")
	})
	if took := time.Since(started); took > 4*time.Second {
		t.Fatalf("withdrawal waited %s instead of draining on the cooperative stop", took)
	}
	if got := waitForFile(t, gate+".stopped", 4*time.Second, "cooperative stop"); !strings.Contains(got, "cancel") {
		t.Fatalf("handler stopped for the wrong reason: %q", got)
	}
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
}

func TestLiveProviderProgressShowsInTheResourcesInspectorWhileRunning(t *testing.T) {
	fixture, gate := startSlowProviderRead(t, nil)
	waitForFile(t, gate+".entered", 10*time.Second, "provider entry")
	if _, err := os.Stat(gate + ".progress-error"); err == nil {
		t.Fatal("provider could not send progress")
	}
	fixture.send("inspect", "prompt", "/workbench resources kb")
	notice := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), "kb://")
	})
	message := fmt.Sprint(notice["message"])
	if !strings.Contains(message, "running 1: scanning index (3/10)") {
		t.Fatalf("inspector did not show live progress: %s", message)
	}
	fixture.sendRaw("abort", "abort", nil)
	waitForFile(t, gate+".stopped", 8*time.Second, "cooperative stop")
	fixture.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
	fixture.send("after", "prompt", "/workbench resources kb")
	idle := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), "kb://") && !strings.Contains(fmt.Sprint(e["message"]), "scanning")
	})
	if strings.Contains(fmt.Sprint(idle["message"]), "running") {
		t.Fatalf("finished operation still reported as running: %v", idle["message"])
	}
}
