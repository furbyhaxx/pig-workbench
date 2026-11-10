package host

import (
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
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
	row := askRow(`Answers to the ask_user form that was left unanswered (Release): {"answers":{"target":{"values":["staging"]},"note":{"custom":"after lunch","note":"not Friday"}}}`)
	lines := row.render(func(_, text string) string { return text }, false, 80)
	want := []string{"● ask · Release · 2 answered", "│ target: staging", "│ note: after lunch (note: not Friday)"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ask row = %q", lines)
	}
}
