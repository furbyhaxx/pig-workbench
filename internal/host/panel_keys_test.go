package host

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func must(result sdk.RemoteComponentResult, err error) sdk.RemoteComponentResult {
	if err != nil {
		panic(err)
	}
	return result
}

// Panels act on alt+<letter> only: a plain printable key must change nothing and must not close the panel.
func TestPlainPrintableKeysDoNothingInActionPanels(t *testing.T) {
	height := func() int { return 24 }
	calls := 0
	jobs := &jobsPanel{rows: func() []jobRow { return []jobRow{{id: "j1"}} }, act: func(string, string) (string, error) { calls++; return "", nil }, height: height}
	instructions := &instructionsPanel{
		pins:   func() []instructionPin { return []instructionPin{{Path: "/a"}} },
		rules:  func() (string, error) { calls++; return "", nil },
		pin:    func(string) (string, error) { calls++; return "", nil },
		unpin:  func(string) (string, error) { calls++; return "", nil },
		height: height,
	}
	sessions := &sessionsPanel{rows: func() ([]peerRow, bool, error) { return []peerRow{{key: "peer"}}, false, nil }, act: func(string, string) (string, error) { calls++; return "", nil }, height: height}
	trust := &trustPanel{rules: func() ([]string, []string) { return []string{"allow /a"}, nil }, edit: func(string, string) error { calls++; return nil }, height: height}
	array := &arrayEditor{items: []string{"one"}}
	for key := byte(0x20); key < 0x7f; key++ {
		data := string(key)
		for name, done := range map[string]bool{
			"jobs":         must(jobs.HandleInput(data)).Done,
			"instructions": must(instructions.HandleInput(data)).Done,
			"sessions":     must(sessions.HandleInput(data)).Done,
			"trust":        must(trust.HandleInput(data)).Done,
		} {
			if done {
				t.Fatalf("%q closed the %s panel", data, name)
			}
		}
		array.handleInput(data)
	}
	if calls != 0 || jobs.confirm != "" || instructions.mode != "" || sessions.composing || trust.adding || len(array.items) != 1 {
		t.Fatalf("a plain key acted: calls=%d confirm=%q mode=%q composing=%v adding=%v items=%v", calls, jobs.confirm, instructions.mode, sessions.composing, trust.adding, array.items)
	}
}

func TestAltLetterRunsThePanelAction(t *testing.T) {
	trust := &trustPanel{rules: func() ([]string, []string) { return nil, nil }, height: func() int { return 24 }}
	must(trust.HandleInput("\x1ba"))
	if !trust.adding {
		t.Fatal("alt+a did not start adding a trust rule")
	}
	jobs := &jobsPanel{rows: func() []jobRow { return []jobRow{{id: "j1"}} }, height: func() int { return 24 }}
	must(jobs.HandleInput("\x1bk"))
	if jobs.confirm != "j1" {
		t.Fatalf("alt+k did not ask to stop the selected job: %q", jobs.confirm)
	}
}

func TestPanelHintsFollowARemappedKey(t *testing.T) {
	previous := tui.GetKeybindings()
	t.Cleanup(func() { tui.SetKeybindings(previous) })
	tui.SetKeybindings(tui.NewKeybindingsManager(tui.TUIKeybindings, tui.KeybindingsConfig{"tui.select.confirm": {"ctrl+j"}, "tui.select.cancel": {"ctrl+q"}}))
	jobs := &jobsPanel{rows: func() []jobRow { return []jobRow{{id: "j1"}} }, height: func() int { return 24 }}
	encoded, err := json.Marshal(jobs.View(140))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ctrl+j open", "ctrl+q close", "alt+k stop"} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("hint lacks %q: %s", want, encoded)
		}
	}
}

func TestNarrowPanelHintKeepsEveryAction(t *testing.T) {
	hint := "alt+m message · alt+t trust · alt+b block · alt+h hide/show · escape close"
	lines := hintLines(hint, 36)
	if joined := strings.Join(lines, " · "); joined != hint || len(lines) < 2 {
		t.Fatalf("hint parts were lost or not wrapped: %q", lines)
	}
	for _, line := range lines {
		if tui.VisibleWidth(line) > 34 {
			t.Errorf("line wider than the panel: %q", line)
		}
	}
	if got := hintLines(hint, 140); len(got) != 1 {
		t.Errorf("a wide panel wraps the hint: %q", got)
	}
}
