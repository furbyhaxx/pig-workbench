package host

import (
	"fmt"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"

	"github.com/furbyhaxx/pig-workbench/internal/ask"
	"github.com/furbyhaxx/pig-workbench/internal/fileedit"
)

var ownWidths = []int{36, 57, 80, 140}

func init() {
	themePainter = func(sdk.Context) painter { return func(_, text string) string { return text } }
}

// ownResult is what the host hands a renderer for a returned tool result: its text and the Details the tool attached.
func ownResult(t *testing.T, tool string, args map[string]any, result sdk.ToolResult) sdk.ToolRenderResult {
	t.Helper()
	out, err := presented(tool, func(sdk.Context, map[string]any) (any, error) { return result, nil })(sdk.Context{}, args)
	if err != nil {
		t.Fatal(err)
	}
	return sdk.ToolRenderResult{Content: []map[string]any{{"type": "text", "text": result.Content}}, Details: out.(sdk.ToolResult).Details}
}

func ownText(text string) sdk.ToolRenderResult {
	return sdk.ToolRenderResult{Content: []map[string]any{{"type": "text", "text": text}}}
}

func renderOwn(t *testing.T, tool string, args map[string]any, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int) (call, out []string) {
	t.Helper()
	a := &app{session: presentationSession(t)}
	renderers := a.resolveToolRenderer(tool, func() *sdk.ToolRenderers { return nil })
	if renderers == nil {
		t.Fatalf("%s did not resolve through the presentation catalog", tool)
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
		result := ownResult(t, c.tool, c.args, sdk.ToolResult{Content: c.text})
		for _, width := range ownWidths {
			call, out := renderOwn(t, c.tool, c.args, result, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{DurationMs: &duration}, width)
			if len(call) != 1 || !strings.HasPrefix(call[0], strings.TrimSuffix(c.wantCall, " ")) && width >= 57 {
				t.Errorf("%s call at %d = %q, want prefix %q", c.tool, width, call, c.wantCall)
			}
			head := out[0]
			if !strings.HasPrefix(head, "✓ "+c.tool+" · succeeded") {
				t.Errorf("%s result header at %d = %q", c.tool, width, head)
			}
			if width == 140 && !strings.Contains(head, "time 1.25s") {
				t.Errorf("%s header lost its time at 140: %q", c.tool, head)
			}
		}
	}
}

func TestOwnBashShowsTheTailAndFailureStatus(t *testing.T) {
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	text := strings.Join(lines, "\n")
	code := 2
	failed := ownResult(t, "bash", map[string]any{"command": "make"}, sdk.ToolResult{Content: "Job ab12-1: exited (exit 2)\n" + text, IsError: true, Details: jobPage{jobFacts: jobFacts{State: "exited", ExitCode: &code}}})
	_, out := renderOwn(t, "bash", map[string]any{"command": "make"}, failed, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 80)
	joined := strings.Join(out, "\n")
	if !strings.HasPrefix(out[0], "✗ bash · failed · exit 2 · lines 30") || !strings.Contains(joined, "20 earlier lines") || !strings.HasSuffix(joined, "line 30") || strings.Contains(joined, "line 5\n") {
		t.Fatalf("bash tail rendering:\n%s", joined)
	}
	_, expanded := renderOwn(t, "bash", map[string]any{"command": "make"}, failed, sdk.ToolRenderResultOptions{Expanded: true}, sdk.ToolRenderContext{IsError: true}, 80)
	if joined := strings.Join(expanded, "\n"); !strings.Contains(joined, "line 1\n") || !strings.Contains(joined, "Job ab12-1: exited (exit 2)") {
		t.Fatalf("expanded bash does not show the whole result:\n%s", joined)
	}
	_, out = renderOwn(t, "bash", map[string]any{"command": "make"}, ownText(text), sdk.ToolRenderResultOptions{IsPartial: true}, sdk.ToolRenderContext{}, 80)
	if joined := strings.Join(out, "\n"); out[0] != "● bash · running" || !strings.Contains(joined, "20 earlier lines") || !strings.HasSuffix(joined, "line 30") {
		t.Fatalf("streaming bash:\n%s", joined)
	}
}

