//go:build integration

package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveToolsRecoverAfterTheWorkingLocationIsDeleted(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(tool string, args map[string]any) scriptStep {
		return func([]string) scriptedCall { return scriptedCall{tool, args} }
	}
	results := runScripted(t, root, "cd,bash,jobs,ls,read",
		call("cd", map[string]any{"path": "gone"}),
		call("bash", map[string]any{"command": "rmdir " + filepath.Join(root, "gone")}),
		call("jobs", map[string]any{"action": "list"}),
		call("bash", map[string]any{"command": "echo still-here"}),
		call("cd", map[string]any{"reset": true}),
		call("read", map[string]any{"path": "kept.txt"}),
	)
	if len(results) != 6 {
		t.Fatalf("got %d results: %q", len(results), results)
	}
	for i, result := range results {
		if strings.Contains(result, "resolve detection directory") {
			t.Fatalf("step %d failed in project detection: %s", i, result)
		}
	}
	if !strings.Contains(results[3], "gone") || strings.Contains(results[3], "still-here") || !strings.Contains(results[3], "no longer exists") {
		t.Errorf("bash in a deleted location should name it: %s", results[3])
	}
	if !strings.Contains(results[4], root) || !strings.Contains(results[5], "kept") {
		t.Errorf("cd reset did not restore a usable location: %q %q", results[4], results[5])
	}
}
