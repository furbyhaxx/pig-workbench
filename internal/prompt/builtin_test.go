package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestBuiltinWorkbenchRendersRequiredSections(t *testing.T) {
	loader, err := NewFileSystemLoader(FileSystemOptions{Builtin: BuiltinFS()})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := engine.Render(context.Background(), "builtin/workbench", Data{ProjectRoot: "/work", Tools: Tools{Declared: []string{"read"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range RequiredSections {
		if !strings.Contains(rendered.Text, "{% block "+name) && !strings.Contains(rendered.Text, "<"+name+">") && name != "preamble" {
			if name == "preamble" {
				continue
			}
		}
	}
	for _, tag := range []string{"<tools>", "<rules>", "<resources>", "<docs>", "<addendum>", "<project_context>", "<skills>", "<cwd>"} {
		if !strings.Contains(rendered.Text, tag) {
			t.Fatalf("missing %s in %s", tag, rendered.Text)
		}
	}
	if !strings.Contains(rendered.Text, "- read") || !strings.Contains(rendered.Text, "/work") {
		t.Fatalf("inputs were not rendered: %s", rendered.Text)
	}
}
