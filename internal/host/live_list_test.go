//go:build integration

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveListingOutputIsBoundedAndHiddenPolicyIsExplicit(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"big", "small"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 600; i++ {
		if err := os.WriteFile(filepath.Join(root, "big", fmt.Sprintf("item-%04d-%s.txt", i, strings.Repeat("a", 120))), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"visible.txt", ".hidden.txt"} {
		if err := os.WriteFile(filepath.Join(root, "small", name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	results := runScripted(t, root, "ls",
		func([]string) scriptedCall { return scriptedCall{"ls", map[string]any{"path": "big", "limit": 2000}} },
		func([]string) scriptedCall {
			return scriptedCall{"ls", map[string]any{"path": "small", "includeHidden": false}}
		},
		func([]string) scriptedCall { return scriptedCall{"ls", map[string]any{"path": "small"}} },
	)
	if len(results) != 3 || len(results[0]) > 50<<10 || strings.Count(results[0], "\n") > 2000 || !strings.Contains(results[0], "more entries") {
		t.Fatalf("listing exceeded bounds or claimed false completion: bytes=%d", len(results[0]))
	}
	if !strings.Contains(results[1], "visible.txt") || strings.Contains(results[1], ".hidden.txt") || !strings.Contains(results[2], ".hidden.txt") {
		t.Fatalf("hidden policy not applied: %v", results[1:])
	}
}
