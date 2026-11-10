package host

import (
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"

	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

var ownToolNames = []string{"read", "write", "edit", "grep", "find", "ls", "bash", "jobs", "cd", "ask_user"}

func TestEveryOwnToolResolvesToABuiltinDescriptor(t *testing.T) {
	catalog := newPresentationCatalog()
	for _, tool := range ownToolNames {
		entry, ok := catalog.lookup(tool)
		if !ok || entry.owner != builtinOwner || entry.builtin == nil || entry.descriptor.Label != tool {
			t.Errorf("%s: no builtin descriptor: %+v", tool, entry)
		}
	}
	if len(catalog.list()) != 0 {
		t.Errorf("builtin descriptors are listed as contributions: %v", catalog.list())
	}
	foreign := presentation.Descriptor{ToolNames: []string{"read"}, Label: "Read", PresentationVersion: 1, Preset: presentation.PresetGeneric}
	if err := catalog.add("r-1", "other", "read", foreign); err == nil {
		t.Error("a contribution claimed an own tool")
	}
}

func builtinSnapshotOf(t *testing.T, tool string, params map[string]any, result sdk.ToolResult) presentation.Snapshot {
	t.Helper()
	out, err := presented(tool, func(sdk.Context, map[string]any) (any, error) { return result, nil })(sdk.Context{}, params)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := out.(sdk.ToolResult)
	if wrapped.Content != result.Content || wrapped.IsError != result.IsError {
		t.Fatalf("%s: the model-facing result changed", tool)
	}
	snapshot, err := decodeToolSnapshot(builtinRegistration(tool).descriptor, wrapped.Details)
	if err != nil {
		t.Fatalf("%s: snapshot does not pass the SPEC-0003 decoder: %v", tool, err)
	}
	return snapshot
}

func snapshotMetric(snapshot presentation.Snapshot, id string) (float64, bool) {
	for _, metric := range snapshot.Metrics {
		if metric.ID == id {
			return metric.Value, true
		}
	}
	return 0, false
}

func TestReadAndSearchSnapshotsCarryOutcomeTargetAndMetrics(t *testing.T) {
	incomplete := map[string]any{"complete": false, "failedTargets": 1, "resources": []any{}}
	read := builtinSnapshotOf(t, "read", map[string]any{"paths": []any{"a.go", "b.go"}}, sdk.ToolResult{Content: "1:aa|x\n2:bb|y\n", Details: incomplete})
	if lines, _ := snapshotMetric(read, "lines"); *read.Outcome != presentation.OutcomeSucceeded || read.Target == nil || read.Target.Value != "a.go" || lines != 2 || read.Complete == nil || *read.Complete || read.Summary != "1 target of 2 targets failed" {
		t.Errorf("read snapshot: %+v", read)
	}
	if _, ok := incomplete[presentationDetailsKey]; !ok || incomplete["failedTargets"] != 1 {
		t.Errorf("the tool's own details were replaced: %v", incomplete)
	}
	failed := builtinSnapshotOf(t, "read", map[string]any{"path": "kb://x/y"}, sdk.ToolResult{Content: "nope", IsError: true})
	if *failed.Outcome != presentation.OutcomeFailed || failed.Target.Kind != presentation.TargetURI {
		t.Errorf("failed read snapshot: %+v", failed)
	}
	grep := builtinSnapshotOf(t, "grep", map[string]any{"pattern": "x", "path": "internal"}, sdk.ToolResult{Content: "a:1:1: x\n", Details: map[string]any{"complete": true, "matches": []string{"m1", "m2", "m3"}}})
	if matches, _ := snapshotMetric(grep, "matches"); matches != 3 || grep.Target.Value != "internal" || !*grep.Complete {
		t.Errorf("grep snapshot: %+v", grep)
	}
	for _, tool := range []string{"ls", "find"} {
		snapshot := builtinSnapshotOf(t, tool, map[string]any{"pattern": "*.go"}, sdk.ToolResult{Content: "a.go\n", Details: map[string]any{"complete": true, "entries": []any{"a.go"}}})
		if entries, _ := snapshotMetric(snapshot, "entries"); entries != 1 || snapshot.Target != nil {
			t.Errorf("%s snapshot: %+v", tool, snapshot)
		}
	}
}
