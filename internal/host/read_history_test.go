package host

import (
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestReadRepetitionHintRetainsContentAndResetsOnChanges(t *testing.T) {
	var state readHistory
	target := resource.Target{URI: "file:///source", Selection: resource.Selection{}}
	page := resource.ReadResult{Content: "1:source\n", Facts: resource.Page{CanonicalURI: target.URI, Revision: "rev1", CoordinateSpace: "source-lines", TotalLines: 1}}
	for i := 1; i <= 3; i++ {
		hint := state.hint(target, page)
		if (i < 3 && hint != "") || (i == 3 && !strings.Contains(hint, "Repeated read")) {
			t.Fatalf("read %d hint=%q", i, hint)
		}
	}
	if page.Content != "1:source\n" {
		t.Fatal("hint replaced the original content")
	}
	page.Facts.Revision = "rev2"
	if hint := state.hint(target, page); hint != "" {
		t.Fatalf("revision inherited counter: %q", hint)
	}
	target.Selection = resource.Selection{Raw: true}
	if hint := state.hint(target, page); hint != "" {
		t.Fatalf("selection inherited counter: %q", hint)
	}
	page.Facts.CoordinateSpace = "derived"
	if hint := state.hint(target, page); hint != "" {
		t.Fatalf("view inherited counter: %q", hint)
	}
	page.Facts.Revision = ""
	for range 3 {
		if hint := state.hint(target, page); hint != "" {
			t.Fatalf("unknown revision was treated as stable: %q", hint)
		}
	}
}
