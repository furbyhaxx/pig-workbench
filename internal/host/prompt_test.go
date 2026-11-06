package host

import "testing"

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
