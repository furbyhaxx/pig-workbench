package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/syntax"
)

func summaryFixture(t *testing.T, functions int) (string, []byte) {
	t.Helper()
	var body strings.Builder
	body.WriteString("package fixture\n\n")
	for index := 1; index <= functions; index++ {
		fmt.Fprintf(&body, "func function%d(value int) int {\n", index)
		for line := 0; line < 40; line++ {
			fmt.Fprintf(&body, "\tvalue += %d\n", line)
		}
		body.WriteString("\treturn value\n}\n\n")
	}
	text := body.String()
	path := filepath.Join(t.TempDir(), "fixture.go")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path, []byte(text)
}

func TestSummaryViewHonoursSettings(t *testing.T) {
	path, _ := summaryFixture(t, 6)
	target, err := resource.ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := config.New(config.Paths{User: filepath.Join(t.TempDir(), "a.toml"), Project: filepath.Join(t.TempDir(), "b.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	app := &app{syntax: syntax.New()}
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"read.summarize.enabled", false},
		{"read.summarize.min-total-lines", int64(1000)},
	} {
		t.Run(tc.key, func(t *testing.T) {
			baseline, err := manager.Current()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Save(t.Context(), config.Runtime, baseline, []config.Edit{{Key: tc.key, Value: tc.value}}, nil); err != nil {
				t.Fatal(err)
			}
			snapshot, err := manager.Current()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok, err := app.summaryView(t.Context(), target, snapshot); err != nil || ok {
				t.Fatalf("setting did not disable summary: ok=%v err=%v", ok, err)
			}
			if _, err := manager.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: tc.key, Reset: true}}, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
	baseline, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := app.summaryView(t.Context(), target, baseline); err != nil || !ok {
		t.Fatalf("default settings must allow summary: ok=%v err=%v", ok, err)
	}
}

func TestSummaryViewFoldsBodiesOnly(t *testing.T) {
	path, source := summaryFixture(t, 6)
	app := &app{syntax: syntax.New()}
	configuration, err := config.New(config.Paths{User: filepath.Join(t.TempDir(), "a.toml"), Project: filepath.Join(t.TempDir(), "b.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := configuration.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := configuration.Current()
	if err != nil {
		t.Fatal(err)
	}
	target, err := resource.ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	view, ok, err := app.summaryView(context.Background(), target, snapshot)
	if err != nil || !ok {
		t.Fatalf("summary not produced: ok=%v err=%v", ok, err)
	}
	if strings.Count(view.Content, "func function") != 6 {
		t.Fatalf("declarations missing: %s", view.Content)
	}
	if !strings.Contains(view.Content, "omitted") || !strings.Contains(view.Content, "never marks folded lines seen") {
		t.Fatalf("fold markers or honesty notice missing: %s", view.Content)
	}
	if view.Facts.ResourceComplete || view.Facts.SelectionComplete || view.Facts.Truncation != "summary" {
		t.Fatalf("summary reported as complete: %+v", view.Facts)
	}
	for _, span := range view.Facts.FullyShownRanges {
		for _, fold := range view.Facts.OmittedRanges {
			if span.Start <= fold[1] && fold[0] <= span.End {
				t.Fatalf("summary marked folded lines %v as seen: %+v", fold, span)
			}
		}
	}
	if len(view.Facts.OmittedRanges) == 0 {
		t.Fatal("omitted intervals not reported")
	}
	total := strings.Count(string(source), "\n")
	if view.Facts.TotalLines != total {
		t.Fatalf("total lines %d != %d", view.Facts.TotalLines, total)
	}
	for _, span := range view.Facts.OmittedRanges {
		if span[1] > total {
			t.Fatalf("summary invents a line after EOF: %v > %d", span, total)
		}
	}
	ranged := target
	ranged.Selection, _ = resource.ParseSelector(":10-20:raw")
	if _, ok, err := app.summaryView(context.Background(), ranged, snapshot); ok || err != nil {
		t.Fatalf("explicit selection was summarised: ok=%v err=%v", ok, err)
	}
	small, _ := summaryFixture(t, 1)
	smallTarget, err := resource.ParsePath(filepath.Dir(small), small, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := app.summaryView(context.Background(), smallTarget, snapshot); ok || err != nil {
		t.Fatalf("small file was summarised: ok=%v err=%v", ok, err)
	}
	nonCode := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(nonCode, []byte(strings.Repeat("text line\n", 400)), 0600); err != nil {
		t.Fatal(err)
	}
	textTarget, err := resource.ParsePath(filepath.Dir(nonCode), nonCode, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := app.summaryView(context.Background(), textTarget, snapshot); ok || err != nil {
		t.Fatalf("non-code file was summarised: ok=%v err=%v", ok, err)
	}
}

func summarySnapshot(t *testing.T, edits ...config.Edit) *config.Snapshot {
	t.Helper()
	manager, err := config.New(config.Paths{User: filepath.Join(t.TempDir(), "a.toml"), Project: filepath.Join(t.TempDir(), "b.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	baseline, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) == 0 {
		return baseline
	}
	if _, err := manager.Save(t.Context(), config.Runtime, baseline, edits, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func summarize(t *testing.T, name, text string, snapshot *config.Snapshot) (resource.ReadResult, bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := resource.ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	view, ok, err := (&app{syntax: syntax.New()}).summaryView(t.Context(), target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return view, ok
}

func TestSummaryFoldThresholdsFollowTheSettings(t *testing.T) {
	var body strings.Builder
	body.WriteString("package fixture\n\n")
	for index := 0; index < 30; index++ {
		body.WriteString("// note\n// note\n// note\n// note\n// note\n")
		fmt.Fprintf(&body, "func short%d(value int) int {\n\tvalue++\n\tvalue++\n\tvalue++\n\tvalue++\n\treturn value\n}\n\n", index)
	}
	text := body.String()
	if _, ok := summarize(t, "short.go", text, summarySnapshot(t)); ok {
		t.Fatal("five-line bodies and comments were folded under the default thresholds")
	}
	bodies, ok := summarize(t, "short.go", text, summarySnapshot(t, config.Edit{Key: "read.summarize.min-body-lines", Value: int64(5)}))
	if !ok || len(bodies.Facts.OmittedRanges) != 30 || strings.Contains(bodies.Content, "return value") || !strings.Contains(bodies.Content, "// note") {
		t.Fatalf("min-body-lines=5 did not fold exactly the bodies: ok=%v folds=%d", ok, len(bodies.Facts.OmittedRanges))
	}
	comments, ok := summarize(t, "short.go", text, summarySnapshot(t, config.Edit{Key: "read.summarize.min-comment-lines", Value: int64(5)}))
	if !ok || len(comments.Facts.OmittedRanges) != 30 || !strings.Contains(comments.Content, "return value") || strings.Count(comments.Content, "// note") != 60 {
		t.Fatalf("min-comment-lines=5 did not fold exactly the comment interiors: ok=%v folds=%d", ok, len(comments.Facts.OmittedRanges))
	}
}

func TestProseSummaryIsOffUntilEnabled(t *testing.T) {
	var body strings.Builder
	for index := 0; index < 10; index++ {
		fmt.Fprintf(&body, "## Section %d\n\n", index)
		body.WriteString(strings.Repeat("A sentence of prose.\n", 30))
	}
	text := body.String()
	if _, ok := summarize(t, "notes.md", text, summarySnapshot(t)); ok {
		t.Fatal("Markdown was summarised with read.summarize.prose off")
	}
	view, ok := summarize(t, "notes.md", text, summarySnapshot(t, config.Edit{Key: "read.summarize.prose", Value: true}))
	if !ok {
		t.Fatal("Markdown was not summarised with read.summarize.prose on")
	}
	if strings.Count(view.Content, "## Section") != 10 || strings.Contains(view.Content, "A sentence of prose.") || len(view.Facts.OmittedRanges) != 10 {
		t.Fatalf("prose summary must keep the ten headings and fold the text: %s", view.Content)
	}
}
