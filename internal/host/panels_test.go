package host

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
)

func TestPageWindowClampsAndNeverReturnsOutOfRangeRows(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	for _, tc := range []struct {
		top, rows, wantTop int
		want               []string
	}{
		{-5, 2, 0, []string{"a", "b"}},
		{1, 2, 1, []string{"b", "c"}},
		{99, 2, 3, []string{"d", "e"}},
		{0, 10, 0, lines},
	} {
		top, got := pageWindow(lines, tc.top, tc.rows)
		if top != tc.wantTop || !slices.Equal(got, tc.want) {
			t.Fatalf("pageWindow(top=%d rows=%d) = %d %v", tc.top, tc.rows, top, got)
		}
	}
	if top, got := pageWindow(nil, 3, 4); top != 0 || len(got) != 0 {
		t.Fatalf("empty input: %d %v", top, got)
	}
}

func TestPanelTextWrapsToWidthAndStripsTerminalControl(t *testing.T) {
	hostile := "ok \x1b[31mred\x1b[0m\x1b]0;title\x07 " + strings.Repeat("word ", 60)
	for _, width := range []int{36, 57, 80, 140} {
		for _, line := range wrapLines(hostile, width) {
			if tui.VisibleWidth(line) > width || strings.ContainsRune(line, '\x1b') || strings.ContainsRune(line, '\x07') {
				t.Fatalf("width %d unsafe line %q", width, line)
			}
		}
	}
}

