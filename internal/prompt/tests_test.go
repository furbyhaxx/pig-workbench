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