func TestOwnReadHidesContentUntilExpandedOrFailed(t *testing.T) {
	args := map[string]any{"path": "a.go"}
	result := ownResult(t, "read", args, sdk.ToolResult{Content: "1:aa|one\n2:bb|two\n"})
	_, collapsed := renderOwn(t, "read", args, result, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 80)
	if len(collapsed) != 1 || !strings.Contains(collapsed[0], "lines 2") || strings.Contains(collapsed[0], "a.go") {
		t.Fatalf("collapsed read = %q", collapsed)
	}
	_, expanded := renderOwn(t, "read", args, result, sdk.ToolRenderResultOptions{Expanded: true}, sdk.ToolRenderContext{}, 80)
	if len(expanded) != 3 {
		t.Fatalf("expanded read = %q", expanded)
	}
	_, refused := renderOwn(t, "read", map[string]any{"path": "missing.go"}, ownText("no such file"), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 80)
	if len(refused) != 2 || refused[0] != "✗ read · failed" || refused[1] != "no such file" {
		t.Fatalf("a call the tool refused with an error = %q", refused)
	}
	failed := ownResult(t, "read", args, sdk.ToolResult{Content: "kb://x: provider failed", IsError: true})
	_, out := renderOwn(t, "read", args, failed, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 80)
	if len(out) != 2 || !strings.HasPrefix(out[0], "✗ read · failed") || out[1] != "kb://x: provider failed" {
		t.Fatalf("failed read = %q", out)
	}
}

func TestOwnEditShowsItsDiff(t *testing.T) {
	effects := []fileedit.Effect{{Path: "/w/a.go", Bytes: []byte("one\nTWO\n")}}
	outcomes := []fileedit.Outcome{{Path: "/w/a.go", BeforeRevision: "r1", AfterRevision: "r2", Committed: true}}
	details := map[string]any{presentationBlocksKey: editDiffBlocks(effects, [][]byte{[]byte("one\ntwo\n")}, outcomes)}
	result := ownResult(t, "edit", map[string]any{"path": "a.go", "edits": []any{}}, sdk.ToolResult{Content: "Edit batch committed.\n", Details: details})
	_, out := renderOwn(t, "edit", map[string]any{"path": "a.go"}, result, sdk.ToolRenderResultOptions{Expanded: true}, sdk.ToolRenderContext{}, 80)
	joined := strings.Join(out, "\n")
	if !strings.HasPrefix(out[0], "✓ edit · succeeded · added 1 · removed 1") || !strings.Contains(joined, "-two") || !strings.Contains(joined, "+TWO") || strings.Contains(joined, "Edit batch committed") {
		t.Fatalf("edit row:\n%s", joined)
	}
}

func TestOwnToolsResolveThroughTheCatalogAndForeignToolsDoNot(t *testing.T) {
	a := &app{session: presentationSession(t)}
	for _, name := range ownToolNames {
		if a.resolveToolRenderer(name, func() *sdk.ToolRenderers { return nil }) == nil {
			t.Errorf("%s fell through to the host default", name)
		}
	}
	if a.resolveToolRenderer("web_search", func() *sdk.ToolRenderers { return nil }) != nil {
		t.Error("a tool nobody registered a presentation for must fall through")
	}
}

