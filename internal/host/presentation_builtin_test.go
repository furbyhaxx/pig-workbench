package host

import (
	"strconv"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"

	"github.com/furbyhaxx/pig-workbench/internal/fileedit"
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

func TestEditAndWriteSnapshotsCarryDiffBlocks(t *testing.T) {
	old := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n"
	next := "one\ntwo\nthree\nfour\nFIVE\nfive and a half\nsix\nseven\neight\n"
	effects := []fileedit.Effect{{Path: "/w/a.go", Bytes: []byte(next)}, {Path: "/w/gone.go", Delete: true}, {Path: "/w/kept.go", Bytes: []byte("x")}}
	outcomes := []fileedit.Outcome{{Path: "/w/a.go", BeforeRevision: "r1", AfterRevision: "r2", Committed: true}, {Path: "/w/gone.go", BeforeRevision: "r3", Committed: true}, {Path: "/w/kept.go"}}
	details := map[string]any{"partial": false, presentationBlocksKey: editDiffBlocks(effects, [][]byte{[]byte(old), nil, []byte("x")}, outcomes)}
	edit := builtinSnapshotOf(t, "edit", map[string]any{"path": "a.go", "edits": []any{}}, sdk.ToolResult{Content: "Edit batch committed.\n", Details: details})
	if _, leaked := details[presentationBlocksKey]; leaked {
		t.Error("the staging key stayed in Details")
	}
	if len(edit.Blocks) != 2 || edit.Blocks[1].Diff.Operation != presentation.DiffDelete || edit.Blocks[1].Diff.After != nil {
		t.Fatalf("edit blocks: %+v", edit.Blocks)
	}
	update := edit.Blocks[0].Diff
	if update.Operation != presentation.DiffUpdate || update.Before.Revision != "r1" || update.After.Revision != "r2" || len(update.Hunks) != 1 {
		t.Fatalf("update block: %+v", update)
	}
	hunk := update.Hunks[0]
	if hunk.OldStart != 2 || hunk.NewStart != 2 || hunk.OldLines != 7 || hunk.NewLines != 8 {
		t.Errorf("hunk coordinates are not source lines: %+v", hunk)
	}
	added, _ := snapshotMetric(edit, "added")
	removed, _ := snapshotMetric(edit, "removed")
	if added != 2 || removed != 1 {
		t.Errorf("added %v removed %v", added, removed)
	}
	if hunks := diffHunks("a\x00b\n", "a\x1b[31mb\nc\n"); len(hunks) != 1 || strings.ContainsAny(hunks[0].Lines[0].Text, "\x00\x1b") {
		t.Errorf("control bytes reached a hunk line: %+v", hunks)
	}
	if hunks := diffHunks(strings.Repeat("a\n", maxDiffWindowLines+5), strings.Repeat("b\n", maxDiffWindowLines+5)); hunks != nil {
		t.Error("an oversized change was diffed")
	}

	created := builtinSnapshotOf(t, "write", map[string]any{"path": "new.txt"}, sdk.ToolResult{Content: "Wrote", Details: map[string]any{"bytes": 12, presentationBlocksKey: []presentation.Block{writeDiffBlock("/w/new.txt", "", "r9", true)}}})
	if bytes, _ := snapshotMetric(created, "bytes"); bytes != 12 || len(created.Blocks) != 1 || created.Blocks[0].Diff.Operation != presentation.DiffCreate || created.Blocks[0].Diff.Before != nil {
		t.Errorf("write snapshot: %+v", created)
	}
	replaced := writeDiffBlock("/w/new.txt", "r9", "r10", false).Diff
	if replaced.Operation != presentation.DiffUpdate || replaced.Before.Revision != "r9" || replaced.After.Revision != "r10" {
		t.Errorf("replacement block: %+v", replaced)
	}
}

func TestBashAndJobsSnapshotsCarryTailExitAndOutcome(t *testing.T) {
	code := 3
	var long strings.Builder
	for i := range 100 {
		long.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	failed := builtinSnapshotOf(t, "bash", map[string]any{"command": "make"}, sdk.ToolResult{Content: "Job ab12-1: exited (exit 3)\n" + long.String() + "\x1b[31mboom\x1b[0m\n", IsError: true, Details: jobPage{jobFacts: jobFacts{ID: "ab12-1", Command: "make", State: "exited", ExitCode: &code}, Truncated: true}})
	exit, hasExit := snapshotMetric(failed, "exitCode")
	lines, _ := snapshotMetric(failed, "lines")
	if *failed.Outcome != presentation.OutcomeFailed || !hasExit || exit != 3 || lines != 101 || failed.Complete == nil || *failed.Complete || failed.Summary != "" {
		t.Errorf("failed bash snapshot: %+v", failed)
	}
	if len(failed.Blocks) != 1 || failed.Blocks[0].Kind != presentation.BlockCode {
		t.Fatalf("bash blocks: %+v", failed.Blocks)
	}
	tail := failed.Blocks[0].Code.Text
	if strings.Count(tail, "\n") != jobTailLines-1 || !strings.HasSuffix(tail, "[31mboom[0m") || strings.Contains(tail, "Job ab12") || strings.Contains(tail, "line 10\n") {
		t.Errorf("tail is not the last %d lines without escapes: %q", jobTailLines, tail)
	}

	zero := 0
	clean := builtinSnapshotOf(t, "bash", map[string]any{"command": "true"}, sdk.ToolResult{Content: "(no output)", Details: jobPage{jobFacts: jobFacts{State: "exited", ExitCode: &zero}}})
	if exit, ok := snapshotMetric(clean, "exitCode"); !ok || exit != 0 || *clean.Outcome != presentation.OutcomeSucceeded {
		t.Errorf("clean bash snapshot: %+v", clean)
	}
	for state, want := range map[string]presentation.Outcome{"timedOut": presentation.OutcomeTimedOut, "cancelled": presentation.OutcomeCancelled, "unknown": presentation.OutcomeUnknown, "running": presentation.OutcomeSucceeded} {
		snapshot := builtinSnapshotOf(t, "jobs", map[string]any{"action": "status"}, sdk.ToolResult{Content: "Job ab12-1: " + state + "\n", Details: jobPage{jobFacts: jobFacts{State: state, Running: state == "running"}}})
		if *snapshot.Outcome != want || snapshot.Summary != state || len(snapshot.Blocks) != 0 {
			t.Errorf("%s: %+v", state, snapshot)
		}
	}
	list := builtinSnapshotOf(t, "jobs", map[string]any{"action": "list"}, sdk.ToolResult{Content: "{}", Details: jobList{Jobs: []jobFacts{{ID: "a"}, {ID: "b"}}}})
	if count, _ := snapshotMetric(list, "jobs"); count != 2 || len(list.Blocks) != 0 {
		t.Errorf("list snapshot: %+v", list)
	}
	wait := builtinSnapshotOf(t, "jobs", map[string]any{"action": "wait"}, sdk.ToolResult{Content: "Wait timed out", Details: map[string]any{"timedOutWait": true}})
	if *wait.Outcome != presentation.OutcomeTimedOut {
		t.Errorf("wait snapshot: %+v", wait)
	}
}
