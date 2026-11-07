package host

import (
	"github.com/furbyhaxx/pig-workbench/api"
	"testing"
)

func TestMutationReceiptCannotInventCommitOrHidePartialEffects(t *testing.T) {
	good := func() *api.MutationReceipt {
		return &api.MutationReceipt{OperationID: "op", IdempotencyKey: "op", CanonicalURI: "mut://x/a", Status: "committed", Effects: []api.MutationEffect{{URI: "mut://x/a", Status: "committed"}, {URI: "mut://x/b", Status: "committed"}}, ChangedResources: []string{"mut://x/a", "mut://x/b"}}
	}
	if !validMutationReceipt(good(), "op", "mut://x/a", "mut://x/b", "move") {
		t.Fatal("valid move refused")
	}
	for name, change := range map[string]func(*api.MutationReceipt){
		"wrong correlation":          func(r *api.MutationReceipt) { r.OperationID = "other" },
		"missing destination":        func(r *api.MutationReceipt) { r.Effects = r.Effects[:1] },
		"unexpected resource":        func(r *api.MutationReceipt) { r.ChangedResources = []string{"mut://x/private"} },
		"not applied with effects":   func(r *api.MutationReceipt) { r.Status = "notApplied" },
		"unknown reported committed": func(r *api.MutationReceipt) { r.Effects[1].Status = "unknown" },
		"duplicate effects":          func(r *api.MutationReceipt) { r.Effects[1] = r.Effects[0] },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := good()
			change(receipt)
			if validMutationReceipt(receipt, "op", "mut://x/a", "mut://x/b", "move") {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	partial := good()
	partial.Status = "partial"
	partial.Effects[0].Status = "unknown"
	partial.ChangedResources = []string{"mut://x/b"}
	if !validMutationReceipt(partial, "op", "mut://x/a", "mut://x/b", "move") {
		t.Fatal("truthful partial move refused")
	}
}
