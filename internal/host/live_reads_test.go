//go:build integration

package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
