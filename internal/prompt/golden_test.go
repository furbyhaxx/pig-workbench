package prompt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestFixtureSectionOrder(t *testing.T) {
	root := filepath.Join("testdata", "host-options")
	loader, err := NewFileSystemLoader(FileSystemOptions{Builtin: BuiltinFS()})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tools", "rules", "resources", "docs", "project_context", "cwd"}
	for _, name := range []string{"f-default", "f-append", "f-hidden", "f-noread", "f-skills", "f-system"} {
		data := fixtureData(t, filepath.Join(root, name+"-options.json"))
		rendered, err := engine.Render(context.Background(), "builtin/workbench", data)
		if err != nil {
			t.Fatal(name, err)
		}
		got := sectionOrder(rendered.Text)
		expect := want
		if name == "f-append" {
			expect = []string{"tools", "rules", "resources", "docs", "addendum", "project_context", "cwd"}
		}
		if stringsJoin(got) != stringsJoin(expect) {
			t.Fatalf("%s order = %v, want %v\n%s", name, got, expect, rendered.Text)
		}
		if name == "f-system" && !contains(rendered.Text, "CUSTOM") && !contains(rendered.Text, data.Host.CustomPrompt) {
			t.Fatalf("%s did not use the custom preamble", name)
		}
	}
}

func fixtureData(t *testing.T, path string) Data {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var options struct {
		Append        string   `json:"appendSystemPrompt"`
		Custom        string   `json:"customPrompt"`
		SelectedTools []string `json:"selectedTools"`
		HiddenTools   []string `json:"hiddenTools"`
		Cwd           string   `json:"cwd"`
		ContextFiles  []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"contextFiles"`
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		t.Fatal(err)
	}
	declared := []string{}
	hidden := map[string]bool{}
	for _, name := range options.HiddenTools {
		hidden[name] = true
	}
	for _, name := range options.SelectedTools {
		if !hidden[name] {
			declared = append(declared, name)
		}
	}
	var instructions []Instruction
	for _, file := range options.ContextFiles {
		instructions = append(instructions, Instruction{Path: file.Path, DisplayPath: file.Path, Content: file.Content})
	}
	return Data{
		ProjectRoot:  options.Cwd,
		Tools:        Tools{Selected: options.SelectedTools, Declared: declared, Hidden: options.HiddenTools},
		Host:         Host{Append: options.Append, CustomPrompt: options.Custom},
		Instructions: instructions,
	}
}

var sectionTag = regexp.MustCompile(`<([a-z_]+)>`)

func sectionOrder(text string) []string {
	var order []string
	for _, match := range sectionTag.FindAllStringSubmatch(text, -1) {
		name := match[1]
		if name == "project_instructions" {
			continue
		}
		order = append(order, name)
	}
	return order
}

func stringsJoin(values []string) string {
	out := ""
	for _, value := range values {
		out += value + ","
	}
	return out
}

func contains(text, part string) bool {
	return part != "" && len(text) >= len(part) && (text == part || len(part) > 0 && (stringIndex(text, part) >= 0))
}

func stringIndex(text, part string) int {
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
