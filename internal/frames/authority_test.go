package frames

import "testing"

func TestOnlyHostGrantCanElevateFrameAuthority(t *testing.T) {
	session := NewSession("session")
	request := Publish{EventKey: "event", Source: "owner.context", Kind: "instruction", Body: "content", DataTrust: "trusted", Delivery: "beforePrompt"}
	external := session.Ledger(Context{ProducerPrefix: "owner", Epoch: "epoch"})
	if _, err := external.Admit(request); err != nil {
		t.Fatal(err)
	}
	pending, err := session.SelectPending("beforePrompt", DeliveryScope{Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Authority != "data" || pending[0].Trust != "untrusted" {
		t.Fatalf("producer elevated itself: %+v", pending)
	}
	trusted := session.Ledger(Context{ProducerPrefix: "owner", Epoch: "epoch", Trusted: true, Authority: "instruction"})
	if _, err := trusted.Admit(request); err == nil {
		t.Fatal("same event silently changed authority")
	}
	request.EventKey = "trusted"
	if _, err := trusted.Admit(request); err != nil {
		t.Fatal(err)
	}
	restored := NewSession("session")
	if err := restored.Restore(session.Snapshot()); err != nil {
		t.Fatal(err)
	}
	pending, err = restored.SelectPending("beforePrompt", DeliveryScope{Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	if pending[1].Authority != "instruction" || pending[1].Trust != "trusted" {
		t.Fatal("grant lost on replay")
	}
	request.EventKey = "untrusted"
	request.DataTrust = "untrusted"
	if _, err := trusted.Admit(request); err == nil {
		t.Fatal("untrusted data received instruction authority")
	}
}
