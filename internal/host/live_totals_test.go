//go:build integration

package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveProviderContradictoryCoverageNeverReachesTheModelAsSource(t *testing.T) {
	for _, mode := range []string{"zero", "negative", "gap", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WORKBENCH_REFERENCE_BAD_COVERAGE", mode)
			results := runScriptedWith(t, t.TempDir(), "read", "", []string{"reference-producer"}, func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "kb://sources/a.md"}} })
			if len(results) != 1 || !strings.Contains(results[0], "provider") || strings.Contains(results[0], "alpha one") {
				t.Fatalf("contradictory source coverage was emitted: %v", results)
			}
		})
	}
}

func TestLiveProviderUnknownTotalsRemainAbsentAndKnownEmptyRemainsZero(t *testing.T) {
	root := t.TempDir()
	runScriptedWith(t, root, "read", "", []string{"reference-producer"},
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"path": "kb://totals/unknown"}}
		},
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "kb://totals/empty"}} },
	)
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*.jsonl"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	}
	seen := map[string]bool{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if line == "" {
				continue
			}
			var entry struct {
				Message struct {
					Role     string
					ToolName string
					Details  struct{ Resources []map[string]any }
				}
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Message.Role != "toolResult" || entry.Message.ToolName != "read" {
				continue
			}
			for _, resource := range entry.Message.Details.Resources {
				uri, _ := resource["canonicalUri"].(string)
				switch uri {
				case "kb://totals/unknown":
					if _, present := resource["totalLines"]; present {
						t.Fatalf("unknown total was fabricated in live persisted details: %+v", resource)
					}
					seen[uri] = true
				case "kb://totals/empty":
					if total, present := resource["totalLines"]; !present || total != float64(0) || resource["resourceComplete"] != true {
						t.Fatalf("known empty total was lost: %+v", resource)
					}
					seen[uri] = true
				}
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("did not verify both live result shapes: %+v", seen)
	}
}
