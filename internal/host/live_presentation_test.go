//go:build integration

package host

import (
	"fmt"
	"strings"
	"testing"
)

func TestLivePresentationOwnerCannotClaimAnotherExtensionsTool(t *testing.T) {
	fixture := startRPCWithExtensions(t, t.TempDir(), true, []string{"reference-producer"})
	fixture.send("claim", "prompt", "/reference claim-other")
	event := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), "ownershipProbe")
	})
	if !strings.Contains(fmt.Sprint(event["message"]), `"ownershipProbe":"denied"`) {
		t.Fatalf("foreign tool was claimed without verified ownership: %+v", event)
	}
}
