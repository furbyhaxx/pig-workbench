package prompt

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFamilyGlobAndRegexMatchAndMiss(t *testing.T) {
	loader, err := NewFileSystemLoader(FileSystemOptions{Builtin: fstest.MapFS{
		"builtin/workbench": {Data: []byte("{% if model is family('claude') %}family{% endif %}{% if model.id is glob('gpt-5*') %}glob{% endif %}{% if model.id is regex('^deepseek-(v|r)[0-9]') %}regex{% endif %}")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	hit, err := engine.Render(context.Background(), "builtin/workbench", Data{Model: Model{ID: "gpt-5.4", Family: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hit.Text, "family") || !strings.Contains(hit.Text, "glob") || strings.Contains(hit.Text, "regex") {
		t.Fatalf("hit = %q", hit.Text)
	}
	miss, err := engine.Render(context.Background(), "builtin/workbench", Data{Model: Model{ID: "deepseek-v3", Family: "deepseek"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(miss.Text, "family") || strings.Contains(miss.Text, "glob") || !strings.Contains(miss.Text, "regex") {
		t.Fatalf("miss = %q", miss.Text)
	}
}

func TestHasToolAndVariantSelectTextPerModelAndLoadout(t *testing.T) {
	loader := memoryLoader{
		"main":                   `{% if has_tool("edit") and not has_tool("apply_patch") %}use edit{% else %}no edit{% endif %}|{% include variant("tone.md") %}`,
		"tone.md":                "shared tone",
		"family/claude/tone.md":  "claude tone",
		"family/claude/other.md": "unused",
		"bad":                    `{% include variant("absent.md") %}`,
		"badtool":                `{{ has_tool(1) }}`,
	}
	engine := newTestEngine(t, loader, Options{})
	render := func(name, family string, declared ...string) (string, error) {
		out, err := engine.Render(context.Background(), name, Data{Model: Model{Family: family}, Tools: Tools{Declared: declared}})
		return out.Text, err
	}
	for _, test := range []struct {
		family, want string
		tools        []string
	}{
		{"claude", "use edit|claude tone", []string{"edit"}},
		{"gpt", "use edit|shared tone", []string{"edit"}},
		{"", "no edit|shared tone", []string{"read"}},
		{"claude", "no edit|claude tone", []string{"edit", "apply_patch"}},
	} {
		if got, err := render("main", test.family, test.tools...); err != nil || got != test.want {
			t.Errorf("family %q tools %v: got %q, %v; want %q", test.family, test.tools, got, err, test.want)
		}
	}
	if _, err := render("bad", "claude"); err == nil {
		t.Error("variant of a template that exists nowhere must fail, not render empty")
	}
	if _, err := render("badtool", "claude"); err == nil {
		t.Error("has_tool with a non-string name must fail")
	}
}
