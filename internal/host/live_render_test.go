//go:build integration

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLiveToolRowsShowTheirTargetsInTUI runs real tool calls in a TUI and reads the transcript the
// user sees (regression: multi-read and hashline edit rows rendered only as \"read ...\" and \"edit ...\").
func TestLiveToolRowsShowTheirTargetsInTUI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Justfile"), []byte("default:\n\techo hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := []scriptedCall{
		{"read", map[string]any{"path": "Justfile"}},
		{"read", map[string]any{"paths": []any{"Justfile", "Justfile:1-2"}}},
		{"write", map[string]any{"path": "new.txt", "content": "x\n"}},
		{"ls", map[string]any{"path": "."}},
		{"find", map[string]any{"pattern": "*.txt"}},
		{"cd", map[string]any{"path": "."}},
		{"edit", map[string]any{"hashline": "[Justfile#BEEF]\nPUT >$:\n+new\n"}},
		{"grep", map[string]any{"pattern": "echo", "path": "Justfile"}},
		{"bash", map[string]any{"command": "echo from-bash"}},
	}
	s := startTUIScripted(t, root, calls)
	s.typeText("exercise the tools")
	s.keys("Enter")
	s.waitFor("done", 40*time.Second)
	s.tmux("resize-window", "-t", "wb", "-x", "260", "-y", "200")
	time.Sleep(time.Second)
	history := s.history()
	for _, want := range []string{"read · Justfile", "read · Justfile, Justfile:1-2", "edit · Justfile", "edit · failed", "cd · .", "write · new.txt", "write · succeeded", "ls · .", "find · *.txt", "grep · echo · Justfile", "grep · succeeded", "bash · echo from-bash", "bash · succeeded", "tag BEEF is not a retained version"} {
		if !strings.Contains(history, want) {
			t.Fatalf("the transcript lacks %q:\n%s", want, history)
		}
	}
	for _, bad := range []string{"read ...", "edit ..."} {
		if strings.Contains(history, bad) {
			t.Fatalf("a call row has no target (%q):\n%s", bad, history)
		}
	}
	if dir := os.Getenv("WORKBENCH_CAPTURE_DIR"); dir != "" {
		target := filepath.Join(dir, "tool-rows")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, width := range []int{140, 80, 57, 36} {
			s.tmux("resize-window", "-t", "wb", "-x", strconv.Itoa(width), "-y", "200")
			time.Sleep(time.Second)
			if err := os.WriteFile(filepath.Join(target, fmt.Sprintf("w%d.txt", width)), []byte(s.history()), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