func TestOwnAskUserRowNamesTheOutcomeAndListsAnswers(t *testing.T) {
	form := map[string]any{"questions": []any{map[string]any{"id": "db", "label": "Database"}, map[string]any{"id": "port", "label": "Port"}}}
	for name, tc := range map[string]struct {
		result ask.Result
		want   []string
	}{
		"cancelled": {ask.Result{Cancelled: true}, []string{"○ ask_user · cancelled"}},
		"timedOut":  {ask.Result{TimedOut: true, Reason: "deadline"}, []string{"✗ ask_user · timedOut", "deadline"}},
		"answered":  {ask.Result{Answers: map[string]ask.Answered{"db": {Values: []string{"postgres"}}, "port": {Custom: "5432"}}}, []string{"✓ ask_user · succeeded · answers 2", "Database | postgres", "Port | 5432"}},
	} {
		_, out := renderOwn(t, "ask_user", form, ownResult(t, "ask_user", form, sdk.ToolResult{Content: "{}", Details: tc.result}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 80)
		joined := strings.Join(out, "\n")
		for _, want := range tc.want {
			if !strings.Contains(joined, want) {
				t.Errorf("%s rendered\n%s\nwant %q", name, joined, want)
			}
		}
	}
}

func TestRowStatusDiffersByGlyphAndTokenAndSurvivesWithoutColour(t *testing.T) {
	plain := themePainter
	themePainter = func(sdk.Context) painter {
		return func(token, text string) string { return "<" + token + ">" + text + "</>" }
	}
	t.Cleanup(func() { themePainter = plain })
	a := &app{session: presentationSession(t)}
	renderers := a.resolveToolRenderer("read", func() *sdk.ToolRenderers { return nil })
	args := map[string]any{"path": "a.go"}
	head := func(result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext) string {
		out, err := renderers.Result(sdk.Context{}, result, options, render, 80)
		if err != nil {
			t.Fatal(err)
		}
		return out[0]
	}
	rows := map[string]string{
		"<success>✓</> read · succeeded":              head(ownResult(t, "read", args, sdk.ToolResult{Content: "1:aa|x\n"}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}),
		"<warning>▲</> read · succeeded · incomplete": head(ownResult(t, "read", args, sdk.ToolResult{Content: "1:aa|x\n", Details: map[string]any{"complete": false}}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}),
		"<error>✗</> read · failed":                   head(ownResult(t, "read", args, sdk.ToolResult{Content: "no", IsError: true}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}),
		"<accent>●</> read · running":                 head(ownText("1:aa|x"), sdk.ToolRenderResultOptions{IsPartial: true}, sdk.ToolRenderContext{}),
	}
	for want, got := range rows {
		if !strings.HasPrefix(got, want) {
			t.Errorf("row %q, want prefix %q", got, want)
		}
		if word := strings.Fields(want)[3]; !strings.Contains(got, word) {
			t.Errorf("row %q lost its status word %q, the only signal without colour", got, word)
		}
	}
}

func TestOwnRowBodiesShowPathsRelativeToTheWorkingLocation(t *testing.T) {
	for tool, text := range map[string]string{
		"write": "Wrote 4 bytes to file:///w/app/notes.txt\nRevision: r1\n",
		"find":  "/w/app/internal/a.go\n/w/app/b.go\n/etc/hosts\n",
		"grep":  "/w/app/internal/a.go:3:1: x\n",
	} {
		entry := builtinRegistration(tool)
		result := ownResult(t, tool, map[string]any{"path": "."}, sdk.ToolResult{Content: text})
		snapshot, err := decodeToolSnapshot(entry.descriptor, result.Details)
		if err != nil {
			t.Fatal(err)
		}
		record := presentationRecord{snapshot: snapshot, valid: true}
		out := renderPresentationResult(entry.descriptor, entry.builtin, result, sdk.ToolRenderResultOptions{Expanded: true}, sdk.ToolRenderContext{}, 80, builtinDisplay(entry, 0), record, "/w/app")
		joined := strings.Join(out[1:], "\n")
		if strings.Contains(joined, "/w/app") || strings.Contains(joined, "file://") {
			t.Errorf("%s body keeps an absolute path below the working location:\n%s", tool, joined)
		}
		if tool == "find" && (!strings.Contains(joined, "internal/a.go\nb.go") || !strings.Contains(joined, "/etc/hosts")) {
			t.Errorf("find body:\n%s", joined)
		}
	}
}

func TestOwnRowsStateOnlyDurationsWorthReading(t *testing.T) {
	result := ownResult(t, "ls", map[string]any{"path": "."}, sdk.ToolResult{Content: "a.go\n"})
	quick, slow := int64(12), int64(2500)
	_, out := renderOwn(t, "ls", map[string]any{"path": "."}, result, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{DurationMs: &quick}, 80)
	if strings.Contains(out[0], "time") {
		t.Errorf("a 12 ms call states its time: %q", out[0])
	}
	_, out = renderOwn(t, "ls", map[string]any{"path": "."}, result, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{DurationMs: &slow}, 80)
	if !strings.Contains(out[0], "time 2.50s") {
		t.Errorf("a 2.5 s call lost its time: %q", out[0])
	}
}
