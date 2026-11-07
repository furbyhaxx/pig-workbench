//go:build integration

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
