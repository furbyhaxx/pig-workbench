package host

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
)

func TestActivityLineDropsLowestPriorityFirstAndNeverCutsAnItem(t *testing.T) {
	items := []activityItem{
		{Key: "ask", Label: "waiting for you", Glyph: "?", Count: 1, Severity: "warning", Priority: 100},
		{Key: "jobs.failed", Label: "failed", Glyph: "✗", Count: 1, Severity: "error", Priority: 90},
		{Key: "jobs", Label: "jobs", Glyph: "⚙", Count: 2, Priority: 80},
		{Key: "peers", Label: "live sessions", Glyph: "⚇", Count: 3, Priority: 60},
		{Key: "skills", Label: "skills", Glyph: "◆", Count: 5, Priority: 40},
	}
	if got := renderActivity(items, 140); got != "1 waiting for you  1 failed  2 jobs  3 live sessions  5 skills" {
		t.Fatalf("wide = %q", got)
	}
	if got := renderActivity(items, 49); got != "?1 ✗1 ⚙2 ⚇3 ◆5" {
		t.Fatalf("narrow forms are glyph plus number: %q", got)
	}
	for _, width := range []int{36, 57, 80, 140} {
		line := renderActivity(items, width)
		if tui.VisibleWidth(line) > width {
			t.Fatalf("width %d: %q is %d columns", width, line, tui.VisibleWidth(line))
		}
	}
	at57 := renderActivity(items, 57)
	if !strings.Contains(at57, "waiting for you") || strings.Contains(at57, "skills") {
		t.Fatalf("at 57 columns the lowest priority drops first: %q", at57)
	}
	if got := renderActivity(items[:1], 6); got != "?1" {
		t.Fatalf("a narrow terminal keeps the glyph form: %q", got)
	}
	if got := renderActivity(items[:1], 1); got != "" {
		t.Fatalf("an item that does not fit is dropped, not cut: %q", got)
	}
	if renderActivity(nil, 80) != "" {
		t.Fatal("no items render nothing")
	}
}

func TestActivityCountsFailedJobsAfterRealExit(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("process group supervision requires Unix")
	}
	manager, err := jobs.NewManager(jobs.ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close jobs: %v", err)
		}
	})
	for _, command := range []string{"exit 0", "exit 3"} {
		id, err := manager.Start(context.Background(), jobs.Spec{Command: []string{"sh", "-c", command}})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err = manager.Wait(ctx, id)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
	s := &session{jobs: manager, status: &statusBoard{}}
	items := s.activityItems()
	if len(items) != 1 || items[0].Key != "jobs.failed" || items[0].Count != 1 || renderActivity(items, 80) != "1 failed" {
		t.Fatalf("activity after one successful and one failed job: %+v", items)
	}
}

func TestActivityLineStripsTerminalSequencesFromProducerLabels(t *testing.T) {
	items := []activityItem{{Key: "x/y", Label: "build\x1b[2J\x1b]52;c;payload\a", Glyph: "•", Count: 2, Priority: 1}}
	if got := renderActivity(items, 80); strings.ContainsAny(got, "\x1b\a") {
		t.Fatalf("control sequences reached the line: %q", got)
	}
}
