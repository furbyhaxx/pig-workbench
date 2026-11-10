package host

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestSessionSelfFramesListsReceiptsPagedAndReadsOneFrame(t *testing.T) {
	ledger := frames.NewSession("session-1")
	ctx := frames.Context{SessionID: "session-1", ProducerPrefix: "pig-workbench.context", BranchLeaf: "leaf", Epoch: "epoch", Trusted: true}
	var ids []string
	for i := 0; i < 50; i++ {
		receipt, err := ledger.Ledger(ctx).Admit(frames.Publish{EventKey: fmt.Sprint(i), Kind: "info", Source: "pig-workbench.context.date", Title: fmt.Sprint("title-", i), Body: fmt.Sprint("BODY-", i), Delivery: "beforePrompt", DataTrust: "trusted"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, receipt.FrameID)
	}
	s := &session{contextFrames: &contextFrameState{ledger: ledger}}
	provider := sessionProvider{ctx: func() (sdk.Context, bool) { return sdk.Context{}, true }, source: func() (*session, bool) { return s, true }}

	var listed []string
	cursor := ""
	for page := 0; ; page++ {
		listing, err := provider.List(context.Background(), "session://self/frames/", cursor, 16)
		if err != nil {
			t.Fatal(err)
		}
		if len(listing.Entries) > 16 {
			t.Fatalf("page %d has %d entries, limit 16", page, len(listing.Entries))
		}
		for _, entry := range listing.Entries {
			listed = append(listed, entry.Name)
		}
		if listing.Complete {
			break
		}
		cursor = listing.NextCursor
	}
	if strings.Join(listed, ",") != strings.Join(ids, ",") {
		t.Fatalf("paged listing differs from the ledger: %d of %d", len(listed), len(ids))
	}

	read := func(uri string) string {
		result, err := provider.Read(context.Background(), resource.ReadRequest{URI: uri, Requested: uri, Budget: resource.Budget{Lines: 2000, Bytes: 50 << 10}})
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		return result.Content
	}
	if list := read("session://self/frames"); !strings.Contains(list, ids[0]) || !strings.Contains(list, "pending") || strings.Contains(list, "BODY-0") {
		t.Fatalf("receipt list lacks ids and states or leaks bodies:\n%s", list)
	}
	one := read("session://self/frames/" + ids[7])
	for _, want := range []string{"frame " + ids[7], "state pending", "source pig-workbench.context.date", "title title-7", "BODY-7"} {
		if !strings.Contains(one, want) {
			t.Errorf("frame view lacks %q:\n%s", want, one)
		}
	}
	if _, err := provider.Stat(context.Background(), "session://self/frames/f-missing"); err == nil {
		t.Fatal("an unknown frame id must not stat")
	}
}
