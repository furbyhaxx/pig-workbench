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
		if span.End != span.Start {
			t.Fatalf("summary marked a body range as seen: %+v", span)
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
