package host

import (
	"context"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/prompt"
)

func TestPromptOverrideResolvesAutoToForceOnStockPiG(t *testing.T) {
	for _, delivery := range []string{"auto", "force"} {
		got, ok := promptOverride(true, delivery, "rendered")
		if !ok || got != "rendered" {
			t.Fatalf("%s = %q %v", delivery, got, ok)
		}
	}
	if _, ok := promptOverride(true, "sections", "rendered"); ok {
		t.Fatal("sections replaced the host prompt without the fork host")
	}
	if _, ok := promptOverride(false, "auto", "rendered"); ok {
		t.Fatal("a disabled prompt replaced the host prompt")
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

func TestRenderErrorDoesNotReplaceTheHostPrompt(t *testing.T) {
	text, force, recorded := applyPromptRender(true, "", errPrompt, "force", "")
	if force || text != "" || recorded == "" {
		t.Fatalf("error path = %q %v %q", text, force, recorded)
	}
	text, force, recorded = applyPromptRender(true, "", errPrompt, "force", "last good")
	if !force || text != "last good" || recorded == "" {
		t.Fatalf("an error after a good render must keep that render and stay reported: %q %v %q", text, force, recorded)
	}
	text, force, recorded = applyPromptRender(true, "rendered", nil, "force", "last good")
	if !force || text != "rendered" || recorded != "" {
		t.Fatalf("success path = %q %v %q", text, force, recorded)
	}
}

var errPrompt = promptRenderError{}

type promptRenderError struct{}

func (promptRenderError) Error() string { return "render failed" }

func TestUnmatchedLateSectionIsDeliveredOnTheNextPromptOfItsBranch(t *testing.T) {
	ledger := frames.NewSession("s")
	scope := frames.DeliveryScope{Epoch: "epoch", BranchLeaf: "leaf-1", Ancestors: map[string]bool{"": true, "leaf-1": true}}
	id, err := admitLate(ledger, scope, "extra")
	if err != nil {
		t.Fatal(err)
	}
	later := frames.DeliveryScope{Epoch: "epoch", BranchLeaf: "leaf-2", Ancestors: map[string]bool{"": true, "leaf-1": true, "leaf-2": true}}
	pending, err := ledger.SelectPending("nextPrompt", later)
	if err != nil || len(pending) != 1 || pending[0].FrameID != id {
		t.Fatalf("late section not deliverable after the leaf advanced: %v %v", pending, err)
	}
	again, err := admitLate(ledger, later, "extra")
	if err != nil || again != id {
		t.Fatalf("repeat request admitted a second frame: %q vs %q (%v)", again, id, err)
	}
	other := frames.DeliveryScope{Epoch: "epoch", BranchLeaf: "x", Ancestors: map[string]bool{"": true, "x": true}}
	if pending, _ := ledger.SelectPending("nextPrompt", other); len(pending) != 0 {
		t.Fatalf("late section leaked to another branch: %v", pending)
	}
}
