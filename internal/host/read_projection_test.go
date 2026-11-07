package host

import (
	"reflect"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func projectionMessage(id, text string, complete bool) map[string]any {
	page := resource.ReadResult{Content: text, Facts: resource.Page{CanonicalURI: "file:///source", Revision: "revision", CoordinateSpace: "source-lines", TotalLines: 2, ShownRanges: []resource.Range{{Start: 1, End: 1}}, FullyShownRanges: []resource.Range{{Start: 1, End: 1}}, ResourceComplete: complete, SelectionComplete: true}}
	if complete {
		page.Facts.ShownRanges[0].End = 2
		page.Facts.FullyShownRanges[0].End = 2
	}
	return map[string]any{"role": "toolResult", "toolName": "read", "toolCallId": id, "content": []any{map[string]any{"type": "text", "text": text + "\nRetained annotation"}}, "details": map[string]any{"readProjection": readProof(page)}}
}

func TestReadProjectionPreservesHistoryProvenanceAndAnnotations(t *testing.T) {
	old := projectionMessage("old", "partial source", false)
	newer := projectionMessage("new", "complete source", true)
	input := []any{old, newer}
	projected := projectReadMessages(input)
	if projected == nil || !containsTextBlock(projected[0].(map[string]any)["content"], "superseded by read new") || !containsTextBlock(projected[0].(map[string]any)["content"], "Retained annotation") {
		t.Fatalf("projection: %#v", projected)
	}
	if !containsTextBlock(old["content"], "partial source") || old["toolCallId"] != "old" || !reflect.DeepEqual(projected[1], newer) {
		t.Fatal("projection mutated original history or tool provenance")
	}
	if again := projectReadMessages(input); !reflect.DeepEqual(projected, again) {
		t.Fatal("replay did not reconstruct projection")
	}
}

func TestReadProjectionRefusesUnprovedSupersession(t *testing.T) {
	for name, alter := range map[string]func(map[string]any){
		"different revision": func(m map[string]any) {
			m["details"].(map[string]any)["readProjection"].(*readProjection).Page.Revision = "different"
		},
		"summary": func(m map[string]any) {
			m["details"].(map[string]any)["readProjection"].(*readProjection).Page.Truncation = "summary"
		},
		"derived": func(m map[string]any) {
			m["details"].(map[string]any)["readProjection"].(*readProjection).Page.CoordinateSpace = "derived"
		},
		"incomplete": func(m map[string]any) {
			m["details"].(map[string]any)["readProjection"].(*readProjection).Page.ResourceComplete = false
		},
		"partial line": func(m map[string]any) {
			m["details"].(map[string]any)["readProjection"].(*readProjection).Page.FullyShownRanges = nil
		},
		"clipped content": func(m map[string]any) { m["content"] = []any{map[string]any{"type": "text", "text": "clipped"}} },
		"error":           func(m map[string]any) { m["isError"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			old := projectionMessage("old", "partial source", false)
			newer := projectionMessage("new", "complete source", true)
			alter(newer)
			if result := projectReadMessages([]any{old, newer}); result != nil {
				t.Fatalf("unsafe projection: %#v", result)
			}
		})
	}
	full := projectionMessage("old", "full source", true)
	if result := projectReadMessages([]any{full, projectionMessage("new", "complete source", true)}); result != nil {
		t.Fatal("full source was masked")
	}
	if !strings.Contains(full["content"].([]any)[0].(map[string]any)["text"].(string), "full source") {
		t.Fatal("full source changed")
	}
}
