package host

import (
	"fmt"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"

	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

func lifecycleDetails(revision int, phase, outcome string) map[string]any {
	snapshot := map[string]any{"version": 1, "revision": revision, "phase": phase, "title": "Retained title"}
	if outcome != "" {
		snapshot["outcome"] = outcome
	}
	return map[string]any{"workbenchPresentation": snapshot}
}

func TestRendererUsesRetainedLifecycleAcrossWidthAndBadLaterDetails(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	a.session = s
	registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	renderers := a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return nil })
	entry, _ := s.presentations.lookup("build_run")
	key := presentationCallKey{Owner: entry.owner, Key: entry.key, CallID: "render-call"}
	s.presentationData.observe(key, "build_run", entry.descriptor, lifecycleDetails(2, "settled", "failed"), true)
	for _, width := range []int{36, 57, 80, 140, 80} {
		lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: map[string]any{"malformed": "not reparsed"}}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{ToolCallID: "render-call"}, width)
		if err != nil || !strings.Contains(strings.Join(lines, "\n"), "failed") || strings.Contains(strings.Join(lines, "\n"), "presentation unavailable") {
			t.Fatalf("width %d reparsed or regressed retained data: %v %v", width, lines, err)
		}
	}
	s.presentationData.observe(key, "build_run", entry.descriptor, lifecycleDetails(3, "settled", "succeeded"), true)
	lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{ToolCallID: "render-call"}, 140)
	if err != nil || !strings.Contains(strings.Join(lines, "\n"), "failed") || !strings.Contains(strings.Join(lines, "\n"), "conflicting final") {
		t.Fatalf("layout cache concealed terminal conflict: %v %v", lines, err)
	}
}

func TestPresentationCacheRetentionIsBoundedAndFinalCannotRemainRunning(t *testing.T) {
	descriptor, err := presentation.DecodeDescriptor([]byte(buildPresentationDescriptor))
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{1, 60000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var cache presentationCache
			count := 300
			if size > 1 {
				count = 160
			}
			for i := 0; i < count; i++ {
				details := lifecycleDetails(1, "settled", "succeeded")
				details["workbenchPresentation"].(map[string]any)["blocks"] = []any{map[string]any{"kind": "text", "text": strings.Repeat("x", size)}}
				cache.observe(presentationCallKey{Owner: "producer", Key: "tool", CallID: fmt.Sprint(i)}, "tool", descriptor, details, true)
			}
			if _, exists := cache.lookup(presentationCallKey{Owner: "producer", Key: "tool", CallID: "0"}); exists {
				t.Fatal("oldest presentation was not evicted at the retention limit")
			}
			if record, exists := cache.lookup(presentationCallKey{Owner: "producer", Key: "tool", CallID: fmt.Sprint(count - 1)}); !exists || !record.valid {
				t.Fatalf("recent valid presentation lost: %+v %v", record, exists)
			}
		})
	}
	var cache presentationCache
	key := presentationCallKey{Owner: "producer", Key: "tool", CallID: "nonterminal-final"}
	cache.observe(key, "tool", descriptor, lifecycleDetails(1, "running", ""), false)
	cache.observe(key, "tool", descriptor, lifecycleDetails(2, "running", ""), true)
	cache.observe(key, "tool", descriptor, lifecycleDetails(3, "running", ""), false)
	if record, _ := cache.lookup(key); record.valid || !record.final {
		t.Fatalf("nonterminal final resurrected a running card: %+v", record)
	}
}

func TestPresentationCacheNeverResurrectsSettledOrConflictingFinals(t *testing.T) {
	descriptor, err := presentation.DecodeDescriptor([]byte(buildPresentationDescriptor))
	if err != nil {
		t.Fatal(err)
	}
	key := presentationCallKey{Owner: "producer", Key: "tool", CallID: "call"}
	var cache presentationCache
	cache.observe(key, "tool", descriptor, lifecycleDetails(1, "running", ""), false)
	cache.observe(key, "tool", descriptor, lifecycleDetails(2, "settled", "failed"), true)
	cache.observe(key, "tool", descriptor, lifecycleDetails(3, "running", ""), false)
	record, ok := cache.lookup(key)
	if !ok || record.snapshot.Phase != presentation.PhaseSettled || record.snapshot.Outcome == nil || *record.snapshot.Outcome != presentation.OutcomeFailed {
		t.Fatalf("late running update resurrected terminal state: %+v", record)
	}
	cache.observe(key, "tool", descriptor, lifecycleDetails(3, "settled", "succeeded"), true)
	record, _ = cache.lookup(key)
	if *record.snapshot.Outcome != presentation.OutcomeFailed || !strings.Contains(record.diagnostic, "conflicting final") {
		t.Fatalf("conflicting final became retroactive success: %+v", record)
	}
	cache.observe(key, "tool", descriptor, lifecycleDetails(1, "preparing", ""), false)
	record, _ = cache.lookup(key)
	if record.snapshot.Revision != 2 {
		t.Fatalf("old update replaced settled facts: %+v", record)
	}
}

func TestPresentationCacheOwnsValidatedDataAndSeparatesCalls(t *testing.T) {
	descriptor, err := presentation.DecodeDescriptor([]byte(buildPresentationDescriptor))
	if err != nil {
		t.Fatal(err)
	}
	var cache presentationCache
	first := presentationCallKey{Owner: "producer", Key: "tool", CallID: "one"}
	second := presentationCallKey{Owner: "producer", Key: "tool", CallID: "two"}
	details := lifecycleDetails(1, "settled", "succeeded")
	cache.observe(first, "tool", descriptor, details, true)
	details["workbenchPresentation"].(map[string]any)["title"] = "mutated after delivery"
	cache.observe(second, "tool", descriptor, lifecycleDetails(1, "running", ""), false)
	record, _ := cache.lookup(first)
	if record.snapshot.Title != "Retained title" || record.snapshot.Phase != presentation.PhaseSettled {
		t.Fatalf("retained data changed across caller mutation/call identity: %+v", record)
	}
}
