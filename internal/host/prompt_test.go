package host

import (
	"context"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/prompt"
)

func TestPromptOverrideStaysOffUnlessForced(t *testing.T) {
	if _, ok := promptOverride(true, "auto", "rendered"); ok {
		t.Fatal("auto replaced the host prompt")
	}
	got, ok := promptOverride(true, "force", "rendered")
	if !ok || got != "rendered" {
		t.Fatalf("force = %q %v", got, ok)
	}
}

func TestWalkReplaceChangesOnlyTheExactServedPrompt(t *testing.T) {
	payload := map[string]any{"messages": []any{map[string]any{"content": "served"}}}
	replaced, n := walkReplace(payload, "served", "served\n\n<mcp_servers>\n</mcp_servers>")
	if n != 1 {
		t.Fatal(n)
	}
	content := replaced.(map[string]any)["messages"].([]any)[0].(map[string]any)["content"]
	if content == "served" {
		t.Fatal("served text was not extended")
	}
	if _, n := walkReplace(payload, "other", "x"); n != 0 {
		t.Fatal("unrelated text changed")
	}
}

func TestPromptEpochIsStableForTheSameText(t *testing.T) {
	if promptEpoch("same") != promptEpoch("same") {
		t.Fatal("epoch changed for identical text")
	}
	if promptEpoch("same") == promptEpoch("other") {
		t.Fatal("different renders shared an epoch")
	}
}

func TestPromptEpochChangesWhenTheToolListChanges(t *testing.T) {
	first := renderEpoch(t, []string{"read"})
	second := renderEpoch(t, []string{"read"})
	changed := renderEpoch(t, []string{"read", "edit"})
	if first != second {
		t.Fatalf("same tools changed epoch %s %s", first, second)
	}
	if first == changed {
		t.Fatal("a tool-list change kept the epoch")
	}
}

func renderEpoch(t *testing.T, tools []string) string {
	t.Helper()
	loader, err := prompt.NewFileSystemLoader(prompt.FileSystemOptions{Builtin: prompt.BuiltinFS()})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := prompt.New(prompt.Options{Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := engine.Render(context.Background(), "builtin/workbench", prompt.Data{Tools: prompt.Tools{Declared: tools}})
	if err != nil {
		t.Fatal(err)
	}
	return promptEpoch(rendered.Text)
}
