package host

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
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

func TestActivityLineStripsTerminalSequencesFromProducerLabels(t *testing.T) {
	items := []activityItem{{Key: "x/y", Label: "build\x1b[2J\x1b]52;c;payload\a", Glyph: "•", Count: 2, Priority: 1}}
	if got := renderActivity(items, 80); strings.ContainsAny(got, "\x1b\a") {
		t.Fatalf("control sequences reached the line: %q", got)
	}
}
