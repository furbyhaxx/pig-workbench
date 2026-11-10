package host

import (
	"strings"
	"testing"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
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
	for _, width := range []int{36, 57, 80, 140} {
		row, err := a.renderFrameMessage(sdk.Context{}, map[string]any{"content": text}, sdk.MessageRenderOptions{}, width)
		if err != nil {
			t.Fatal(err)
		}
		if len(row) != 1 || !strings.HasPrefix(row[0], "◆ instructions.steering") || strings.Contains(row[0], "<system-") {
			t.Fatalf("width %d: collapsed row = %q", width, row)
		}
		open, err := a.renderFrameMessage(sdk.Context{}, map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}, sdk.MessageRenderOptions{Expanded: true}, width)
		if err != nil {
			t.Fatal(err)
		}
		last := open[len(open)-1]
		if !strings.Contains(last, "instruction · trusted instruction · seq 7") && width >= 80 {
			t.Fatalf("width %d: provenance = %q", width, last)
		}
		if !strings.Contains(strings.Join(open, "\n"), "more lines") {
			t.Fatalf("width %d: expansion is not bounded", width)
		}
		for _, line := range append(row, open...) {
			if tui.VisibleWidth(line) > width || strings.Contains(line, "\x1b[31m") {
				t.Fatalf("width %d: unfitted or unsanitized line %q", width, line)
			}
		}
	}
	if _, err := a.renderFrameMessage(sdk.Context{}, map[string]any{"content": "not a frame"}, sdk.MessageRenderOptions{}, 80); err == nil {
		t.Fatal("a message that is not a frame must fall back to the host renderer")
	}
}