func testPanel(t *testing.T) *settingsPanel {
	t.Helper()
	manager, err := config.New(config.Paths{User: filepath.Join(t.TempDir(), "Workbench.toml"), Project: filepath.Join(t.TempDir(), "p.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	saves := 0
	return newSettingsPanel(snapshot, []config.Scope{config.User, config.Runtime}, func() int { return 30 }, func(scope config.Scope, edits []config.Edit) (config.SaveResult, *config.Snapshot, error) {
		saves++
		result, err := manager.Save(t.Context(), scope, snapshot, edits, nil)
		fresh, _ := manager.Current()
		return result, fresh, err
	})
}

func TestSettingsPanelStagesTypedValuesAndRefusesBadInput(t *testing.T) {
	panel := testPanel(t)
	byKey := map[string]config.Definition{}
	for _, definition := range panel.definitions {
		byKey[definition.Key] = definition
	}
	toggle, ok := panel.next(byKey["edit.enforce-seen-lines"])
	if !ok || toggle != false {
		t.Fatalf("boolean did not flip from its default true: %v %v", toggle, ok)
	}
	first, _ := panel.next(byKey["edit.mode"])
	if first != "replace" {
		t.Fatalf("choice did not advance past hashline: %v", first)
	}
	if _, ok := panel.next(byKey["edit.snapshot.max-paths"]); ok {
		t.Fatal("an integer must be typed, not cycled")
	}
	paths := byKey["edit.snapshot.max-paths"]
	for _, bad := range []string{"", "abc", "0", "999999999"} {
		if _, err := parseTyped(paths, bad); err == nil {
			t.Fatalf("%q was accepted for an integer with bounds %d..%d", bad, paths.Min, paths.Max)
		}
	}
	if value, err := parseTyped(paths, " 300 "); err != nil || value != int64(300) {
		t.Fatalf("valid integer: %v %v", value, err)
	}
}

func TestSettingsPanelSaveKeepsStagedEditsOnFailureAndClearsThemOnSuccess(t *testing.T) {
	panel := testPanel(t)
	panel.staged["no.such.key"] = stagedChange{value: true}
	panel.flush()
	if !panel.dirty() || !strings.HasPrefix(panel.message, "Not saved:") {
		t.Fatalf("a failed save lost the staged edit or hid the reason: %q dirty=%v", panel.message, panel.dirty())
	}
	delete(panel.staged, "no.such.key")
	panel.staged["edit.enforce-seen-lines"] = stagedChange{value: false}
	panel.staged["edit.mode"] = stagedChange{reset: true}
	panel.flush()
	if panel.dirty() || !strings.HasPrefix(panel.message, "Saved ") {
		t.Fatalf("successful save did not clear staging: %q", panel.message)
	}
	if text, source := panel.valueOf(config.Definition{Key: "edit.enforce-seen-lines"}); text != "false" || source != "user" {
		t.Fatalf("saved value not shown with its source: %s [%s]", text, source)
	}
}

func TestSettingsPanelSearchFiltersByKeyAndDescription(t *testing.T) {
	panel := testPanel(t)
	all := len(panel.visible())
	panel.filter = "SNAPSHOT.MAX"
	filtered := panel.visible()
	if len(filtered) == 0 || len(filtered) >= all {
		t.Fatalf("filter did not narrow: %d of %d", len(filtered), all)
	}
	for _, definition := range filtered {
		if !strings.Contains(strings.ToLower(definition.Key+definition.Description), "snapshot.max") {
			t.Fatalf("%s does not match", definition.Key)
		}
	}
	panel.filter = "zzz-no-match"
	if _, ok := panel.current(); ok {
		t.Fatal("empty result reported a current setting")
	}
}

func TestEveryHubEntryNamesAVerbTheDispatcherAndCompletionsKnow(t *testing.T) {
	a := &app{}
	for _, item := range a.hubItemsStatic() {
		completions, err := completeWorkbench(item.value, nil, config.Catalog())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, completion := range completions {
			found = found || completion.Value == item.value
		}
		if !found {
			t.Errorf("hub entry %q is not a completable /workbench verb", item.value)
		}
	}
}

func TestJobListTextIsHumanReadableNotJSON(t *testing.T) {
	s := &session{jobMeta: jobMetadataStore{values: map[string]jobMetadata{}}}
	manager, err := jobs.NewManager(jobs.ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	s.jobs = manager
	if got := s.jobListText(); got != "No jobs in this session." {
		t.Fatalf("empty list: %q", got)
	}
	id, err := manager.Start(t.Context(), jobs.Spec{Command: []string{"sh", "-c", "exit 3"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Wait(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	s.jobMeta.values[id] = jobMetadata{Command: "exit 3"}
	got := s.jobListText()
	if !strings.Contains(got, id) || !strings.Contains(got, "exited (exit 3)") || !strings.Contains(got, "exit 3") || strings.HasPrefix(got, "[") {
		t.Fatalf("job row is not readable: %q", got)
	}
}

func newJobTestSession(t *testing.T) *session {
	t.Helper()
	manager, err := jobs.NewManager(jobs.ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close() })
	root := t.TempDir()
	settings, err := config.New(config.Paths{User: filepath.Join(root, "Workbench.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	return &session{jobs: manager, settings: settings, jobMeta: jobMetadataStore{values: map[string]jobMetadata{}}}
}

func TestJobWaitUntilOutputReturnsBeforeExitAndPerStreamCursorsAreIndependent(t *testing.T) {
	s := newJobTestSession(t)
	id, err := s.jobs.Start(t.Context(), jobs.Spec{Command: []string{"sh", "-c", "echo out1; echo err1 >&2; sleep 5"}})
	if err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := s.waitOutput(wait, id, jobs.StreamStderr, 0); err != nil {
		t.Fatalf("wait for stderr output: %v", err)
	}
	result, err := s.jobResult(id, jobs.StreamStderr, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "err1") || strings.Contains(result.Content, "out1") || !strings.Contains(result.Content, "running") {
		t.Fatalf("stderr view wrong: %q", result.Content)
	}
	next := result.Details.(jobPage).Cursor
	short, cancelShort := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancelShort()
	if err := s.waitOutput(short, id, jobs.StreamStderr, next); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("no new stderr bytes must time out while the job runs, got %v", err)
	}
	if err := s.jobs.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if err := s.waitOutput(t.Context(), id, jobs.StreamStderr, next); err != nil {
		t.Fatalf("an ended job must release an output wait: %v", err)
	}
}

func TestJobStreamNameIsValidated(t *testing.T) {
	for _, name := range []string{"", "combined", "stdout", "stderr"} {
		if _, err := jobStream(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, err := jobStream("stdin"); err == nil {
		t.Fatal("unknown stream accepted")
	}
}

func TestHubFitsDescriptionsAndShortensTheTitlePath(t *testing.T) {
	const location = "/srv/checkouts/a-rather-long-organisation-name/some-project/internal/host"
	panel := &hubPanel{location: location, height: func() int { return 24 }, items: []hubItem{{"tools", "Tools", "11 registered · inspect schemas, owners and exposure"}}}
	for _, width := range []int{57, 80} {
		encoded, err := json.Marshal(panel.View(width).Root)
		if err != nil {
			t.Fatal(err)
		}
		var texts []string
		var collect func(any)
		collect = func(value any) {
			switch typed := value.(type) {
			case string:
				texts = append(texts, typed)
			case []any:
				for _, item := range typed {
					collect(item)
				}
			case map[string]any:
				for _, item := range typed {
					collect(item)
				}
			}
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		collect(decoded)
		var description, title string
		for _, text := range texts {
			if strings.HasPrefix(text, "11 registered") {
				description = text
			}
			if strings.HasPrefix(text, "Workbench · ") {
				title = text
			}
		}
		if !strings.HasSuffix(description, "…") || tui.VisibleWidth(description) > width-hubLabelColumn {
			t.Errorf("width %d: description %q must fit beside the label column and end with an ellipsis", width, description)
		}
		if title == "" || strings.Contains(title, location) || !strings.HasSuffix(title, "host") || tui.VisibleWidth(title) > width-2 {
			t.Errorf("width %d: title %q must shorten the path and keep its last segment", width, title)
		}
	}
}
