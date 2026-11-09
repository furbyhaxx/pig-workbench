package host

import (
	"fmt"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

var ownWidths = []int{36, 57, 80, 140}

func ownText(text string) sdk.ToolRenderResult {
	return sdk.ToolRenderResult{Content: []map[string]any{{"type": "text", "text": text}}}
}

func renderOwn(t *testing.T, tool string, args map[string]any, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int) (call, out []string) {
	t.Helper()
	renderers := ownToolRenderers(tool)
	if renderers == nil {
		t.Fatalf("%s has no own renderer", tool)
	}
	render.Args, render.ArgsComplete = args, true
	call, err := renderers.Call(sdk.Context{}, args, render, width)
	if err != nil {
		t.Fatal(err)
	}
	out, err = renderers.Result(sdk.Context{}, result, options, render, width)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range append(append([]string(nil), call...), out...) {
		if tui.VisibleWidth(line) > width {
			t.Errorf("%s at %d overflows: %q", tool, width, line)
		}
		if strings.ContainsAny(line, "\x1b\x07") {
			t.Errorf("%s at %d leaked a control byte: %q", tool, width, line)
		}
	}
	return call, out
}

func TestOwnToolsShareOneRowGrammar(t *testing.T) {
	duration := int64(1250)
	long := strings.Repeat("deep/path/", 12) + "main.go"
	cases := []struct {
		tool, text string
		args       map[string]any
		wantCall   string
	}{
		{"bash", "ok\n", map[string]any{"command": "go test ./..."}, "bash · go test ./..."},
		{"jobs", "state: running\n", map[string]any{"action": "wait", "id": "job-1"}, "jobs · wait · job-1"},
		{"write", "Wrote 4 bytes\nRevision: r1\n", map[string]any{"path": "notes.txt", "content": "text"}, "write · notes.txt"},
		{"read", "1:abc|x\n", map[string]any{"path": long}, "read · "},
		{"grep", "a.go:3: x\n", map[string]any{"pattern": "TODO", "glob": "*.go"}, "grep · TODO"},
		{"ls", "a.go\nb.go\n", map[string]any{"path": "internal"}, "ls · internal"},
		{"find", "a.go\n", map[string]any{"pattern": "*.go", "path": "."}, "find · *.go"},
		{"edit", "Applied\n", map[string]any{"hashline": "[a.go#1234]\n"}, "edit · "},
		{"cd", "Working location\n", map[string]any{"reset": true}, "cd · (reset)"},
		{"ask_user", "{}", map[string]any{"questions": []any{1, 2}}, "ask_user · 2 questions"},
	}
	for _, c := range cases {
		for _, width := range ownWidths {
			call, out := renderOwn(t, c.tool, c.args, ownText(c.text), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{DurationMs: &duration}, width)
			if len(call) != 1 || !strings.HasPrefix(call[0], strings.TrimSuffix(c.wantCall, " ")) && width >= 57 {
				t.Errorf("%s call at %d = %q, want prefix %q", c.tool, width, call, c.wantCall)
			}
			head := out[0]
			if !strings.HasPrefix(head, c.tool) {
				t.Errorf("%s result header at %d = %q", c.tool, width, head)
			}
			if width == 140 && (!strings.HasPrefix(head, c.tool+" · succeeded") || !strings.Contains(head, "time 1.25s")) {
				t.Errorf("%s header lost status or time at 140: %q", c.tool, head)
			}
		}
	}
}

func TestOwnBashShowsTheTailAndFailureStatus(t *testing.T) {
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	_, out := renderOwn(t, "bash", map[string]any{"command": "make"}, ownText(strings.Join(lines, "\n")), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 80)
	joined := strings.Join(out, "\n")
	if !strings.Contains(out[0], "failed") || !strings.Contains(joined, "20 earlier lines") || !strings.Contains(joined, "line 30") || strings.Contains(joined, "line 5\n") {
		t.Fatalf("bash tail rendering:\n%s", joined)
	}
	_, out = renderOwn(t, "bash", map[string]any{"command": "make"}, ownText(strings.Join(lines, "\n")), sdk.ToolRenderResultOptions{IsPartial: true}, sdk.ToolRenderContext{}, 80)
	if !strings.Contains(out[0], "running") {
		t.Fatalf("partial bash header = %q", out[0])
	}
}

func TestOwnReadHidesContentUntilExpandedOrFailed(t *testing.T) {
	result := ownText("1:aa|one\n2:bb|two\n")
	_, collapsed := renderOwn(t, "read", map[string]any{"path": "a.go"}, result, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 80)
	if len(collapsed) != 1 || !strings.Contains(collapsed[0], "lines 2") {
		t.Fatalf("collapsed read = %q", collapsed)
	}
	_, expanded := renderOwn(t, "read", map[string]any{"path": "a.go"}, result, sdk.ToolRenderResultOptions{Expanded: true}, sdk.ToolRenderContext{}, 80)
	if len(expanded) != 3 {
		t.Fatalf("expanded read = %q", expanded)
	}
	_, failed := renderOwn(t, "read", map[string]any{"path": "missing.go"}, ownText("no such file"), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 80)
	if len(failed) != 2 || !strings.Contains(failed[0], "failed") || failed[1] != "no such file" {
		t.Fatalf("failed read = %q", failed)
	}
}

func TestResolverServesOwnToolsBeforeAnyContributedPresentation(t *testing.T) {
	a := &app{}
	for name := range ownToolStyles {
		if a.resolveToolRenderer(name, func() *sdk.ToolRenderers { return nil }) == nil {
			t.Errorf("%s fell through to the host default", name)
		}
	}
	if a.resolveToolRenderer("web_search", func() *sdk.ToolRenderers { return nil }) != nil {
		t.Error("a foreign tool must not receive an own renderer")
	}
}

func TestOwnAskUserStatusNamesTheOutcome(t *testing.T) {
	for text, want := range map[string]string{
		`{"cancelled":true}`:                                "cancelled",
		`{"timedOut":true,"reason":"deadline"}`:             "timedOut",
		`{"answers":{"a":{"value":"x"},"b":{"value":"y"}}}`: "answered 2",
	} {
		_, out := renderOwn(t, "ask_user", map[string]any{"questions": []any{1}}, ownText(text), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 80)
		if len(out) != 1 || !strings.Contains(out[0], want) {
			t.Errorf("%s rendered %q, want %q", text, out, want)
		}
	}
}
