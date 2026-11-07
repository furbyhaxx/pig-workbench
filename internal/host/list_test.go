package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestBoundedListingContinuationDoesNotSkipOrDuplicateRows(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 600; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("item-%04d-%s.txt", i, strings.Repeat("a", 120))), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	router := resource.NewRouter()
	if err := router.RegisterBuiltin(resource.FileProvider{}); err != nil {
		t.Fatal(err)
	}
	s := &session{router: router, generation: "test"}
	target, err := resource.ParsePath(root, ".", "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	complete := false
	for range 20 {
		result, err := listContainer(t.Context(), s, target, cursor, 2000, true)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(result.Details)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Content) > 50<<10 || strings.Count(result.Content, "\n") > 2000 || len(encoded) > 50<<10 {
			t.Fatalf("listing exceeded result bounds: content=%d metadata=%d", len(result.Content), len(encoded))
		}
		facts := result.Details.(map[string]any)
		for _, entry := range facts["entries"].([]resource.Entry) {
			if seen[entry.URI] {
				t.Fatalf("duplicated row %s", entry.URI)
			}
			seen[entry.URI] = true
		}
		if facts["complete"] == true {
			complete = true
			break
		}
		cursor = facts["nextCursor"].(string)
		if cursor == "" {
			t.Fatal("incomplete listing lost continuation")
		}
	}
	if !complete || len(seen) != 600 {
		t.Fatalf("listing skipped rows or never completed: complete=%v count=%d", complete, len(seen))
	}
}

func TestListingHiddenPolicyIsReportedAndApplied(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"visible", ".hidden"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	router := resource.NewRouter()
	if err := router.RegisterBuiltin(resource.FileProvider{}); err != nil {
		t.Fatal(err)
	}
	s := &session{router: router, generation: "test"}
	target, err := resource.ParsePath(root, ".", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := listContainer(t.Context(), s, target, "", 200, false)
	if err != nil {
		t.Fatal(err)
	}
	facts := result.Details.(map[string]any)
	if facts["includeHidden"] != false || facts["hiddenSkipped"] != 1 || strings.Contains(result.Content, ".hidden") || !strings.Contains(result.Content, "visible") {
		t.Fatalf("hidden facts differ from output: %+v %s", facts, result.Content)
	}
}
