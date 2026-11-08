package host

import (
	"fmt"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

const webSearchDescriptor = `{"toolNames":["web_search"],"label":"Web search","presentationVersion":1,"preset":"search","defaultDisplay":{"density":"normal","previewLines":6},"callFields":[{"pointer":"/query","label":"query","format":"plain"}],"metricDefinitions":[{"id":"results","label":"results","unit":"count"}]}`

func webSearchRenderers(t *testing.T) *sdk.ToolRenderers {
	t.Helper()
	a := &app{}
	s := presentationSession(t)
	registerPresentation(t, a, s, "pig-web", "i-1", "web_search", webSearchDescriptor)
	a.session = s
	return a.resolveToolRenderer("web_search", func() *sdk.ToolRenderers { return nil })
}

func webSearchDetails(snapshot map[string]any) map[string]any {
	base := map[string]any{"version": 1, "revision": 1, "phase": "settled", "outcome": "succeeded", "title": "Web search", "target": map[string]any{"kind": "label", "value": "go generics"}, "complete": true}
	for key, value := range snapshot {
		base[key] = value
	}
	return map[string]any{"workbenchPresentation": base}
}

func TestWebSearchRendersTypedURICellsAtEveryWidth(t *testing.T) {
	renderers := webSearchRenderers(t)
	long := "https://example.test/" + strings.Repeat("deep/path/", 12) + "page"
	details := webSearchDetails(map[string]any{
		"metrics": []any{map[string]any{"id": "results", "value": 2, "unit": "count"}},
		"blocks": []any{map[string]any{"kind": "table", "table": map[string]any{
			"columns": []any{map[string]any{"id": "title", "label": "Title", "cell": "text"}, map[string]any{"id": "url", "label": "URL", "cell": "uri"}},
			"rows": []any{
				[]any{map[string]any{"kind": "text", "value": "Generics tutorial"}, map[string]any{"kind": "uri", "value": long}},
				[]any{map[string]any{"kind": "text", "value": "Second hit"}, map[string]any{"kind": "uri", "value": "https://example.test/a"}},
			},
		}}},
	})
	for _, width := range []int{36, 57, 80, 140} {
		lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, width)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(lines, "\n")
		for _, line := range lines {
			if tui.VisibleWidth(line) > width {
				t.Errorf("width %d overflows: %q", width, line)
			}
		}
		if strings.ContainsAny(joined, "\x1b\x07") {
			t.Errorf("width %d leaked a terminal control byte: %q", width, joined)
		}
		// The one-line header truncates with an ellipsis; metrics must survive wherever they fit.
		if !strings.Contains(joined, "Web search") || width >= 57 && !strings.Contains(joined, "results 2") {
			t.Errorf("width %d lost the header facts:\n%s", width, joined)
		}
	}
	call, err := renderers.Call(sdk.Context{}, map[string]any{"query": "go generics"}, sdk.ToolRenderContext{}, 80)
	if err != nil || len(call) != 1 || !strings.Contains(call[0], "go generics") {
		t.Fatalf("call row = %q, %v", call, err)
	}
}

func TestPresentationHostileTerminalBytesNeverReachTheRow(t *testing.T) {
	renderers := webSearchRenderers(t)
	details := webSearchDetails(map[string]any{"blocks": []any{map[string]any{"kind": "text", "text": "hit \x1b]0;pwned\x07 title"}}})
	raw := []map[string]any{{"type": "text", "text": "raw \x1b[31mred\x1b[0m result"}}
	for _, width := range []int{36, 80} {
		lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Content: raw, Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, width)
		if err != nil {
			t.Fatal(err)
		}
		if joined := strings.Join(lines, "\n"); strings.ContainsAny(joined, "\x1b\x07") || !strings.Contains(joined, "result") {
			t.Errorf("width %d: %q", width, lines)
		}
	}
}

func TestPresentationPlainRowPerLifecycleState(t *testing.T) {
	renderers := webSearchRenderers(t)
	cases := []struct {
		name    string
		details map[string]any
		options sdk.ToolRenderResultOptions
		render  sdk.ToolRenderContext
		want    []string
		without []string
	}{
		{"preparing", webSearchDetails(map[string]any{"phase": "preparing", "outcome": nil, "complete": nil}), sdk.ToolRenderResultOptions{IsPartial: true}, sdk.ToolRenderContext{}, []string{"preparing", "partial output"}, []string{"succeeded"}},
		{"running", webSearchDetails(map[string]any{"phase": "running", "outcome": nil, "complete": nil}), sdk.ToolRenderResultOptions{IsPartial: true}, sdk.ToolRenderContext{}, []string{"running", "partial output"}, []string{"succeeded"}},
		{"succeeded but incomplete", webSearchDetails(map[string]any{"complete": false}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, []string{"succeeded", "incomplete"}, nil},
		{"failed after partial commit", webSearchDetails(map[string]any{"outcome": "failed", "effects": "partial", "error": map[string]any{"code": "upstream", "message": "search backend refused"}}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, []string{"failed", "partial effects", "upstream: search backend refused"}, []string{"succeeded"}},
		{"cancelled", webSearchDetails(map[string]any{"outcome": "cancelled"}), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, []string{"cancelled"}, []string{"succeeded"}},
		{"producer success cannot hide a host error", webSearchDetails(nil), sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, []string{"failed (host)"}, []string{"· succeeded"}},
	}
	for _, c := range cases {
		for key, value := range c.details["workbenchPresentation"].(map[string]any) {
			if value == nil {
				delete(c.details["workbenchPresentation"].(map[string]any), key)
			}
		}
		lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: c.details}, c.options, c.render, 80)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		joined := strings.Join(lines, "\n")
		for _, want := range c.want {
			if !strings.Contains(joined, want) {
				t.Errorf("%s lacks %q:\n%s", c.name, want, joined)
			}
		}
		for _, unwanted := range c.without {
			if strings.Contains(joined, unwanted) {
				t.Errorf("%s must not show %q:\n%s", c.name, unwanted, joined)
			}
		}
	}
}

