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
