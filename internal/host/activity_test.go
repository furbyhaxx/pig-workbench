package host

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
	"github.com/furbyhaxx/pig-workbench/internal/location"
)

func TestActivityLineDropsLowestPriorityFirstAndNeverCutsAnItem(t *testing.T) {
	items := []activityItem{
		{Key: "state", Text: "Waiting for you", Glyph: "●", Count: 1, Severity: "warning", Priority: 110},
		{Key: "jobs.failed", Label: "failed", Glyph: "✗", Count: 1, Severity: "error", Priority: 90},
		{Key: "jobs", Label: "jobs", Glyph: "⚙", Count: 2, Priority: 80},
		{Key: "peers", Label: "live sessions", Glyph: "⚇", Count: 3, Priority: 60},
		{Key: "skills", Label: "skills", Glyph: "◆", Count: 5, Priority: 40},
	}
	if got := renderActivity(items, 140); got != "Waiting for you  1 failed  2 jobs  3 live sessions  5 skills" {
		t.Fatalf("wide = %q", got)
	}
	if got := renderActivity(items, 49); got != "Waiting for you ✗1 ⚙2 ⚇3 ◆5" {
		t.Fatalf("narrow forms are glyph plus number: %q", got)
	}
	for _, width := range []int{36, 57, 80, 140} {
		line := renderActivity(items, width)
		if tui.VisibleWidth(line) > width {
			t.Fatalf("width %d: %q is %d columns", width, line, tui.VisibleWidth(line))
		}
	}
	at57 := renderActivity(items, 57)
	if !strings.Contains(at57, "Waiting for you") || strings.Contains(at57, "skills") {
		t.Fatalf("at 57 columns the lowest priority drops first: %q", at57)
	}
	if got := renderActivity(items[1:2], 6); got != "✗1" {
		t.Fatalf("a narrow terminal keeps the glyph form: %q", got)
	}
	if got := renderActivity(items[1:2], 1); got != "" {
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
	if len(items) != 2 || items[1].Key != "jobs.failed" || items[1].Count != 1 || renderActivity(items, 80) != "Ready  1 failed" {
		t.Fatalf("activity after one successful and one failed job: %+v", items)
	}
}

func TestActivityLineStripsTerminalSequencesFromProducerLabels(t *testing.T) {
	items := []activityItem{{Key: "x/y", Label: "build\x1b[2J\x1b]52;c;payload\a", Glyph: "•", Count: 2, Priority: 1}}
	if got := renderActivity(items, 80); strings.ContainsAny(got, "\x1b\a") {
		t.Fatalf("control sequences reached the line: %q", got)
	}
}

func TestActivityLeadsWithTheTurnStateAndNamesAnAwayLocation(t *testing.T) {
	origin := t.TempDir()
	away := filepath.Join(origin, "nested")
	if err := os.Mkdir(away, 0o700); err != nil {
		t.Fatal(err)
	}
	locations, err := location.New(context.Background(), origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{status: &statusBoard{}, location: locations, contextFrames: &contextFrameState{views: map[string]string{"instructions.steering/a": "x", "instructions.host/b": "y", "session.date/": "z"}}}
	if got := renderActivity(s.activityItems(), 80); got != "Ready  2 instructions" {
		t.Fatalf("idle at the origin: %q", got)
	}
	s.phase.setWorking(true)
	s.phase.since = time.Now().Add(-125 * time.Second)
	if _, err := locations.Transition(context.Background(), away, false, func(location.Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := renderActivity(s.activityItems(), 80); got != "Working 2m05s  2 instructions  in nested" {
		t.Fatalf("working away from the origin: %q", got)
	}
	s.phase.setCompacting(true)
	if got := renderActivity(s.activityItems(), 40); got != "Compacting ≡2 ⌂" {
		t.Fatalf("narrow while compacting: %q", got)
	}
	s.phase.setCompacting(false)
	s.phase.setWorking(false)
	if got := renderActivity(s.activityItems(), 80); !strings.HasPrefix(got, "Ready") {
		t.Fatalf("after the turn: %q", got)
	}
}

func TestActivityPokeNeverBlocksAndCoalesces(t *testing.T) {
	s := &session{activity: &activityLine{poke: make(chan struct{}, 1)}}
	for range 100 {
		s.pokeActivity()
	}
	if len(s.activity.poke) != 1 {
		t.Fatalf("pending pokes = %d", len(s.activity.poke))
	}
	(&session{}).pokeActivity()
}