func TestPresentationUnusableDetailsKeepTheRawResult(t *testing.T) {
	renderers := webSearchRenderers(t)
	raw := []map[string]any{{"type": "text", "text": "1 result: Fixture result"}}
	for name, details := range map[string]any{
		"absent":              nil,
		"unsupported version": map[string]any{"workbenchPresentation": map[string]any{"version": 99, "phase": "settled"}},
		"malformed":           map[string]any{"workbenchPresentation": "not an object"},
	} {
		lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Content: raw, Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 80)
		if err != nil || !strings.Contains(strings.Join(lines, "\n"), "Fixture result") {
			t.Errorf("%s lost the raw result: %q, %v", name, lines, err)
		}
	}
}

func TestEveryPresetAndStateFitsEveryWidthWithBoundedHeight(t *testing.T) {
	long := "https://example.test/" + strings.Repeat("deep/path/", 12) + "page"
	wide := "日本語のとても長い見出し " + strings.Repeat("wide ", 30)
	blocks := []any{
		map[string]any{"kind": "text", "text": strings.Repeat(wide+"\n", 30), "role": "error"},
		map[string]any{"kind": "code", "text": strings.Repeat("go build ./...\n", 40), "language": "sh"},
		map[string]any{"kind": "notice", "text": wide, "severity": "warning"},
		map[string]any{"kind": "table", "table": map[string]any{
			"columns": []any{map[string]any{"id": "u", "label": "URL", "cell": "uri"}},
			"rows":    []any{[]any{map[string]any{"kind": "uri", "value": long}}},
		}},
	}
	states := map[string]map[string]any{
		"preparing": {"phase": "preparing", "outcome": nil, "complete": nil},
		"running":   {"phase": "running", "outcome": nil, "complete": nil},
		"succeeded": {},
		"failed":    {"outcome": "failed", "effects": "partial", "error": map[string]any{"code": "upstream", "message": wide}},
		"cancelled": {"outcome": "cancelled"},
		"timedOut":  {"outcome": "timedOut"},
		"denied":    {"outcome": "denied"},
		"unknown":   {"outcome": "unknown", "effects": "unknown"},
	}
	for _, preset := range []string{"generic", "text", "fileRead", "fileEdit", "search", "process", "questionnaire", "composite"} {
		for _, density := range []string{"compact", "normal", "full"} {
			descriptor := fmt.Sprintf(`{"toolNames":["build_run"],"label":"Matrix","presentationVersion":1,"preset":%q,"defaultDisplay":{"density":%q,"previewLines":5},"callFields":[{"pointer":"/target","label":"target","format":"path"}],"metricDefinitions":[]}`, preset, density)
			a := &app{}
			s := presentationSession(t)
			registerPresentation(t, a, s, "build-tools", "i-1", "build_run", descriptor)
			a.session = s
			renderers := a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return nil })
			if renderers == nil {
				t.Fatalf("%s: no renderer", preset)
			}
			for name, state := range states {
				snapshot := map[string]any{"title": wide, "target": map[string]any{"kind": "uri", "value": long}, "blocks": blocks}
				for key, value := range state {
					snapshot[key] = value
				}
				details := webSearchDetails(snapshot)
				for key, value := range details["workbenchPresentation"].(map[string]any) {
					if value == nil {
						delete(details["workbenchPresentation"].(map[string]any), key)
					}
				}
				for _, width := range []int{36, 57, 80, 140} {
					lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{IsPartial: name == "running"}, sdk.ToolRenderContext{}, width)
					if err != nil {
						t.Fatalf("%s/%s/%s/%d: %v", preset, density, name, width, err)
					}
					if strings.Contains(strings.Join(lines, "\n"), "presentation unavailable") {
						t.Fatalf("%s/%s/%s/%d: fixture fell back to the raw view: %q", preset, density, name, width, lines)
					}
					if len(lines) == 0 || len(lines) > 60 {
						t.Errorf("%s/%s/%s/%d: %d rows", preset, density, name, width, len(lines))
					}
					for _, line := range lines {
						if tui.VisibleWidth(line) > width || strings.ContainsAny(line, "\x1b\x07") {
							t.Errorf("%s/%s/%s/%d: bad row %q", preset, density, name, width, line)
						}
					}
				}
			}
		}
	}
}
