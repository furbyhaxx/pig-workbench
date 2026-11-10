package host

import (
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/ask"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
)

func TestFrameMessageRendersOneFittedRowAndABoundedExpansion(t *testing.T) {
	body := "Instructions from: /repo/AGENTS.md\n" + strings.Repeat("- a rule that is long enough to need wrapping at narrow widths \x1b[31mred\x1b[0m\n", 60)
	text, err := frames.EncodeEnvelope(frames.Frame{EventKey: "k", Kind: "instruction", Source: "instructions.steering", Body: body, Trust: "trusted", Authority: "instruction", Sequence: 7, Nonce: "ABCD", CreatedAt: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	a := &app{}
	plain := func(_, text string) string { return text }
	for _, width := range []int{36, 57, 80, 140} {
		row, err := a.frameRowFor(text, width)
		if err != nil {
			t.Fatal(err)
		}
		closed := row.render(plain, false, width)
		if len(closed) != 1 || !strings.HasPrefix(closed[0], "◆ instructions.steering · ") || (width >= 80 && !strings.Contains(closed[0], "/repo/AGENTS.md · loaded")) || strings.Contains(closed[0], "<system-") {
			t.Fatalf("width %d: collapsed row = %q", width, closed)
		}
		if width >= 80 && !strings.Contains(closed[0], "· 60 lines · ") {
			t.Fatalf("width %d: no line count: %q", width, closed[0])
		}
		open := row.render(plain, true, width)
		if last := open[len(open)-1]; width >= 80 && !strings.Contains(last, "instruction · trusted instruction · seq 7") {
			t.Fatalf("width %d: provenance = %q", width, last)
		}
		if !strings.Contains(strings.Join(open, "\n"), "20 more lines") {
			t.Fatalf("width %d: expansion is not bounded", width)
		}
		for _, line := range append(closed, open...) {
			if tui.VisibleWidth(line) > width || strings.Contains(line, "\x1b[31m") {
				t.Fatalf("width %d: unfitted or unsanitized line %q", width, line)
			}
		}
	}
	if _, err := a.frameRowFor("not a frame", 80); err == nil {
		t.Fatal("a message that is not a frame must fall back to the host renderer")
	}
}

func TestAskRowListsEveryAnswerOnItsOwnLineWithoutExpansion(t *testing.T) {
	form := ask.Form{Title: "Release", Questions: []ask.Question{{ID: "target", Label: "Deploy target"}, {ID: "when", Label: "Timing"}}}
	text := recoveredAnswers(form, ask.Result{Answers: map[string]ask.Answered{"target": {Values: []string{"staging"}}, "when": {Custom: "after lunch", Note: "not Friday"}}})
	if !strings.Contains(text, "\nDeploy target [target]: staging\n") {
		t.Fatalf("model text = %q", text)
	}
	row := askRow(text)
	lines := row.render(func(_, text string) string { return text }, false, 80)
	want := []string{"● ask · Release · 2 answered", "│ Deploy target: staging", "│ Timing: after lunch (note: not Friday)"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ask row = %q", lines)
	}
}

func TestContextFramesSummarizeInPlainWords(t *testing.T) {
	at := time.Date(2026, 11, 10, 12, 26, 0, 0, time.UTC)
	for source, c := range map[string][2]string{
		"session.date":         {`{"date":"2026-11-10","timezone":"CET +01:00"}`, "2026-11-10 13:26 CET"},
		"session.environment":  {`{"architecture":"amd64","hostCwd":"/x","os":"linux"}`, "linux/amd64"},
		"session.project":      {`{"id":"global","name":"Global","slug":"global","root":"/x"}`, "Global (global)"},
		"session.model":        {`{"provider":"claude","id":"fable","thinking":"high","contextWindow":425000}`, "claude/fable:high · 425k context"},
		"session.capabilities": {`{"activeTools":["a","b"],"schemes":["file"],"skills":[{"name":"x"}],"editMode":"hashline"}`, "2 tools, 1 schemes, 1 skills, hashline edits"},
	} {
		if got := contextSummary(frames.Envelope{Source: source, Body: c[0], CreatedAt: at}); got != c[1] {
			t.Errorf("%s = %q, want %q", source, got, c[1])
		}
	}
}

func TestDateFrameUsesTheConfiguredZone(t *testing.T) {
	// 23:30 UTC is already the next day in Tokyo and still the previous day in Honolulu.
	now := time.Date(2026, 11, 10, 23, 30, 0, 0, time.UTC)
	for zone, want := range map[string][2]string{
		"Asia/Tokyo":       {"2026-11-11", "JST +09:00"},
		"Pacific/Honolulu": {"2026-11-10", "HST -10:00"},
		"UTC":              {"2026-11-10", "UTC +00:00"},
	} {
		facts, _ := dateFacts(zone, now).(map[string]any)
		if facts["date"] != want[0] || facts["timezone"] != want[1] {
			t.Errorf("%s: got %v, want %v", zone, facts, want)
		}
	}
	if system, _ := dateFacts("system", now).(map[string]any); system["date"] != now.Local().Format("2006-01-02") {
		t.Errorf("system zone: %v", system)
	}
	if got := dateFacts("Mars/Olympus", now); got != dateUnavailable {
		t.Errorf("an unknown zone must give no date, got %v", got)
	}
	body := `{"date":"2026-11-11","timezone":"JST +09:00"}`
	if got := contextSummary(frames.Envelope{Source: "session.date", Body: body, CreatedAt: now}); got != "2026-11-11 08:30 JST" {
		t.Errorf("row = %q", got)
	}
	if got := contextSummary(frames.Envelope{Source: "session.date", Body: dateUnavailable, CreatedAt: now}); got != dateUnavailable {
		t.Errorf("row for an unknown zone = %q", got)
	}
}

func TestSkillMessageRendersOneRowWithItsBodyBehindExpansion(t *testing.T) {
	plain := func(_, text string) string { return text }
	row := skillRow("Skill rust loaded from /skills/rust\nline one\nline two\n")
	closed := row.render(plain, false, 80)
	if len(closed) != 1 || !strings.HasPrefix(closed[0], "◆ skill · Skill rust loaded from /skills/rust · 2 lines") {
		t.Fatalf("collapsed skill row = %q", closed)
	}
	if open := row.render(plain, true, 80); len(open) != 3 || !strings.Contains(open[1], "line one") {
		t.Fatalf("expanded skill row = %q", open)
	}
}

func TestFrameRowStylesChangeOnlyTheBodyFraming(t *testing.T) {
	plain := func(_, text string) string { return text }
	row := frameRow{glyph: "i", label: "session.date", facts: []string{"today"}, body: []string{"a", "b"}, open: true}
	rail := row.render(plain, false, 60)
	row.style = "row"
	bare := row.render(plain, false, 60)
	row.style = "card"
	card := row.render(plain, false, 60)
	if rail[0] != bare[0] || rail[0] != card[0] {
		t.Fatalf("headers differ: %q %q %q", rail[0], bare[0], card[0])
	}
	if rail[1] != "│ a" || bare[1] != "  a" || len(card) != len(rail)+1 || !strings.HasPrefix(card[len(card)-1], "└─") {
		t.Fatalf("rail=%q row=%q card=%q", rail, bare, card)
	}
	for _, lines := range [][]string{rail, bare, card} {
		for _, line := range lines {
			if tui.VisibleWidth(line) > 60 {
				t.Fatalf("line overflows: %q", line)
			}
		}
	}
}
