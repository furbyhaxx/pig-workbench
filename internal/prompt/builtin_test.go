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
	for _, tag := range []string{"<tools>", "<rules>", "<resources>", "<docs>", "<project_context>", "<cwd>"} {
		if !strings.Contains(rendered.Text, tag) {
			t.Fatalf("missing %s in %s", tag, rendered.Text)
		}
	}
	if !strings.Contains(rendered.Text, "- read") || !strings.Contains(rendered.Text, "/work") {
		t.Fatalf("inputs were not rendered: %s", rendered.Text)
	}
}

func TestBuiltinWorkbenchCarriesHostInputs(t *testing.T) {
	loader, err := NewFileSystemLoader(FileSystemOptions{Builtin: BuiltinFS()})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	data := Data{
		ProjectRoot: "/work",
		Tools: Tools{
			Declared:   []string{"edit", "read"},
			Hidden:     []string{"secret_tool"},
			Snippets:   map[string]string{"read": "Read files", "edit": "Edit files"},
			Guidelines: map[string][]string{"read": {"Read before editing"}, "secret_tool": {"HIDDEN-GUIDELINE"}},
		},
		Host: Host{
			PromptGuidelines: []string{"PROMPT-GUIDELINE"},
			Skills: []HostSkill{
				{Name: "alpha", Description: "first skill", FilePath: "/skills/alpha/SKILL.md"},
				{Name: "off", Description: "manual only", FilePath: "/skills/off/SKILL.md", DisableModelInvocation: true},
			},
		},
		Workbench: Workbench{Edit: EditFeatures{Mode: "hashline"}, Schemes: []Scheme{{Name: "docs"}, {Name: "file"}}},
	}
	rendered, err := engine.Render(context.Background(), "builtin/workbench", data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- read: Read files", "- edit: Edit files", "- Read before editing", "- PROMPT-GUIDELINE", "Be concise", "tagged {hashline} batches", "docs://, file://"} {
		if !strings.Contains(rendered.Text, want) {
			t.Errorf("missing %q in:\n%s", want, rendered.Text)
		}
	}
	for _, unwanted := range []string{"HIDDEN-GUIDELINE", "manual only", "secret_tool"} {
		if strings.Contains(rendered.Text, unwanted) {
			t.Errorf("rendered %q despite it being hidden:\n%s", unwanted, rendered.Text)
		}
	}
	data.Workbench.Edit.Mode = "replace"
	rendered, err = engine.Render(context.Background(), "builtin/workbench", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Text, "exact-text {path, edits:[{oldText,newText}]}") || strings.Contains(rendered.Text, "tagged {hashline} batches in this session") {
		t.Errorf("replace mode rule is wrong:\n%s", rendered.Text)
	}
}

func TestBuiltinWorkbenchNeverListsSkills(t *testing.T) {
	loader, err := NewFileSystemLoader(FileSystemOptions{Builtin: BuiltinFS()})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	host := Host{Skills: []HostSkill{{Name: "alpha", Description: "routing text", FilePath: "/s/alpha/SKILL.md"}}}
	rendered, err := engine.Render(context.Background(), "builtin/workbench", Data{ProjectRoot: "/work", Host: host})
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range []string{"<skills>", "alpha", "routing text", "SKILL.md"} {
		if strings.Contains(rendered.Text, listed) {
			t.Errorf("the system prompt carries %q; skills are listed by the session.skills frame only", listed)
		}
	}
}
