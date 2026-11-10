package host

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/frames"
)

// A long session stores every frame twice, pending and delivered, each with its full body.
func TestLedgerFromEntriesLoadsHistoryAboveThePendingLimitWithoutRedelivery(t *testing.T) {
	const id = "session-1"
	source := frames.NewSession(id)
	ctx := frames.Context{ProducerPrefix: "pig-workbench.context", BranchLeaf: "leaf", Epoch: "epoch", Trusted: true}
	scope := frames.DeliveryScope{BranchLeaf: "leaf", Epoch: "epoch"}
	var entries []json.RawMessage
	store := func() {
		snapshot := source.Snapshot()
		raw, err := json.Marshal(map[string]any{"type": "custom", "customType": "pig-workbench.frame", "data": snapshot[len(snapshot)-1]})
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, raw)
	}
	for i := 0; i < 400; i++ {
		receipt, err := source.Ledger(ctx).Admit(frames.Publish{EventKey: fmt.Sprint(i), Kind: "info", Source: "pig-workbench.context.date", Body: strings.Repeat("x", 8*1024), Delivery: "beforePrompt", DataTrust: "trusted"})
		if err != nil {
			t.Fatal(err)
		}
		store()
		if err := source.MarkDelivered(scope, []string{receipt.FrameID}); err != nil {
			t.Fatal(err)
		}
		store()
	}

	ledger, _, err := ledgerFromEntries(entries, id)
	if err != nil {
		t.Fatalf("3.2 MiB of delivered frames did not load: %v", err)
	}
	pending, err := ledger.SelectPending("beforePrompt", scope)
	if err != nil || len(pending) != 0 {
		t.Fatalf("resume would deliver %d frames again (%v)", len(pending), err)
	}
	if _, err := ledger.Ledger(ctx).Admit(frames.Publish{EventKey: "after", Kind: "info", Source: "pig-workbench.context.date", Body: "next", DataTrust: "trusted"}); err != nil {
		t.Fatalf("admission after resume: %v", err)
	}
}
