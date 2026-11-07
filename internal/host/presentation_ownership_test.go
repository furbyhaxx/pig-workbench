package host

import (
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func TestPresentationClaimRequiresVerifiedToolOwnership(t *testing.T) {
	tools := []sdk.ToolInfo{{Name: "owned", Source: "producer"}, {Name: "foreign", Source: "another"}, {Name: "unknown"}}
	for _, names := range [][]string{{"foreign"}, {"unknown"}, {"absent"}, {"owned", "foreign"}} {
		if failure := verifyPresentationOwner(tools, "producer", names); failure == nil {
			t.Fatalf("unverified tool claim accepted: %v", names)
		}
	}
	if failure := verifyPresentationOwner(tools, "producer", []string{"owned"}); failure != nil {
		t.Fatal(failure)
	}
	if failure := verifyPresentationOwner(nil, "producer", []string{"owned"}); failure == nil {
		t.Fatal("missing host metadata granted a claim")
	}
}
